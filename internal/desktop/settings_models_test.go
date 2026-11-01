package desktop

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestModelMetadataDialogIsIsolatedAndPersistsToRealCore(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "modal", Name: "Models", Protocol: model.Chat, BaseURL: "https://example.com/v1",
		Models: []model.Model{{ID: "first"}, {ID: "second"}}}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.customProviderSettings(c) })
		app.settingsRemovalDialog(c)
	}, 1000, 800)
	if err := tt.Click("Edit model first"); err != nil {
		t.Fatal(err)
	}
	if !app.modelsUI.modelOpen {
		t.Fatal("model metadata did not open as a separate dialog")
	}
	app.modelsUI.draft.model.ID = "edited-first"
	app.modelsUI.draft.context = "64000"
	app.modelsUI.draft.model.ReasoningLevels = []string{"low", "high"}
	tt.Frame()
	before, _ := core.Snapshot(context.Background())
	if before.CustomProviders[0].Provider.Models[0].ID != "first" {
		t.Fatal("dialog draft leaked into the core")
	}
	tt.Key(0, ui.KeyEscape)
	if app.modelsUI.modelOpen {
		t.Fatal("Escape did not close model metadata")
	}
	if err := tt.Click("Edit model first"); err != nil {
		t.Fatal(err)
	}
	if app.modelsUI.draft.model.ID != "first" {
		t.Fatal("canceled model changes were retained as saved values")
	}
	app.modelsUI.draft.model.ID = "edited-first"
	app.modelsUI.draft.context = "64000"
	app.modelsUI.draft.model.ReasoningLevels = []string{"low", "high"}
	tt.Frame()
	if err := tt.Click("Save model"); err != nil {
		t.Fatal(err)
	}
	after, _ := core.Snapshot(context.Background())
	got := after.CustomProviders[0].Provider.Models
	if app.modelsUI.modelOpen || got[0].ID != "edited-first" || got[0].ContextWindow != 64000 ||
		!slices.Equal(got[0].ReasoningLevels, []string{"low", "high"}) || got[1].ID != "second" {
		t.Fatalf("dialog did not save only the selected model: %+v, %s", got, app.errorText)
	}
	if err := tt.Click("Add model"); err != nil {
		t.Fatal(err)
	}
	app.modelsUI.draft.model.ID = "third"
	tt.Frame()
	if err := tt.Click("Save model"); err != nil {
		t.Fatal(err)
	}
	after, _ = core.Snapshot(context.Background())
	if len(after.CustomProviders[0].Provider.Models) != 3 {
		t.Fatal("add-model dialog did not persist a new model")
	}
	if err := tt.Click("Provider actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove provider"); err != nil {
		t.Fatal(err)
	}
	before, _ = core.Snapshot(context.Background())
	if len(before.CustomProviders) != 1 || !app.settingsRemoval.Open {
		t.Fatal("provider was removed before confirmation")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Provider actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove provider"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Confirm delete"); err != nil {
		t.Fatal(err)
	}
	after, _ = core.Snapshot(context.Background())
	if len(after.CustomProviders) != 0 {
		t.Fatal("confirmed removal did not reach core")
	}
}

func TestModelSettingsDeleteAndProviderToggleUseRealCore(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{
		ID: "custom", Name: "My provider", BaseURL: "https://example.com/v1", Protocol: model.Chat, APIKey: "private-value",
		Models: []model.Model{{ID: "first"}, {ID: "second"}},
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
	}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	app.apply(state)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(16).Children(func() { app.customProviderSettings(c) })
	}, 1000, 1100)
	for _, label := range []string{"Enable model first", "Move model 1 up", "Move model 1 down", "Model actions first"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("model row retained obsolete control %q", label)
		}
	}
	if err := tt.Click("Remove model first"); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	if len(state.Models) != 1 || state.Models[0].ID != "custom/second" ||
		len(state.CustomProviders[0].Provider.Models) != 1 ||
		state.CustomProviders[0].Provider.Models[0].ID != "second" || !state.CustomProviders[0].HasAPIKey {
		t.Fatal("model deletion did not update execution catalog while retaining the other model and credentials")
	}
	if err := tt.Click("Enable provider My provider"); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	if len(state.Models) != 0 || !state.CustomProviders[0].Provider.Disabled {
		t.Fatal("disabled provider still supplied selectable models")
	}
}

