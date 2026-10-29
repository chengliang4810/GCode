//go:build uifixture && windows

package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var fixtureGetDPI = syscall.NewLazyDLL("user32.dll").NewProc("GetDpiForWindow")
var fixturePostMessage = syscall.NewLazyDLL("user32.dll").NewProc("PostMessageW")
var fixtureFindWindow = syscall.NewLazyDLL("user32.dll").NewProc("FindWindowExW")
var fixtureWindowProcess = syscall.NewLazyDLL("user32.dll").NewProc("GetWindowThreadProcessId")
var fixtureForeground = syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow")

func nativeForegroundWindow() uintptr { hwnd, _, _ := fixtureForeground.Call(); return hwnd }

func nativePointerTarget(hwnd uintptr) (uintptr, error) {
	if hwnd == 0 {
		return 0, fmt.Errorf("a fixture HWND is required for pointer transport")
	}
	// The pinned backend creates this direct WS_CHILD surface in
	// internal/windows/surface.go:20,183-190. It receives native input.
	class, err := syscall.UTF16PtrFromString("MyGoSurface")
	if err != nil {
		return 0, err
	}
	child, _, _ := fixtureFindWindow.Call(hwnd, 0, uintptr(unsafe.Pointer(class)), 0)
	runtime.KeepAlive(class)
	if child == 0 {
		return 0, fmt.Errorf("own fixture native surface child was not found")
	}
	var pid uint32
	fixtureWindowProcess.Call(child, uintptr(unsafe.Pointer(&pid)))
	if pid != uint32(os.Getpid()) {
		return 0, fmt.Errorf("fixture pointer target belongs to a different process")
	}
	return child, nil
}

func syntheticFixturePointer(hwnd uintptr) error {
	return syntheticFixturePointerAt(hwnd, 10, 10)
}

func syntheticFixturePointerAt(hwnd uintptr, x, y float32) error {
	if math.IsNaN(float64(x)) || math.IsNaN(float64(y)) || math.IsInf(float64(x), 0) || math.IsInf(float64(y), 0) || x < 1 || y < 0 {
		return fmt.Errorf("invalid synthetic pointer coordinates")
	}
	target, err := nativePointerTarget(hwnd)
	if err != nil {
		return err
	}
	dpi, err := nativeWindowDPI(hwnd)
	if err != nil {
		return err
	}
	xPrevious := uintptr(math.Round(float64(x-1) * float64(dpi) / 96))
	xFinal := uintptr(math.Round(float64(x) * float64(dpi) / 96))
	yFinal := uintptr(math.Round(float64(y) * float64(dpi) / 96))
	if xFinal > 32767 || yFinal > 32767 {
		return fmt.Errorf("synthetic pointer coordinate exceeds message range")
	}
	// A newly seeded scene clears its input observation, but the engine may
	// still be at10,10. Force a different safe point before the final one.
	for _, px := range []uintptr{xPrevious, xFinal} {
		ok, _, postErr := fixturePostMessage.Call(target, 0x0200, 0, px|yFinal<<16)
		if ok == 0 {
			return fmt.Errorf("post synthetic fixture pointer: %w", postErr)
		}
	}
	return nil
}

func nativeWindowDPI(hwnd uintptr) (uint32, error) {
	if hwnd == 0 {
		return 0, fmt.Errorf("a fixture HWND is required for DPI measurement")
	}
	dpi, _, _ := fixtureGetDPI.Call(hwnd)
	if dpi == 0 {
		return 0, fmt.Errorf("window DPI query returned no DPI")
	}
	return uint32(dpi), nil
}

func isReparse(info os.FileInfo) bool {
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
