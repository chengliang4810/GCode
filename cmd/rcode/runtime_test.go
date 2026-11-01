package main

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/storage"
)

func TestClientRuntimeMaintenancePreparesLocalDataAndStops(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stop, err := startClientRuntimeMaintenance(store)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for _, name := range []string{"logs", "cache/gcode", "tmp/gcode", "browser", "downloads"} {
		if info, err := os.Stat(filepath.Join(store.Directory(), filepath.FromSlash(name))); err != nil || !info.IsDir() {
			t.Fatalf("client runtime directory %s = %v", name, err)
		}
	}
	done := make(chan struct{})
	go func() {
		stop()
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("client maintenance did not stop and drain its error stream")
	}
	if _, err := store.Resolve(store.ActiveRootID()); err != nil {
		t.Fatalf("stopping client maintenance closed the client's store: %v", err)
	}
}

func TestClientRuntimeMaintenanceRefusesUnavailableRoot(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	marker := filepath.Join(store.Directory(), ".gcode-root.json")
	if err := os.Rename(marker, marker+".offline"); err != nil {
		t.Fatal(err)
	}
	if stop, err := startClientRuntimeMaintenance(store); err == nil || stop != nil {
		t.Fatal("client maintenance accepted an unavailable registered root")
	}
	if _, err := os.Stat(filepath.Join(store.Directory(), "cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed client preparation created a fallback cache: %v", err)
	}
}

func TestClientRuntimeMaintenanceReportsAndDrainsErrors(t *testing.T) {
	previous := log.Writer()
	var console bytes.Buffer
	log.SetOutput(&console)
	defer log.SetOutput(previous)
	failures, done := make(chan error), make(chan struct{})
	go consumeClientRuntimeErrors(failures, done)
	failures <- errors.New("test removable disk unavailable")
	close(failures)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("client maintenance errors were not drained")
	}
	if !strings.Contains(console.String(), "local client runtime maintenance: test removable disk unavailable") {
		t.Fatalf("maintenance failure not visible: %q", console.String())
	}
}
