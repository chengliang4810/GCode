//go:build uifixture && windows

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"
)

func TestWGCCaptureRejectsInvalidInputsBeforeLoadingBridge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captureNativeWindow(ctx, 1, "unused", "unused"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
	for _, test := range []struct {
		name string
		hwnd uintptr
		path string
	}{
		{"zero-hwnd", 0, `D:\test.png`},
		{"relative", 1, "test.png"},
		{"nul", 1, "D:\\test\x00.png"},
		{"utf8", 1, "D:\\" + string([]byte{255}) + ".png"},
		{"too-long", 1, "D:\\" + strings.Repeat("a", 32769) + ".png"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := captureNativeWindow(context.Background(), test.hwnd, "unused", test.path); err == nil {
				t.Fatal("invalid input reached capture")
			}
		})
	}
	if _, err := loadFixtureWGC("relative.dll"); err == nil {
		t.Fatal("relative DLL path was accepted")
	}
}

func TestWGCBridgeABIRejectsInvalidHWNDWithoutCapture(t *testing.T) {
	path := os.Getenv("ZERON_UIFIXTURE_WGC_TEST_DLL")
	if path == "" {
		t.Skip("set ZERON_UIFIXTURE_WGC_TEST_DLL to test the built fixture DLL without capturing")
	}
	proc, err := loadFixtureWGC(path)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(path), "invalid-hwnd-must-not-create.png")
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("contract test requires an absent output: %v", err)
	}
	outputBytes := []byte(output)
	guarded := bytes.Repeat([]byte{127}, 18)
	code, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&outputBytes[0])), uintptr(len(outputBytes)), 1, uintptr(unsafe.Pointer(&guarded[1])), 16)
	runtime.KeepAlive(outputBytes)
	runtime.KeepAlive(guarded)
	if code != 1 || guarded[0] != 127 || guarded[17] != 127 || guarded[16] != 0 || !utf8.Valid(guarded[1:17]) {
		t.Fatalf("C ABI returned code=%d bytes=%v", code, guarded)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("invalid HWND created a file: %v", err)
	}
	if reparse := os.Getenv("ZERON_UIFIXTURE_WGC_TEST_REPARSE_OUTPUT"); reparse != "" {
		t.Run("reparse-output-ancestor", func(t *testing.T) {
			path := []byte(reparse)
			message := make([]byte, 1024)
			code, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)), 1, uintptr(unsafe.Pointer(&message[0])), uintptr(len(message)))
			runtime.KeepAlive(path)
			runtime.KeepAlive(message)
			if code != 1 || !bytes.Contains(message, []byte("ancestor")) {
				t.Fatalf("reparse output was not rejected: code=%d, %s", code, bytes.TrimRight(message, "\x00"))
			}
		})
	}
}
