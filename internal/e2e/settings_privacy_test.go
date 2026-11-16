//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/privacy"
)

// Real native window and local core; no models are downloaded and no provider
// requests are made. Each phase owns its app, socket and persistent directory.
func TestDesktopLocalPrivacySettings(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "profile")
			args := []string{"--local", "--appearance", appearance, "--route", "settings/Privacy", "--width", "900", "--height", "760"}
			app := startAppReady(t, binary, artifacts, "enable", data, "Mask sensitive information", args...)
			app.waitChecked("Mask sensitive information", false)
			app.capture("default-off")
			app.act("Mask sensitive information", "press", "")
			app.waitChecked("Mask sensitive information", true)
			app.waitChecked("Mask secrets", true)
			waitSaved := func(app *nativeApp, enabled bool) {
				app.await("persisted privacy enabled state", func() error {
					bytes, err := os.ReadFile(filepath.Join(data, "engine.json"))
					if err != nil {
						return err
					}
					var disk struct {
						Snapshot struct {
							Settings struct {
								Privacy privacy.Settings `json:"privacy"`
							} `json:"settings"`
						} `json:"snapshot"`
					}
					if err := json.Unmarshal(bytes, &disk); err != nil {
						return err
					}
					if got := disk.Snapshot.Settings.Privacy; got.Enabled != enabled || !got.Secrets {
						return fmt.Errorf("privacy=%+v, want enabled=%v and secrets=true", got, enabled)
					}
					return nil
				})
			}
			waitSaved(app, true)
			app.capture("enabled")
			app.stop()
			app = startAppReady(t, binary, artifacts, "reopen", data, "Mask sensitive information", args...)
			app.waitChecked("Mask sensitive information", true)
			app.waitChecked("Mask secrets", true)
			app.capture("enabled-after-restart")
			app.act("Mask sensitive information", "press", "")
			app.waitChecked("Mask sensitive information", false)
			waitSaved(app, false)
			app.capture("disabled")
		})
	}
}
