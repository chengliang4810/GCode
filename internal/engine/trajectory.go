package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

const (
	trajectoryMaxRecords = 200
	trajectoryMaxBytes   = 32 << 20
	trajectoryRecordSize = 2 << 20
)

var trajectoryFilename = regexp.MustCompile(`^call-[0-9]+-[A-Za-z0-9]+-[0-9]+\.json$`)

// The model worker records diagnostics without copying them into UI snapshots.
// Lock ordering is engine -> trajectory; disk I/O releases the engine lock.
func (e *Engine) trajectoryRecorder(session contracts.Session, turnID, source string) func(model.Invocation) {
	return func(invocation model.Invocation) {
		e.mu.Lock()
		if e.closed || e.sessionIndex(session.ID) < 0 {
			e.mu.Unlock()
			return
		}
		e.trajectoryMu.Lock()
		e.mu.Unlock()
		defer e.trajectoryMu.Unlock()
		err := e.writeTrajectory(session, contracts.ModelInvocation{SessionID: session.ID,
			TurnID: turnID, Source: source, Invocation: invocation})
		if err != nil {
			if e.trajectoryIssues == nil {
				e.trajectoryIssues = make(map[string]error)
			}
			e.trajectoryIssues[session.ID] = fmt.Errorf("record model trajectory: %w", err)
		}
	}
}

