package engine

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

const engineDiskFormat = 2

// 配置与小型索引固定在入口；完整对话必须留在创建时绑定的运行根。
type diskState struct {
	Format    int                `json:"format"`
	Snapshot  contracts.Snapshot `json:"snapshot"`
	Providers []model.Provider   `json:"providers"`
	Records   map[string]string  `json:"records"`
}

type sessionDisk struct {
	Format       int                       `json:"format"`
	SessionID    string                    `json:"sessionId"`
	RootID       string                    `json:"rootId"`
	Session      contracts.Session         `json:"session"`
	Messages     []contracts.Message       `json:"messages"`
	History      []model.Message           `json:"history"`
	Context      []model.Message           `json:"context"`
	Runs         []contracts.Run           `json:"runs"`
	Queue        []contracts.QueuedMessage `json:"queue"`
	ContextUsage contracts.ContextUsage    `json:"contextUsage"`
	Changes      []contracts.Change        `json:"changes"`
	Terminals    []contracts.Terminal      `json:"terminals"`
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
	e.state, e.providers = disk.Snapshot, disk.Providers
	e.sessionRecords = disk.Records
	if e.sessionRecords == nil {
		return errors.New("engine index is missing conversation records")
	}
	e.state.Messages = make(map[string][]contracts.Message)
	e.state.ContextUsage = make(map[string]contracts.ContextUsage)
	e.state.Runs, e.state.Queue = nil, nil
	seen := make(map[string]bool, len(e.state.Sessions))
	for i := range e.state.Sessions {
		session := &e.state.Sessions[i]
		if session.StorageRootID == "" || !validSessionDirectoryID(session.ID) || !validRecordHash(e.sessionRecords[session.ID]) || seen[session.ID] {
			return errors.New("session index has invalid storage identity")
		}
		seen[session.ID] = true
		if err := e.loadSessionLocked(*session); err != nil {
			// 历史盘离线只影响所属对话，不能伪造空历史或阻止新盘上的对话。
			session.StorageError = err.Error()
		}
	}
	return nil
}

