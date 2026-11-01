package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRuntimeLogRotatesWithoutLosingLargeWrites(t *testing.T) {
	store := runtimeTestStore(t)
	output, err := OpenRuntimeLog(store)
	if err != nil {
		t.Fatal(err)
	}
	writer := output.(*runtimeLog)
	writer.maxBytes = 16
	payload := strings.Repeat("x", 55)
	if n, err := output.Write([]byte(payload)); err != nil || n != len(payload) {
		t.Fatalf("write = %d, %v", n, err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(store.Directory(), "logs"))
	if err != nil {
		t.Fatal(err)
	}
	var bytes, files int
	for _, entry := range entries {
		if entry.Name() != runtimeLogFile && !isRuntimeLog(entry.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(store.Directory(), "logs", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 16 {
			t.Fatalf("log file exceeded its bound: %s %d", entry.Name(), len(data))
		}
		bytes += len(data)
		files++
	}
	if bytes != len(payload) || files != 4 {
		t.Fatalf("rotation lost data: %d bytes in %d files", bytes, files)
	}
	if _, err := output.Write([]byte("closed")); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("write after close = %v", err)
	}
}

func TestRuntimeLogSerializesWritesAndBoundsArchives(t *testing.T) {
	store := runtimeTestStore(t)
	output, err := OpenRuntimeLog(store)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	output.(*runtimeLog).maxBytes = 8
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 12 {
				if _, err := output.Write([]byte("record\n")); err != nil {
					t.Error(err)
				}
			}
		})
	}
	workers.Wait()
	entries, err := os.ReadDir(filepath.Join(store.Directory(), "logs"))
	if err != nil {
		t.Fatal(err)
	}
	archives := 0
	for _, entry := range entries {
		if isRuntimeLog(entry.Name()) {
			archives++
		}
	}
	if archives > RuntimeLogMaxFiles-1 {
		t.Fatalf("unbounded log archives: %d", archives)
	}
}

func TestRuntimeLogRotationAndPruningCanRunTogether(t *testing.T) {
	store := runtimeTestStore(t)
	writer, err := OpenRuntimeLog(store)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.(*runtimeLog).maxBytes = 32
	var work sync.WaitGroup
	work.Go(func() {
		for range 30 {
			if _, err := writer.Write([]byte(strings.Repeat("r", 40))); err != nil {
				t.Error(err)
			}
		}
	})
	work.Go(func() {
		for range 30 {
			if err := PruneRuntime(store); err != nil {
				t.Error(err)
			}
		}
	})
	work.Wait()
}

func TestRuntimeLogRequiresIdentityAndExclusiveWriter(t *testing.T) {
	store := runtimeTestStore(t)
	writer, err := OpenRuntimeLog(store)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if second, err := OpenRuntimeLog(store); err == nil {
		_ = second.Close()
		t.Fatal("two writers share the same active log")
	}
	if err := os.Remove(filepath.Join(store.Directory(), rootMarkerFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("must not write")); err == nil {
		t.Fatal("writer ignored root identity loss")
	}
}
