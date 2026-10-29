//go:build uifixture && windows

package main

import "testing"

func TestTabStripFixtureRejectsMissingNativeWindow(t *testing.T) {
	if _, err := nativeWindowDPI(0); err == nil {
		t.Fatal("zero HWND supplied DPI")
	}
	if _, err := nativePointerTarget(0); err == nil {
		t.Fatal("zero HWND supplied a pointer target")
	}
	if err := syntheticFixturePointer(0); err == nil {
		t.Fatal("zero HWND received a synthetic pointer")
	}
}
