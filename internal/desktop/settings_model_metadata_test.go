package desktop

import (
	"context"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"github.com/egoist/mygo/ui"
)

func TestModelSmartConfigurationUsesCachedMetadataAndSavesManualOverrides(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "smart", Name: "My gateway", BaseURL: "https://api.deepseek.com/v1", Protocol: model.Chat}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.openSavedModel(provider.ID, "")
	draft := &app.modelsUI.draft
	if draft.context != "200000" || draft.output != "12800" || !slices.Equal(draft.model.InputTypes, []string{"text"}) {
		t.Fatalf("wrong add-model defaults: %+v", draft)
	}
	draft.model.ID = "deepseek-v4-pro"
	app.lookupModelMetadata()
	expected, err := core.LookupModelMetadata(context.Background(), modelcatalog.Query{ID: draft.model.ID, BaseURL: provider.BaseURL})
	if err != nil || !expected.Found || draft.context != "1048576" || draft.loading ||
		!slices.Equal(draft.model.ReasoningLevels, expected.ReasoningLevels) {
		t.Fatalf("smart config did not use cached metadata: %+v err=%v", draft, err)
	}
	draft.context, draft.output = "123456", "12800"
	app.cancelModelMetadataLookup()
	app.saveModelMetadata()
	snapshot, err := core.Snapshot(context.Background())
	if err != nil || len(snapshot.CustomProviders[0].Provider.Models) != 1 {
		t.Fatalf("save failed: %v", err)
	}
	saved := snapshot.CustomProviders[0].Provider.Models[0]
	if saved.ID != "deepseek-v4-pro" || saved.ContextWindow != 123456 || saved.MaxOutput != 12800 || len(saved.InputTypes) == 0 {
		t.Fatalf("manual override lost: %+v", saved)
	}
	if len(snapshot.Models) != 1 || snapshot.Models[0].Label != saved.ID {
		t.Fatal("catalog must display the request model ID")
	}
	app.openSavedModel(provider.ID, "")
	app.modelsUI.draft.model.ID = "private-model-not-in-catalog"
	app.modelsUI.draft.context = "76543"
	app.lookupModelMetadata()
	if app.modelsUI.draft.context != "76543" || app.modelsUI.draft.output != "12800" || app.modelsUI.draft.loading {
		t.Fatal("unknown model did not retain defaults and manual configuration")
	}
}

func TestModelMetadataOptionsPersistWithoutLeakingCanceledDraft(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "options", Name: "Options", Protocol: model.Responses}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.openSavedModel(provider.ID, "")
	app.modelsUI.draft.model.ID = "my-model"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.modelMetadataDialog(c)
	}, 900, 800)
	for _, level := range configurableReasoningLevels {
		if err := tt.Click("Model reasoning " + model.ReasoningLabel(level)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("Image"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(app.modelsUI.draft.model.ReasoningLevels, configurableReasoningLevels) {
		t.Fatal("checkboxes did not preserve reasoning order")
	}
	if err := tt.Click("Save model"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	saved := snapshot.CustomProviders[0].Provider.Models[0]
	if !slices.Equal(saved.ReasoningLevels, configurableReasoningLevels) || !slices.Equal(saved.InputTypes, []string{"text", "image"}) {
		t.Fatalf("option selections were not saved: %+v", saved)
	}
	app.openSavedModel(provider.ID, saved.ID)
	app.modelsUI.draft.model.InputTypes[0] = "video"
	app.modelsUI.draft.model.ReasoningLevels[0] = "high"
	app.modelsUI.modelOpen = false
	after, _ := core.Snapshot(context.Background())
	if !slices.Equal(after.CustomProviders[0].Provider.Models[0].InputTypes, []string{"text", "image"}) ||
		!slices.Equal(after.CustomProviders[0].Provider.Models[0].ReasoningLevels, configurableReasoningLevels) {
		t.Fatal("canceled draft mutated saved metadata")
	}
}
