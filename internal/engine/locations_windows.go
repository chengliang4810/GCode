//go:build windows

package engine

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// windowsVolumeRoots lists mounted drive letters. Fixed, removable, network and
// RAM volumes are offered; empty card readers and optical drives are not,
// because browsing them can only fail.
func windowsVolumeRoots() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var roots []string
	for letter := 'A'; letter <= 'Z'; letter++ {
		if mask&(1<<uint(letter-'A')) == 0 {
			continue
		}
		root := string(letter) + ":\\"
		utf16, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		switch windows.GetDriveType(utf16) {
		case windows.DRIVE_FIXED, windows.DRIVE_REMOVABLE, windows.DRIVE_REMOTE, windows.DRIVE_RAMDISK:
			roots = append(roots, filepath.ToSlash(root))
		}
	}
	return roots
}
