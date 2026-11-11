package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func newTestSession(t *testing.T, core *Engine, projectID, title string) contracts.Session {
	t.Helper()
	session, err := core.newSessionLocked(projectID, title, "")
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func openStorageTestCore(t *testing.T, home string) *Engine {
	t.Helper()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return core
}

func storeTestMessage(t *testing.T, core *Engine, sessionID, text string) {
	t.Helper()
	core.mu.Lock()
	defer core.mu.Unlock()
	if err := core.ensureSessionDataLocked(sessionID); err != nil {
		t.Fatal(err)
	}
	core.appendUserLocked(sessionID, text)
	core.history[sessionID] = append(core.history[sessionID], model.Message{Role: "user", Text: text})
	core.context[sessionID] = model.CloneMessages(core.history[sessionID])
	core.markSessionDirtyLocked(sessionID)
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
}

func TestConversationStorageSwitchKeepsHistoryAndWorkspaceAtOrigin(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	old := core.state.Sessions[0]
	firstRoot := core.storage.Directory()
	storeTestMessage(t, core, old.ID, "historical-private-body")
	next := filepath.Join(home, "second-data")
	if err := core.Execute(ctx, contracts.Command{Kind: "storage.change", Path: next}); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(next, "sessions", old.ID)); !os.IsNotExist(err) {
		t.Fatal("switch copied a historical session", err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	if core.storage.Directory() != next || core.storage.ConfigDirectory() != firstRoot {
		t.Fatal("configuration moved with the runtime root")
	}
	index := core.sessionIndex(old.ID)
	if core.state.Sessions[index].CWD != old.CWD || core.state.Sessions[index].StorageRootID != old.StorageRootID {
		t.Fatal("old conversation changed storage identity")
	}
	core.mu.Lock()
	err := core.ensureSessionDataLocked(old.ID)
	history := model.CloneMessages(core.history[old.ID])
	core.mu.Unlock()
	if err != nil || len(history) != 1 || history[0].Text != "historical-private-body" {
		t.Fatal("history was not reconstructed across roots")
	}
	storeTestMessage(t, core, old.ID, "continued-in-original-root")
	oldData, err := os.ReadFile(filepath.Join(firstRoot, "sessions", old.ID, core.sessionRecords[old.ID]+".json"))
	if err != nil || !bytes.Contains(oldData, []byte("continued-in-original-root")) {
		t.Fatal("continuation was not saved at origin", err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create", RequestID: "one-new-conversation"}); err != nil {
		t.Fatal(err)
	}
	fresh := core.state.Sessions[0]
	if fresh.StorageRootID == old.StorageRootID || fresh.CWD != filepath.Join(next, "workspaces", fresh.ID) {
		t.Fatalf("new conversation did not bind current root: %+v", fresh)
	}
	count := len(core.state.Sessions)
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create", RequestID: "one-new-conversation"}); err != nil || len(core.state.Sessions) != count {
		t.Fatal("creation retry allocated another directory", err)
	}
	data, err := os.ReadFile(filepath.Join(firstRoot, "engine.json"))
	if err != nil || bytes.Contains(data, []byte("historical-private-body")) {
		t.Fatal("large transcript leaked into fixed configuration", err)
	}
}

func TestIndependentConversationDirectoriesAndDerivedWorkspace(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	ctx := context.Background()
	for range 2 {
		if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
			t.Fatal(err)
		}
	}
	first, second := core.state.Sessions[0], core.state.Sessions[1]
	if !validSessionDirectoryID(first.ID) || !validSessionDirectoryID(second.ID) || first.CWD == second.CWD {
		t.Fatal("independent conversations share their working directory")
	}
	if err := os.WriteFile(filepath.Join(first.CWD, "only-here.txt"), []byte("result"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(second.CWD, "only-here.txt")); !os.IsNotExist(err) {
		t.Fatal("unrelated workspace saw another conversation's files")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: first.ID}); err != nil {
		t.Fatal(err)
	}
	child := core.state.Sessions[0]
	if child.ID == first.ID || child.CWD != first.CWD {
		t.Fatal("derived conversation lost its shared workspace")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.delete", ID: child.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(first.CWD, "only-here.txt")); err != nil {
		t.Fatal("deleting a conversation removed shared working files", err)
	}
}

func TestOfflineHistoricalRootDoesNotBecomeEmptyConversation(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	first := filepath.Join(home, "first-data")
	if err := core.storage.RequestDirectory(first); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	old := core.state.Sessions[0]
	storeTestMessage(t, core, old.ID, "must-not-disappear")
	second := filepath.Join(home, "second-data")
	if err := core.storage.RequestDirectory(second); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	offline := first + "-offline"
	if err := os.Rename(first, offline); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	if core.state.Sessions[core.sessionIndex(old.ID)].StorageError == "" {
		t.Fatal("offline history was exposed as an empty usable conversation")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal("offline old root blocked a new conversation", err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: old.ID, Text: "continue"}}); err == nil {
		t.Fatal("an offline conversation accepted new execution")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("offline storage was silently recreated")
	}
	if err := os.Rename(offline, first); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	err := core.ensureSessionDataLocked(old.ID)
	core.mu.Unlock()
	if err != nil || len(core.history[old.ID]) != 1 {
		t.Fatal("reattached history could not be restored", err)
	}
}

