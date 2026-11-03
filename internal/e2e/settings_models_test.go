//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The native controls call the real local core; only the provider response is
// served by a local fixture. No external account or user profile is accessed.
func TestDesktopLocalModelIconActions(t *testing.T) {
	binary, artifacts := testPaths(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "first" || r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected model diagnostic", http.StatusBadRequest)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	data := filepath.Join(artifacts, "data")
	store, err := storage.Open(storage.Options{Directory: data})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	provider := model.Provider{
		ID: "icons", Name: "Icon actions", Protocol: model.Chat, BaseURL: server.URL,
		Models: []model.Model{{ID: "first"}, {ID: "second"}},
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
	}); err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	app := startApp(t, binary, artifacts, "model-icons", data, "--local")
	app.act("settings-nav-Model settings", "press", "")
	app.waitKey("test-model-icons/first")
	tree, err := app.client.tree()
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range tree.Nodes {
		if strings.HasPrefix(node.Label, "Enable model ") || strings.HasPrefix(node.Label, "Move model ") ||
			strings.HasPrefix(node.Label, "Model actions ") {
			t.Fatalf("obsolete model control remains in native UI: %+v", node)
		}
	}
	app.capture("model-list")
	app.act("test-model-icons/first", "press", "")
	app.await("selected model diagnostic completed", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			if node.Label == "Connection successful" && requests.Load() == 1 {
				return nil
			}
		}
		return fmt.Errorf("model diagnostic has not succeeded; requests=%d", requests.Load())
	})
	app.act("edit-model-icons/first", "press", "")
	app.act("context", "setValue", "234567")
	app.act("save-model", "press", "")
	readModels := func() ([]model.Model, error) {
		payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
		if err != nil {
			return nil, err
		}
		var disk struct {
			Providers []model.Provider `json:"providers"`
		}
		if err := json.Unmarshal(payload, &disk); err != nil {
			return nil, err
		}
		if len(disk.Providers) != 1 {
			return nil, fmt.Errorf("unexpected provider count")
		}
		return disk.Providers[0].Models, nil
	}
	app.await("edited model saved", func() error {
		models, err := readModels()
		if err != nil {
			return err
		}
		if len(models) != 2 || models[0].ContextWindow != 234567 || models[1].ID != "second" {
			return fmt.Errorf("edited models=%+v", models)
		}
		return nil
	})
	app.act("delete-model-icons/first", "press", "")
	app.await("selected model deleted", func() error {
		models, err := readModels()
		if err != nil {
			return err
		}
		if len(models) != 1 || models[0].ID != "second" {
			return fmt.Errorf("remaining models=%+v", models)
		}
		return nil
	})
	app.waitKey("edit-model-icons/second")
	app.capture("after-delete")
}
