//go:build uifixture && !windows

package main

import (
	"errors"
	"os"
)

func syntheticFixturePointer(uintptr) error {
	return errors.New("synthetic fixture pointer transport is supported only on Windows")
}

func syntheticFixturePointerAt(uintptr, float32, float32) error {
	return errors.New("synthetic fixture pointer transport is supported only on Windows")
}

func nativeWindowDPI(uintptr) (uint32, error) { return 0, nil }
func nativeForegroundWindow() uintptr         { return 0 }

func nativePointerTarget(uintptr) (uintptr, error) {
	return 0, errors.New("synthetic fixture pointer target is supported only on Windows")
}
func isReparse(os.FileInfo) bool { return false }
