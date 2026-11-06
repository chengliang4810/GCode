package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestPersistenceIndexExcludesRefreshableFileTree(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	core.mu.Lock()
	core.state.Files = []contracts.File{{
		ID: "private-tree-entry", Path: "private-tree-entry",
		Name: "private-tree-entry", Content: "private-open-editor-body", Loaded: true,
	}}
	err := core.persistLocked()
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(core.storage.ConfigDirectory(), "engine.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("private-tree-entry")) || bytes.Contains(data, []byte("private-open-editor-body")) {
		t.Fatal("refreshable file metadata or editor contents leaked into the durable index")
	}
	if len(core.state.Files) != 1 || core.state.Files[0].Content != "private-open-editor-body" {
		t.Fatal("writing the index changed the live editor projection")
	}
}
