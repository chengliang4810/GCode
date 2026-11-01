package plugins

import (
	"errors"
	"io/fs"
	"runtime"
	"syscall"
	"testing"
)

func TestTransientRenameErrorOnlyMatchesWindowsHandleRaces(t *testing.T) {
	transient := []error{
		&fs.PathError{Op: "rename", Path: "x", Err: syscall.Errno(5)},
		&fs.PathError{Op: "rename", Path: "x", Err: syscall.Errno(32)},
		&fs.PathError{Op: "rename", Path: "x", Err: syscall.Errno(33)},
	}
	permanent := []error{
		&fs.PathError{Op: "rename", Path: "x", Err: syscall.Errno(3)},
		errors.New("plain failure"),
		nil,
	}
	for _, err := range transient {
		if runtime.GOOS == "windows" && !transientRenameError(err) {
			t.Fatalf("expected transient classification for %v", err)
		}
		if runtime.GOOS != "windows" && transientRenameError(err) {
			t.Fatalf("retry must stay Windows-only, classified %v", err)
		}
	}
	for _, err := range permanent {
		if transientRenameError(err) {
			t.Fatalf("persistent error misclassified as transient: %v", err)
		}
	}
}

func TestRenameWithRetryRecoversFromTransientDenials(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("handle-race retry is a Windows-only behavior")
	}
	original := renameFunc
	t.Cleanup(func() { renameFunc = original })

	attempts := 0
	renameFunc = func(oldname, newname string) error {
		attempts++
		if attempts <= 2 {
			return &fs.PathError{Op: "rename", Path: oldname, Err: syscall.Errno(5)}
		}
		return nil
	}
	if err := renameWithRetry("a", "b"); err != nil {
		t.Fatalf("transient denials should clear within the bounded retry: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}

	attempts = 0
	renameFunc = func(oldname, newname string) error {
		attempts++
		return &fs.PathError{Op: "rename", Path: oldname, Err: syscall.Errno(3)}
	}
	if err := renameWithRetry("a", "b"); err == nil {
		t.Fatal("persistent error must surface on the first attempt")
	}
	if attempts != 1 {
		t.Fatalf("persistent error must not be retried, got %d attempts", attempts)
	}
}
