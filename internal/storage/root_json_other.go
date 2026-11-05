//go:build !windows

package storage

import (
	"errors"
	"os"
)

func replaceJSONRoot(root *os.Root, source, target string) error {
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	if err := root.Rename(source, target); err != nil {
		return errors.Join(err, parent.Close())
	}
	return errors.Join(parent.Sync(), parent.Close())
}
