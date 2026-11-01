package desktop

import (
	"context"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"

	"github.com/egoist/mygo/ui"
)

func TestAppearanceReasoningThemeSelectionPersists(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Appearance"
	app.palette = design.Palette("rcode-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 1000, 1400)
	if !tt.HasText("Reasoning theme") {
		t.Fatal("appearance page lost the reasoning theme row")
	}
	if err := tt.Click("Choose Reasoning theme"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Follow theme", "Purple", "Terracotta", "Deep blue", "Sky blue", "Aurora", "Silver", "Gold"} {
		if !tt.HasText(label) {
			t.Fatalf("reasoning theme option %q missing", label)
		}
	}
	if err := tt.Click("Aurora"); err != nil {
		t.Fatal(err)
	}
	if app.state.Settings.Appearance.EffortTheme != contracts.EffortThemeAurora {
		t.Fatalf("selection did not reach app state: %q", app.state.Settings.Appearance.EffortTheme)
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Appearance.EffortTheme != contracts.EffortThemeAurora {
		t.Fatalf("selection was not persisted: %q", snapshot.Settings.Appearance.EffortTheme)
	}
	tt.Frame()
	if !tt.HasText("Aurora") {
		t.Fatal("trigger did not show the selected preset")
	}
	if err := tt.Click("Choose Reasoning theme"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Follow theme"); err != nil {
		t.Fatal(err)
	}
	if app.state.Settings.Appearance.EffortTheme != contracts.EffortThemeAccent {
		t.Fatalf("follow-theme selection did not persist: %q", app.state.Settings.Appearance.EffortTheme)
	}
}

func TestAppearanceReasoningThemeChineseNarrowPage(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.state.Settings.General.Language = "zh-CN"
	app.OpenRoute("settings/Appearance")
	tt := ui.NewTester(app.View, 700, 1000)
	if !tt.HasText("思考强度主题") {
		t.Fatalf("Chinese appearance page lost the reasoning theme row: %v", tt.Texts())
	}
	if err := tt.Click("选择思考强度主题"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"跟随主题", "紫色", "陶土橙", "深蓝", "天蓝", "极光渐变", "银白", "金色"} {
		r, ok := tt.Find(label)
		if !ok || r.X < -1 || r.X+r.W > 701 {
			t.Fatalf("reasoning theme option %q overflows the narrow page: %+v", label, r)
		}
	}
	if err := tt.Click("极光渐变"); err != nil {
		t.Fatal(err)
	}
	if app.state.Settings.Appearance.EffortTheme != contracts.EffortThemeAurora {
		t.Fatalf("Chinese selection did not persist: %q", app.state.Settings.Appearance.EffortTheme)
	}
}

func TestEffortSwatchUsesSharedGeometryAndPreset(t *testing.T) {
	theme := design.Palette("rcode-dark", true)
	effort := design.EffortPaletteFor(contracts.EffortThemeAurora, theme)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.UI())
		c.DisableAnimations()
		c.Root().Background(theme.Background)
		ui.Row(c).Padding(20).Children(func() { controlEffortSwatch(c, effort).Label("Effort swatch") })
	}, 100, 60)
	box, ok := tt.Find("Effort swatch")
	s := design.Controls()
	if !ok || box.W != s.GradientSwatchWidth || box.H != s.GradientSwatchHeight {
		t.Fatalf("swatch left the shared geometry: %+v", box)
	}
	frame := tt.Image()
	for index, want := range effort.Colors {
		x := int(box.X + (float32(index)+.5)*box.W/3)
		got := frame.RGBAAt(x, int(box.Y+box.H/2))
		if absChannel(got.R, want.R) > 3 || absChannel(got.G, want.G) > 3 || absChannel(got.B, want.B) > 3 {
			t.Fatalf("swatch stop %d = %v, want %v", index, got, want)
		}
	}
}

func TestReasoningSliderPainterUsesSelectedEffortPalette(t *testing.T) {
	theme := design.Palette("rcode-dark", true)
	for _, preset := range []contracts.EffortTheme{contracts.EffortThemePurple, contracts.EffortThemeGold, contracts.EffortThemeAurora} {
		t.Run(string(preset), func(t *testing.T) {
			effort := design.EffortPaletteFor(preset, theme)
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.UI())
				c.DisableAnimations()
				c.Root().Background(theme.Background)
				modelEffortPlate(c, theme, effort, false).Absolute().Left(20).Top(20).Size(200, 28).Label("Effort plate")
			}, 240, 80)
			box, ok := tt.Find("Effort plate")
			if !ok || box != (ui.Rect{X: 20, Y: 20, W: 200, H: 28}) {
				t.Fatalf("the preset changed the plate's casting box: %+v", box)
			}
			got := tt.Image().RGBAAt(120, 34)
			want := effort.Colors[1]
			if absChannel(got.R, want.R) > 3 || absChannel(got.G, want.G) > 3 || absChannel(got.B, want.B) > 3 {
				t.Fatalf("plate middle = %v, want middle stop %v", got, want)
			}
		})
	}
}
