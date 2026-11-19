//go:build unix

package engine

import (
	"errors"
	"os"
	"syscall"
)

func openFileSaveSource(parent *os.Root, name string) (*os.File, error) {
	// O_RDWR preserves the old save's write-access requirement, without
	// truncation. NONBLOCK prevents a raced-in FIFO from hanging before the
	// descriptor's type can be checked. Root confines symlink resolution;
	// surrounding Lstat/descriptor identity checks reject final symlinks.
	return parent.OpenFile(name, os.O_RDWR|syscall.O_NONBLOCK, 0)
}

func checkFileSaveLinks(_ *os.File, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot determine file hard-link count")
	}
	if stat.Nlink != 1 {
		return errors.New("saving files with multiple hard links is not supported")
	}
	return nil
}
