package engine

import (
	"errors"
	"os"
	"syscall"
)

func openFileSaveSource(parent *os.Root, name string) (*os.File, error) {
	// Root confines resolution; Lstat/handle identity checks reject final links.
	// No destination write occurs, and the handle is closed before replacement.
	return parent.OpenFile(name, os.O_RDWR, 0)
}

func checkFileSaveLinks(file *os.File, _ os.FileInfo) error {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return errors.New("saving files with multiple hard links is not supported")
	}
	return nil
}
