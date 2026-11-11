package desktop

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestSettingsControlsSwitchGeometryAcrossPages(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, fontSize := range []float32{12, 14, 20} {
			app, _ := runtimeSettingsTestApp(t)
			app.palette = design.Palette("rcode", dark)
			app.state.Settings.Appearance.ReduceMotion = true
			app.state.CustomProviders = []contracts.ProviderConfiguration{{Provider: model.Provider{
				ID: "shared-controls", Name: "Shared provider", Protocol: model.Chat,
				BaseURL: "https://example.com/v1", Models: []model.Model{{ID: "shared-model"}},
			}}}
			app.state.UserResources.MCPServers = []contracts.MCPServer{{
				ID: "shared-mcp", Name: "Shared MCP", Transport: "stdio", Command: "local-tool", Enabled: true,
			}}
			theme := app.palette.UI()
			theme.FontSize = fontSize
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme)
				c.Root().Font(theme.Font).FontSize(fontSize).Background(theme.Background).TextColor(theme.Text)
				app.settingsView(c)
			}, 1100, 1400)
			for _, page := range []struct{ section, label string }{
				{"General", "Toggle Compact mode"},
				{"General", "Toggle Hierarchical context compression"},
				{"Model settings", "Enable provider Shared provider"},
				{"MCP servers", "Enable Shared MCP"},
				{"Subagents", "Enable " + app.state.Settings.Subagents[0].Name},
			} {
				app.selectSettingsSection(page.section)
				tt.Frame()
				r, ok := tt.Find(page.label)
				s := design.Controls()
				if !ok || math.Abs(float64(r.W-s.SwitchWidth)) > .1 ||
					math.Abs(float64(r.H-s.SwitchHeight)) > .1 {
					t.Fatalf("%s switch differs from shared geometry (dark=%v font=%v): %+v",
						page.section, dark, fontSize, r)
				}
			}
		}
	}
}

func TestSettingsControlsKeyboardAndDisabledState(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.state.Settings.Appearance.ReduceMotion = true
	var checked, enabled, disabled bool
	var changes int
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
			controlCheckbox(c.Key("check"), &checked, "Shared check").Disabled(disabled)
			app.controlSwitch(c.Key("switch"), &enabled, "Shared switch", disabled, func(bool) { changes++ })
		})
	}, 500, 300)
	if err := tt.Click("Shared check"); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("shared checkbox lost pointer interaction")
	}
	tt.Key(0, ui.KeySpace)
	if checked {
		t.Fatal("shared checkbox lost keyboard interaction")
	}
	if err := tt.Click("Shared switch"); err != nil {
		t.Fatal(err)
	}
	if !enabled || changes != 1 {
		t.Fatal("shared switch did not publish its new value")
	}
	tt.Key(0, ui.KeySpace)
	if enabled || changes != 2 {
		t.Fatal("shared switch lost keyboard interaction")
	}
	disabled = true
	tt.Frame()
	if err := tt.Click("Shared switch"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Shared check"); err != nil {
		t.Fatal(err)
	}
	if enabled || checked || changes != 2 {
		t.Fatal("disabled shared controls changed state")
	}
}

func TestSettingsControlsCompactButtonGeometryAndInput(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, fontSize := range []float32{14, 20} {
			for _, width := range []int{240, 800} {
				theme := design.Palette("rcode", dark).UI()
				theme.FontSize = fontSize
				clicks, disabled := 0, false
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(theme)
					c.Root().Font(theme.Font).FontSize(fontSize)
					ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
						ui.Row(c).Gap(4).Children(func() {
							chipDismissButton(c.Key("chip"), "Dismiss chip").Disabled(disabled).OnClick(func() { clicks++ })
							attachmentDismissButton(c.Key("attachment"), "Dismiss attachment").Disabled(disabled).OnClick(func() { clicks++ })
							disclosureIconButton(c.Key("disclosure"), "alt-arrow-up", "Fold message").Disabled(disabled).OnClick(func() { clicks++ })
							inlineIconButton(c.Key("inline"), "pen", "Edit path").Disabled(disabled).OnClick(func() { clicks++ })
						})
						compactTextButton(c.Key("compact"), "Folder").AlignSelf(ui.Start).Label("Compact folder")
						textButton(c.Key("regular"), "Folder").AlignSelf(ui.Start).Label("Regular folder")
					})
				}, width, 240)
				s := design.Controls()
				for _, expected := range []struct {
					label string
					w, h  float32
				}{
					{"Dismiss chip", s.ChipDismissSize, s.ChipDismissSize},
					{"Dismiss attachment", s.AttachmentDismissSize, s.AttachmentDismissSize},
					{"Fold message", s.DisclosureWidth, s.DisclosureHeight},
					{"Edit path", s.InlineIconButtonSize, s.InlineIconButtonSize},
				} {
					r, ok := tt.Find(expected.label)
					if !ok || r.W != expected.w || r.H != expected.h || r.X+r.W > float32(width) {
						t.Fatalf("dark=%v font=%v width=%v %s geometry=%+v", dark, fontSize, width, expected.label, r)
					}
					before := clicks
					if err := tt.Click(expected.label); err != nil {
						t.Fatal(err)
					}
					tt.Key(0, ui.KeySpace)
					if clicks != before+2 {
						t.Fatalf("%s pointer/keyboard actions=%d", expected.label, clicks-before)
					}
				}
				compact, _ := tt.Find("Compact folder")
				regular, _ := tt.Find("Regular folder")
				if math.Abs(float64(regular.W-compact.W-2*(s.ButtonPaddingX-s.CompactButtonPaddingX))) > .1 || compact.H != regular.H {
					t.Fatalf("compact padding changed height or width: compact=%+v regular=%+v", compact, regular)
				}
				disabled = true
				tt.Frame()
				before := clicks
				if err := tt.Click("Dismiss chip"); err != nil {
					t.Fatal(err)
				}
				tt.Key(0, ui.KeySpace)
				if clicks != before {
					t.Fatal("disabled compact button handled input")
				}
			}
		}
	}
}

