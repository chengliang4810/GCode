// Package testenv isolates filesystem and shell fixtures from machine defaults.
// It is imported only by test binaries.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Main uses a physical temporary path so storage security checks still reject
// symlinks, including in fixtures. A plain POSIX shell keeps real PTY tests from
// executing the developer's interactive startup files.
func Main(m *testing.M) {
	if runtime.GOOS == "windows" {
		// Windows subprocess fixtures must retain the exact typed environment,
		// including forward slashes supplied by their parent process.
		os.Exit(m.Run())
	}
	directory, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve test temporary directory:", err)
		os.Exit(1)
	}
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		if err := os.Setenv(key, directory); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	locale := "C.UTF-8"
	if runtime.GOOS == "darwin" {
		locale = "en_US.UTF-8"
	}
	for key, value := range map[string]string{"SHELL": "/bin/sh", "PS1": ShellPrompt(), "LANG": locale, "LC_ALL": locale, "LC_CTYPE": locale, "ENV": ""} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

// ShellPrompt is an observable readiness marker for real interactive fixtures.
func ShellPrompt() string {
	if runtime.GOOS == "windows" {
		return "PS "
	}
	return "GCODE_TEST_READY> "
}
