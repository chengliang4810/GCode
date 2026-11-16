package pii

import (
	"errors"
	"os"
	"path/filepath"
)

// openRegular keeps a swapped link from escaping the containing directory.
// Cache parents are private; os.Root additionally enforces the boundary during
// the actual open rather than relying only on a prior path check.
func openRegular(path string, flags int, mode os.FileMode) (*os.File, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("privacy model files must be regular files")
	}
	file, err := root.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("privacy model files must be regular files")
	}
	return file, nil
}
