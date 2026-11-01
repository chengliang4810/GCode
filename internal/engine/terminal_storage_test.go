package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func terminalStorageCore(t *testing.T) (*Engine, string) {
	t.Helper()
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core := &Engine{storage: store}
	t.Cleanup(func() { _ = core.storage.Close() })
	core.state.Terminals = []contracts.Terminal{{
		ID: "terminal-saved", StorageRootID: store.ActiveRootID(), Status: "exited",
		Output: strings.Repeat("private terminal history\n", 10000),
	}}
	return core, home
}

func TestStandaloneTerminalOutputShardsAndCommitsAfterIndex(t *testing.T) {
	core, _ := terminalStorageCore(t)
	output := core.state.Terminals[0].Output
	snapshot := core.state
	if err := core.persistStandaloneTerminals(&snapshot); err != nil {
		t.Fatal(err)
	}
	saved := snapshot.Terminals[0]
	if saved.Output != "" || !validRecordHash(saved.OutputRef) || saved.StorageError != "" {
		t.Fatalf("large terminal output remained in the index: %+v", saved)
	}
	if core.state.Terminals[0].OutputRef != "" || core.state.Terminals[0].Output != output {
		t.Fatal("staging changed the committed reference or runtime output")
	}
	path := filepath.Join(core.storage.Directory(), "terminals", saved.OutputRef+".json")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte("private terminal history")) {
		t.Fatal("output was not sharded at its bound root", err)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != saved.OutputRef {
		t.Fatal("immutable reference does not match the actual bytes")
	}
	// 模拟固定索引提交成功后，宿主才公开新的引用。
	core.commitStandaloneTerminals(&snapshot)
	core.state = snapshot
	if err := core.loadStandaloneTerminals(); err != nil {
		t.Fatal(err)
	}
	if core.state.Terminals[0].Output != output || core.state.Terminals[0].StorageError != "" {
		t.Fatal("standalone terminal history did not reload from the reference")
	}
}

func TestStandaloneTerminalFailedIndexKeepsOldReferenceAndBlob(t *testing.T) {
	core, _ := terminalStorageCore(t)
	first := core.state
	if err := core.persistStandaloneTerminals(&first); err != nil {
		t.Fatal(err)
	}
	core.commitStandaloneTerminals(&first)
	oldRef := core.state.Terminals[0].OutputRef
	oldPath := filepath.Join(core.storage.Directory(), "terminals", oldRef+".json")
	core.state.Terminals[0].Output = "new output that is not yet indexed"
	next := core.state
	if err := core.persistStandaloneTerminals(&next); err != nil {
		t.Fatal(err)
	}
	if next.Terminals[0].OutputRef == oldRef || core.state.Terminals[0].OutputRef != oldRef {
		t.Fatal("staged output replaced the previous committed reference")
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatal("old history was removed before index commit", err)
	}
	// 丢弃 next 即索引替换失败；旧索引仍能恢复旧正文。
	core.state.Terminals[0].Output = ""
	if err := core.loadStandaloneTerminals(); err != nil || core.state.Terminals[0].StorageError != "" ||
		core.state.Terminals[0].Output != strings.Repeat("private terminal history\n", 10000) {
		t.Fatal("discarding a staged index damaged the previous history", err)
	}
	// 后续真正提交新索引后，才允许删除旧引用。
	core.state.Terminals[0].Output = "new output that is not yet indexed"
	core.commitStandaloneTerminals(&next)
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("unreferenced old blob was not cleaned after commit")
	}
}

