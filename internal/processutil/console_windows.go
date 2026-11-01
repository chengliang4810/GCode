package processutil

import (
	"os/exec"
	"syscall"
)

// HideConsole prevents a background command from allocating a Windows console.
// It preserves other process settings and does not hide a GUI application's UI.
// Do not use it for commands attached to a real terminal or ConPTY.
func HideConsole(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	const createNoWindow = 0x08000000
	command.SysProcAttr.CreationFlags |= createNoWindow
}
