package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestProviderModelFetchDialogAddsSelectedModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("model list used wrong endpoint: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"fresh-one"},{"id":"fresh-two"},{"id":"existing"}]}`)
	}))
	t.Cleanup(server.Close)

	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "fetcher", Name: "Fetch fixture", Protocol: model.Chat, BaseURL: server.URL + "/v1", APIKey: "fetch-key", Models: []model.Model{{ID: "existing"}}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	app.route, app.section = "settings", "Model settings"
	// Exercise the native command queue without attaching a graphics window.
	app.win = &mygo.Window{}
	t.Cleanup(func() { app.win = nil })

	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	tt.Frame()
	if app.modelsUI.selected != "fetcher" || app.modelsUI.inlineID != "fetcher" {
		t.Fatalf("fixture provider was not selected: %q / %q", app.modelsUI.selected, app.modelsUI.inlineID)
	}

	// Fetching renders progress, then the selection dialog opens on its own.
	if err := tt.Click("Fetch models"); err != nil {
		t.Fatal(err)
	}
	command := <-app.jobs
	if command.Kind != "provider.models.fetch" || command.ID != "fetcher" {
		t.Fatalf("unexpected fetch command: %+v", command)
	}
	if err := core.Execute(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	app.commandFinished(command, nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot, _ = core.Snapshot(context.Background())
		if snapshot.CustomProviders[0].ModelFetch != nil && snapshot.CustomProviders[0].ModelFetch.State != "fetching" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("model fetch never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
	app.apply(snapshot)
	if !app.modelsUI.fetchDialogOpen {
		t.Fatal("a ready fetch did not open the selection dialog")
	}
	tt.Frame()
	if !tt.HasText("Fetched models") || !tt.HasText("fresh-one") || !tt.HasText("Already configured") {
		t.Fatalf("dialog did not list fetched models: %v", tt.Texts())
	}
	if err := tt.Click("fresh-two"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Add 1 models") {
		t.Fatalf("selection count did not follow the unchecked row: %v", tt.Texts())
	}
	if err := tt.Click("Add 1 models"); err != nil {
		t.Fatal(err)
	}
	if app.modelsUI.fetchDialogOpen {
		t.Fatal("confirming kept the selection dialog open")
	}
	save := <-app.jobs
	if save.Kind != "provider.save" {
		t.Fatalf("confirmation did not save the provider: %+v", save)
	}
	if err := core.Execute(context.Background(), save); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	app.apply(snapshot)
	tt.Frame()
	saved := snapshot.CustomProviders[0].Provider
	if len(saved.Models) != 2 || saved.Models[0].ID != "existing" || saved.Models[1].ID != "fresh-one" {
		t.Fatalf("confirmation did not add only the selected models: %+v", saved.Models)
	}
	if !snapshot.CustomProviders[0].HasAPIKey {
		t.Fatal("adding fetched models dropped the saved provider key")
	}
	if app.modelsUI.fetchDialogOpen {
		t.Fatal("a consumed fetch result reopened its dialog")
	}
}
