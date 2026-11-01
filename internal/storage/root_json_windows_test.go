package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWriteJSONRootWaitsForWindowsReader(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteJSONRoot(root, "state.json", "old"); err != nil {
		t.Fatal(err)
	}
	// os.Root.Open 允许 FILE_SHARE_DELETE；用普通读句柄构造真实替换冲突。
	reader, err := os.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result := make(chan error, 1)
	go func() { result <- WriteJSONRoot(root, "state.json", "new") }()
	select {
	case err := <-result:
		t.Fatalf("replacement did not wait for the open reader: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	before, err := root.ReadFile("state.json")
	if err != nil || strings.TrimSpace(string(before)) != `"old"` {
		t.Fatalf("replacement lost old content while waiting: %q %v", before, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replacement did not finish after reader closed")
	}
	after, err := root.ReadFile("state.json")
	if err != nil || strings.TrimSpace(string(after)) != `"new"` {
		t.Fatalf("replacement result: %q %v", after, err)
	}
}

func TestWriteJSONRootPermanentDenialPreservesOldFile(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteJSONRoot(root, "state.json", "old"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "state.json")
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	start := time.Now()
	err = WriteJSONRoot(root, "state.json", "new")
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) || time.Since(start) > 2*time.Second {
		t.Fatalf("permanent denial did not honor retry budget: %v", err)
	}
	data, err := root.ReadFile("state.json")
	if err != nil || strings.TrimSpace(string(data)) != `"old"` {
		t.Fatalf("failed replacement lost old file: %q %v", data, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("failed replacement left staging files: %v %v", entries, err)
	}
}
