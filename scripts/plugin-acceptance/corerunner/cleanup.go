package corerunner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func removeOwnedProbeHome(probeRoot, home string) error {
	probeRoot, err := filepath.Abs(probeRoot)
	if err != nil {
		return err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(filepath.Base(probeRoot), "probe-") || !samePath(home, filepath.Join(probeRoot, "home")) {
		return errors.New("refusing cleanup outside the exact owned probe home")
	}
	canonical, err := filepath.EvalSymlinks(probeRoot)
	if err != nil {
		return fmt.Errorf("verify owned probe cleanup root: %w", err)
	}
	if !samePath(canonical, probeRoot) {
		return errors.New("owned probe cleanup root has symbolic-link ancestors")
	}
	info, err := os.Lstat(home)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("owned probe home is no longer a real directory")
	}
	root, err := os.OpenRoot(probeRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.RemoveAll("home"); err != nil {
		return fmt.Errorf("remove private probe home containing copied test credentials: %w", err)
	}
	return nil
}