func (e *Engine) loadSessionLocked(session contracts.Session) error {
	root, err := e.storage.Resolve(session.StorageRootID)
	if err != nil {
		return err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	record := e.sessionRecords[session.ID]
	if !validRecordHash(record) {
		return errors.New("conversation record is missing from the index")
	}
	folder, err := openSessionDirectory(handle, session.ID)
	if err != nil {
		return err
	}
	defer folder.Close()
	data, err := folder.ReadFile(record + ".json")
	if err != nil {
		return fmt.Errorf("read conversation %s: %w", session.ID, err)
	}
	var disk sessionDisk
	if err := json.Unmarshal(data, &disk); err != nil {
		return fmt.Errorf("read conversation %s: %w", session.ID, err)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != record {
		return errors.New("conversation record checksum does not match its index")
	}
	if disk.Format != engineDiskFormat || disk.SessionID != session.ID || disk.RootID != session.StorageRootID ||
		disk.Session.ID != session.ID || disk.Session.StorageRootID != session.StorageRootID {
		return errors.New("conversation storage identity does not match its index")
	}
	e.state.Messages[session.ID] = disk.Messages
	e.history[session.ID], e.context[session.ID] = disk.History, disk.Context
	e.state.Runs = append(e.state.Runs, disk.Runs...)
	e.state.Queue = append(e.state.Queue, disk.Queue...)
	e.state.ContextUsage[session.ID] = disk.ContextUsage
	e.state.Changes = append(e.state.Changes, disk.Changes...)
	e.state.Terminals = append(e.state.Terminals, disk.Terminals...)
	e.state.Sessions[e.sessionIndex(session.ID)] = disk.Session
	e.loadedSessions[session.ID] = true
	// Record the committed bytes before recovery changes any shared slices.
	e.sessionHashes[session.ID] = hash
	e.reconcileSessionLocked(session.ID)
	return nil
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

func (e *Engine) persistLocked(required ...string) (resultErr error) {
	e.persistErrors = nil
	records := maps.Clone(e.sessionRecords)
	hashes := maps.Clone(e.sessionHashes)
	if records == nil {
		records = map[string]string{}
	}
	if hashes == nil {
		hashes = map[string][32]byte{}
	}
	type pendingRecord struct{ rootID, sessionID, name string }
	var written []pendingRecord
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
		disk := sessionDisk{
			Format: engineDiskFormat, SessionID: session.ID, RootID: session.StorageRootID,
			Session:  session,
			Messages: e.state.Messages[session.ID], History: e.history[session.ID], Context: e.context[session.ID],
			ContextUsage: e.state.ContextUsage[session.ID],
		}
		disk.Session.StorageError = ""
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
			continue
		}
		record := hex.EncodeToString(hash[:])
		err = e.writeSessionRecord(session, record, disk)
		if err != nil {
			e.state.Sessions[e.sessionIndex(session.ID)].StorageError = err.Error()
			e.persistErrors = append(e.persistErrors, fmt.Errorf("save conversation %s: %w", session.ID, err))
			if records[session.ID] == "" || slices.Contains(required, session.ID) {
				return fmt.Errorf("save conversation %s: %w", session.ID, err)
			}
			// An unavailable historical root must not prevent commits on other roots.
			continue
		}
		written = append(written, pendingRecord{session.StorageRootID, session.ID, record})
		records[session.ID], hashes[session.ID] = record, hash
	}
	snapshot := e.state
	snapshot.Runtime = nil
	snapshot.CustomProviders = nil
	snapshot.UserResources = contracts.UserResourceState{}
	snapshot.Plugins = contracts.PluginState{}
	snapshot.Messages, snapshot.ContextUsage = nil, nil
	snapshot.Runs, snapshot.Queue = nil, nil
	snapshot.Sessions = slices.Clone(snapshot.Sessions)
	for i := range snapshot.Sessions {
		snapshot.Sessions[i].StorageError = ""
		snapshot.Sessions[i].Goal = nil
	}
	// Session-owned diffs and terminal output live with their conversation.
	// Unowned Git diffs are a refreshable projection of the repository.
	snapshot.Changes = slices.DeleteFunc(slices.Clone(snapshot.Changes), func(change contracts.Change) bool { return change.SessionID != "" })
	for i := range snapshot.Changes {
		snapshot.Changes[i].OldContent, snapshot.Changes[i].NewContent, snapshot.Changes[i].Hunks = "", "", nil
	}
	snapshot.Terminals = slices.DeleteFunc(slices.Clone(snapshot.Terminals), func(terminal contracts.Terminal) bool { return terminal.SessionID != "" })
	// 文件正文只是展示缓存，不得跟随小型配置索引长期增长。
	snapshot.Files = slices.Clone(snapshot.Files)
	for i := range snapshot.Files {
		snapshot.Files[i].Content, snapshot.Files[i].Loaded = "", false
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
	if err := storage.WriteJSON(filepath.Join(e.storage.ConfigDirectory(), "engine.json"),
		diskState{Format: engineDiskFormat, Snapshot: snapshot, Providers: e.providers, Records: records}); err != nil {
		return err
	}
	e.sessionRecords, e.sessionHashes = records, hashes
	e.commitStandaloneTerminals(&snapshot)
	for _, record := range written {
		e.cleanSessionRecords(record.rootID, record.sessionID, record.name)
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

func (e *Engine) writeSessionRecord(session contracts.Session, record string, disk sessionDisk) error {
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
	return storage.WriteJSONRoot(folder, record+".json", disk)
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

func (e *Engine) cleanSessionRecords(rootID, sessionID, current string) {
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
	dir, err := folder.Open(".")
	if err != nil {
		return
	}
	defer dir.Close()
	names, _ := dir.Readdirnames(-1)
	for _, name := range names {
		value := strings.TrimSuffix(name, ".json")
		if name == value+".json" && validRecordHash(value) && value != current {
			// Only the application's matching conversation records may be
			// collected; a hash-shaped filename alone does not grant ownership.
			data, err := folder.ReadFile(name)
			if err != nil {
				continue
			}
			hash := sha256.Sum256(data)
			var record sessionDisk
			if hex.EncodeToString(hash[:]) == value && json.Unmarshal(data, &record) == nil &&
				record.Format == engineDiskFormat && record.SessionID == sessionID && record.RootID == rootID {
				_ = folder.Remove(name)
			}
		}
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
