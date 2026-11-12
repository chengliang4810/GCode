package engine

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gcode/internal/accounting"
	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

const engineDiskFormat = 2

// 配置与小型索引固定在入口；完整对话必须留在创建时绑定的运行根。
type diskState struct {
	Format    int               `json:"format"`
	Snapshot  persistedSnapshot `json:"snapshot"`
	Providers []model.Provider  `json:"providers"`
	Records   map[string]string `json:"records"`
}

type sessionDisk struct {
	Format        int                       `json:"format"`
	SessionID     string                    `json:"sessionId"`
	RootID        string                    `json:"rootId"`
	Session       contracts.Session         `json:"session"`
	Messages      []contracts.Message       `json:"messages"`
	History       []model.Message           `json:"history"`
	Context       []model.Message           `json:"context"`
	ContextMemory agent.ContextMemory       `json:"contextMemory"`
	Prompt        model.Prompt              `json:"prompt"`
	Runs          []contracts.Run           `json:"runs"`
	ModelRequests []contracts.ModelRequest  `json:"modelRequests"`
	Queue         []contracts.QueuedMessage `json:"queue"`
	ContextUsage  contracts.ContextUsage    `json:"contextUsage"`
	Changes       []contracts.Change        `json:"changes"`
	Terminals     []contracts.Terminal      `json:"terminals"`
}

