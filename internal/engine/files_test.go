package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

// A pipe in the project used to block file.open while the engine lock was held,
// which stops every command, snapshot and save.
func TestFileCommandsRejectNonRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes need a different setup on Windows")
	}
	home, workspace := t.TempDir(), t.TempDir()
	pipe := filepath.Join(workspace, "pipe.txt")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Skipf("cannot create a named pipe: %v", err)
	}
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
		t.Fatal(err)
	}
	state := waitForProjectFiles(t, core)
	var file contracts.File
	for _, candidate := range state.Files {
		if candidate.Path == "pipe.txt" || filepath.Base(candidate.Path) == "pipe.txt" {
			file = candidate
		}
	}
	if file.ID == "" {
		t.Skip("project enumeration did not register the pipe")
	}
	done := make(chan error, 1)
	go func() { done <- core.Execute(ctx, contracts.Command{Kind: "file.open", ID: file.ID}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("opening a named pipe succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("opening a named pipe blocked the engine lock")
	}
	if _, err := os.Stat(pipe); err != nil {
		t.Fatal(err)
	}
}
