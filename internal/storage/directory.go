package storage

import (
	"errors"
	"os"
	"path/filepath"
)

// MkdirRoot creates one directory and synchronizes its parent entry. Existing
// entries retain os.Root.Mkdir's error and are never replaced or removed.
func MkdirRoot(root *os.Root, name string, mode os.FileMode) error {
	if err := root.Mkdir(name, mode); err != nil {
		return err
	}
	return syncParentRoot(root, name)
}

// RemoveRoot removes one entry and synchronizes its parent. It never recursively
// removes directories; callers still decide which owned entries may be deleted.
func RemoveRoot(root *os.Root, name string) error {
	if err := root.Remove(name); err != nil {
		return err
	}
	return syncParentRoot(root, name)
}

func syncParentRoot(root *os.Root, name string) error {
	parent, err := root.OpenRoot(filepath.Dir(name))
	if err != nil {
		return err
	}
	return errors.Join(syncDirectoryRoot(parent), parent.Close())
}
