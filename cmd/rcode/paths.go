package main

import (
	"os"
	"path/filepath"
	"runtime"

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

type nativeDirectories struct {
	userData  string
	cache     string
	logs      string
	temp      string
	browser   string
	downloads string
}

func nativeDirectoryPaths(configRoot, runtimeRoot, scenario string) nativeDirectories {
	if scenario != "" {
		configRoot = filepath.Join(configRoot, "acceptance", scenario)
		runtimeRoot = filepath.Join(runtimeRoot, "acceptance", scenario)
	}
	return nativeDirectories{
		userData:  filepath.Join(configRoot, "native"),
		cache:     filepath.Join(runtimeRoot, "cache", "gcode"),
		logs:      filepath.Join(runtimeRoot, "logs"),
		temp:      filepath.Join(runtimeRoot, "tmp", "gcode"),
		browser:   filepath.Join(runtimeRoot, "browser"),
		downloads: filepath.Join(runtimeRoot, "downloads"),
	}
}

// Native preferences stay with configuration; disposable and browser data follow
// the active runtime root selected before this process starts.
func configureNativeDirectories(configRoot, runtimeRoot, scenario string) error {
	paths := nativeDirectoryPaths(configRoot, runtimeRoot, scenario)
	for name, path := range map[mygo.PathName]string{
		mygo.PathUserData:  paths.userData,
		mygo.PathCache:     paths.cache,
		mygo.PathLogs:      paths.logs,
		mygo.PathTemp:      paths.temp,
		mygo.PathDownloads: paths.downloads,
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		mygo.App.SetPath(name, path)
	}
	if runtime.GOOS == "windows" {
		if err := os.MkdirAll(paths.browser, 0755); err != nil {
			return err
		}
		return mygo.App.SetWebViewDataDirectory(paths.browser)
	}
	return nil
}