func TestConversationCommitFailureKeepsPreviousRecord(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	storeTestMessage(t, core, session.ID, "committed history")
	previous := core.sessionRecords[session.ID]
	index := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	backup := index + ".test-backup"
	if err := os.Rename(index, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(index, 0700); err != nil {
		t.Fatal(err)
	}
	title := "uncommitted title"
	err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session.ID, Title: &title}})
	if err == nil {
		t.Fatal("blocked index accepted a conversation change")
	}
	if core.sessionRecords[session.ID] != previous || core.state.Sessions[0].Title != session.Title {
		t.Fatal("failed commit advanced the record or left changed memory")
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, index); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	err = core.ensureSessionDataLocked(session.ID)
	titleAfterRestart := core.state.Sessions[core.sessionIndex(session.ID)].Title
	historyAfterRestart := model.CloneMessages(core.history[session.ID])
	core.mu.Unlock()
	if err != nil || titleAfterRestart != session.Title || len(historyAfterRestart) != 1 {
		t.Fatal("a rejected operation changed the restarted conversation")
	}
}

func TestLateLoadFailedCommandDoesNotLoseHistory(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	storeTestMessage(t, core, session.ID, "late-loaded-body")
	core.mu.Lock()
	core.loadedSessions[session.ID] = false
	delete(core.history, session.ID)
	delete(core.context, session.ID)
	delete(core.state.Messages, session.ID)
	core.mu.Unlock()
	err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: session.ID, Text: "fails without a model"}})
	if err == nil {
		t.Fatal("missing provider accepted")
	}
	releaseView := pinSessionView(t, core, session.ID)
	defer releaseView()
	if len(core.history[session.ID]) != 1 || core.history[session.ID][0].Text != "late-loaded-body" {
		t.Fatal("failed validation erased lazily restored history")
	}
	storeTestMessage(t, core, session.ID, "continued")
	data, err := os.ReadFile(filepath.Join(core.storage.Directory(), "sessions", session.ID, core.sessionRecords[session.ID]+".json"))
	if err != nil || !bytes.Contains(data, []byte("late-loaded-body")) {
		t.Fatal("continued record lost restored history", err)
	}
}

func TestLargeConversationDataStaysOutsideConfigurationIndex(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	core.mu.Lock()
	body := "private-large-runtime-data-" + strings.Repeat("x", 128<<10)
	core.state.Sessions[0].Goal = &contracts.SessionGoal{ID: "goal-test", Instructions: body}
	core.state.Changes = append(core.state.Changes, contracts.Change{ID: "diff-test", SessionID: session.ID, NewContent: body})
	core.state.Terminals = append(core.state.Terminals, contracts.Terminal{ID: "terminal-test", SessionID: session.ID, Output: body})
	err := core.persistLocked(session.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(core.storage.ConfigDirectory(), "engine.json"))
	if err != nil || bytes.Contains(data, []byte("private-large-runtime-data-")) {
		t.Fatal("large goal, diff, or terminal payload leaked into the configuration index", err)
	}
	directory := filepath.Join(core.storage.Directory(), "sessions", session.ID)
	for range 3 {
		storeTestMessage(t, core, session.ID, "next revision")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("full transcript versions accumulated: %d files", len(entries))
	}
}

