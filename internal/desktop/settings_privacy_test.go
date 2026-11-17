package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"gcode/internal/privacy"
	"gcode/third_party/magpie/redact"
	"github.com/egoist/mygo/ui"
)

func TestPrivacySettingsTogglePersists(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Privacy"
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.settingsView(c)
	}, 900, 900)
	if app.state.Settings.Privacy.Enabled || tester.HasText("Mask secrets") {
		t.Fatal("masking settings must start disabled")
	}
	if err := tester.Click("Toggle Mask sensitive information"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	snapshot, err := backend.Snapshot(t.Context())
	if err != nil || !snapshot.Settings.Privacy.Enabled || !snapshot.Settings.Privacy.Secrets || !tester.HasText("Mask secrets") {
		t.Fatalf("switch did not persist enabled policy: %+v %v", snapshot.Settings.Privacy, err)
	}
	if err := tester.Click("Toggle Mask sensitive information"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	snapshot, _ = backend.Snapshot(t.Context())
	if snapshot.Settings.Privacy.Enabled || tester.HasText("Mask secrets") {
		t.Fatal("disabling did not stop/hide masking")
	}
}

func TestPrivacyWordsSaveOnBlurAndActionSelectionPersists(t *testing.T) {
	app, backend := settingsTestApp(t)
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.privacyListInput(c, "test-words", "Sensitive words", app.state.Settings.Privacy.Words, func(values []string) {
			app.state.Settings.Privacy.Words = values
		})
		textButton(c, "Other")
	}, 900, 500)
	if err := tester.Click("Sensitive words"); err != nil {
		t.Fatal(err)
	}
	tester.Type("InternalProject")
	tester.Frame()
	snapshot, _ := backend.Snapshot(t.Context())
	if len(snapshot.Settings.Privacy.Words) != 0 {
		t.Fatal("words saved while editing")
	}
	if err := tester.Click("Other"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	snapshot, _ = backend.Snapshot(t.Context())
	if len(snapshot.Settings.Privacy.Words) != 1 || snapshot.Settings.Privacy.Words[0] != "InternalProject" {
		t.Fatal("words did not save on blur")
	}
	actions := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.privacyActions(c)
	}, 900, 800)
	if err := actions.Click("Choose API_KEY"); err != nil {
		t.Fatal(err)
	}
	if err := actions.Click("Block"); err != nil {
		t.Fatal(err)
	}
	actions.Frame()
	snapshot, _ = backend.Snapshot(t.Context())
	if snapshot.Settings.Privacy.Actions["API_KEY"] != "block" {
		t.Fatal("action selector lost its bound edit")
	}
}

func TestPrivacyActionsIncludeEveryBuiltInKind(t *testing.T) {
	app, _ := settingsTestApp(t)
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.privacyActions(c)
	}, 1200, 1800)

	for _, kind := range append([]string{"TERM", "CUSTOM"}, redact.BuiltInKinds()...) {
		if _, ok := tester.Find("Choose " + kind); !ok {
			t.Fatalf("privacy action selector is missing built-in kind %q", kind)
		}
	}
}

func TestPrivacySettingsStayWithinSharedControlBoundary(t *testing.T) {
	source, err := os.ReadFile("settings_privacy.go")
	if err != nil {
		t.Fatal(err)
	}
	violations, err := inspectSettingsControls("settings_privacy.go", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Errorf("%s: %s", violation.position, violation.message)
	}
}

