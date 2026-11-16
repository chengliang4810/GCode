//go:build !windows

package storage

import (
	"errors"
	"os"
)

func syncDirectoryRoot(root *os.Root) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
