// Package applog connects application diagnostics to the active runtime root.
package applog

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"sync"

	"gcode/internal/storage"
)

// StartRuntime mirrors the standard logger to console and the bounded runtime
// log. The returned idempotent cleanup restores the previous output and releases
// the log lock; call it before launching a replacement process. The store must
// remain open while log entries are written.
// File write and close failures are reported directly to console because the
// standard logger's Print methods do not return output errors.
func StartRuntime(store *storage.Store, console io.Writer) (func(), error) {
	if console == nil {
		return nil, errors.New("runtime log console is required")
	}
	file, err := storage.OpenRuntimeLog(store)
	if err != nil {
		return nil, fmt.Errorf("open runtime log: %w", err)
	}
	output := &runtimeOutput{console: console, file: file}
	previous := log.Writer()
	log.SetOutput(output)
	var once sync.Once
	return func() {
		once.Do(func() {
			log.SetOutput(previous)
			// Close reports failures to console before returning them.
			_ = output.Close()
		})
	}, nil
}

type runtimeOutput struct {
	mu      sync.Mutex
	console io.Writer
	file    io.WriteCloser
	closed  bool
}

func (w *runtimeOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, fs.ErrClosed
	}
	consoleN, consoleErr := w.console.Write(data)
	if consoleN != len(data) && consoleErr == nil {
		consoleErr = io.ErrShortWrite
	}
	fileN, fileErr := w.file.Write(data)
	if fileN != len(data) && fileErr == nil {
		fileErr = io.ErrShortWrite
	}
	if fileErr != nil {
		_, _ = fmt.Fprintf(w.console, "GCode runtime log write failed: %v\n", fileErr)
	}
	return consoleN, errors.Join(consoleErr, fileErr)
}

func (w *runtimeOutput) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	err := w.file.Close()
	if err != nil {
		_, _ = fmt.Fprintf(w.console, "GCode runtime log close failed: %v\n", err)
	}
	return err
}