func TestSettingsControlsGallery(t *testing.T) {
	for _, dark := range []bool{false, true} {
		app, _ := runtimeSettingsTestApp(t)
		app.palette = design.Palette("rcode", dark)
		app.state.Settings.Appearance.ReduceMotion = true
		text, secret, selected := "Shared text input", "", "Choice"
		on, off, check := true, false, true
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(app.palette.UI())
			c.Root().Font("Geist").FontSize(design.UIFontSize).Background(app.palette.Background).TextColor(app.palette.Text)
			ui.Column(c).Fill().Padding(24).Gap(16).Children(func() {
				ui.Text(c, "Shared settings controls").FontSize(22).FontWeight(600)
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					textButton(c, "Default")
					primaryButton(c, "Primary")
					secondaryButton(c, "Secondary")
					dangerButton(c, "Remove")
					primaryButton(c, "Disabled").Disabled(true)
					iconButton(c, "refresh", "Refresh")
				})
				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					app.controlSwitch(c.Key("on"), &on, "Enabled switch", false, nil)
					app.controlSwitch(c.Key("off"), &off, "Off switch", false, nil)
					app.controlSwitch(c.Key("disabled"), &on, "Disabled switch", true, nil)
					controlCheckbox(c.Key("check"), &check, "Checkbox")
				})
				controlTextInput(c.Key("text"), &text).FillWidth().Label("Text field")
				controlTextInput(c.Key("disabled-input"), &text).FillWidth().Label("Disabled field").Disabled(true)
				controlSecretInput(c.Key("secret"), &secret, false, secret != "", func(input ui.Element) {
					input.Label("Secret field").Placeholder("API Key")
				}, func() {})
				sel := controlSelect(c.Key("select"), &selected)
				sel.Trigger.FillWidth().Label("Choice field").Children(func() {
					ui.Text(c, selected).Grow(1)
					icon(c, "alt-arrow-down", 13)
				})
				sel.Popup(func(panel ui.Element) {
					controlSelectPopup(c, panel).Children(func() {
						controlSelectItem(c, sel.Item("Choice")).Children(func() { ui.Text(c, "Choice") })
					})
				})
			})
		}, 900, 520)
		if err := tt.Click("Text field"); err != nil {
			t.Fatal(err)
		}
		input, _ := tt.Find("Text field")
		choice, _ := tt.Find("Choice field")
		iconRect, _ := tt.Find("Refresh")
		if input.H != choice.H || iconRect.W != design.Controls().IconButtonSize ||
			iconRect.H != design.Controls().IconButtonSize {
			t.Fatalf("shared geometry differs: input=%+v choice=%+v icon=%+v", input, choice, iconRect)
		}
		for _, label := range []string{"Default", "Primary", "Secondary", "Remove", "Disabled"} {
			r, ok := tt.Find(label)
			if !ok || math.Abs(float64(r.H-max(design.Controls().ButtonHeight, design.UIFontSize*1.5+design.Controls().ButtonPaddingY*2))) > .1 {
				t.Fatalf("%s button variant differs from the shared height: %+v", label, r)
			}
		}
		if directory := os.Getenv("RCODE_CONTROL_EVIDENCE"); directory != "" {
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			name := "controls-light.png"
			if dark {
				name = "controls-dark.png"
			}
			file, err := os.Create(filepath.Join(directory, name))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, tt.Image())
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("write control evidence: %v %v", err, closeErr)
			}
		}
	}
}

func TestSettingsControlsPrimaryUsesAccent(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, accent := range []string{"theme", "blue"} {
			app, _ := runtimeSettingsTestApp(t)
			app.palette = design.ApplyAccent(design.Palette("rcode", dark), accent)
			app.state.CustomProviders = []contracts.ProviderConfiguration{{Provider: model.Provider{
				ID: "accent-provider", Name: "Accent provider", Protocol: model.Chat,
			}}}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				c.Root().Font("Geist").FontSize(design.UIFontSize).Background(app.palette.Background).TextColor(app.palette.Text)
				ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.customProviderSettings(c) })
			}, 1000, 800)
			for _, label := range []string{"Add provider", "Add model"} {
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %s", label)
				}
				pixel := tt.Image().RGBAAt(int(r.X+5), int(r.Y+r.H/2))
				expected := app.palette.Accent
				if pixel.R != expected.R || pixel.G != expected.G || pixel.B != expected.B {
					t.Fatalf("%s must follow theme accent (dark=%v accent=%s): got=%v expected=%v",
						label, dark, accent, pixel, expected)
				}
			}
		}
	}
}

func TestSettingsControlsSliderReservesNativeTickHeight(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	for _, fontSize := range []float32{6.5, 13, 16.25} {
		value := float64(1)
		theme := app.palette.UI()
		theme.FontSize = fontSize
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme)
			ui.Column(c).Fill().Padding(16).Children(func() {
				controlStepSlider(c, &value, 1, 4, .1).Width(168).Label("Ticked slider")
			})
		}, 400, 160)
		r, ok := tt.Find("Ticked slider")
		if !ok || r.H < theme.Space(7) {
			t.Fatalf("font %v clips the native slider tick area: %+v", fontSize, r)
		}
	}
}
