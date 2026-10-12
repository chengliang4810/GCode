package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var fileLocks sync.Map

func WithPathLock(path string, operation func() error) error {
	canonical, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	canonical = canonicalLockPath(filepath.Clean(canonical))
	value, _ := fileLocks.LoadOrStore(canonical, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return operation()
}

func canonicalLockPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	missing := make([]string, 0)
	current := path
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		missing = append(missing, filepath.Base(current))
		current = parent
		if _, err := os.Lstat(current); err != nil && !errors.Is(err, os.ErrNotExist) {
			return path
		}
	}
}
