package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
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

	// The dialog and progress must exist before the queued request is executed.
	if err := tt.Click("Fetch models"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.modelsUI.fetchDialogOpen || !tt.HasText("Fetching models…") {
		t.Fatal("fetch dialog must open before executing the request")
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
	app.win = nil // Synchronous catalog lookup in the component harness.
	app.apply(snapshot)
	app.win = &mygo.Window{}
	if !app.modelsUI.fetchDialogOpen {
		t.Fatal("a ready fetch did not open the selection dialog")
	}
	tt.Frame()
	if !tt.HasText("Fetched models") || !tt.HasText("fresh-one") || !tt.HasText("existing") {
		t.Fatalf("dialog did not list fetched models: %v", tt.Texts())
	}
	if err := tt.Click("fresh-two"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("2 selected") {
		t.Fatalf("selection count did not follow the unchecked row: %v", tt.Texts())
	}
	if err := tt.Click("Save models"); err != nil {
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

type fetchedMetadataService struct {
	contracts.Service
	lookup func(context.Context, modelcatalog.Query) (modelcatalog.Metadata, error)
}

func (s fetchedMetadataService) LookupModelMetadata(ctx context.Context, query modelcatalog.Query) (modelcatalog.Metadata, error) {
	return s.lookup(ctx, query)
}

func TestProviderModelFetchMetadataSelectionAndManualOverrides(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "fetcher", Name: "Fetch fixture", Protocol: model.Messages, BaseURL: "https://example.com/v1",
		Models: []model.Model{{ID: "existing", ContextWindow: 10000}}}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.route, app.section = "settings", "Model settings"
	app.modelsUI.inlineID, app.modelsUI.selected = provider.ID, provider.ID
	queries := []string{}
	app.service = fetchedMetadataService{Service: core, lookup: func(ctx context.Context, query modelcatalog.Query) (modelcatalog.Metadata, error) {
		queries = append(queries, query.ID)
		if query.BaseURL != provider.BaseURL {
			t.Fatalf("lookup lost provider URL: %+v", query)
		}
		return modelcatalog.Metadata{Found: query.ID == "known", ContextWindow: 65536, MaxOutput: 8192,
			InputTypes: []string{"text", "image"}, ReasoningLevels: []string{"minimal", "low", "high"}}, nil
	}}
	app.state.CustomProviders[0].ModelFetch = &contracts.ModelFetch{ProviderID: provider.ID, State: "ready",
		Models: []string{"known", "unknown", "existing"}, StartedAt: time.Now()}
	app.modelsUI.fetchDialogOpen = true
	app.syncModelFetchDialog()
	if !slices.Equal(queries, []string{"known", "unknown"}) {
		t.Fatalf("new models were not queried immediately: %v", queries)
	}
	rows := app.modelsUI.fetchRows
	if rows[0].Draft.context != "65536" || rows[0].Draft.output != "8192" ||
		!slices.Equal(rows[0].Draft.model.InputTypes, []string{"text", "image"}) ||
		!slices.Equal(rows[0].Draft.model.ReasoningLevels, []string{"low", "high"}) ||
		rows[1].Draft.context != "200000" || !rows[2].Configured {
		t.Fatalf("metadata/defaults/configured models are incorrect: %+v", rows)
	}
	for _, row := range rows {
		if !row.Checked {
			t.Fatalf("row %q was not selected by default", row.ID)
		}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.modelFetchDialog(c)
	}, 1000, 800)
	if err := tt.Click("Select all models"); err != nil {
		t.Fatal(err)
	}
	if app.selectedFetchedModelCount() != 0 {
		t.Fatal("master checkbox did not deselect all importable models")
	}
	if err := tt.Click("Select all models"); err != nil {
		t.Fatal(err)
	}
	if app.selectedFetchedModelCount() != 3 {
		t.Fatal("master checkbox did not select existing and new models")
	}
	if err := tt.Click("Select all models"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("known"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Already configured") || tt.HasText("Invert selection") || tt.HasText("No model details found. Review the defaults before importing.") {
		t.Fatal("obsolete model annotations remain")
	}
	if err := tt.Click("Image input for known"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("existing"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Image input for existing"); err != nil {
		t.Fatal(err)
	}
	app.modelsUI.fetchRows[2].Draft.context = "33333"
	// Invalid values must leave the editable list open for correction.
	app.modelsUI.fetchRows[0].Draft.context = "-1"
	app.confirmFetchModels()
	tt.Frame()
	if !app.modelsUI.fetchDialogOpen || !tt.HasText("Context") {
		t.Fatal("invalid token count hid the editable list")
	}
	app.modelsUI.fetchRows[0].Draft.context = "123456"
	app.confirmFetchModels()
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	saved := snapshot.CustomProviders[0].Provider.Models
	if len(saved) != 2 || saved[0].ID != "existing" || saved[0].ContextWindow != 33333 || !slices.Contains(saved[0].InputTypes, "image") ||
		saved[1].ID != "known" || saved[1].ContextWindow != 123456 || saved[1].MaxOutput != 8192 ||
		slices.Contains(saved[1].InputTypes, "image") || !slices.Equal(saved[1].ReasoningLevels, []string{"low", "high"}) {
		t.Fatalf("selected metadata/manual overrides were not persisted: %+v", saved)
	}
}

func TestProviderModelFetchEditsExistingModelsWithoutReplacingHiddenFields(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "existing-only", Name: "Models", Protocol: model.Chat, Models: []model.Model{
		{ID: "editable", ContextWindow: 10000, MaxOutput: 4096, InputPrice: .3, InputTypes: []string{"text", "audio"}, ReasoningLevels: []string{"high"}},
		{ID: "unchecked", ContextWindow: 20000, InputTypes: []string{"text"}},
	}}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.route, app.section = "settings", "Model settings"
	app.modelsUI.inlineID = provider.ID
	app.state.CustomProviders[0].ModelFetch = &contracts.ModelFetch{ProviderID: provider.ID, State: "ready",
		Models: []string{"editable", "unchecked"}, StartedAt: time.Now()}
	app.modelsUI.fetchDialogOpen = true
	app.syncModelFetchDialog()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.modelFetchDialog(c)
	}, 900, 600)
	if app.selectedFetchedModelCount() != 2 {
		t.Fatal("existing models were not selected by default")
	}
	if err := tt.Click("unchecked"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Image input for editable"); err != nil {
		t.Fatal(err)
	}
	app.modelsUI.fetchRows[0].Draft.context = "34567"
	app.modelsUI.fetchRows[1].Draft.context = "99999"
	if err := tt.Click("Save models"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	saved := snapshot.CustomProviders[0].Provider.Models
	if len(saved) != 2 || saved[0].ContextWindow != 34567 || saved[0].MaxOutput != 4096 || saved[0].InputPrice != .3 ||
		!slices.Equal(saved[0].InputTypes, []string{"text", "image", "audio"}) ||
		!slices.Equal(saved[0].ReasoningLevels, []string{"high"}) || saved[1].ContextWindow != 20000 {
		t.Fatalf("existing model edits lost hidden values or changed an unchecked model: %+v", saved)
	}
}

func TestProviderModelFetchDismissAndError(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "Model settings"
	app.modelsUI.inlineID = "fetcher"
	app.modelsUI.fetchDialogOpen, app.modelsUI.fetchLoading = true, true
	app.modelSettingsCommandFinished(contracts.Command{Kind: "provider.models.fetch", ID: "fetcher"}, fmt.Errorf("fixture request failed"))
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.modelFetchDialog(c)
	}, 900, 600)
	if !tt.HasText("fixture request failed") || !tt.HasText("Retry") || app.modelsUI.fetchLoading {
		t.Fatal("fetch error was not recoverable inside the dialog")
	}
	app.win = &mygo.Window{}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	command := <-app.jobs
	app.win = nil
	if command.Kind != "provider.models.fetch" || !tt.HasText("Fetching models…") {
		t.Fatal("retry did not restart the request inside the dialog")
	}
	tt.Key(0, ui.KeyEscape)
	if app.modelsUI.fetchDialogOpen {
		t.Fatal("Escape did not dismiss the fetch dialog")
	}
	app.state.CustomProviders = []contracts.ProviderConfiguration{{Provider: model.Provider{ID: "fetcher"},
		ModelFetch: &contracts.ModelFetch{ProviderID: "fetcher", State: "ready", Models: []string{"late"}, StartedAt: time.Now()}}}
	app.syncModelFetchDialog()
	if app.modelsUI.fetchDialogOpen || len(app.modelsUI.fetchRows) != 0 {
		t.Fatal("late model list reopened a dismissed dialog")
	}
}

