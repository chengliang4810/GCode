//go:build uifixture && windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

var fixtureWGC struct {
	sync.Mutex
	path string
	dll  *syscall.DLL
	proc *syscall.Proc
	err  error
}

// loadFixtureWGC keeps the DLL loaded for the fixture process lifetime. WGC's
// WinRT callbacks, MTA registration and encoder workers must not outlive it.
func loadFixtureWGC(path string) (*syscall.Proc, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("capture bridge path must be absolute")
	}
	path = filepath.Clean(path)
	fixtureWGC.Lock()
	defer fixtureWGC.Unlock()
	if fixtureWGC.dll != nil {
		if !strings.EqualFold(fixtureWGC.path, path) {
			return nil, fmt.Errorf("capture bridge already loaded from another path")
		}
		return fixtureWGC.proc, fixtureWGC.err
	}
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("load test capture bridge: %w", err)
	}
	// Retain even an incompatible DLL. Do not run a DLL's unload path while
	// unknown initialization threads or process-wide registrations may exist.
	fixtureWGC.dll, fixtureWGC.path = dll, path
	fail := func(err error) (*syscall.Proc, error) {
		fixtureWGC.err = err
		return nil, err
	}
	version, err := dll.FindProc("rcode_fixture_wgc_abi_version")
	if err != nil {
		return fail(fmt.Errorf("find capture bridge ABI: %w", err))
	}
	if value, _, _ := version.Call(); value != 1 {
		return fail(fmt.Errorf("unsupported capture bridge ABI %d", value))
	}
	proc, err := dll.FindProc("rcode_fixture_wgc_capture")
	if err != nil {
		return fail(fmt.Errorf("find capture bridge entry point: %w", err))
	}
	fixtureWGC.proc = proc
	return proc, nil
}

// captureNativeWindow must run in the existing fixture worker, while the UI
// thread continues presenting. hwnd is read on the UI thread, never retained
// as a Window, ui.Element or ui.Context. WGC saves the original PNG and a
// metadata sidecar; the returned bytes are read unchanged from that file.
func captureNativeWindow(ctx context.Context, hwnd uintptr, bridgePath, outputPath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hwnd == 0 {
		return nil, fmt.Errorf("a fixture HWND is required")
	}
	if !filepath.IsAbs(outputPath) || !utf8.ValidString(outputPath) || strings.ContainsRune(outputPath, 0) {
		return nil, fmt.Errorf("capture output must be an absolute UTF-8 path without NUL")
	}
	path := []byte(outputPath)
	if len(path) == 0 || len(path) > 32768 {
		return nil, fmt.Errorf("capture output path exceeds the bridge byte limit")
	}
	proc, err := loadFixtureWGC(bridgePath)
	if err != nil {
		return nil, err
	}
	timeout := 15 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}
	timeoutMS := max(1, (timeout+time.Millisecond-1)/time.Millisecond)
	message := make([]byte, 4096)
	code, _, _ := proc.Call(hwnd, uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)), uintptr(timeoutMS), uintptr(unsafe.Pointer(&message[0])), uintptr(len(message)))
	// DLL only borrows the byte buffers for the synchronous call.
	runtime.KeepAlive(path)
	runtime.KeepAlive(message)
	if code != 0 {
		if end := bytes.IndexByte(message, 0); end >= 0 {
			message = message[:end]
		}
		return nil, fmt.Errorf("native WGC capture failed (%d): %s", code, message)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read original WGC PNG: %w", err)
	}
	return data, nil
}