func TestStandaloneTerminalCorruptionRemainsVisibleWithoutBlockingIndex(t *testing.T) {
	core, _ := terminalStorageCore(t)
	snapshot := core.state
	if err := core.persistStandaloneTerminals(&snapshot); err != nil {
		t.Fatal(err)
	}
	core.commitStandaloneTerminals(&snapshot)
	core.state = snapshot
	ref := core.state.Terminals[0].OutputRef
	path := filepath.Join(core.storage.Directory(), "terminals", ref+".json")
	if err := os.WriteFile(path, []byte(`{"output":"wrong bytes"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.loadStandaloneTerminals(); err != nil {
		t.Fatal("one corrupt terminal blocked global startup", err)
	}
	if core.state.Terminals[0].StorageError == "" || core.state.Terminals[0].Output != "" {
		t.Fatal("corrupt terminal history appeared as a usable empty history")
	}
	after := core.state
	if err := core.persistStandaloneTerminals(&after); err != nil {
		t.Fatal("one corrupt historical terminal blocked another index commit", err)
	}
	if after.Terminals[0].OutputRef != ref || after.Terminals[0].Output != "" {
		t.Fatal("failed history load replaced its committed reference")
	}
}

func TestStandaloneTerminalOfflineRootDoesNotBlockAnotherRoot(t *testing.T) {
	core, home := terminalStorageCore(t)
	firstPath := filepath.Join(home, "first")
	if err := core.storage.RequestDirectory(firstPath); err != nil {
		t.Fatal(err)
	}
	_ = core.storage.Close()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core.storage = store
	core.state.Terminals[0].StorageRootID = store.ActiveRootID()
	snapshot := core.state
	if err := core.persistStandaloneTerminals(&snapshot); err != nil {
		t.Fatal(err)
	}
	core.commitStandaloneTerminals(&snapshot)
	if err := store.RequestDirectory(filepath.Join(home, "second")); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if err := os.Rename(firstPath, firstPath+"-offline"); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core.storage, core.state = store, snapshot
	if err := core.loadStandaloneTerminals(); err != nil || core.state.Terminals[0].StorageError == "" {
		t.Fatal("offline terminal root was not isolated", err)
	}
	core.state.Terminals = append(core.state.Terminals, contracts.Terminal{
		ID: "terminal-online", StorageRootID: store.ActiveRootID(), Status: "exited", Output: "online history",
	})
	next := core.state
	if err := core.persistStandaloneTerminals(&next); err != nil {
		t.Fatal("offline old terminal blocked the online terminal", err)
	}
	if next.Terminals[0].OutputRef != snapshot.Terminals[0].OutputRef ||
		!validRecordHash(next.Terminals[1].OutputRef) {
		t.Fatal("offline reference was lost or online output was not stored")
	}
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatal("offline terminal root was recreated")
	}
}

func TestStandaloneTerminalReferencesCannotTraverseOrBorrowIdentity(t *testing.T) {
	core, _ := terminalStorageCore(t)
	snapshot := core.state
	if err := core.persistStandaloneTerminals(&snapshot); err != nil {
		t.Fatal(err)
	}
	ref := snapshot.Terminals[0].OutputRef
	for _, bad := range []string{"../state", `..\state`, "state.json", strings.Repeat("a", 63)} {
		item := snapshot.Terminals[0]
		item.OutputRef = bad
		if _, err := core.readTerminalOutput(item); err == nil {
			t.Fatalf("accepted unsafe output reference %q", bad)
		}
	}
	other := snapshot.Terminals[0]
	other.ID, other.OutputRef = "terminal-another", ref
	if _, err := core.readTerminalOutput(other); err == nil {
		t.Fatal("another terminal borrowed the first terminal's output identity")
	}
}

func TestStandaloneTerminalHelperLeavesSessionOwnedOutputToItsShard(t *testing.T) {
	core, _ := terminalStorageCore(t)
	core.state.Terminals[0].SessionID = "session-owned"
	snapshot := core.state
	if err := core.persistStandaloneTerminals(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Terminals[0].Output != core.state.Terminals[0].Output || snapshot.Terminals[0].OutputRef != "" {
		t.Fatal("standalone helper took ownership of a session terminal")
	}
	if _, err := os.Stat(filepath.Join(core.storage.Directory(), "terminals")); !os.IsNotExist(err) {
		t.Fatal("session-owned terminal created a standalone output directory")
	}
}

func TestStandaloneTerminalCloseRemovesOnlyCommittedUnreferencedOutput(t *testing.T) {
	for _, failIndex := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "failed-index"}[failIndex], func(t *testing.T) {
			core, _ := terminalStorageCore(t)
			snapshot := core.state
			if err := core.persistStandaloneTerminals(&snapshot); err != nil {
				t.Fatal(err)
			}
			core.commitStandaloneTerminals(&snapshot)
			item := core.state.Terminals[0]
			path := filepath.Join(core.storage.Directory(), "terminals", item.OutputRef+".json")
			if failIndex {
				if err := os.Mkdir(filepath.Join(core.storage.ConfigDirectory(), "engine.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := core.closeTerminalLocked(item.ID)
			if failIndex {
				if err == nil {
					t.Fatal("failed index was accepted")
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatal("failed close deleted committed history", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("successful close left unreferenced output", err)
			}
		})
	}
}

func TestStandaloneTerminalFirstOutputFailureSurvivesRestart(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	core.state.Terminals = []contracts.Terminal{{
		ID: "terminal-first-output", StorageRootID: core.storage.ActiveRootID(),
		Status: "exited", Output: "first unsaved output",
	}}
	blocker := filepath.Join(core.storage.Directory(), "terminals")
	if err := os.WriteFile(blocker, []byte("block writes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.persistLocked(); err != nil {
		t.Fatal("one failed output blocked the configuration index", err)
	}
	if !core.state.Terminals[0].OutputIncomplete || core.state.Terminals[0].OutputRef != "" {
		t.Fatal("first failed output was not marked incomplete")
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	item := core.state.Terminals[0]
	if !item.OutputIncomplete || item.StorageError == "" || item.Output != "" {
		t.Fatalf("restart pretended an unsaved output was complete empty history: %+v", item)
	}
	core.state.Terminals[0].Output = "actual new runtime output"
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	if core.state.Terminals[0].OutputIncomplete || core.state.Terminals[0].StorageError != "" ||
		!validRecordHash(core.state.Terminals[0].OutputRef) {
		t.Fatal("successful new output did not clear the incomplete marker")
	}
}

func TestStandaloneTerminalFailedUpdateKeepsIncompleteFlagAfterOldReferenceReload(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	core.state.Terminals = []contracts.Terminal{{
		ID: "terminal-prior-output", StorageRootID: core.storage.ActiveRootID(),
		Status: "exited", Output: "committed output",
	}}
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	oldRef := core.state.Terminals[0].OutputRef
	directory := filepath.Join(core.storage.Directory(), "terminals")
	saved := directory + ".held"
	if err := os.Rename(directory, saved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory, []byte("block writes"), 0600); err != nil {
		t.Fatal(err)
	}
	core.state.Terminals[0].Output += "\nunsaved tail"
	if err := core.persistLocked(); err != nil {
		t.Fatal("failed terminal update blocked global persistence", err)
	}
	if core.state.Terminals[0].OutputRef != oldRef || !core.state.Terminals[0].OutputIncomplete {
		t.Fatal("failed update lost its committed reference or incompleteness marker")
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(saved, directory); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	item := core.state.Terminals[0]
	if item.Output != "committed output" || item.OutputRef != oldRef || !item.OutputIncomplete || item.StorageError == "" {
		t.Fatalf("old-reference reload hid the unsaved tail: %+v", item)
	}
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	if !core.state.Terminals[0].OutputIncomplete || core.state.Terminals[0].StorageError == "" {
		t.Fatal("rewriting previously saved output falsely marked the exited terminal complete")
	}
}
