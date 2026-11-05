//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Native controls and real local persistence. No provider request is made.
func TestDesktopLocalModelPickerLayout(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, modelCount := range []int{2, 12} {
			t.Run(fmt.Sprintf("%s-models-%d", appearance, modelCount), func(t *testing.T) {
				binary, artifacts := testPaths(t)
				data := filepath.Join(artifacts, "data")
				store, err := storage.Open(storage.Options{Directory: data})
				if err != nil {
					t.Fatal(err)
				}
				core, err := openExecutionFixture(t, engine.Options{Storage: store})
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{"alpha", "beta", "gamma"} {
					provider := model.Provider{ID: id, Name: "我的模型服务 " + id, Protocol: model.Chat, BaseURL: "http://127.0.0.1:1/v1"}
					count := modelCount
					if id == "beta" {
						count = 1
					}
					for index := 0; index < count; index++ {
						provider.Models = append(provider.Models, model.Model{
							ID:              fmt.Sprintf("m%d", index),
							ReasoningLevels: []string{"low", "high", "max"},
						})
					}
					if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
						_ = core.Close()
						t.Fatal(err)
					}
				}
				snapshot, err := core.Snapshot(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				settings := snapshot.Settings
				settings.Appearance.FontSize = 16
				if modelCount == 12 {
					settings.Appearance.FontSize = 20
				}
				settings.General.Language = "zh-CN"
				settings.ModelPicker.ProviderID = "alpha"
				settings.ModelPicker.Selections = map[string]contracts.ModelSelection{"alpha": {ModelID: "alpha/m0", Reasoning: "Max"}}
				if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
					t.Fatal(err)
				}
				if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Picker position"}); err != nil {
					t.Fatal(err)
				}
				snapshot, err = core.Snapshot(context.Background())
				if err != nil || len(snapshot.Sessions) != 1 {
					t.Fatalf("seed conversation: sessions=%d err=%v", len(snapshot.Sessions), err)
				}
				sessionID := snapshot.Sessions[0].ID
				if err := core.Close(); err != nil {
					t.Fatal(err)
				}
				// Seed display-only transcript data while this private host is stopped.
				// An empty saved session still renders the home composer.
				editFixtureConversation(t, data, sessionID, func(_ *contracts.Session, messages *[]contracts.Message) {
					*messages = []contracts.Message{{
						ID: "picker-message", SessionID: sessionID, Role: "user",
						Blocks: []contracts.Block{{ID: "picker-block", Kind: "text", Text: "Saved conversation for menu positioning."}},
					}}
				})
				language := "en"
				if appearance == "dark" {
					language = "zh-CN"
				}
				app := startApp(t, binary, artifacts, "picker", data, "--local", "--appearance", appearance, "--language", language, "--width", "900", "--height", "600")
				app.act("back-to-conversations", "press", "")
				app.act("model-trigger", "press", "")
				app.act("model-provider:beta", "press", "")
				homeInput := app.waitKey("input")
				var homeGap float64
				app.await("home provider menu follows model button", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					homeGap, err = modelMenuButtonGap(tree)
					return err
				})
				app.capture("single-model-position")
				stationaryProvider := app.waitKey("model-provider:alpha")
				stationarySettings := app.waitKey("model-settings")

				app.act("model-provider:alpha", "press", "")
				app.await("model count does not move the provider rail", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					provider, err := tree.findKey("model-provider:alpha")
					if err != nil {
						return err
					}
					settings, err := tree.findKey("model-settings")
					if err != nil {
						return err
					}
					if provider.Bounds != stationaryProvider.Bounds || settings.Bounds != stationarySettings.Bounds {
						return fmt.Errorf("model count moved provider rail: provider=%+v want=%+v settings=%+v want=%+v", provider.Bounds, stationaryProvider.Bounds, settings.Bounds, stationarySettings.Bounds)
					}
					return nil
				})
				settingsEntry := app.waitKey("model-settings")
				provider := app.waitKey("model-provider:alpha")
				modelRow := app.waitKey("model-choice:alpha/m0")
				if settingsEntry.Bounds.X != provider.Bounds.X || settingsEntry.Bounds.Y <= provider.Bounds.Y || !(modelRow.Bounds.X > provider.Bounds.X+provider.Bounds.Width || modelRow.Bounds.X+modelRow.Bounds.Width < provider.Bounds.X) {
					t.Fatalf("invalid model settings/provider rail: settings=%+v provider=%+v model=%+v", settingsEntry.Bounds, provider.Bounds, modelRow.Bounds)
				}

				tree, err := app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				_, searchErr := tree.findKey("model-search")
				if (searchErr == nil) != (modelCount > 7) {
					t.Fatal("search visibility does not follow catalog size")
				}
				trigger := app.waitKey("model-trigger")
				if math.Abs(provider.Bounds.X+provider.Bounds.Width/2-trigger.Bounds.X-trigger.Bounds.Width/2) > 1 {
					t.Fatal("supplier menu is not centered on the model button")
				}
				if _, err := modelMenuButtonGap(tree); err != nil {
					t.Fatal(err)
				}

				for _, node := range tree.Nodes {
					var key string
					_ = json.Unmarshal(node.Key, &key)
					if strings.HasPrefix(key, "model-favorite:") || key == "model-provider:favorites" || node.Label == "Favorites" || node.Label == "收藏" {
						t.Fatalf("obsolete favorites or duplicate settings entry: %+v", node)
					}
					if node.Role == "text" && strings.Contains(node.Label, "Ctrl+") {
						var key string
						_ = json.Unmarshal(node.Key, &key)
						if key != "sidebar-new-shortcut" {
							t.Fatalf("unwanted secondary row text: %+v", node)
						}
					}
					if node.Role == "slider" {
						t.Fatal("model list contains reasoning slider")
					}
				}

				app.capture("model-list")
				// Browsing follows the pointer without committing a model.
				provider = app.waitKey("model-provider:beta")
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{
					"type": "pointerMove", "x": provider.Bounds.X + provider.Bounds.Width/2, "y": provider.Bounds.Y + provider.Bounds.Height/2,
				}}}, nil); err != nil {
					t.Fatal(err)
				}
				app.waitKey("model-choice:beta/m0")
				provider = app.waitKey("model-provider:alpha")
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{
					"type": "pointerMove", "x": provider.Bounds.X + provider.Bounds.Width/2, "y": provider.Bounds.Y + provider.Bounds.Height/2,
				}}}, nil); err != nil {
					t.Fatal(err)
				}
				app.waitKey("model-choice:alpha/m1")
				app.await("provider hover has no duplicate name tooltip", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					for _, node := range tree.Nodes {
						if node.Role == "tooltip" && node.Label == "我的模型服务 alpha" {
							return fmt.Errorf("redundant provider tooltip remains")
						}
					}
					return nil
				})
				if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
					t.Fatal(err)
				}
				app.capture("provider-hover-without-tooltip")
				app.act("model-settings", "press", "")
				app.waitKey("add-provider")
				app.act("edit-model-alpha/m1", "press", "")
				idField := app.waitKey("id")
				if idField.Value != "m1" {
					t.Fatalf("wrong model editor opened: %+v", idField)
				}

				app.capture("specific-model-settings")
				app.act("context", "setValue", "234567")
				app.act("save-model", "press", "")
				app.await("picker model settings saves only the requested model", func() error {
					payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
					if err != nil {
						return err
					}
					var disk struct {
						Providers []model.Provider   `json:"providers"`
						Snapshot  contracts.Snapshot `json:"snapshot"`
					}
					if err := json.Unmarshal(payload, &disk); err != nil {
						return err
					}
					if disk.Snapshot.Settings.ModelPicker.Selections["alpha"].ModelID != "alpha/m0" {
						return fmt.Errorf("settings click changed selected chat model")
					}
					for _, provider := range disk.Providers {
						if provider.ID == "alpha" {
							for _, candidate := range provider.Models {
								if candidate.ID == "m1" && candidate.ContextWindow == 234567 {
									return nil
								}
							}
						}
					}
					return fmt.Errorf("edited model not persisted")
				})
				app.act("back-to-conversations", "press", "")
				app.act("model-trigger", "press", "")
				app.act("model-provider:alpha", "press", "")

				app.act("model-choice:alpha/m0", "press", "")
				app.await("model selection closes the unified picker", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					if _, err := tree.findKey("model-provider:alpha"); err == nil {
						return fmt.Errorf("model picker still open")
					}
					return nil
				})
				app.act("reasoning-trigger", "press", "")
				app.waitKey("effort-slider")
				app.capture("reasoning-picker")
				app.act("effort-slider", "decrement", "")

				app.await("independent reasoning choice persisted", func() error {
					payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
					if err != nil {
						return err
					}
					var disk struct {
						Snapshot contracts.Snapshot `json:"snapshot"`
					}
					if err := json.Unmarshal(payload, &disk); err != nil {
						return err
					}
					if got := disk.Snapshot.Settings.ModelPicker.Selections["alpha"]; got.ModelID != "alpha/m0" || got.Reasoning != "High" {
						return fmt.Errorf("persisted selection: %+v", got)
					}
					return nil
				})
				app.act("model-trigger", "press", "")
				app.act("model-settings", "press", "")
				app.waitKey("add-provider")
				app.capture("model-settings")
				app.act("settings-nav-General", "press", "")
				app.waitKey("Compact mode")
				tree, err = app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tree.findKey("Compact model picker"); err == nil {
					t.Fatal("removed compact picker setting remains")
				}
				payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(payload), "compactModelPicker") {
					t.Fatal("obsolete compact picker configuration remains")
				}
				app.act("back-to-conversations", "press", "")
				app.act("sidebar-session:"+sessionID, "press", "")
				app.await("saved conversation renders the smaller input", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					input, err := tree.findKey("input")
					if err != nil {
						return err
					}
					if input.Bounds.Height >= homeInput.Bounds.Height {
						return fmt.Errorf("input did not shrink: home=%+v conversation=%+v", homeInput.Bounds, input.Bounds)
					}
					return nil
				})
				if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
					t.Fatal(err)
				}
				app.capture("conversation-compact")
				assertCompactComposerHasNoScrollbar(t, app, 900)
				app.act("model-trigger", "press", "")
				app.act("model-provider:beta", "press", "")
				app.await("conversation uses the same button gap as home", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					gap, err := modelMenuButtonGap(tree)
					if err != nil {
						return err
					}
					if math.Abs(gap-homeGap) > 1 {
						return fmt.Errorf("menu gap changed: home=%g conversation=%g", homeGap, gap)
					}
					return nil
				})
				app.capture("conversation-model-position")
			})
		}
	}
}

