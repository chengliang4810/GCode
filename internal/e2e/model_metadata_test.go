//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"gcode/internal/storage"
)

// Real local core and native controls; two conflicting metadata fixtures are
// cached locally, with no provider or models.dev request during lookup.
func TestDesktopLocalSmartModelConfiguration(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			// Two conflicting cached entries reproduce the reported multi-match case.
			catalog := json.RawMessage(`{"beta":{"models":{"deepseek-v4.1-flash":{"id":"deepseek-v4.1-flash","limit":{"context":64000,"output":8000}}}},"alpha":{"models":{"deepseek-v4.1-flash":{"id":"deepseek-v4.1-flash","limit":{"context":32000,"output":4000},"modalities":{"input":["text"]},"reasoning_options":[{"type":"effort","values":["low","high"]}]}}}}`)
			catalogPath := filepath.Join(data, "cache", "gcode", "models-dev.json")
			if err := storage.WriteJSON(catalogPath, catalog); err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			provider := model.Provider{ID: "smart", Name: "My models", Protocol: model.Chat, BaseURL: "https://gateway.example/v1"}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
				t.Fatal(err)
			}
			expected, err := core.LookupModelMetadata(context.Background(), modelcatalog.Query{ID: "deepseek-v4.1-flash", BaseURL: provider.BaseURL})
			if err != nil || !expected.Found {
				t.Fatalf("cached metadata: %+v %v", expected, err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(catalogPath)
			if err != nil {
				t.Fatal(err)
			}
			language := "en"
			if appearance == "dark" {
				language = "zh-CN"
			}
			app := startApp(t, binary, artifacts, "smart-model", data, "--local", "--appearance", appearance, "--language", language, "--width", "900", "--height", "600")
			app.act("settings-nav-Model settings", "press", "")
			app.act("add-model", "press", "")
			contextField, outputField := app.waitKey("context"), app.waitKey("output")
			if contextField.Value != "200000" || outputField.Value != "12800" {
				t.Fatal("new-model token defaults changed")
			}
			tree, err := app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tree.findKey("model-smart-config"); err == nil {
				t.Fatal("smart configuration switch remains")
			}
			if expected.Provider != "alpha" || expected.ContextWindow != 32000 {
				t.Fatalf("wrong first match: %+v", expected)
			}
			app.capture("add-model-defaults")
			// Two quick changes exercise the old-result guard in the async UI lookup.
			app.act("id", "setValue", "unlisted-draft")
			app.act("id", "setValue", "deepseek-v4.1-flash")
			app.await("cached model metadata fills the native form", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				contextField, err := tree.findKey("context")
				if err != nil {
					return err
				}
				outputField, err := tree.findKey("output")
				if err != nil {
					return err
				}
				if contextField.Value != fmt.Sprint(expected.ContextWindow) || outputField.Value != fmt.Sprint(expected.MaxOutput) {
					return fmt.Errorf("metadata not applied: context=%s output=%s", contextField.Value, outputField.Value)
				}
				for _, level := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
					node, err := tree.findKey("model-reasoning:" + level)
					if err != nil {
						return err
					}
					if node.checked() != slices.Contains(expected.ReasoningLevels, level) {
						return fmt.Errorf("wrong reasoning checkbox: %s", level)
					}
				}
				for _, node := range tree.Nodes {
					var key string
					_ = json.Unmarshal(node.Key, &key)
					if key == "name" || key == "model-smart-config" || node.Label == "Advanced settings" || node.Label == "Model capabilities" || node.Label == "Smart configuration" || node.Label == "智能配置" || strings.Contains(node.Label, "configure manually") || strings.Contains(node.Label, "请手动配置") || strings.Contains(node.Label, "local model catalog") || strings.Contains(node.Label, "本地模型目录") {
						return fmt.Errorf("obsolete model field remains: %+v", node)
					}
				}
				return nil
			})
			app.capture("smart-model-configured")
			app.act("context", "setValue", "234567")
			app.act("output", "setValue", "12800")
			app.act("model-input:image", "press", "")
			app.act("save-model", "press", "")
			app.await("smart form preserves manual selections in the host", func() error {
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
				if len(disk.Providers) != 1 || len(disk.Providers[0].Models) != 1 {
					return fmt.Errorf("model not saved")
				}
				saved := disk.Providers[0].Models[0]
				if saved.ID != "deepseek-v4.1-flash" || saved.ContextWindow != 234567 || saved.MaxOutput != 12800 ||
					!slices.Contains(saved.InputTypes, "image") || !slices.Equal(saved.ReasoningLevels, expected.ReasoningLevels) {
					return fmt.Errorf("wrong saved metadata: %+v", saved)
				}
				if len(disk.Snapshot.Models) != 1 || disk.Snapshot.Models[0].Label != saved.ID {
					return fmt.Errorf("model list retained a display-name alias")
				}
				return nil
			})
			after, err := os.Stat(catalogPath)
			if err != nil || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("a form lookup rewrote or refreshed the cache")
			}
			app.waitKey("edit-model-smart/deepseek-v4.1-flash")
			app.capture("model-id-list")
			app.act("edit-model-smart/deepseek-v4.1-flash", "press", "")
			if app.waitKey("context").Value != "234567" {
				t.Fatal("reopening discarded manual values")
			}
			app.act("reset-model", "press", "")
			app.await("reset reapplies cached configuration", func() error {
				field := app.waitKey("context")
				if field.Value != fmt.Sprint(expected.ContextWindow) {
					return fmt.Errorf("reset not applied")
				}
				return nil
			})
			app.capture("model-reset")
		})
	}
}