func TestModelSettingsSaveFailureRetainsDraft(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.addCustomProvider()
	app.runtimeUI.provider.Name = "Draft"
	app.runtimeUI.provider.BaseURL = "https://example.com/v1"
	app.runtimeUI.key = "draft-secret"
	app.runtimeUI.provider.Models = []model.Model{{ID: "draft-model"}}
	app.markInlineProviderDirty()
	// The native asynchronous completion path restores the same draft; an
	// obsolete failure must not replace a newly selected provider.
	command := contracts.Command{Kind: "provider.save", ID: "inline-failure", Provider: &contracts.ProviderInput{Provider: app.runtimeUI.provider}}
	command.Provider.Provider.APIKey = app.runtimeUI.key
	app.modelsUI.selected = command.Provider.Provider.ID
	app.modelsUI.inlineSaving, app.modelsUI.inlinePending = true, command.Provider
	app.modelsUI.inlinePendingKey = command.ID
	app.commandFinished(command, os.ErrPermission)
	if app.modelsUI.inlineSaving || app.modelsUI.inlineError == "" || app.runtimeUI.key != "draft-secret" || app.runtimeUI.provider.Models[0].ID != "draft-model" {
		t.Fatal("failed save lost the editable provider draft")
	}
	app.runtimeUI.provider.ID = "newer-provider"
	app.modelsUI.inlineID, app.modelsUI.selected = "newer-provider", "newer-provider"
	app.runtimeUI.key = "newer-secret"
	app.modelsUI.inlineError = ""
	app.commandFinished(command, os.ErrPermission)
	if app.modelsUI.inlineError != "" || app.runtimeUI.key != "newer-secret" {
		t.Fatal("stale provider failure overwrote a newer draft")
	}
}

func TestModelSettingsRevealAndCancelKeepUnsubmittedDraft(t *testing.T) {
	for _, test := range []struct {
		name, action string
	}{
		{name: "reveal", action: "Show API key"},
		{name: "cancel", action: "Cancel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, core := runtimeSettingsTestApp(t)
			provider := model.Provider{ID: "draft-actions", Name: "Draft actions", Protocol: model.Chat, BaseURL: "https://example.com/v1",
				Models: []model.Model{{ID: "model"}}}
			app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
			}, 1000, 800)
			if err := tt.Click("API key"); err != nil {
				t.Fatal(err)
			}
			tt.Type("unsubmitted-secret")
			if err := tt.Click(test.action); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := core.Snapshot(context.Background())
			if snapshot.CustomProviders[0].HasAPIKey {
				t.Fatal("a local draft action unexpectedly submitted the secret")
			}
			if test.name == "reveal" && (app.runtimeUI.key != "unsubmitted-secret" || !app.modelsUI.showKey) {
				t.Fatal("revealing the secret lost the current draft")
			}
			if test.name == "cancel" && (app.runtimeUI.key != "" || app.modelsUI.inlineDirty) {
				t.Fatal("cancel did not discard the unsubmitted draft")
			}
		})
	}
}

func TestModelSettingsRevealSavedKey(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{700, 1000} {
			t.Run(fmt.Sprintf("dark=%v/width=%v", dark, width), func(t *testing.T) {
				app, core := runtimeSettingsTestApp(t)
				app.palette = design.Palette("rcode", dark)
				provider := model.Provider{ID: "saved-key", Name: "Saved key", Protocol: model.Chat,
					BaseURL: "https://example.com/v1", APIKey: "saved-original-secret"}
				app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
				var displayed string
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(app.palette.UI())
					ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
					displayed = c.Resolve(app.modelsUI.inputs[2]).InputDisplayText()
				}, width, 800)
				if err := tt.Click("Show API key"); err != nil {
					t.Fatal(err)
				}
				if displayed != provider.APIKey {
					t.Fatal("eye button did not display the saved API key")
				}
				if app.modelsUI.inlineDirty || app.runtimeUI.key != "" {
					t.Fatal("revealing a saved key changed the edit draft")
				}
				if err := tt.Click("Hide API key"); err != nil {
					t.Fatal(err)
				}
				if displayed == provider.APIKey {
					t.Fatal("hiding the API key left it visible")
				}
				if app.modelsUI.revealedKey != "" {
					t.Fatal("hidden key retained its reveal buffer")
				}
				if err := tt.Click("Show API key"); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click("API key"); err != nil {
					t.Fatal(err)
				}
				tt.Key(ui.Ctrl, ui.KeyA)
				tt.Type("replacement-secret")
				if app.runtimeUI.key != "replacement-secret" {
					t.Fatal("editing a revealed key did not update the draft")
				}
				tt.Key(0, ui.KeyEnter)
				saved, err := core.ReadProviderKey(context.Background(), provider.ID)
				if err != nil || saved != "replacement-secret" {
					t.Fatal("edited key did not reach the core")
				}
				if err := tt.Click("Show API key"); err != nil {
					t.Fatal(err)
				}
				if displayed != "replacement-secret" {
					t.Fatal("eye button displayed an obsolete key after save")
				}
				app.selectSettingsSection("General")
				if app.modelsUI.revealedKey != "" || app.modelsUI.showKey {
					t.Fatal("leaving model settings retained the revealed key")
				}
				snapshot, err := core.Snapshot(context.Background())
				if err != nil || snapshot.CustomProviders[0].Provider.APIKey != "" || !snapshot.CustomProviders[0].HasAPIKey {
					t.Fatal("revealing the key changed public snapshot redaction")
				}
			})
		}
	}
}

