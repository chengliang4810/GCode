//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalMenuBarPanel(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			data := filepath.Join(artifacts, theme, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			language := "en"
			if theme == "dark" {
				language = "zh-CN"
			}
			app := startApp(t, binary, artifacts, theme+"-menubar", data, "--local", "--appearance", theme, "--language", language)
			app.act("sidebar-usage", "press", "")
			app.act("menubar-enabled", "press", "")
			app.await("menu bar setting persisted", func() error {
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if !snapshot.Settings.MenuBar {
					return fmt.Errorf("setting not saved")
				}
				return nil
			})
			app.act("menubar-preview", "press", "")
			panel := 0
			lastOpen := time.Now()
			app.await("visible tray-anchored native panel", func() error {
				var result struct {
					Windows []struct {
						ID          int `json:"id"`
						Title, Kind string
						Visible     bool
					}
				}
				if err := app.client.call("windows", nil, &result); err != nil {
					return err
				}
				for _, window := range result.Windows {
					if strings.HasPrefix(window.Title, "GCode · ") && window.Kind == "content" {
						if window.Visible {
							panel = window.ID
							return nil
						}
						// Focusable panels legitimately dismiss when the OS focus moves.
						if time.Since(lastOpen) > time.Second {
							lastOpen = time.Now()
							if err := app.client.call("act", map[string]any{"window": 1, "key": "menubar-preview", "action": "press"}, nil); err != nil {
								return err
							}
						}
					}
				}
				return fmt.Errorf("panel not visible: %+v", result.Windows)
			})
			act := func(key string) {
				t.Helper()
				app.await("panel control "+key, func() error {
					var tree accessTree
					if err := app.client.call("tree", map[string]any{"window": panel}, &tree); err != nil {
						return err
					}
					for _, node := range tree.Nodes {
						var value string
						_ = json.Unmarshal(node.Key, &value)
						if value == key {
							return app.client.call("act", map[string]any{"window": panel, "key": key, "action": "press"}, nil)
						}
					}
					return fmt.Errorf("control %s absent", key)
				})
			}
			capture := func(name string) {
				t.Helper()
				var result struct {
					PNG []byte `json:"png"`
				}
				if err := app.client.call("capture", map[string]any{"window": panel}, &result); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(app.artifacts, name+".png"), result.PNG, 0600); err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS == "darwin" {
					var windows struct {
						Windows []struct {
							ID      int
							Visible bool
							Bounds  struct{ X, Y, Width, Height float64 }
						}
					}
					if err := app.client.call("windows", nil, &windows); err != nil {
						t.Fatal(err)
					}
					for _, window := range windows.Windows {
						if window.ID == panel && window.Visible {
							b := window.Bounds
							region := fmt.Sprintf("%.0f,%.0f,%.0f,%.0f", b.X, b.Y, b.Width, b.Height)
							if output, err := exec.Command("screencapture", "-x", "-R"+region, filepath.Join(app.artifacts, name+"-screen.png")).CombinedOutput(); err != nil {
								t.Fatalf("native material capture: %v %s", err, output)
							}
							if name == "menubar-unread" {
								statusRegion := fmt.Sprintf("%.0f,0,360,32", b.X)
								if output, err := exec.Command("screencapture", "-x", "-R"+statusRegion, filepath.Join(app.artifacts, "menubar-status-screen.png")).CombinedOutput(); err != nil {
									t.Fatalf("status capture: %v %s", err, output)
								}
							}
						}
					}
				}
			}
			capture("menubar-unread")
			act("menubar-tab-active")
			capture("menubar-active")
			act("menubar-tab-today")
			app.await("real core usage loaded", func() error {
				var tree accessTree
				if err := app.client.call("tree", map[string]any{"window": panel}, &tree); err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == "Tokens today" || node.Label == "今日 Token" {
						return nil
					}
				}
				return fmt.Errorf("usage not loaded")
			})
			capture("menubar-today")
			act("menubar-settings")
			app.act("menubar-enabled", "press", "")
			app.await("menu bar disabled and persisted", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if state.Settings.MenuBar {
					return fmt.Errorf("still enabled")
				}
				return nil
			})
			app.stop()
		})
	}
}
