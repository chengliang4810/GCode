//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Real native desktop and local core; provider model list and catalog are local
// fixtures. The blocked endpoint proves the dialog opens before the response.
func TestDesktopLocalModelFetchImport(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					http.Error(w, "unexpected endpoint", http.StatusNotFound)
					return
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":[{"id":"known"},{"id":"unknown"},{"id":"existing"}]}`)
			}))
			t.Cleanup(server.Close)
			t.Cleanup(unblock)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			catalog := json.RawMessage(`{"fixture":{"models":{"known":{"id":"known","limit":{"context":65536,"output":8192},"modalities":{"input":["text","image"]},"reasoning_options":[{"type":"effort","values":["low","high"]}]}}}}`)
			if err := storage.WriteJSON(filepath.Join(data, "cache", "gcode", "models-dev.json"), catalog); err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			provider := model.Provider{ID: "fetcher", Name: "Fetch fixture", Protocol: model.Chat,
				BaseURL: server.URL, Models: []model.Model{{ID: "existing", ContextWindow: 10000}}}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save",
				Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			language := "en"
			if appearance == "dark" {
				language = "zh-CN"
			}
			app := startApp(t, binary, artifacts, "model-fetch", data, "--local", "--appearance", appearance,
				"--language", language, "--width", "900", "--height", "600")
			app.act("settings-nav-Model settings", "press", "")
			fetchButton, addButton := app.waitKey("fetch-models"), app.waitKey("add-model")
			gap := addButton.Bounds.X - fetchButton.Bounds.X - fetchButton.Bounds.Width
			if gap < 10 {
				t.Fatalf("model action buttons have no separation: %.2f", gap)
			}
			app.capture("model-action-buttons")
			app.act("fetch-models", "press", "")
			app.waitKey("fetch-models-loading")
			app.capture("model-fetch-loading")
			unblock()
			app.await("catalog metadata fills fetched models", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				contextField, err := tree.findKey("fetched-context-known")
				if err != nil {
					return err
				}
				image, err := tree.findKey("fetched-input-known:image")
				if err != nil {
					return err
				}
				confirm, err := tree.findKey("confirm-fetch")
				if err != nil {
					return err
				}
				if contextField.Value != "65536" || !image.checked() || slices.Contains(confirm.States, "disabled") {
					return fmt.Errorf("metadata still pending: context=%q image=%t", contextField.Value, image.checked())
				}
				for _, id := range []string{"known", "unknown", "existing"} {
					row, err := tree.findKey("fetched-model-" + id)
					if err != nil || !row.checked() {
						return fmt.Errorf("row %s not checked by default: %v", id, err)
					}
				}
				for _, id := range []string{"known", "unknown", "existing"} {
					row, _ := tree.findKey("fetched-model-" + id)
					field, err := tree.findKey("fetched-context-" + id)
					if err != nil {
						return err
					}
					image, err := tree.findKey("fetched-input-" + id + ":image")
					if err != nil {
						return err
					}
					center := row.Bounds.Y + row.Bounds.Height/2
					if math.Abs(center-field.Bounds.Y-field.Bounds.Height/2) > 1 || math.Abs(center-image.Bounds.Y-image.Bounds.Height/2) > 1 {
						return fmt.Errorf("model %s is not on a single line", id)
					}
				}
				for _, node := range tree.Nodes {
					var key string
					_ = json.Unmarshal(node.Key, &key)
					if key == "fetched-invert-selection" || node.Label == "Already configured" || node.Label == "已配置" ||
						strings.Contains(node.Label, "No model details found") || strings.Contains(node.Label, "未找到模型参数") {
						return fmt.Errorf("obsolete model annotation: %s", node.Label)
					}
					if strings.HasPrefix(key, "fetched-output-") || strings.HasPrefix(key, "fetched-reasoning-") ||
						(strings.HasPrefix(key, "fetched-input-") && !strings.HasSuffix(key, ":image")) {
						return fmt.Errorf("import list exposes an extra model parameter: %s", key)
					}
				}
				return nil
			})
			app.capture("model-fetch-default-selection")
			app.act("fetched-select-all", "press", "")
			app.await("all importable models deselected", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				row, err := tree.findKey("fetched-model-known")
				if err != nil || row.checked() {
					return fmt.Errorf("known model still selected")
				}
				return nil
			})
			app.waitChecked("fetched-model-existing", false)
			app.act("fetched-model-known", "press", "")
			app.waitChecked("fetched-model-known", true)
			app.act("fetched-context-known", "setValue", "123456")
			app.act("fetched-input-known:image", "press", "")
			app.waitChecked("fetched-input-known:image", false)
			app.await("manual token limit applied", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				field, err := tree.findKey("fetched-context-known")
				if err != nil || field.Value != "123456" {
					return fmt.Errorf("edited context not applied: %v", err)
				}
				return nil
			})
			app.act("fetched-model-existing", "press", "")
			app.waitChecked("fetched-model-existing", true)
			app.act("fetched-context-existing", "setValue", "33333")
			app.act("fetched-input-existing:image", "press", "")
			app.waitChecked("fetched-input-existing:image", true)
			app.await("existing context edited", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				field, err := tree.findKey("fetched-context-existing")
				if err != nil || field.Value != "33333" {
					return fmt.Errorf("existing model edit pending: %v", err)
				}
				return nil
			})
			app.capture("model-fetch-manual-settings")
			app.act("confirm-fetch", "press", "")
			app.await("selected model and edited capabilities persisted", func() error {
				payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
				if err != nil {
					return err
				}
				var disk struct {
					Providers []model.Provider `json:"providers"`
				}
				if err := json.Unmarshal(payload, &disk); err != nil {
					return err
				}
				if len(disk.Providers) != 1 || len(disk.Providers[0].Models) != 2 {
					return fmt.Errorf("wrong persisted model count")
				}
				models := disk.Providers[0].Models
				if models[0].ID != "existing" || models[0].ContextWindow != 33333 || !slices.Contains(models[0].InputTypes, "image") ||
					models[1].ID != "known" || models[1].ContextWindow != 123456 ||
					models[1].MaxOutput != 8192 || slices.Contains(models[1].InputTypes, "image") ||
					!slices.Equal(models[1].ReasoningLevels, []string{"low", "high"}) {
					return fmt.Errorf("wrong persisted configuration: %+v", models)
				}
				return nil
			})
			app.capture("model-fetch-imported")
		})
	}
}
