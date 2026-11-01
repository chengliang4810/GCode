//go:build e2e && windows

package e2e

import (
	"os/exec"

	"gcode/internal/processutil"
)

func configureProcess(cmd *exec.Cmd) {
	// HideWindow sets SW_HIDE and would hide the native app's first window,
	// preventing real compositor/input tests. Suppress only console creation.
	processutil.HideConsole(cmd)
}
