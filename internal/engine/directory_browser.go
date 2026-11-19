package engine

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gcode/internal/contracts"
)

const (
	directoryBrowseTimeout = 5 * time.Second
	directoryBrowseWorkers = 4
	directoryReadBatch     = 128
)

type directoryReader interface {
	ReadDir(int) ([]fs.DirEntry, error)
	Close() error
}

// directoryBrowser's dependencies are immutable. Tests use their own browser
// and permits, never replacing the process-wide limiter or filesystem hooks.
type directoryBrowser struct {
	permits chan struct{}
	timeout time.Duration
	home    func() (string, error)
	abs     func(string) (string, error)
	open    func(string) (directoryReader, error)
	roots   func() []string
}

// Shared across Engine instances: opening another engine must not circumvent
// the bound when a network filesystem is unresponsive.
var hostDirectoryBrowser = directoryBrowser{
	permits: make(chan struct{}, directoryBrowseWorkers),
	timeout: directoryBrowseTimeout,
	home:    os.UserHomeDir,
	abs:     filepath.Abs,
	open: func(path string) (directoryReader, error) {
		return os.Open(path)
	},
	roots: volumeRoots,
}

// runDirectoryQuery bounds the caller's wait, not the OS call itself. All OS
// work (including home/absolute-path resolution and Close) happens in the one
// admitted worker. No goroutine is started while saturated, and only the real
// worker releases its permit. A stuck syscall can retain at most the host-wide
// worker count; Go cannot forcibly interrupt an arbitrary filesystem syscall.
func runDirectoryQuery[T any](ctx context.Context, b *directoryBrowser, deviceID string, query func(context.Context) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if deviceID != "" && deviceID != "local" {
		return zero, errors.New("device not found")
	}
	select {
	case b.permits <- struct{}{}:
	default:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return zero, contracts.ErrDirectoryBrowseBusy
	}
	type response struct {
		value T
		err   error
	}
	// A single buffered response lets an abandoned worker finish without a
	// receiver. Its result belongs exclusively to the worker until this send.
	done := make(chan response, 1)
	go func() {
		defer func() { <-b.permits }()
		if err := ctx.Err(); err != nil {
			done <- response{err: err}
			return
		}
		value, err := query(ctx)
		done <- response{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case result := <-done:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return result.value, result.err
	}
}
