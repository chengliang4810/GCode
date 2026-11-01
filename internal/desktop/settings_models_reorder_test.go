package desktop

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestModelSettingsDragReordersProvidersAndModels(t *testing.T) {
	for _, theme := range []string{"rcode-light", "rcode-dark"} {
		t.Run(theme, func(t *testing.T) {
			app, core := runtimeSettingsTestApp(t)
			app.palette = design.Palette(theme, theme == "rcode-dark")
			for _, id := range []string{"a", "b", "c"} {
				app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{
					Provider: model.Provider{ID: id, Name: id, Protocol: model.Chat,
						BaseURL: "https://example.com/v1", APIKey: "test-secret",
						Models: []model.Model{{ID: "first"}, {ID: "second"}, {ID: "third"}}},
				}})
			}
			app.modelsUI.selected = "a"
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
			}, 1180, 980)
			assertOrder := func(providers, models []string) {
				t.Helper()
				snapshot, err := core.Snapshot(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				var providerIDs, modelIDs []string
				for _, config := range snapshot.CustomProviders {
					providerIDs = append(providerIDs, config.Provider.ID)
					if !config.HasAPIKey {
						t.Fatal("reordering removed a saved credential")
					}
					if config.Provider.ID == "a" {
						for _, candidate := range config.Provider.Models {
							modelIDs = append(modelIDs, candidate.ID)
						}
					}
				}
				if !slices.Equal(providers, providerIDs) || !slices.Equal(models, modelIDs) {
					t.Fatalf("providers=%v models=%v; want %v %v", providerIDs, modelIDs, providers, models)
				}
				if app.modelsUI.selected != "a" {
					t.Fatal("a reorder changed the selected provider")
				}
			}
			drag := func(source, target string, after, cancel bool) {
				t.Helper()
				before, err := core.Snapshot(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				from, found := tt.Find(source)
				to, targetFound := tt.Find(target)
				if !found || !targetFound {
					t.Fatalf("missing drag source or target: %s %s", source, target)
				}
				x, y := to.X+to.W/2, to.Y+2
				if after {
					y = to.Y + to.H - 2
				}
				tt.Press(from.X+from.W/2, from.Y+from.H/2)
				tt.Move(from.X+from.W/2+6, from.Y+from.H/2)
				tt.Move(x, y)
				tt.Image() // Render the actual drag preview and insertion indicator.
				moving, _ := core.Snapshot(context.Background())
				if !reflect.DeepEqual(before.CustomProviders, moving.CustomProviders) {
					t.Fatal("dragging persisted an order before the pointer was released")
				}
				if cancel {
					tt.Key(0, ui.KeyEscape)
				}
				tt.Release(x, y)
			}
			drag("Provider row a", "Provider row c", true, false)
			assertOrder([]string{"b", "c", "a"}, []string{"first", "second", "third"})
			drag("Provider row a", "Provider row b", false, false)
			assertOrder([]string{"a", "b", "c"}, []string{"first", "second", "third"})
			drag("Model row first", "Model row third", true, false)
			assertOrder([]string{"a", "b", "c"}, []string{"second", "third", "first"})
			drag("Model row first", "Model row second", false, false)
			assertOrder([]string{"a", "b", "c"}, []string{"first", "second", "third"})
			drag("Provider row a", "Provider row c", true, true)
			drag("Model row first", "Model row third", true, true)
			// Drops on the same item or a different list are harmless.
			drag("Provider row a", "Provider row a", true, false)
			drag("Model row first", "Provider row c", true, false)
			drag("Provider row a", "Model row third", true, false)
			assertOrder([]string{"a", "b", "c"}, []string{"first", "second", "third"})
		})
	}
}

func TestModelSettingsReorderQueuePreservesDraftAndReportsFailure(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{
		Provider: model.Provider{ID: "a", Name: "Provider", Protocol: model.Chat,
			Models: []model.Model{{ID: "first"}, {ID: "second"}}},
	}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.customProviderSettings(c) })
	}, 1180, 980)
	// Retain a focused, unsubmitted credential while the reorder is in flight.
	if err := tt.Click("API key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("unsubmitted-key")
	app.win = &mygo.Window{}
	t.Cleanup(func() { app.win = nil })
	from, _ := tt.Find("Model row first")
	to, _ := tt.Find("Model row second")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+to.H-2)
	tt.Release(to.X+to.W/2, to.Y+to.H-2)
	if !app.modelSettingsBusy() {
		t.Fatal("reorder did not enter pending state")
	}
	select {
	case command := <-app.jobs:
		if command.Kind != "provider.models.reorder" || command.Reorder == nil {
			t.Fatalf("drag submitted %s instead of a reorder", command.Kind)
		}
		if app.runtimeUI.key != "unsubmitted-key" || !app.modelsUI.inlineDirty {
			t.Fatal("reorder discarded an unsubmitted provider draft")
		}
		if app.submitProviderSettings(app.runtimeUI.provider) {
			t.Fatal("a provider save can overwrite an in-flight reorder")
		}
		app.commandFinished(command, errors.New("reorder failed"))
		tt.Frame()
		if app.modelSettingsBusy() || !tt.HasText("reorder failed") || app.runtimeUI.key != "unsubmitted-key" {
			t.Fatal("failed reorder did not restore interaction and retain the draft")
		}
	default:
		t.Fatal("drag did not queue a command")
	}
	snapshot, _ := core.Snapshot(context.Background())
	if snapshot.CustomProviders[0].Provider.Models[0].ID != "first" || snapshot.CustomProviders[0].HasAPIKey {
		t.Fatal("failed reorder or dragging a handle saved unintended changes")
	}
	// Retry successfully, then save the outstanding draft against the latest
	// model order rather than restoring the pre-drag snapshot.
	from, _ = tt.Find("Model row first")
	to, _ = tt.Find("Model row second")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+to.H-2)
	tt.Release(to.X+to.W/2, to.Y+to.H-2)
	complete := func(command contracts.Command) {
		t.Helper()
		err := core.Execute(context.Background(), command)
		app.commandFinished(command, err)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, _ := core.Snapshot(context.Background())
		app.apply(snapshot)
		tt.Frame()
	}
	select {
	case command := <-app.jobs:
		complete(command)
	default:
		t.Fatal("failed reorder could not be retried")
	}
	if !app.saveInlineProvider() {
		t.Fatal("draft could not be saved after reordering")
	}
	select {
	case command := <-app.jobs:
		complete(command)
	default:
		t.Fatal("draft save did not queue")
	}
	snapshot, _ = core.Snapshot(context.Background())
	if snapshot.CustomProviders[0].Provider.Models[0].ID != "second" || !snapshot.CustomProviders[0].HasAPIKey {
		t.Fatal("saving a retained draft overwrote the reordered model list")
	}
}
