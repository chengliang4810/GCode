//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestDesktopLocalAutomationsAndIsland(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Automation native proof\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			data := filepath.Join(artifacts, theme, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			if err = core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "native-scheduler", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "model"}}}}}); err != nil {
				t.Fatal(err)
			}
			if err = core.Close(); err != nil {
				t.Fatal(err)
			}
			language := "en"
			scheduleLabel := "Schedule manual"
			if theme == "dark" {
				language = "zh-CN"
				scheduleLabel = "执行计划 manual"
			}
			app := startApp(t, binary, artifacts, theme+"-native", data, "--local", "--appearance", theme, "--language", language)
			app.act("back-to-conversations", "press", "")
			create := app.waitKey("sidebar-new-conversation")
			entry := app.waitKey("sidebar-automations")
			if entry.Bounds.Y < create.Bounds.Y+create.Bounds.Height {
				t.Fatal("automation entry is not below new conversation")
			}
			app.capture("sidebar-entry")
			app.act("sidebar-automations", "press", "")
			app.act("automation-new", "press", "")
			app.act("automation-name", "setValue", "Native scheduled review")
			app.act("automation-prompt", "setValue", "Reply with Automation native proof")
			app.act("automation-mode", "press", "")
			pluginActLabel(app, scheduleLabel)
			app.capture("automation-form")
			app.act("automation-save", "press", "")
			runKey := app.awaitKeyPrefix("automation-run-")
			app.act(runKey, "press", "")
			app.await("completed automation persisted by real core", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.AutomationRuns) != 1 || state.AutomationRuns[0].Status != "completed" {
					return fmt.Errorf("runs=%+v", state.AutomationRuns)
				}
				return nil
			})
			app.capture("automation-history")
			app.act("sidebar-settings", "press", "")
			app.act("settings-nav-Dynamic Island", "press", "")
			app.act("island-enabled", "press", "")
			islandID := 0
			app.await("separate visible native island window", func() error {
				var result struct {
					Windows []struct {
						ID      int    `json:"id"`
						Title   string `json:"title"`
						Visible bool   `json:"visible"`
						Kind    string `json:"kind"`
					} `json:"windows"`
				}
				if err := app.client.call("windows", nil, &result); err != nil {
					return err
				}
				for _, w := range result.Windows {
					if (strings.Contains(w.Title, "Dynamic Island") || strings.Contains(w.Title, "灵动岛")) && w.Visible && w.Kind == "content" {
						islandID = w.ID
						return nil
					}
				}
				return fmt.Errorf("island missing: %+v", result.Windows)
			})
			assertIslandBounds := func(width, height float64) {
				t.Helper()
				app.await("top-edge island geometry", func() error {
					var result struct {
						Windows []struct {
							ID     int                                   `json:"id"`
							Bounds struct{ X, Y, Width, Height float64 } `json:"bounds"`
						} `json:"windows"`
					}
					if err := app.client.call("windows", nil, &result); err != nil {
						return err
					}
					for _, window := range result.Windows {
						if window.ID == islandID {
							if window.Bounds.Width != width || window.Bounds.Height != height || window.Bounds.Y != 0 {
								return fmt.Errorf("island bounds=%+v want %vx%v at screen top", window.Bounds, width, height)
							}
							return nil
						}
					}
					return fmt.Errorf("island missing")
				})
			}
			compactWidth, compactHeight := design.IslandGeometry(false, 1)
			assertIslandBounds(float64(compactWidth), float64(compactHeight))
			var compactCapture struct {
				PNG []byte `json:"png"`
			}
			if err := app.client.call("capture", map[string]any{"window": islandID}, &compactCapture); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(app.artifacts, "island-compact.png"), compactCapture.PNG, 0600); err != nil {
				t.Fatal(err)
			}
			if err := app.client.call("act", map[string]any{"window": islandID, "key": "island-expand", "action": "press"}, nil); err != nil {
				t.Fatal(err)
			}
			expandedWidth, expandedHeight := design.IslandGeometry(true, 1)
			assertIslandBounds(float64(expandedWidth), float64(expandedHeight))
			app.await("completed task in island", func() error {
				var tree accessTree
				if err := app.client.call("tree", map[string]any{"window": islandID}, &tree); err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					var key string
					_ = json.Unmarshal(node.Key, &key)
					if strings.HasPrefix(key, "island-open-") {
						return nil
					}
				}
				return fmt.Errorf("no completed task card in island")
			})
			var capture struct {
				PNG []byte `json:"png"`
			}
			if err := app.client.call("capture", map[string]any{"window": islandID}, &capture); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(app.artifacts, "island-expanded.png"), capture.PNG, 0600); err != nil {
				t.Fatal(err)
			}
			var tree json.RawMessage
			if err := app.client.call("tree", map[string]any{"window": islandID}, &tree); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(app.artifacts, "island-expanded-tree.json"), tree, 0600); err != nil {
				t.Fatal(err)
			}
			app.act("Character", "press", "")
			character := "Black cat"
			if language == "zh-CN" {
				character = "黑猫"
			}
			pluginActLabel(app, character)
			app.act("island-sound-enabled", "press", "")
			app.await("character and event sound preferences persisted", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				prefs := state.Settings.Island
				if prefs.Mascot != "blackcat" || !prefs.SoundEnabled || prefs.StartSound != "startup-chime" {
					return fmt.Errorf("preferences=%+v", prefs)
				}
				return nil
			})
			app.act("island-preview-start", "press", "")
			app.capture("island-settings")
			app.act("island-subtle", "press", "")
			app.await("subtle reminder preference persisted", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if !state.Settings.Island.SubtleReminders {
					return fmt.Errorf("preference not saved")
				}
				return nil
			})
			compactWidth, compactHeight = design.IslandGeometry(false, 1)
			assertIslandBounds(float64(compactWidth), float64(compactHeight))
			move := func(x, y int) {
				t.Helper()
				if err := app.client.call("input", map[string]any{"window": islandID, "events": []map[string]any{{"type": "pointerMove", "x": x, "y": y}}}, nil); err != nil {
					t.Fatal(err)
				}
			}
			move(149, 16)
			expandedWidth, expandedHeight = design.IslandGeometry(true, 1)
			assertIslandBounds(float64(expandedWidth), float64(expandedHeight))
			var expandedTree accessTree
			if err := app.client.call("tree", map[string]any{"window": islandID}, &expandedTree); err != nil {
				t.Fatal(err)
			}
			if _, err := expandedTree.findKey("island-close"); err == nil {
				t.Fatal("the island must not provide a close control")
			}
			move(200, 100)
			expandedWidth, expandedHeight = design.IslandGeometry(true, 1)
			assertIslandBounds(float64(expandedWidth), float64(expandedHeight))
			move(500, 200)
			compactWidth, compactHeight = design.IslandGeometry(false, 1)
			assertIslandBounds(float64(compactWidth), float64(compactHeight))
			app.act("island-enabled", "press", "")
			app.waitChecked("island-enabled", false)
			app.stop()
		})
	}
}