func TestDirtyPersistenceSkipsUnchangedConversationThatCannotEncode(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	for range 2 {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
			t.Fatal(err)
		}
	}
	target, unchanged := core.state.Sessions[0], core.state.Sessions[1]
	releaseTarget := pinSessionView(t, core, target.ID)
	defer releaseTarget()
	releaseUnchanged := pinSessionView(t, core, unchanged.ID)
	defer releaseUnchanged()
	core.mu.Lock()
	// A year outside JSON's supported RFC 3339 range makes the historical
	// execution record unencodable. It is intentionally left unchanged so a
	// targeted save must not inspect it.
	core.state.Messages[unchanged.ID] = []contracts.Message{{SessionID: unchanged.ID, Role: "user", CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), Blocks: []contracts.Block{{Kind: "text", Text: "corrupt in-memory history"}}}}
	title := "targeted update"
	core.state.Sessions[core.sessionIndex(target.ID)].Title = title
	err := core.persistLocked(target.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatalf("targeted save was blocked by unchanged history: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(core.storage.Directory(), "sessions", target.ID, core.sessionRecords[target.ID]+".json"))
	if err != nil || !bytes.Contains(data, []byte(title)) {
		t.Fatalf("target conversation was not saved: %v", err)
	}
}

func TestSessionRecordCleanupLeavesUnknownRecords(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	oldRecord := core.sessionRecords[session.ID]
	oldPath := filepath.Join(core.storage.Directory(), "sessions", session.ID, oldRecord+".json")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	unknownData := append(append([]byte(nil), data...), '\n')
	unknownHash := sha256.Sum256(unknownData)
	unknownRecord := hex.EncodeToString(unknownHash[:])
	if err := os.WriteFile(filepath.Join(core.storage.Directory(), "sessions", session.ID, unknownRecord+".json"), unknownData, 0600); err != nil {
		t.Fatal(err)
	}
	var disk sessionDisk
	if err := json.Unmarshal(unknownData, &disk); err != nil || disk.SessionID != session.ID {
		t.Fatalf("unknown fixture is not a valid session record: %v", err)
	}
	title := "replacement record"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session.ID, Title: &title}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(core.storage.Directory(), "sessions", session.ID, unknownRecord+".json")); err != nil {
		t.Fatalf("unknown session record was removed: %v", err)
	}
}

func TestSessionDirectoriesCannotEscapeRuntimeRoot(t *testing.T) {
	for _, name := range []string{"sessions", "workspaces"} {
		t.Run(name, func(t *testing.T) {
			core := openStorageTestCore(t, t.TempDir())
			defer core.Close()
			outside := t.TempDir()
			directory := filepath.Join(core.storage.Directory(), name)
			if err := os.Remove(directory); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, directory); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink privilege unavailable: %v", err)
				}
				t.Fatal(err)
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err == nil {
				t.Fatal("external linked directory was accepted for a conversation")
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatal("session reservation escaped the runtime root", err)
			}
		})
	}
}

func TestConversationDirectoryCannotAliasAnotherConversation(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	for range 2 {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
			t.Fatal(err)
		}
	}
	first, second := core.state.Sessions[0], core.state.Sessions[1]
	storeTestMessage(t, core, first.ID, "first private history")
	storeTestMessage(t, core, second.ID, "second private history")
	folder := filepath.Join(core.storage.Directory(), "sessions", first.ID)
	original := folder + "-original"
	if err := os.Rename(folder, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second.ID, folder); err != nil {
		_ = os.Rename(original, folder)
		if runtime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove(folder)
		_ = os.Rename(original, folder)
	}()
	title := "must not write into another conversation"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: first.ID, Title: &title}}); err == nil {
		t.Fatal("a link to another conversation's directory was accepted")
	}
	body, err := os.ReadFile(filepath.Join(core.storage.Directory(), "sessions", second.ID, core.sessionRecords[second.ID]+".json"))
	if err != nil || !bytes.Contains(body, []byte("second private history")) {
		t.Fatal("another conversation's committed history was removed", err)
	}
}

