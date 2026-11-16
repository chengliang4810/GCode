package pii

import (
	"fmt"
	"os"
	"path/filepath"
)

func cacheDirectory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil || !filepath.IsAbs(root) {
		return "", fmt.Errorf("a private user cache directory is required for privacy models")
	}
	return filepath.Join(root, "GCode", "privacy-models"), nil
}

// PrepareCache refuses an unavailable, shared or symlinked cache. In particular,
// a service with no user cache must never execute a helper from a public /tmp.
func PrepareCache() error {
	root, err := cacheDirectory()
	if err != nil {
		return err
	}
	base := filepath.Dir(filepath.Dir(root))
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	for path := base; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !cacheParentTrusted(info) {
			return fmt.Errorf("privacy model cache has an untrusted parent: %s", path)
		}
		if path == filepath.Dir(path) {
			break
		}
	}
	for _, path := range []string{base, filepath.Dir(root), root} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !cacheDirectoryPrivate(info) {
			return fmt.Errorf("privacy model cache must be a user-owned directory without symlinks or shared write access: %s", path)
		}
		if path != base {
			if err := os.Chmod(path, 0700); err != nil {
				return err
			}
		}
	}
	return nil
}

func privateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !cacheDirectoryPrivate(info) {
		return fmt.Errorf("privacy model directory is not private: %s", path)
	}
	return os.Chmod(path, 0700)
}

func regularModelFile(path string, size int64) bool {
	info, err := os.Lstat(filepath.Dir(path))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	info, err = os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && (size < 0 || info.Size() == size)
}

// cacheRoot uses the application's device-local cache. Models and inference
// caches are not part of synced settings or conversation persistence.
func cacheRoot() string {
	root, err := cacheDirectory()
	if err != nil {
		panic(err) // callers validate availability before enabling a model
	}
	return root
}