func TestProviderModelFetchEmptyAndCanceledMetadata(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "Model settings"
	app.modelsUI.inlineID = "fetcher"
	config := contracts.ProviderConfiguration{Provider: model.Provider{ID: "fetcher"},
		ModelFetch: &contracts.ModelFetch{ProviderID: "fetcher", State: "ready", Error: "The provider returned no models.", StartedAt: time.Now()}}
	app.state.CustomProviders = []contracts.ProviderConfiguration{config}
	app.modelsUI.fetchDialogOpen = true
	app.syncModelFetchDialog()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.modelFetchDialog(c)
	}, 900, 600)
	if !tt.HasText("The provider returned no models.") || app.modelsUI.fetchError != "" {
		t.Fatal("an empty result was presented as a failed request")
	}
	app.service = fetchedMetadataService{Service: core, lookup: func(ctx context.Context, query modelcatalog.Query) (modelcatalog.Metadata, error) {
		// Close while a lookup is pending, before its result is published.
		app.dismissModelFetch()
		if ctx.Err() == nil {
			t.Fatal("closing the dialog did not cancel metadata work")
		}
		return modelcatalog.Metadata{Found: true, ContextWindow: 999999}, nil
	}}
	app.modelsUI.fetchDialogOpen = true
	app.modelsUI.fetchSeen = ""
	app.state.CustomProviders[0].ModelFetch.Models = []string{"late"}
	app.syncModelFetchDialog()
	if app.modelsUI.fetchDialogOpen || app.modelsUI.fetchMetadataLoading || len(app.modelsUI.fetchRows) != 0 {
		t.Fatal("a canceled metadata result restored closed dialog state")
	}
}
