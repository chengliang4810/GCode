package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"gcode/internal/processutil"
)

func TestShutdownDeadlineForcesOnceAtFiveSeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		deadline := newShutdownDeadline(desktopShutdownTimeout, func() { calls.Add(1) })
		deadline.Start()
		time.Sleep(4 * time.Second)
		deadline.Start() // Repeated close/quit callbacks must not reset the budget.
		time.Sleep(time.Second - time.Nanosecond)
		if calls.Load() != 0 {
			t.Fatal("shutdown forced before the five-second grace period")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatalf("force calls=%d, want 1", calls.Load())
		}
		deadline.Start()
		time.Sleep(desktopShutdownTimeout)
		if calls.Load() != 1 {
			t.Fatal("repeated close scheduled another forced exit")
		}
	})
}

func TestShutdownDeadlineDisarmsOnlyAfterAllCleanup(t *testing.T) {
	for _, beforeStart := range []bool{false, true} {
		name := "after-cleanup"
		if beforeStart {
			name = "before-start"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var calls atomic.Int32
				deadline := newShutdownDeadline(desktopShutdownTimeout, func() { calls.Add(1) })
				if beforeStart {
					deadline.Stop()
					deadline.Start()
				} else {
					deadline.Start()
					// One shared budget includes service and profile/log cleanup.
					time.Sleep(2 * time.Second)
					time.Sleep(2 * time.Second)
					deadline.Stop()
				}
				time.Sleep(2 * desktopShutdownTimeout)
				synctest.Wait()
				if calls.Load() != 0 {
					t.Fatal("completed shutdown left a live forced-exit timer")
				}
			})
		})
	}
}

type blockedShutdownLog struct{}

func (blockedShutdownLog) Write([]byte) (int, error) { select {} }

func TestShutdownDeadlineExitsBlockedProcess(t *testing.T) {
	if os.Getenv("RCODE_TEST_SHUTDOWN_PROCESS") == "1" {
		deadline := newShutdownDeadline(desktopShutdownTimeout, forceDesktopShutdown)
		deadline.Start()
		// The normal logger is deliberately unusable. A logging deadlock
		// must not turn the five-second exit policy into another infinite wait.
		log.SetOutput(blockedShutdownLog{})
		select {}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShutdownDeadlineExitsBlockedProcess$")
	command.Env = append(os.Environ(), "RCODE_TEST_SHUTDOWN_PROCESS=1")
	processutil.HideConsole(command)
	started := time.Now()
	err := command.Run()
	elapsed := time.Since(started)
	var exit *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("blocked process did not force exit: %v, context=%v", err, ctx.Err())
	}
	if elapsed < desktopShutdownTimeout || elapsed > 8*time.Second {
		t.Fatalf("forced exit outside five-second deadline plus process startup tolerance: %v", elapsed)
	}
}