func assertCompactComposerHasNoScrollbar(t *testing.T, app *nativeApp, windowWidth float64) {
	t.Helper()
	input := app.waitKey("input")
	if input.Bounds.Height != 47 {
		t.Fatalf("empty conversation input lost its compact height: %+v", input.Bounds)
	}
	file, err := os.Open(filepath.Join(app.artifacts, "conversation-compact.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	scale := float64(img.Bounds().Dx()) / windowWidth
	x := int((input.Bounds.X + input.Bounds.Width - 6) * scale)
	neighbor := int((input.Bounds.X + input.Bounds.Width - 18) * scale)
	for y := int((input.Bounds.Y + 5) * scale); y < int((input.Bounds.Y+input.Bounds.Height-5)*scale); y++ {
		r, g, b, a := img.At(x, y).RGBA()
		nr, ng, nb, na := img.At(neighbor, y).RGBA()
		if r != nr || g != ng || b != nb || a != na {
			t.Fatalf("empty native composer painted a scrollbar at (%d,%d)", x, y)
		}
	}
}

// Compare the actual popup and trigger, independent of editor height or page mode.
func modelMenuButtonGap(tree accessTree) (float64, error) {
	trigger, err := tree.findKey("model-trigger")
	if err != nil {
		return 0, err
	}
	provider, err := tree.findKey("model-provider:alpha")
	if err != nil {
		return 0, err
	}
	if math.Abs(provider.Bounds.X+provider.Bounds.Width/2-trigger.Bounds.X-trigger.Bounds.Width/2) > 1 {
		return 0, fmt.Errorf("supplier menu drifted from model button center")
	}
	for _, node := range tree.Nodes {
		if node.Role == "popup" {
			gap := trigger.Bounds.Y - node.Bounds.Y - node.Bounds.Height
			if math.Abs(gap-6) > 1 {
				return gap, fmt.Errorf("model menu button gap=%g: popup=%+v trigger=%+v", gap, node.Bounds, trigger.Bounds)
			}
			for _, row := range tree.Nodes {
				var key string
				_ = json.Unmarshal(row.Key, &key)
				if key == "model-choice:beta/m0" {
					browsed, err := tree.findKey("model-provider:beta")
					if err != nil {
						return gap, err
					}
					if math.Abs(row.Bounds.Y-browsed.Bounds.Y) > 1 {
						return gap, fmt.Errorf("submenu is not aligned to browsed provider: provider=%+v model=%+v", browsed.Bounds, row.Bounds)
					}
					return gap, nil
				}
			}
			return gap, nil
		}
	}
	return 0, fmt.Errorf("model popup missing")
}
