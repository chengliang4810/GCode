package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"gcode/third_party/magpie/pii"
	"github.com/egoist/mygo/ui"
)

func TestSettingsModelPickerNamingAndPrivacy(t *testing.T) {
	t.Run("naming", func(t *testing.T) {
		app, backend := settingsTestApp(t)
		app.state.Providers = []contracts.Provider{{ID: "a", Name: "Provider A"}, {ID: "b", Name: "Provider B"}}
		app.state.Models = []contracts.Model{{ID: "a/one", ProviderID: "a", Label: "One"}, {ID: "b/two", ProviderID: "b", Label: "Two"}}
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(app.palette.UI())
			app.generalSettings(c)
		}, 650, 1100)
		for _, label := range []string{"Thread naming model", "Provider B", "Thread naming model b/two"} {
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		}
		snapshot, err := backend.Snapshot(t.Context())
		if err != nil || snapshot.Settings.General.ThreadNamingModel != "b/two" {
			t.Fatalf("naming model was not saved: %v", err)
		}
	})
	t.Run("privacy", func(t *testing.T) {
		app, backend := settingsTestApp(t)
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(app.palette.UI())
			app.privacyLocalModels(c)
		}, 650, 900)
		for _, label := range []string{"Choose Local privacy model", "Local privacy model " + pii.PrivacyFilter} {
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		}
		snapshot, err := backend.Snapshot(t.Context())
		if err != nil || snapshot.Settings.Privacy.ModelName != pii.PrivacyFilter {
			t.Fatalf("local privacy model was not saved: %v", err)
		}
	})
}
