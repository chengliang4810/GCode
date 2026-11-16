package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGitOutputDoesNotRunProjectFilesystemMonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture; Windows is cross-compiled separately")
	}
	directory := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", directory}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s: %v", output, err)
		}
	}
	run("init")
	monitor := filepath.Join(directory, "monitor.sh")
	marker := filepath.Join(directory, "monitor-ran")
	if err := os.WriteFile(monitor, []byte("#!/bin/sh\nprintf executed > monitor-ran\nprintf 'token\\0'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	run("config", "core.fsmonitor", monitor)
	if _, err := gitOutput(t.Context(), directory, "status", "--porcelain"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("project monitor ran in application git: %v", err)
	}
	// Prove the fixture really is executable by the same Git installation.
	run("status", "--porcelain")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("control did not invoke configured monitor: %v", err)
	}
}
