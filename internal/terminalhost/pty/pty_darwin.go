// Copyright (c) 2026 MyGo contributors. MIT license; see LICENSE.
// Adapted from the pinned MyGo v0.6.4 PTY implementation.

package pty

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The ioctls of grantpt, unlockpt and ptsname on macOS (sys/ttycom.h).
const (
	tiocptygrant = 0x20007454 // _IO('t', 84)
	tiocptyunlk  = 0x20007452 // _IO('t', 82)
	tiocptygname = 0x40807453 // _IOC(IOC_OUT, 't', 83, 128)
)

func openMaster() (*os.File, string, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", fmt.Errorf("pty: %w", err)
	}
	// Registering an unopened Darwin PTY with Go's netpoller enables
	// nonblocking reads before grant/unlock. Keep this character device in
	// blocking mode; session shutdown kills its child before waiting on EOF.
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	var name [128]byte
	for _, step := range []struct {
		req uintptr
		arg unsafe.Pointer
	}{{tiocptygrant, nil}, {tiocptyunlk, nil}, {tiocptygname, unsafe.Pointer(&name[0])}} {
		if err := ioctl(master, step.req, step.arg); err != nil {
			master.Close()
			return nil, "", fmt.Errorf("pty: %w", err)
		}
	}
	n := bytes.IndexByte(name[:], 0)
	if n < 0 {
		n = len(name)
	}
	return master, string(name[:n]), nil
}
