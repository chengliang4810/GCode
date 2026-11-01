//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Keep native tests sequential: keyboard focus and the desktop compositor are
// shared resources, even though each test owns its process and data directory.
func TestDesktopScenarioSettings(t *testing.T) {
	binary, artifacts := testPaths(t)
	app := startApp(t, binary, artifacts, "scenario", filepath.Join(artifacts, "data"), "--scenario", "default")
	app.capture("general")
	original := app.waitKey("Compact mode").checked()
	app.act("Compact mode", "press", "")
	app.waitChecked("Compact mode", !original)

	app.act("settings-search", "setValue", "Autosave")
	app.waitKey("search-Autosave")
	app.act("search-Autosave", "press", "")
	app.waitKey("Autosave")
	autosave := app.waitKey("Autosave").checked()
	app.act("Autosave", "press", "")
	app.waitChecked("Autosave", !autosave)
	app.capture("searched-files")

	app.act("settings-search", "setValue", "")
	app.act("settings-nav-Appearance", "press", "")
	app.waitKey("appearance-mode-dark")
	app.capture("appearance")
	app.act("settings-nav-Notifications", "press", "")
	app.waitKey("Session sounds")
	app.act("settings-nav-General", "press", "")
	app.waitChecked("Compact mode", !original)
}

// This test exercises the actual in-process Go core, its async UI event path,
// disk persistence and a fresh desktop process. It needs no provider or network.
func TestDesktopLocalSettingsPersistence(t *testing.T) {
	binary, artifacts := testPaths(t)
	data := filepath.Join(artifacts, "data")
	app := startApp(t, binary, artifacts, "first-launch", data, "--local")
	app.waitKey("data-directory") // The real core exposes its owned storage root.
	original := app.waitKey("Compact mode").checked()
	want := !original
	app.act("Compact mode", "press", "")
	app.waitChecked("Compact mode", want)
	app.await("real core persisted compact mode", func() error {
		return persistedCompactMode(filepath.Join(data, "engine.json"), want)
	})
	app.capture("persisted")
	app.stop()

	restarted := startApp(t, binary, artifacts, "restart", data, "--local")
	restarted.waitKey("data-directory")
	restarted.waitChecked("Compact mode", want)
	restarted.await("persisted compact mode after restart", func() error {
		return persistedCompactMode(filepath.Join(data, "engine.json"), want)
	})
	restarted.capture("restored")
}

func persistedCompactMode(path string, want bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var disk struct {
		Snapshot struct {
			Settings struct {
				General struct {
					CompactMode *bool `json:"compactMode"`
				} `json:"general"`
			} `json:"settings"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return err
	}
	got := disk.Snapshot.Settings.General.CompactMode
	if got == nil || *got != want {
		return fmt.Errorf("persisted compactMode=%v, want %t", got, want)
	}
	return nil
}
