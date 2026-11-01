package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func runtimeTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func runtimeTestFile(t *testing.T, root, name, content string, modified time.Time) string {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filename, modified, modified); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestPrepareRuntimeRequiresRegisteredRootIdentity(t *testing.T) {
	store := runtimeTestStore(t)
	root := store.Directory()
	marker := filepath.Join(root, rootMarkerFile)
	if err := os.Rename(marker, marker+".offline"); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntime(store); err == nil {
		t.Fatal("runtime directories created without registered root identity")
	}
	if _, err := os.Stat(filepath.Join(root, "logs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed preparation created runtime state: %v", err)
	}
	if err := os.Rename(marker+".offline", marker); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntime(store); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"logs", "cache/gcode", "tmp/gcode", "browser", "downloads"} {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(directory))); err != nil || !info.IsDir() {
			t.Fatalf("runtime directory %s missing: %v", directory, err)
		}
	}
}

func TestPruneRuntimeOnlyRemovesOwnedDisposableFiles(t *testing.T) {
	store := runtimeTestStore(t)
	root := store.Directory()
	old := time.Now().Add(-60 * 24 * time.Hour)
	preserved := []string{
		"sessions/session/messages.json", "workspaces/project/source.go",
		"worktrees/task/source.go", "browser/Cookies", "cache/user-file.txt",
		"tmp/user-file.txt", "logs/user.log", "logs/gcode.log", "logs/nested/gcode-old.log",
		"downloads/user-download.zip",
	}
	for _, name := range append(preserved, "logs/gcode-old.log", "cache/gcode/nested/old.bin", "tmp/gcode/old.tmp") {
		runtimeTestFile(t, root, name, "preserve unless application-owned", old)
	}
	if err := PruneRuntime(store); err != nil {
		t.Fatal(err)
	}
	for _, name := range preserved {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("unrelated data %s changed: %v", name, err)
		}
	}
	for _, name := range []string{"logs/gcode-old.log", "cache/gcode/nested/old.bin", "tmp/gcode/old.tmp"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expired runtime file %s retained: %v", name, err)
		}
	}
}

func TestRuntimeRetentionEnforcesAgeCountAndBytes(t *testing.T) {
	for _, test := range []struct {
		name  string
		limit runtimeRetention
		age   time.Duration
	}{
		{name: "age", limit: runtimeRetention{files: 10, bytes: 100, age: time.Hour}, age: 2 * time.Hour},
		{name: "count", limit: runtimeRetention{files: 1, bytes: 100, age: 24 * time.Hour}},
		{name: "bytes", limit: runtimeRetention{files: 10, bytes: 4, age: 24 * time.Hour}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := runtimeTestStore(t)
			now := time.Now()
			root, _, err := openRuntimeRoot(store)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			runtimeTestFile(t, store.Directory(), "logs/gcode-older.log", "1234", now.Add(-test.age-time.Minute))
			runtimeTestFile(t, store.Directory(), "logs/gcode-newer.log", "1234", now.Add(-test.age))
			if err := pruneRuntimeDirectory(root, "logs", test.limit, true, now, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := root.Stat("logs/gcode-older.log"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("oldest file survived retention: %v", err)
			}
			_, err = root.Stat("logs/gcode-newer.log")
			if test.name == "age" && !errors.Is(err, os.ErrNotExist) || test.name != "age" && err != nil {
				t.Fatalf("newest file violated %s policy: %v", test.name, err)
			}
		})
	}
}

func TestRuntimePruningNeverFollowsSymlinks(t *testing.T) {
	store := runtimeTestStore(t)
	if err := PrepareRuntime(store); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	target := runtimeTestFile(t, external, "keep.txt", "outside", time.Now().Add(-60*24*time.Hour))
	link := filepath.Join(store.Directory(), "cache", "gcode", "outside")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	if err := PruneRuntime(store); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "outside" {
		t.Fatalf("pruning escaped its namespace: %q, %v", data, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("pruning removed the symlink")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	namespace := filepath.Join(store.Directory(), "tmp", "gcode")
	if err := os.Remove(namespace); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, namespace); err != nil {
		t.Fatal(err)
	}
	if err := PruneRuntime(store); err == nil {
		t.Fatal("pruning accepted a symlink as its namespace root")
	}
}

func TestRuntimeMaintenanceReportsFailureAndStops(t *testing.T) {
	store := runtimeTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	failures := StartRuntimeMaintenance(ctx, store)
	select {
	case err := <-failures:
		if err == nil {
			t.Fatal("closed storage was silently accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("maintenance did not report its error")
	}
	cancel()
	select {
	case _, open := <-failures:
		if open {
			t.Fatal("maintenance did not close after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("maintenance leaked after cancellation")
	}
}
