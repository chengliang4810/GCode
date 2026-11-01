//go:build !windows

package engine

// windowsVolumeRoots has no meaning off Windows; the caller uses the POSIX root.
func windowsVolumeRoots() []string { return nil }
