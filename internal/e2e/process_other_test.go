//go:build e2e && !windows

package e2e

import "os/exec"

func configureProcess(_ *exec.Cmd) {}
