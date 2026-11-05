//go:build !windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
)

func replaceFile(source, target string) error {
	parent, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		return errors.Join(err, parent.Close())
	}
	// Flushing file contents does not make the replacement directory entry
	// durable. Keep the opened parent alive through rename and its fsync.
	return errors.Join(parent.Sync(), parent.Close())
}