func TestCloseFlushesOnlineConversationWhenHistoricalRootIsOffline(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	t.Cleanup(func() { _ = core.Close() })
	firstRoot := filepath.Join(home, "first")
	if err := core.storage.RequestDirectory(firstRoot); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	old := core.state.Sessions[0]
	storeTestMessage(t, core, old.ID, "committed old history")
	secondRoot := filepath.Join(home, "second")
	if err := core.storage.RequestDirectory(secondRoot); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	fresh := core.state.Sessions[0]
	core.mu.Lock()
	var ensureErr error
	for _, session := range []contracts.Session{old, fresh} {
		if ensureErr = core.ensureSessionDataLocked(session.ID); ensureErr != nil {
			break
		}
		core.appendUserLocked(session.ID, "pending final flush")
		core.history[session.ID] = append(core.history[session.ID], model.Message{Role: "user", Text: "pending final flush"})
		core.state.Sessions[core.sessionIndex(session.ID)].Title = "Uncommitted title"
		core.markSessionDirtyLocked(session.ID)
	}
	core.mu.Unlock()
	if ensureErr != nil {
		t.Fatal(ensureErr)
	}
	// Windows deliberately holds the registered root lock open. Hiding its
	// identity marker makes Resolve unavailable without renaming an open root.
	marker := filepath.Join(firstRoot, ".gcode-root.json")
	offline := marker + ".offline"
	if err := os.Rename(marker, offline); err != nil {
		t.Fatal(err)
	}
	err := core.Close()
	if err == nil {
		t.Fatal("shutdown concealed unsaved historical data")
	}
	if err := os.Rename(offline, marker); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	if core.state.Sessions[core.sessionIndex(old.ID)].Title != old.Title {
		t.Fatal("the index committed metadata for a conversation whose record could not be saved")
	}
	core.mu.Lock()
	ensureErr = core.ensureSessionDataLocked(fresh.ID)
	if ensureErr == nil {
		ensureErr = core.ensureSessionDataLocked(old.ID)
	}
	freshHistory := model.CloneMessages(core.history[fresh.ID])
	oldHistory := model.CloneMessages(core.history[old.ID])
	core.mu.Unlock()
	if ensureErr != nil {
		t.Fatal(ensureErr)
	}
	if len(freshHistory) != 1 || freshHistory[0].Text != "pending final flush" {
		t.Fatal("an offline historical root prevented the online conversation's final flush")
	}
	if len(oldHistory) != 1 || oldHistory[0].Text != "committed old history" {
		t.Fatal("an offline save overwrote previously committed history")
	}
}

func TestCompoundSessionChangesRollBackIfAnyRecordCannotCommit(t *testing.T) {
	for _, kind := range []string{"group.delete", "session.reorder", "session.project.reorder"} {
		t.Run(kind, func(t *testing.T) {
			core := openStorageTestCore(t, t.TempDir())
			defer core.Close()
			first := newTestSession(t, core, "", "First")
			second := newTestSession(t, core, "", "Second")
			core.mu.Lock()
			core.state.SessionGroups = []contracts.SessionGroup{{ID: "group", Title: "Group"}}
			for rank, session := range []contracts.Session{first, second} {
				index := core.sessionIndex(session.ID)
				core.state.Sessions[index].GroupID, core.state.Sessions[index].GroupOrder = "group", rank
				if kind == "session.project.reorder" {
					core.state.Sessions[index].ProjectID = "project"
				}
			}
			err := core.persistLocked(first.ID, second.ID)
			before := contracts.CloneSnapshot(core.state)
			beforeRecord := core.sessionRecords[first.ID]
			core.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			folder := filepath.Join(core.storage.Directory(), "sessions", first.ID)
			backup := folder + ".blocked"
			if err := os.Rename(folder, backup); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(folder, []byte("unavailable conversation directory"), 0600); err != nil {
				_ = os.Rename(backup, folder)
				t.Fatal(err)
			}
			defer func() {
				_ = os.Remove(folder)
				_ = os.Rename(backup, folder)
			}()
			command := contracts.Command{Kind: kind, ID: "group"}
			if kind == "session.reorder" {
				command = contracts.Command{Kind: kind, Reorder: &contracts.ReorderInput{
					SourceID: first.ID, TargetID: second.ID, GroupID: "group", After: true,
				}}
			}
			if kind == "session.project.reorder" {
				command = contracts.Command{Kind: kind, Reorder: &contracts.ReorderInput{
					SourceID: first.ID, TargetID: second.ID, After: true,
				}}
			}
			if err := core.Execute(t.Context(), command); err == nil {
				t.Fatal("a compound change succeeded without saving every affected conversation")
			}
			state, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(state.SessionGroups) != len(before.SessionGroups) || core.sessionRecords[first.ID] != beforeRecord {
				t.Fatal("failed compound change replaced its committed group or record")
			}
			for _, session := range state.Sessions {
				for _, previous := range before.Sessions {
					if session.ID == previous.ID && (session.GroupID != previous.GroupID || session.GroupOrder != previous.GroupOrder || session.ProjectOrder != previous.ProjectOrder) {
						t.Fatal("failed compound change left partially updated session ordering")
					}
				}
			}
		})
	}
}
