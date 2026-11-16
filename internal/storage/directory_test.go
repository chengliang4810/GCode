package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedDirectoryMutationsKeepExistingEntries(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := MkdirRoot(root, "sessions", 0700); err != nil {
		t.Fatal(err)
	}
	if err := MkdirRoot(root, "sessions", 0700); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing directory: %v", err)
	}
	if err := root.WriteFile(filepath.Join("sessions", "keep.json"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoot(root, "sessions"); err == nil {
		t.Fatal("removed a nonempty directory")
	}
	if data, err := root.ReadFile(filepath.Join("sessions", "keep.json")); err != nil || string(data) != "owned" {
		t.Fatalf("existing data changed: %q, %v", data, err)
	}
	if err := RemoveRoot(root, filepath.Join("sessions", "keep.json")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoot(root, "sessions"); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Lstat("sessions"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory still exists: %v", err)
	}
}

func TestOwnedDirectoryMutationsStayWithinRoot(t *testing.T) {
	directory, outside := t.TempDir(), t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := MkdirRoot(root, filepath.Join(outside, "escape"), 0700); err == nil {
		t.Fatal("created outside root")
	}
	file := filepath.Join(outside, "keep")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoot(root, file); err == nil {
		t.Fatal("removed outside root")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
		t.Fatalf("outside data changed: %q, %v", data, err)
	}
}
