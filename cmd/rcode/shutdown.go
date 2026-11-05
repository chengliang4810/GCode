package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

const desktopShutdownTimeout = 5 * time.Second

// shutdownDeadline belongs to the process, not an individual cleanup step.
// A blocked service, logger or profile close cannot extend the grace period.
type shutdownDeadline struct {
	mu      sync.Mutex
	timer   *time.Timer
	stopped bool
	timeout time.Duration
	force   func()
}

func newShutdownDeadline(timeout time.Duration, force func()) *shutdownDeadline {
	return &shutdownDeadline{timeout: timeout, force: force}
}

func (d *shutdownDeadline) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped || d.timer != nil {
		return
	}
	d.timer = time.AfterFunc(d.timeout, func() {
		d.mu.Lock()
		if d.stopped {
			d.mu.Unlock()
			return
		}
		d.stopped = true
		d.mu.Unlock()
		d.force()
	})
}

func (d *shutdownDeadline) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
}

func forceDesktopShutdown() {
	message := "GCode shutdown exceeded 5s; forcing exit; pending state and terminal output may not be saved"
	// Logging may itself be the blocked cleanup resource. Try both sinks
	// without letting either prevent the process deadline from taking effect.
	go log.Print(message)
	go func() { _, _ = fmt.Fprintln(os.Stderr, message) }()
	os.Exit(1)
}
