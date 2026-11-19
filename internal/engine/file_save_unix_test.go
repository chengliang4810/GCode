//go:build unix

package engine

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFileSaveRejectsFIFO(t *testing.T) {
	t.Parallel()
	root, dir := fileSaveFixture(t)
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- saveEditorFile(root, "pipe", []byte("new"), 0, fileSaveIO{})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("saved a FIFO")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("saving FIFO blocked")
	}
	assertNoFileSaveStage(t, dir)
}

func TestFileSaveRejectsFIFOReplacement(t *testing.T) {
	t.Parallel()
	root, dir := fileSaveFixture(t)
	done := make(chan error, 1)
	go func() {
		done <- saveEditorFile(root, "document.txt", []byte("new"), fileVersion([]byte("original")), fileSaveIO{
			beforeCommit: func() error {
				if err := root.Rename("document.txt", "old.txt"); err != nil {
					return err
				}
				return syscall.Mkfifo(filepath.Join(dir, "document.txt"), 0600)
			},
		})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("overwrote a FIFO replacement")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("saving FIFO replacement blocked")
	}
	info, err := root.Lstat("document.txt")
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("external FIFO replaced: %v, %v", info, err)
	}
	assertFileSaveContent(t, root, "old.txt", "original")
	assertNoFileSaveStage(t, dir)
}

func TestFileSaveRejectsReadOnlySource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file write permissions")
	}
	t.Parallel()
	root, dir := fileSaveFixture(t)
	if err := root.Chmod("document.txt", 0400); err != nil {
		t.Fatal(err)
	}
	err := saveEditorFile(root, "document.txt", []byte("new"), fileVersion([]byte("original")), fileSaveIO{})
	if err == nil {
		t.Fatal("atomic replacement bypassed source write permissions")
	}
	assertFileSaveContent(t, root, "document.txt", "original")
	assertNoFileSaveStage(t, dir)
}
