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

func TestWriteJSONWaitsForWindowsReaderWithoutLosingOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteJSON(path, "old"); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result := make(chan error, 1)
	go func() { result <- WriteJSON(path, "new") }()
	select {
	case err := <-result:
		t.Fatalf("replacement did not wait for the open reader: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	before, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(before)) != `"old"` {
		t.Fatalf("retry lost the original file: %q, %v", before, err)
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
		t.Fatal("replacement did not finish after the reader closed")
	}
	after, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(after)) != `"new"` {
		t.Fatalf("replacement did not save the new file: %q, %v", after, err)
	}
}

func TestWriteJSONWindowsPermanentDenialPreservesOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteJSON(path, "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0600); err != nil {
			t.Error(err)
		}
	})
	start := time.Now()
	err := WriteJSON(path, "new")
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) || time.Since(start) > 2*time.Second {
		t.Fatalf("permanent denial was not returned within the retry budget: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) != `"old"` {
		t.Fatalf("failed replacement lost the original file: %q, %v", data, err)
	}
	staged, err := filepath.Glob(filepath.Join(dir, ".write-*"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("failed replacement left staged files: %v, %v", staged, err)
	}
}

func TestDirectoryRegistrationWriteFailureLeavesRegistryUnchanged(t *testing.T) {
	home := t.TempDir()
	s := mustOpen(t, Options{Home: home})
	locationPath := filepath.Join(s.ConfigDirectory(), locationFile)
	before, err := os.ReadFile(locationPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locationPath, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locationPath, 0600) })
	target := filepath.Join(home, "not-registered")
	if err := s.RequestDirectory(target); err == nil {
		t.Fatal("read-only location registry accepted a change")
	}
	after, err := os.ReadFile(locationPath)
	if err != nil || string(before) != string(after) || s.Pending() != "" || len(s.Roots()) != 1 {
		t.Fatal("failed registration changed the persisted or in-memory registry")
	}
	if _, err := os.Stat(filepath.Join(target, rootMarkerFile)); !os.IsNotExist(err) {
		t.Fatal("failed registration left an unregistered identity marker")
	}
	if err := os.Chmod(locationPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestDirectory(target); err != nil {
		t.Fatal("retry after correcting permissions did not succeed", err)
	}
}
