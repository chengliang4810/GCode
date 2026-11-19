//go:build !windows

package model

// MkdirTemp already creates the directory with mode 0700.
func protectWireCaptureDirectory(string) error { return nil }
