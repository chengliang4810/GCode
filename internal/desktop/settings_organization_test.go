package desktop

import (
	"crypto/sha256"
	"image/color"
	"slices"
	"testing"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSettingsNavigationOrderAndDistinctIcons(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		app, _ := settingsTestApp(t)
		if runtime {
			app.state.Runtime = &contracts.RuntimeInfo{}
		}
		app.state.Capabilities.Appshots = true
		app.state.Capabilities.Cloud = true
		app.state.User.SignedIn = true
		sections := app.sections()
		if slices.Index(sections, "Privacy") != slices.Index(sections, "Usage statistics")+1 {
			t.Fatal("privacy must immediately follow usage statistics")
		}
		if slices.Index(sections, "Dynamic Island") != slices.Index(sections, "Shortcuts")-1 {
			t.Fatal("island must immediately precede shortcuts")
		}
		seen := map[[32]byte]string{}
		for _, section := range sections {
			name := settingsIcon(section)
			svg := assets.Icon(name)
			if svg == nil {
				svg = assets.Glyph(name)
			}
			if svg == nil {
				t.Fatalf("missing icon for %s: %s", section, name)
			}
			image := svg.Image(24, 24, color.Black)
			digest := sha256.Sum256(image.Pix)
			if previous, ok := seen[digest]; ok {
				t.Fatalf("%s and %s render the same icon", previous, section)
			}
			seen[digest] = section
		}
	}
}

func TestSettingsGeneralDataDirectoryLast(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "General"
	tt := ui.NewTester(app.View, 1320, 1800)
	directory, ok := tt.Find("Data directory")
	if !ok {
		t.Fatal("data directory missing")
	}
	naming, ok := tt.Find("Thread naming")
	if !ok {
		t.Fatal("thread naming missing")
	}
	if directory.Y <= naming.Y {
		t.Fatalf("directory must follow general settings: directory=%+v naming=%+v", directory, naming)
	}
}

func TestSettingsUsageMenuBarAndConversationGrouping(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.generalSettings(c) })
	}, 1200, 1400)
	if tt.HasText("Show tasks and usage in menu bar") {
		t.Fatal("menu bar preference is still in general settings")
	}
	compact, ok := tt.Find("Compact mode")
	if !ok {
		t.Fatal("compact mode absent")
	}
	compression, ok := tt.Find("Hierarchical context compression")
	if !ok {
		t.Fatal("compression absent")
	}
	escape, ok := tt.Find("Stop agent with Escape")
	if !ok {
		t.Fatal("escape preference absent")
	}
	if !(compact.Y < compression.Y && compression.Y < escape.Y) {
		t.Fatalf("conversation grouping is wrong: compact=%+v compression=%+v escape=%+v", compact, compression, escape)
	}
	tt = ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.appUsageSettings(c) })
	}, 1200, 1400)
	if !tt.HasText("Show tasks and usage in menu bar") {
		t.Fatal("menu bar preference is missing above usage content")
	}

	app.menuBar.Tab = "today"
	tt = ui.NewTester(app.menuPanelView, 360, 520)
	if err := tt.Click("Menu bar settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.route != "settings" || app.section != "Usage statistics" {
		t.Fatal("menu panel settings action did not follow relocated preference")
	}
}
