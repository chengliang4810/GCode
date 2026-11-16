//go:build !windows

package pii

import (
	"os"
	"syscall"
)

func cacheDirectoryOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func cacheDirectoryPrivate(info os.FileInfo) bool {
	return cacheDirectoryOwned(info) && info.Mode().Perm()&0022 == 0
}

func cacheParentTrusted(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 && stat.Uid != uint32(os.Geteuid()) {
		return false
	}
	return info.Mode().Perm()&0022 == 0 || stat.Uid == 0 && info.Mode()&os.ModeSticky != 0
}