func TestPrivacyCustomRuleCreateEditAndDelete(t *testing.T) {
	app, backend := settingsTestApp(t)
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.privacyRules(c)
	}, 900, 900)
	if err := tester.Click("Rule pattern"); err != nil {
		t.Fatal(err)
	}
	tester.Type("private-")
	if err := tester.Click("Add masking rule"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	snapshot, _ := backend.Snapshot(t.Context())
	if len(snapshot.Settings.Privacy.Rules) != 1 || snapshot.Settings.Privacy.Rules[0].Prefix != "private-" {
		t.Fatal("rule was not added")
	}
	if err := tester.Click("Edit masking rule"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Rule pattern"); err != nil {
		t.Fatal(err)
	}
	tester.Key(ui.Cmd, ui.KeyA)
	tester.Type("internal-")
	if err := tester.Click("Update masking rule"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	snapshot, _ = backend.Snapshot(t.Context())
	if len(snapshot.Settings.Privacy.Rules) != 1 || snapshot.Settings.Privacy.Rules[0].Prefix != "internal-" {
		t.Fatal("rule edit did not replace the existing rule")
	}
	if err := tester.Click("Delete masking rule"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	snapshot, _ = backend.Snapshot(t.Context())
	if len(snapshot.Settings.Privacy.Rules) != 0 {
		t.Fatal("rule was not deleted")
	}
}

func TestPrivacyDetailPanelsRender(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{600, 1320} {
			t.Run(fmt.Sprintf("dark=%v/width=%d", dark, width), func(t *testing.T) {
				app, _ := settingsTestApp(t)
				app.state.PrivacyModels = []privacy.ModelStatus{
					{Model: "astrlink-guard", Supported: true, Size: 38958270},
					{Model: "privacy-filter", Supported: true, Size: 1637572192},
				}
				app.palette = design.Palette(map[bool]string{true: "rcode-dark", false: "rcode-light"}[dark], dark)
				tester := ui.NewTester(func(c *ui.Context) {
					localize.Set(c, "zh-CN")
					c.SetTheme(app.palette.UI())
					c.Root().Font("Geist").Background(app.palette.Background).TextColor(app.palette.Text)
					ui.Scroll(c).Fill().Children(func() {
						app.privacyLocalModels(c)
						app.privacyRules(c)
						app.privacyActions(c)
					})
				}, width, 1000)
				for _, label := range []string{"Download model", "Rule kind", "Rule pattern"} {
					if !tester.HasText(i18n.In("zh-CN", label)) {
						// Inputs expose their label to Find rather than Texts.
						if _, ok := tester.Find(i18n.In("zh-CN", label)); !ok {
							t.Fatalf("missing detail panel control %q", label)
						}
					}
				}
				directory := os.Getenv("RCODE_PRIVACY_EVIDENCE")
				if directory != "" {
					if err := os.MkdirAll(directory, 0755); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(directory, fmt.Sprintf("privacy-details-dark-%v-%d.png", dark, width))
					file, err := os.Create(path)
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tester.Image())
					_ = file.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestPrivacySettingsRenderThemesAndNarrowWindows(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{600, 1320} {
			for _, language := range []string{"en", "zh-CN"} {
				t.Run(fmt.Sprintf("%s/dark=%v/width=%d", language, dark, width), func(t *testing.T) {
					app, _ := settingsTestApp(t)
					app.state.Settings.Privacy = privacy.Settings{Enabled: true, Secrets: true, Personal: true}
					app.section = "Privacy"
					app.palette = design.Palette(map[bool]string{true: "rcode-dark", false: "rcode-light"}[dark], dark)
					tester := ui.NewTester(func(c *ui.Context) {
						localize.Set(c, language)
						c.SetTheme(app.palette.UI())
						c.Root().Font("Geist").Background(app.palette.Background).TextColor(app.palette.Text)
						app.settingsView(c)
					}, width, 1000)
					for _, label := range []string{"Privacy", "Mask sensitive information", "Mask secrets", "Mask personal data"} {
						target := i18n.In(language, label)
						if label == "Privacy" {
							target = i18n.In(language, "Settings page heading") + ": " + target
						}
						bounds, ok := tester.Find(target)
						if !ok || bounds.X < -1 || bounds.X+bounds.W > float32(width)+1 {
							t.Fatalf("missing or clipped %q: %+v", label, bounds)
						}
					}
					directory := os.Getenv("RCODE_PRIVACY_EVIDENCE")
					if directory != "" {
						if err := os.MkdirAll(directory, 0755); err != nil {
							t.Fatal(err)
						}
						path := filepath.Join(directory, fmt.Sprintf("privacy-%s-dark-%v-%d.png", language, dark, width))
						file, err := os.Create(path)
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(file, tester.Image())
						closeErr := file.Close()
						if err != nil || closeErr != nil {
							t.Fatal(err, closeErr)
						}
					}
				})
			}
		}
	}
}
