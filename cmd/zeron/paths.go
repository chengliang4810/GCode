package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

func validScenario(scenario string) bool {
	switch scenario {
	case "", "default", "empty", "loading", "live", "transcript", "error", "offline", "questions", "permissions", "stress", "showcase":
		return true
	default:
		return false
	}
}

func nativeDataDirectory(configRoot, statePath, scenario string) string {
	if statePath != "" {
		return filepath.Join(filepath.Dir(statePath), "native")
	}
	return filepath.Join(configRoot, "gcode-zeron-ui", "acceptance", scenario)
}

// The display name stays Zeron; every framework-owned store stays independent.
func configureNativeDirectories(statePath, scenario string) error {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	root, err := filepath.Abs(nativeDataDirectory(configRoot, statePath, scenario))
	if err != nil {
		return err
	}
	for name, path := range map[mygo.PathName]string{mygo.PathUserData: root, mygo.PathCache: filepath.Join(root, "cache"), mygo.PathLogs: filepath.Join(root, "logs")} {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		mygo.App.SetPath(name, path)
	}
	return nil
}
