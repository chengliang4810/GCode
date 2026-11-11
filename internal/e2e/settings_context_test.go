//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopLocalHierarchicalContextSettings(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "profile")
			app := startApp(t, binary, artifacts, "enable", data, "--local", "--appearance", appearance, "--width", "900", "--height", "700")
			key := "Hierarchical context compression"
			app.waitChecked(key, false)
			app.capture("default-off")
			app.act(key, "press", "")
			app.waitChecked(key, true)
			app.await("persisted hierarchical context enabled", func() error { return persistedHierarchicalContext(filepath.Join(data, "engine.json"), true) })
			app.capture("enabled")
			app.stop()
			app = startApp(t, binary, artifacts, "resume", data, "--local", "--appearance", appearance, "--width", "900", "--height", "700")
			app.waitChecked(key, true)
			app.capture("enabled-after-restart")
			app.act(key, "press", "")
			app.waitChecked(key, false)
			app.await("persisted hierarchical context disabled", func() error { return persistedHierarchicalContext(filepath.Join(data, "engine.json"), false) })
			app.capture("disabled")
		})
	}
}

func persistedHierarchicalContext(path string, want bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var disk struct {
		Snapshot struct {
			Settings struct {
				General struct {
					Enabled *bool `json:"hierarchicalContext"`
				} `json:"general"`
			} `json:"settings"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return err
	}
	got := disk.Snapshot.Settings.General.Enabled
	if got == nil || *got != want {
		return fmt.Errorf("persisted hierarchicalContext=%v, want %t", got, want)
	}
	return nil
}
