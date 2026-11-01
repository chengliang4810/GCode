package agent

import (
	"os/exec"
	"strconv"
	"syscall"
)

func prepareProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Cancel = func() error {
		kill := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return kill.Run()
	}
}