func (e *Engine) trajectoryDirectory(session contracts.Session, create bool) (*os.Root, error) {
	directory, err := e.storage.Resolve(session.StorageRootID)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	folder, err := openSessionDirectory(root, session.ID)
	if err != nil {
		return nil, err
	}
	defer folder.Close()
	if create {
		if err := folder.Mkdir("trajectory", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	if !create {
		if _, err := folder.Lstat("trajectory"); err != nil {
			return nil, err
		}
	}
	return openRealDirectory(folder, "trajectory")
}

func (e *Engine) writeTrajectory(session contracts.Session, record contracts.ModelInvocation) error {
	folder, err := e.trajectoryDirectory(session, true)
	if err != nil {
		return err
	}
	defer folder.Close()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > trajectoryRecordSize {
		record.Truncated = true
		record.System, record.Messages, record.Tools = "[request context omitted: record size limit]", nil, nil
		if record.Response != nil {
			response := *record.Response
			response.Message = model.Message{Role: "assistant", Text: "[response content omitted: record size limit]"}
			record.Response = &response
		}
	}
	name := fmt.Sprintf("call-%019d-%s-%03d.json", record.StartedAt.UnixNano(), record.ID, record.Attempt)
	if !trajectoryFilename.MatchString(name) {
		return errors.New("invalid model invocation identity")
	}
	if err := storage.WriteJSONRoot(folder, name, record); err != nil {
		return err
	}
	files, err := trajectoryFiles(folder)
	if err != nil {
		return err
	}
	var total int64
	for _, file := range files {
		total += file.size
	}
	dropped := false
	for len(files) > trajectoryMaxRecords || total > trajectoryMaxBytes {
		file := files[0]
		files = files[1:]
		total -= file.size
		if !trajectoryRecordOwned(folder, file, session.ID) {
			continue
		}
		if err := folder.Remove(file.name); err != nil {
			return err
		}
		dropped = true
	}
	if dropped {
		return storage.WriteJSONRoot(folder, "retention.json", struct{ Truncated bool }{true})
	}
	return nil
}

type trajectoryFile struct {
	name string
	size int64
}

func trajectoryRecordOwned(folder *os.Root, file trajectoryFile, sessionID string) bool {
	if file.size > trajectoryRecordSize {
		return false
	}
	handle, err := folder.Open(file.name)
	if err != nil {
		return false
	}
	defer handle.Close()
	var record contracts.ModelInvocation
	if json.NewDecoder(io.LimitReader(handle, trajectoryRecordSize+1)).Decode(&record) != nil ||
		record.SessionID != sessionID || record.ID == "" || record.StartedAt.IsZero() {
		return false
	}
	expected := fmt.Sprintf("call-%019d-%s-%03d.json", record.StartedAt.UnixNano(), record.ID, record.Attempt)
	return file.name == expected
}

func trajectoryFiles(folder *os.Root) ([]trajectoryFile, error) {
	dir, err := folder.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var files []trajectoryFile
	for _, entry := range entries {
		if !trajectoryFilename.MatchString(entry.Name()) {
			continue
		}
		info, err := folder.Lstat(entry.Name())
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("trajectory record must be a regular file")
		}
		files = append(files, trajectoryFile{entry.Name(), info.Size()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

func (e *Engine) ReadModelTrajectory(ctx context.Context, sessionID string) (contracts.ModelTrajectory, error) {
	result := contracts.ModelTrajectory{SessionID: sessionID}
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return result, err
	}
	index := e.sessionIndex(sessionID)
	if index < 0 {
		e.mu.Unlock()
		return result, errors.New("conversation not found")
	}
	session := e.state.Sessions[index]
	activeTurn := ""
	if run := e.runs[sessionID]; run != nil {
		activeTurn = run.id
	}
	namingActive := e.naming[sessionID] != nil
	e.trajectoryMu.Lock()
	e.mu.Unlock()
	defer e.trajectoryMu.Unlock()
	folder, err := e.trajectoryDirectory(session, false)
	if errors.Is(err, os.ErrNotExist) {
		return result, e.trajectoryIssues[sessionID]
	}
	if err != nil {
		return result, err
	}
	defer folder.Close()
	files, err := trajectoryFiles(folder)
	if err != nil {
		return result, err
	}
	if _, err := folder.Lstat("retention.json"); err == nil {
		result.Truncated = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	var total int64
	for i := len(files) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		file := files[i]
		if len(result.Records) >= trajectoryMaxRecords || total+file.size > trajectoryMaxBytes {
			result.Truncated = true
			break
		}
		if file.size > trajectoryRecordSize {
			return result, errors.New("trajectory record exceeds size limit")
		}
		handle, err := folder.Open(file.name)
		if err != nil {
			return result, err
		}
		var record contracts.ModelInvocation
		decodeErr := json.NewDecoder(io.LimitReader(handle, trajectoryRecordSize+1)).Decode(&record)
		closeErr := handle.Close()
		if err := errors.Join(decodeErr, closeErr); err != nil {
			return result, fmt.Errorf("read trajectory record: %w", err)
		}
		if record.SessionID != sessionID || record.ID == "" || record.StartedAt.IsZero() {
			return result, errors.New("trajectory record identity does not match its conversation")
		}
		active := record.TurnID != "" && record.TurnID == activeTurn || record.Source == "naming" && namingActive
		if record.CompletedAt == nil && !active {
			record.Error = "model call has no completion record"
		}
		total += file.size
		result.Truncated = result.Truncated || record.Truncated
		result.Records = append(result.Records, record)
	}
	sort.Slice(result.Records, func(i, j int) bool {
		a, b := result.Records[i], result.Records[j]
		if a.StartedAt.Equal(b.StartedAt) {
			return a.ID < b.ID
		}
		return a.StartedAt.Before(b.StartedAt)
	})
	return result, e.trajectoryIssues[sessionID]
}

// Called only by the existing explicit conversation deletion path, while its
// engine lock is held. Unknown files are left in place.
func (e *Engine) removeTrajectory(session contracts.Session) {
	e.trajectoryMu.Lock()
	defer e.trajectoryMu.Unlock()
	folder, err := e.trajectoryDirectory(session, false)
	if err != nil {
		return
	}
	files, err := trajectoryFiles(folder)
	if err == nil {
		for _, file := range files {
			if trajectoryRecordOwned(folder, file, session.ID) {
				_ = folder.Remove(file.name)
			}
		}
		_ = folder.Remove("retention.json")
	}
	_ = folder.Close()
	directory, err := e.storage.Resolve(session.StorageRootID)
	if err == nil {
		root, err := os.OpenRoot(directory)
		if err == nil {
			defer root.Close()
			name := filepath.ToSlash(filepath.Join("sessions", session.ID, "trajectory"))
			if strings.HasPrefix(name, "sessions/") {
				_ = root.Remove(name)
			}
		}
	}
	delete(e.trajectoryIssues, session.ID)
}
