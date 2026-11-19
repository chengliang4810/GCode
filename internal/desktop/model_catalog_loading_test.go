package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

// The remote catalog never owns the model list: the list comes from the local
// provider configuration. A refresh in flight therefore keeps the list and only
// replaces it with a skeleton while no model is configured at all.
func TestModelCatalogLoadingKeepsConfiguredModelList(t *testing.T) {
	a := testApp(t, "default")
	if len(a.state.Models) == 0 {
		t.Fatal("the fixture must offer configured models")
	}
	a.state.Settings.Appearance.ReduceMotion = true
	a.saveSettings()
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogLoading}
	tt := ui.NewTester(a.View, 900, 600)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Loading models") {
		t.Fatal("a loading catalog hid the locally configured model list")
	}
	if !tt.HasText("Sonnet 4.6") {
		t.Fatalf("the model list disappeared while the catalog refreshed: %v", tt.Texts())
	}
	if !tt.HasText("Search models") {
		t.Fatal("the list chrome disappeared while the catalog refreshed")
	}
}

func TestModelCatalogLoadingWithoutModelsShowsSkeleton(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.ReduceMotion = true
	a.saveSettings()
	a.state.Models = nil
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogLoading}
	tt := ui.NewTester(a.View, 900, 600)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Loading models") {
		t.Fatalf("an empty model list did not wait for the catalog: %v", tt.Texts())
	}
	if tt.HasText("Search models") {
		t.Fatal("the skeleton kept the unstable list chrome")
	}
}

// The model limits come from the remote catalog, so the settings dialog says so
// while a refresh is in flight and drops the hint once the catalog settles.
func TestModelMetadataHintsWhileCatalogLoads(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "hint", Name: "Hint", Protocol: model.Responses}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.openSavedModel(provider.ID, "")
	app.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogLoading}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.modelMetadataDialog(c)
	}, 900, 800)
	if !tt.HasText("Loading model details…") {
		t.Fatalf("the metadata dialog did not report the loading catalog: %v", tt.Texts())
	}
	app.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogReady}
	tt.Frame()
	if tt.HasText("Loading model details…") {
		t.Fatal("the metadata hint survived the settled catalog")
	}
}
