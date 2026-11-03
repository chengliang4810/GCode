//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestDesktopLocalProviderInputsSaveOnBlur(t *testing.T) {
	binary, artifacts := testPaths(t)
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
	provider := model.Provider{ID: "blur", Name: "Blur fixture", Protocol: model.Chat}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
	}); err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	app := startApp(t, binary, artifacts, "provider-blur", data, "--local")
	app.act("settings-nav-Model settings", "press", "")
	read := func() (model.Provider, error) {
		contents, err := os.ReadFile(filepath.Join(data, "engine.json"))
		if err != nil {
			return model.Provider{}, err
		}
		var disk struct {
			Providers []model.Provider `json:"providers"`
		}
		if err := json.Unmarshal(contents, &disk); err != nil {
			return model.Provider{}, err
		}
		for _, saved := range disk.Providers {
			if saved.ID == provider.ID {
				return saved, nil
			}
		}
		return model.Provider{}, fmt.Errorf("fixture provider missing")
	}
	app.act("custom-provider-name", "focus", "")
	app.act("custom-provider-name", "setValue", "Renamed fixture")
	app.act("custom-provider-url", "focus", "")
	app.await("name persisted on blur", func() error {
		saved, err := read()
		if err == nil && saved.Name != "Renamed fixture" {
			err = fmt.Errorf("name not saved")
		}
		return err
	})
	app.act("custom-provider-url", "setValue", "https://example.invalid/v1")
	app.act("value", "focus", "")
	app.await("URL persisted on blur", func() error {
		saved, err := read()
		if err == nil && saved.BaseURL != "https://example.invalid/v1" {
			err = fmt.Errorf("URL not saved")
		}
		return err
	})
	app.act("value", "setValue", "native-fixture-secret")
	app.act("custom-provider-name", "focus", "")
	app.await("key persisted on blur", func() error {
		saved, err := read()
		if err == nil && saved.APIKey != "native-fixture-secret" {
			err = fmt.Errorf("key not saved")
		}
		return err
	})
	app.act("provider-enabled", "press", "")
	app.await("provider switch persisted immediately", func() error {
		saved, err := read()
		if err == nil && !saved.Disabled {
			err = fmt.Errorf("provider switch not saved")
		}
		return err
	})
	tree, err := app.client.tree()
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range tree.Nodes {
		if node.Label == "Save provider" {
			t.Fatal("manual provider save remains")
		}
	}
	app.capture("provider-inputs-saved")
}
