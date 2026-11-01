//go:build !windows

package processutil

import "os/exec"

// HideConsole is unnecessary on Unix, where exec does not allocate a terminal.
func HideConsole(_ *exec.Cmd) {}
