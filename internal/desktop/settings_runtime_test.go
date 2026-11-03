package desktop

import (
	"context"
	"fmt"
	"gcode/internal/assets"
	"reflect"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
	"github.com/egoist/mygo/ui"
)

func runtimeSettingsTestApp(t *testing.T) (*App, *engine.Engine) {
	t.Helper()
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(core)
	if err != nil {
		core.Close()
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.state.Settings.General.Language = "en"
	app.saveSettings()
	app.palette = design.Palette("rcode-dark", true)
	return app, core
}

func TestDataDirectorySettingsPendingRestartKeepsOriginalConversations(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.state.Runtime = &contracts.RuntimeInfo{
		DataDirectory:    "/original-runtime",
		PendingDirectory: "/next-runtime",
	}
	message := "The new directory takes effect after restart. Existing conversations stay in their original directories; no data is copied."
	for _, dark := range []bool{false, true} {
		for _, width := range []float64{900, 1320} {
			t.Run(fmt.Sprintf("dark-%t-width-%.0f", dark, width), func(t *testing.T) {
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(app.palette.UI())
					app.dataDirectorySettings(c)
				}, int(width), 600)
				if !tt.HasText(message) || !tt.HasText("/original-runtime") {
					t.Fatalf("pending directory did not describe the active root and restart boundary: %v", tt.Texts())
				}
				if tt.HasText("Restart to copy data and use the new directory. The original directory is retained.") {
					t.Fatal("directory settings still promise to copy old data")
				}
				if app.runtimeUI.directory != "/next-runtime" {
					t.Fatal("pending root was not shown in the editable draft")
				}
			})
		}
	}
}

func TestGeneralSettingsDoNotOfferCustomWorktreeStorage(t *testing.T) {
	app, _ := settingsTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.generalSettings(c)
	}, 900, 1200)
	if tt.HasText("Custom worktree location") || tt.HasText("Worktree location") {
		t.Fatal("general settings still expose the removed worktree root")
	}
}

func TestCustomProviderModelsHaveIndependentCapabilities(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.addCustomProvider()
	app.runtimeUI.provider.Name = "User provider"
	app.runtimeUI.provider.BaseURL = "https://example.com/v1"
	app.runtimeUI.provider.Models = []model.Model{
		{ID: "deep", ContextWindow: 200000, MaxOutput: 16000, ReasoningLevels: []string{"low", "high"}},
		{ID: "plain", ContextWindow: 32000, MaxOutput: 4000},
	}
	app.markInlineProviderDirty()
	app.saveProviderSettings(app.runtimeUI.provider)
	if app.modelsUI.inlineError != "" {
		t.Fatalf("inline provider save failed: %s", app.modelsUI.inlineError)
	}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.customProviderSettings(c) }, 900, 1200)
	if err := tt.Click("Edit model deep"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Model reasoning Max"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Save model"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if app.errorText != "" || len(snapshot.CustomProviders) != 1 {
		t.Fatalf("provider save failed: %s", app.errorText)
	}
	saved := snapshot.CustomProviders[0]
	first, second := saved.Provider.Models[0], saved.Provider.Models[1]
	if first.ID != "deep" || first.ContextWindow != 200000 || first.MaxOutput != 16000 || !slices.Equal(first.ReasoningLevels, []string{"low", "high", "max"}) {
		t.Fatalf("first model lost independent settings: %+v", first)
	}
	if second.ID != "plain" || second.ContextWindow != 32000 || second.MaxOutput != 4000 || len(second.ReasoningLevels) != 0 {
		t.Fatalf("capabilities leaked to the second model: %+v", second)
	}
	app.openSavedModel(saved.Provider.ID, "deep")
	app.modelsUI.draft.model.ReasoningLevels[0] = "medium"
	tt.Frame()
	if saved.Provider.Models[0].ReasoningLevels[0] != "low" {
		t.Fatal("editing mutated the authoritative provider before save")
	}
}