func (e *Engine) loadPersistence() error {
	e.sessionHashes = make(map[string][32]byte)
	e.loadedSessions = make(map[string]bool)
	e.sessionRecords = make(map[string]string)
	data, err := os.ReadFile(filepath.Join(e.storage.ConfigDirectory(), "engine.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var disk diskState
	if err := json.Unmarshal(data, &disk); err != nil {
		return fmt.Errorf("read engine index: %w", err)
	}
	if disk.Format != engineDiskFormat || disk.Snapshot.SchemaVersion != contracts.SchemaVersion {
		return errors.New("unsupported engine storage format; existing data was not changed")
	}
	e.indexHash, e.indexHashKnown = sha256.Sum256(data), true
	e.state, e.providers = disk.Snapshot.restore(e.state), disk.Providers
	e.sessionRecords = disk.Records
	if e.sessionRecords == nil {
		return errors.New("engine index is missing conversation records")
	}
	e.state.Messages = make(map[string][]contracts.Message)
	e.state.ContextUsage = make(map[string]contracts.ContextUsage)
	e.state.Queue = nil
	e.state.ModelRequests = nil
	seen := make(map[string]bool, len(e.state.Sessions))
	for i := range e.state.Sessions {
		session := &e.state.Sessions[i]
		if session.StorageRootID == "" || !validSessionDirectoryID(session.ID) || !validRecordHash(e.sessionRecords[session.ID]) || seen[session.ID] {
			return errors.New("session index has invalid storage identity")
		}
		seen[session.ID] = true
		session.ModelRetry = nil
		session.RunningSubagents = 0
		if session.Status == contracts.Working || session.Status == contracts.AwaitingInput {
			session.Status, session.LastOutcome = contracts.Idle, "interrupted"
		}
		if _, err := e.storage.Resolve(session.StorageRootID); err != nil {
			session.StorageError = err.Error()
		}
	}
	return nil
}

func (e *Engine) loadSessionLocked(session contracts.Session) error {
	record := e.sessionRecords[session.ID]
	disk, err := e.readSessionDisk(session, record)
	if err != nil {
		return err
	}
	e.installSessionDiskLocked(disk, record)
	return nil
}

func (e *Engine) readSessionDisk(session contracts.Session, record string) (sessionDisk, error) {
	var disk sessionDisk
	root, err := e.storage.Resolve(session.StorageRootID)
	if err != nil {
		return disk, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return disk, err
	}
	defer handle.Close()
	if !validRecordHash(record) {
		return disk, errors.New("conversation record is missing from the index")
	}
	folder, err := openSessionDirectory(handle, session.ID)
	if err != nil {
		return disk, err
	}
	defer folder.Close()
	data, err := folder.ReadFile(record + ".json")
	if err != nil {
		return disk, fmt.Errorf("read conversation %s: %w", session.ID, err)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != record {
		return disk, errors.New("conversation record checksum does not match its index")
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return disk, fmt.Errorf("read conversation %s: %w", session.ID, err)
	}
	if disk.Format != engineDiskFormat || disk.SessionID != session.ID || disk.RootID != session.StorageRootID ||
		disk.Session.ID != session.ID || disk.Session.StorageRootID != session.StorageRootID {
		return disk, errors.New("conversation storage identity does not match its index")
	}
	return disk, nil
}

func (e *Engine) installSessionDiskLocked(disk sessionDisk, record string) {
	session := disk.Session
	e.state.Messages[session.ID] = disk.Messages
	e.history[session.ID], e.context[session.ID] = disk.History, disk.Context
	if e.contextMemories == nil {
		e.contextMemories = map[string]agent.ContextMemory{}
	}
	e.contextMemories[session.ID] = disk.ContextMemory
	e.prompts[session.ID] = model.ClonePrompt(disk.Prompt)
	e.state.Runs = slices.DeleteFunc(e.state.Runs, func(run contracts.Run) bool { return run.SessionID == session.ID })
	e.state.Runs = append(e.state.Runs, disk.Runs...)
	e.state.ModelRequests = append(e.state.ModelRequests, disk.ModelRequests...)
	e.state.Queue = append(e.state.Queue, disk.Queue...)
	e.state.ContextUsage[session.ID] = disk.ContextUsage
	e.state.Changes = append(e.state.Changes, disk.Changes...)
	e.state.Terminals = append(e.state.Terminals, disk.Terminals...)
	e.state.Sessions[e.sessionIndex(session.ID)] = disk.Session
	e.loadedSessions[session.ID] = true
	// Record the committed bytes before recovery changes any shared slices.
	var hash [32]byte
	decoded, _ := hex.DecodeString(record)
	copy(hash[:], decoded)
	e.sessionHashes[session.ID] = hash
	e.reconcileSessionLocked(session.ID)
}

func (e *Engine) ensureSessionDataLocked(sessionID string) error {
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return errors.New("session not found")
	}
	session := &e.state.Sessions[index]
	if _, err := e.storage.Resolve(session.StorageRootID); err != nil {
		session.StorageError = err.Error()
		return err
	}
	if !e.loadedSessions[sessionID] {
		if err := e.loadSessionLocked(*session); err != nil {
			session.StorageError = err.Error()
			return err
		}
	}
	session.StorageError = ""
	return nil
}

// markSessionDirtyLocked records conversation-owned state that must be included
// in the next persistence attempt. The caller holds Engine.mu.
func (e *Engine) markSessionDirtyLocked(sessionIDs ...string) {
	if len(sessionIDs) == 0 {
		return
	}
	if e.sessionDirty == nil {
		e.sessionDirty = make(map[string]struct{}, len(sessionIDs))
	}
	for _, sessionID := range sessionIDs {
		if sessionID != "" {
			e.sessionDirty[sessionID] = struct{}{}
		}
	}
}

func (e *Engine) persistLocked(required ...string) (resultErr error) {
	e.persistErrors = nil
	e.markSessionDirtyLocked(required...)
	e.refreshStatisticsLocked()
	records := maps.Clone(e.sessionRecords)
	hashes := maps.Clone(e.sessionHashes)
	dirty := maps.Clone(e.sessionDirty)
	if records == nil {
		records = map[string]string{}
	}
	if hashes == nil {
		hashes = map[string][32]byte{}
	}
	type pendingRecord struct{ rootID, sessionID, previous, name string }
	var written []pendingRecord
	var unavailable []string
	saved := make(map[string]struct{}, len(dirty))
	defer func() {
		if resultErr == nil {
			return
		}
		for _, record := range written {
			e.removeSessionRecord(record.rootID, record.sessionID, record.name)
		}
	}()
	for _, session := range e.state.Sessions {
		if !e.loadedSessions[session.ID] {
			continue
		}
		if _, ok := dirty[session.ID]; !ok {
			continue
		}
		disk := sessionDisk{
			Format: engineDiskFormat, SessionID: session.ID, RootID: session.StorageRootID,
			Session:  session,
			Messages: e.state.Messages[session.ID], History: e.history[session.ID], Context: e.context[session.ID],
			ContextMemory: e.contextMemories[session.ID],
			Prompt:        model.ClonePrompt(e.prompts[session.ID]),
			ContextUsage:  e.state.ContextUsage[session.ID],
		}
		disk.Session.StorageError = ""
		disk.Session.ModelRetry = nil
		for _, request := range e.state.ModelRequests {
			if request.SessionID == session.ID {
				disk.ModelRequests = append(disk.ModelRequests, request)
			}
		}
		for _, change := range e.state.Changes {
			if change.SessionID == session.ID {
				disk.Changes = append(disk.Changes, change)
			}
		}
		for _, terminal := range e.state.Terminals {
			if terminal.SessionID == session.ID {
				disk.Terminals = append(disk.Terminals, terminal)
			}
		}
		for _, run := range e.state.Runs {
			if run.SessionID == session.ID {
				disk.Runs = append(disk.Runs, run)
			}
		}
		for _, queued := range e.state.Queue {
			if queued.SessionID == session.ID {
				disk.Queue = append(disk.Queue, queued)
			}
		}
		data, err := json.MarshalIndent(disk, "", "  ")
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		if previous, ok := e.sessionHashes[session.ID]; ok && previous == hash {
			saved[session.ID] = struct{}{}
			continue
		}
		record := hex.EncodeToString(hash[:])
		previous := records[session.ID]
		err = e.writeSessionRecord(session, record, data)
		if err != nil {
			e.state.Sessions[e.sessionIndex(session.ID)].StorageError = err.Error()
			e.persistErrors = append(e.persistErrors, fmt.Errorf("save conversation %s: %w", session.ID, err))
			if records[session.ID] == "" || slices.Contains(required, session.ID) {
				return fmt.Errorf("save conversation %s: %w", session.ID, err)
			}
			// An unavailable historical root must not prevent commits on other roots.
			unavailable = append(unavailable, session.ID)
			continue
		}
		written = append(written, pendingRecord{session.StorageRootID, session.ID, previous, record})
		records[session.ID], hashes[session.ID] = record, hash
		saved[session.ID] = struct{}{}
	}
	snapshot := contracts.Snapshot{
		Terminals: slices.DeleteFunc(slices.Clone(e.state.Terminals), func(terminal contracts.Terminal) bool { return terminal.SessionID != "" }),
	}
	for sessionID := range records {
		if e.sessionIndex(sessionID) < 0 {
			delete(records, sessionID)
			delete(hashes, sessionID)
		}
	}
	if err := e.persistStandaloneTerminals(&snapshot); err != nil {
		return err
	}
	durable := persistenceIndex(e.state)
	durable.Terminals = snapshot.Terminals
	indexPath := filepath.Join(e.storage.ConfigDirectory(), "engine.json")
	if len(unavailable) > 0 {
		// A record and its index summary commit together. An unrelated online
		// save must not publish metadata for a historical record we could not
		// update. This exceptional path reads only the small committed index.
		data, err := os.ReadFile(indexPath)
		if err != nil {
			return err
		}
		var committed diskState
		if err := json.Unmarshal(data, &committed); err != nil {
			return err
		}
		durable.ApplicationUsage = RestoreUnavailableUsage(durable.ApplicationUsage, committed.Snapshot.ApplicationUsage, unavailable)
		durable.Metrics = maps.Clone(durable.Metrics)
		for _, id := range unavailable {
			if summary, exists := committed.Snapshot.Metrics[id]; exists {
				durable.Metrics[id] = summary
			} else {
				delete(durable.Metrics, id)
			}
		}
		// Ancestors must include the committed child's summary, not statistics
		// from a record whose source write failed.
		projection := contracts.Snapshot{Sessions: durable.Sessions, Metrics: durable.Metrics}
		accounting.Refresh(&projection, unavailable)
		durable.Metrics = projection.Metrics
		for i, session := range durable.Sessions {
			if !slices.Contains(unavailable, session.ID) {
				continue
			}
			index := slices.IndexFunc(committed.Snapshot.Sessions, func(old contracts.Session) bool { return old.ID == session.ID })
			if index < 0 {
				return fmt.Errorf("committed index is missing conversation %s", session.ID)
			}
			durable.Sessions[i] = committed.Snapshot.Sessions[index]
		}
		durable.Runs = slices.DeleteFunc(durable.Runs, func(run contracts.Run) bool { return slices.Contains(unavailable, run.SessionID) })
		for _, run := range committed.Snapshot.Runs {
			if slices.Contains(unavailable, run.SessionID) {
				durable.Runs = append(durable.Runs, run)
			}
		}
	}
	index, err := json.MarshalIndent(diskState{Format: engineDiskFormat, Snapshot: durable, Providers: e.providers, Records: records}, "", "  ")
	if err != nil {
		return err
	}
	indexHash := sha256.Sum256(index)
	unchanged := e.indexHashKnown && indexHash == e.indexHash
	if unchanged {
		info, statErr := os.Stat(indexPath)
		unchanged = statErr == nil && info.Mode().IsRegular()
	}
	if !unchanged {
		if err := storage.WriteJSONBytes(indexPath, index); err != nil {
			return err
		}
		e.indexHash, e.indexHashKnown = indexHash, true
	}
	e.sessionRecords, e.sessionHashes = records, hashes
	e.commitStandaloneTerminals(&snapshot)
	for sessionID := range saved {
		delete(e.sessionDirty, sessionID)
	}
	for sessionID := range e.sessionDirty {
		if e.sessionIndex(sessionID) < 0 {
			delete(e.sessionDirty, sessionID)
		}
	}
	for _, record := range written {
		e.cleanSessionRecords(record.rootID, record.sessionID, record.previous, record.name)
	}
	return nil
}

func validRecordHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (e *Engine) writeSessionRecord(session contracts.Session, record string, data []byte) error {
	directory, err := e.storage.Resolve(session.StorageRootID)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	folder, err := openSessionDirectory(root, session.ID)
	if err != nil {
		return err
	}
	defer folder.Close()
	// persistLocked supplies the bytes directly from json.MarshalIndent.
	return storage.WriteBytesRoot(folder, record+".json", data)
}

func (e *Engine) removeSessionRecord(rootID, sessionID, record string) {
	directory, err := e.storage.Resolve(rootID)
	if err != nil {
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return
	}
	defer root.Close()
	folder, err := openSessionDirectory(root, sessionID)
	if err != nil {
		return
	}
	defer folder.Close()
	_ = folder.Remove(record + ".json")
}

func (e *Engine) cleanSessionRecords(rootID, sessionID, previous, current string) {
	if previous == "" || previous == current || !validRecordHash(previous) {
		return
	}
	directory, err := e.storage.Resolve(rootID)
	if err != nil {
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return
	}
	defer root.Close()
	folder, err := openSessionDirectory(root, sessionID)
	if err != nil {
		return
	}
	defer folder.Close()
	// Only the indexed predecessor may be collected; a hash-shaped filename
	// found during a directory scan does not grant the application ownership.
	name := previous + ".json"
	file, err := folder.Open(name)
	if err != nil {
		return
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		_ = file.Close()
		return
	}
	if err := file.Close(); err != nil {
		return
	}
	if hex.EncodeToString(digest.Sum(nil)) == previous {
		_ = folder.Remove(name)
	}
}

// Only empty reservations created by a failed command may be removed. Never
// recursively remove a working directory, including a shared derived workspace.
func (e *Engine) releaseEmptySessionDirectories(session contracts.Session) {
	directory, err := e.storage.Resolve(session.StorageRootID)
	if err != nil {
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return
	}
	defer root.Close()
	if parent, err := openRealDirectory(root, "sessions"); err == nil {
		_ = parent.Remove(session.ID)
		_ = parent.Close()
	}
	if session.CWD == filepath.Join(directory, "workspaces", session.ID) {
		if parent, err := openRealDirectory(root, "workspaces"); err == nil {
			_ = parent.Remove(session.ID)
			_ = parent.Close()
		}
	}
}

func openSessionDirectory(root *os.Root, sessionID string) (*os.Root, error) {
	if !validSessionDirectoryID(sessionID) {
		return nil, errors.New("invalid conversation directory identity")
	}
	parent, err := openRealDirectory(root, "sessions")
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return openRealDirectory(parent, sessionID)
}

// Root confines traversal, but permits links to other locations inside itself.
// An owned directory must also be a real, stable directory so that two
// conversations cannot alias each other's records or garbage collection.
func openRealDirectory(parent *os.Root, name string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("owned data directory must not be a link: %s", name)
	}
	folder, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	file, err := folder.Open(".")
	if err == nil {
		var opened os.FileInfo
		opened, err = file.Stat()
		_ = file.Close()
		if err == nil && !os.SameFile(before, opened) {
			err = errors.New("owned data directory changed while opening")
		}
	}
	if err == nil {
		var after os.FileInfo
		after, err = parent.Lstat(name)
		if err == nil && (!after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after)) {
			err = errors.New("owned data directory changed while opening")
		}
	}
	if err != nil {
		_ = folder.Close()
		return nil, err
	}
	return folder, nil
}

func newShortID() string { return strings.ToLower(rand.Text())[:12] }

func validSessionDirectoryID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (e *Engine) reserveSessionLocked(rootID string) (string, string, error) {
	root, err := e.storage.Resolve(rootID)
	if err != nil {
		return "", "", err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return "", "", err
	}
	defer handle.Close()
	if err := handle.Mkdir("sessions", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", "", err
	}
	parent, err := openRealDirectory(handle, "sessions")
	if err != nil {
		return "", "", err
	}
	defer parent.Close()
	for {
		sessionID := newShortID()
		if e.sessionIndex(sessionID) >= 0 {
			continue
		}
		if err := parent.Mkdir(sessionID, 0700); errors.Is(err, os.ErrExist) {
			continue
		} else if err != nil {
			return "", "", err
		}
		return sessionID, root, nil
	}
}