func TestModelSettingsRevealedKeyIsNotResubmittedByUnrelatedEdits(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "saved", Name: "Saved", Protocol: model.Chat,
		BaseURL: "https://example.com/v1", APIKey: "original-secret"}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	if err := tt.Click("Show API key"); err != nil {
		t.Fatal(err)
	}
	// Another client rotates the key while this UI still displays its old value.
	provider.APIKey = "rotated-secret"
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Provider name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Renamed")
	tt.Key(0, ui.KeyEnter)
	key, err := core.ReadProviderKey(context.Background(), provider.ID)
	if err != nil || key != "rotated-secret" {
		t.Fatal("unrelated edit overwrote the saved key with an old revealed value")
	}
}

type providerKeyReadHook struct {
	contracts.Service
	read func(context.Context, string) (string, error)
}

func (s providerKeyReadHook) ReadProviderKey(ctx context.Context, id string) (string, error) {
	return s.read(ctx, id)
}

func TestModelSettingsDiscardsObsoleteKeyReveal(t *testing.T) {
	for _, action := range []string{"edit", "leave", "switch"} {
		t.Run(action, func(t *testing.T) {
			app, core := runtimeSettingsTestApp(t)
			provider := model.Provider{ID: "saved", Name: "Saved", Protocol: model.Chat,
				BaseURL: "https://example.com/v1", APIKey: "original-secret"}
			app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
			app.modelsUI.selected = provider.ID
			app.syncInlineProvider(app.state.CustomProviders[0])
			app.service = providerKeyReadHook{Service: core, read: func(ctx context.Context, id string) (string, error) {
				// Simulate an intervening UI action before the read commits.
				switch action {
				case "edit":
					app.runtimeUI.key = "new-draft"
					app.markInlineProviderDirty()
				case "leave":
					app.selectSettingsSection("General")
				case "switch":
					app.modelsUI.selected = "other"
					app.syncInlineProvider(contracts.ProviderConfiguration{Provider: model.Provider{ID: "other"}})
				}
				return core.ReadProviderKey(ctx, id)
			}}
			app.toggleProviderKey()
			if app.modelsUI.showKey || app.modelsUI.revealedKey != "" || app.modelsUI.keyLoading {
				t.Fatal("obsolete reveal populated the current editor")
			}
			if action == "edit" && app.runtimeUI.key != "new-draft" {
				t.Fatal("obsolete reveal overwrote a newer key draft")
			}
		})
	}
}

func TestModelSettingsInlineSubmitAndBlurReachRealCore(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "inline-input", Name: "Inline inputs", Protocol: model.Chat, BaseURL: "https://example.com/v1",
		Models: []model.Model{{ID: "model"}}}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	if err := tt.Click("API key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("saved-secret")
	tt.Key(0, ui.KeyEnter)
	snapshot, _ := core.Snapshot(context.Background())
	if !snapshot.CustomProviders[0].HasAPIKey || app.runtimeUI.key != "" || app.modelsUI.inlineDirty {
		t.Fatal("submitting a key did not save it and clear the local secret")
	}
	for _, test := range []struct {
		name, url string
		submit    bool
	}{
		{name: "enter", url: "https://gateway.example.com/v1", submit: true},
		{name: "blur", url: "https://other.example.com/v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := tt.Click("Provider Base URL"); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Ctrl, ui.KeyA)
			tt.Type(test.url)
			if test.submit {
				tt.Key(0, ui.KeyEnter)
			} else if err := tt.Click("Provider name"); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := core.Snapshot(context.Background())
			saved := snapshot.CustomProviders[0]
			if saved.Provider.BaseURL != test.url || !saved.HasAPIKey || app.modelsUI.inlineDirty {
				t.Fatalf("editing URL did not save while retaining the stored key: %+v", saved)
			}
		})
	}
}
