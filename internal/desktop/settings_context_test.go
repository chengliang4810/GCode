package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestHierarchicalContextSettingsSwitch(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{600, 900, 1320} {
			t.Run(fmt.Sprintf("dark-%t-width-%d", dark, width), func(t *testing.T) {
				app, backend := settingsTestApp(t)
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(app.palette.UI())
					app.generalSettings(c)
				}, width, 1400)
				if app.state.Settings.General.HierarchicalContext {
					t.Fatal("default enabled")
				}
				label := "Toggle Hierarchical context compression"
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				snapshot, err := backend.Snapshot(t.Context())
				if err != nil || !snapshot.Settings.General.HierarchicalContext {
					t.Fatal("enable not persisted", err)
				}
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				snapshot, err = backend.Snapshot(t.Context())
				if err != nil || snapshot.Settings.General.HierarchicalContext {
					t.Fatal("disable not persisted", err)
				}
				for _, text := range tt.Texts() {
					if bounds, ok := tt.Find(text); ok && bounds.W > 0 && (bounds.X < -1 || bounds.X+bounds.W > float32(width)+1) {
						t.Fatalf("overflow %q: %+v", text, bounds)
					}
				}
			})
		}
	}
}