func TestRuntimeComposerDefaultsAndUnavailableReasoning(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.state.Runtime = &contracts.RuntimeInfo{}
	app.sessionID = "runtime-session"
	app.state.Providers = []contracts.Provider{{ID: "user", Name: "My provider"}}
	app.state.Models = []contracts.Model{
		{ID: "user/first", ProviderID: "user", Label: "First", ReasoningLevels: []string{"Low", "High", "Max"}},
		{ID: "user/second", ProviderID: "user", Label: "Second", ReasoningLevels: []string{"High", "XHigh"}},
		{ID: "user/plain", ProviderID: "user", Label: "Plain"},
	}
	app.modelID, app.reasoning = "user/first", ""
	app.normalizeReasoning()
	if app.reasoning != "Max" || app.modelLabel() != "First" {
		t.Fatalf("runtime default or label is wrong: %q / %q", app.reasoning, app.modelLabel())
	}
	app.reasoning = "removed-level"
	app.normalizeReasoning()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.centralControls().modelTrigger(c)
		app.centralControls().reasoningTrigger(c)
	}, 900, 600)
	if app.reasoning != "removed-level" {
		t.Fatal("rendering silently replaced the saved reasoning")
	}
	if tt.HasText("Unavailable · removed-level") || tt.HasText("removed-level") {
		t.Fatal("model trigger displayed an unavailable reasoning summary")
	}
	if got := app.centralControls().reasoningChoice().label(); got != "Unavailable · removed-level" {
		t.Fatalf("compact reasoning misrepresented an unavailable level: %q", got)
	}
	app.reasoning = "High"
	tt.Frame()
	if !tt.HasText("High") {
		t.Fatal("model trigger omitted a configured reasoning level")
	}
	app.reasoning = ""
	app.modelID = "user/plain"
	tt.Frame()
	if tt.HasText("Max") || tt.HasText("Reasoning effort") {
		t.Fatal("reasoning controls appeared for an unconfigured model")
	}
	app.state.Runtime = nil
	app.modelID, app.reasoning = "user/first", ""
	app.normalizeReasoning()
	if app.reasoning != "High" {
		t.Fatal("historical mock fixture default changed")
	}
}

func TestRuntimeComposerSwitchSelectsLastConfiguredLevel(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "user", Name: "My provider", Protocol: model.Responses, BaseURL: "https://example.com/v1", Models: []model.Model{
		{ID: "first", ReasoningLevels: []string{"low", "high", "max"}},
		{ID: "second", ReasoningLevels: []string{"high", "xhigh"}},
		{ID: "plain"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(context.Background())
	app.apply(snapshot)
	app.modelID, app.reasoning = "user/first", "High"
	app.modelMenu = false
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(20).Justify(ui.End).Children(func() {
			app.centralControls().modelTrigger(c)
			app.centralControls().reasoningTrigger(c)
		})
	}, 900, 600)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("second"); err != nil {
		t.Fatal(err)
	}
	if app.modelID != "user/second" || app.reasoning != "Extra high" {
		t.Fatalf("switch retained another model's intensity: %q / %q", app.modelID, app.reasoning)
	}
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("plain"); err != nil {
		t.Fatal(err)
	}
	if app.modelID != "user/plain" || app.reasoning != "" {
		t.Fatal("switch to a model without reasoning retained intensity")
	}
	if _, ok := tt.Find("Reasoning effort"); ok {
		t.Fatal("a model without reasoning exposed its effort slider")
	}
}

func TestModelPickerManagementEntryDoesNotSelectModel(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	for _, id := range []string{"first", "second"} {
		provider := model.Provider{ID: id, Name: id, Protocol: model.Chat, BaseURL: "http://127.0.0.1:1/v1",
			Models: []model.Model{{ID: "folder/model"}}}
		if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.apply(before)
	app.modelID = "first/folder/model"
	app.modelProvider = "second"
	app.modelMenu = true
	tt := ui.NewTester(app.View, 1320, 880)
	if err := tt.Click("Model management"); err != nil {
		t.Fatal(err)
	}
	if app.route != "settings" || app.section != "Model settings" {
		t.Fatalf("model management did not open settings: route=%q section=%q", app.route, app.section)
	}
	if app.modelID != "first/folder/model" || app.modelMenu {
		t.Fatal("model management selected another chat model or retained its picker")
	}
	after, err := core.Snapshot(context.Background())
	if err != nil || !reflect.DeepEqual(before.Settings.ModelPicker, after.Settings.ModelPicker) {
		t.Fatalf("opening settings changed model selection preferences: %v", err)
	}
}

func TestUnconfiguredComposerOpensModelSettingsDirectly(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.navigate("chat")
	tt := ui.NewTester(app.View, 900, 600)
	if !tt.HasText("Model management") || tt.HasText("No models configured") {
		t.Fatalf("unconfigured composer has the wrong entry: %v", tt.Texts())
	}
	if err := tt.Click("Model management"); err != nil {
		t.Fatal(err)
	}
	if app.route != "settings" || app.section != "Model settings" || app.modelMenu {
		t.Fatalf("entry did not open model settings directly: route=%q section=%q menu=%v", app.route, app.section, app.modelMenu)
	}
}
