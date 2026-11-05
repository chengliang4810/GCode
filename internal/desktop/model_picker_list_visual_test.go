package desktop

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestModelSearchFontAndLineHeightFollowPreference(t *testing.T) {
	for _, size := range []float32{13, 16, 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			a := modelListSurfaceTestApp(t, "dark")
			a.modelSearchOpen = true
			var expectedAdvance float32
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("rcode-dark", true)
				theme := a.palette.UI()
				theme.FontSize = size
				c.SetTheme(theme)
				c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
				c.DisableAnimations()
				c.Root().Background(a.palette.Shell).Font("Geist").FontSize(theme.FontSize).TextColor(a.palette.Text)
				design.ApplyReferenceTypography(c)
				features := ""
				if runtime.GOOS == "windows" {
					features = "kern=0"
				}
				expectedAdvance, _ = c.MeasureText(0, ui.Span{Text: "MMMM", Font: "Geist", Size: size, Weight: 500, Features: features})
				ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
					ui.Row(c).Height(48).FillWidth().AlignItems(ui.Center).Children(func() { a.centralControls().modelTrigger(c) })
				})
			}, 840, 560)
			tt.Advance(time.Second)
			search, ok := tt.Find("Search models")
			start, focused := tt.TextCaret()
			if !ok || !focused || math.Abs(float64(search.H-size*1.5)) > 1 || math.Abs(float64(start.H-size*1.5)) > 1 {
				t.Fatalf("ComposerTextElement's measured px line changed with UIFont %g: search=%+v caret=%+v", size, search, start)
			}
			tt.Type("MMMM")
			end, focused := tt.TextCaret()
			if !focused || a.modelQuery != "MMMM" || math.Abs(float64(end.X-start.X-expectedAdvance)) > 1 {
				t.Fatalf("native search did not use the selected pixel size: UIFont=%g start=%+v end=%+v advance=%g query=%q", size, start, end, expectedAdvance, a.modelQuery)
			}
			writeModelSurfaceEvidence(t, fmt.Sprintf("search-uifont-%g", size), tt, map[string]any{"scope": "headless native editor regression; selected pixel font size with relative line height", "uiFontSize": size, "search": search, "caretBeforeTyping": start, "caretAfterTyping": end, "expectedTypedAdvance": expectedAdvance})
		})
	}
}

func modelListSurfaceTestApp(t *testing.T, appearance string) *App {
	t.Helper()
	a := modelSurfaceTestApp(t, appearance)
	a.state.Providers = []contracts.Provider{{ID: "openai", Name: "OpenAI", Enabled: true}}
	a.state.Models = []contracts.Model{
		{ID: "gpt-5.4", ProviderID: "openai", Label: "GPT-5.4", ReasoningLevels: []string{"Low", "Medium", "High", "Extra High"}},
		{ID: "gpt-5.3-codex", ProviderID: "openai", Label: "GPT-5.3 Codex"},
		{ID: "gpt-5.2", ProviderID: "openai", Label: "GPT-5.2"},
		{ID: "gpt-5.1-codex-mini", ProviderID: "openai", Label: "GPT-5.1 Codex Mini"},
	}
	a = modelSnapshotApp(t, a.state)
	a.modelMenu, a.effortMenu = true, false
	a.modelCursor = 0
	return a
}

func TestModelPickerSupplierNamesAndSeparateCards(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			a := modelListSurfaceTestApp(t, appearance)
			a.state.Models[0].Label = "deepseek-v4.1-flash-long-model-name"
			tt := ui.NewTester(a.View, 900, 600)
			tt.Advance(time.Second)
			panel, panelOK := tt.Find("Model picker")
			settings, settingsOK := tt.Find("Model management")
			provider, providerOK := tt.Find("OpenAI")
			row, rowOK := tt.Find("GPT-5.3 Codex")
			if !panelOK || !settingsOK || !providerOK || !rowOK {
				t.Fatalf("missing picker controls: %v", tt.Texts())
			}
			trigger, triggerOK := tt.Find("Select model")
			if !triggerOK || provider.W+10 < trigger.W {
				t.Fatalf("supplier card is narrower than trigger: provider=%+v trigger=%+v", provider, trigger)
			}
			if settings.Y < provider.Y+provider.H || !(row.X > provider.X+provider.W || row.X+row.W < provider.X) || settings.Y+settings.H > panel.Y+panel.H {
				t.Fatalf("invalid supplier/model cards: panel=%+v management=%+v provider=%+v row=%+v", panel, settings, provider, row)
			}
			if !tt.HasText("OpenAI") {
				t.Fatal("supplier name must be visible")
			}
			if _, ok := tt.Find("Search models"); ok {
				t.Fatal("small catalog should remain a plain menu")
			}
			for _, text := range tt.Texts() {
				if strings.Contains(text, "Ctrl+") && text != paletteShortcut("Mod+N") {
					t.Fatalf("unwanted text %q", text)
				}
			}
			if _, ok := tt.Find("Favorites"); ok {
				t.Fatal("favorites remains")
			}
			if _, ok := tt.Find("Edit model GPT-5.2"); ok {
				t.Fatal("per-model settings button remains")
			}
			writeModelSurfaceEvidence(t, "supplier-cards-"+appearance, tt, map[string]any{"scope": "implementation layout regression; no pixel parity claim"})
			tt.Key(ui.Cmd, ui.KeyF)
			tt.Type("5.3")
			tt.Key(0, ui.KeyEnter)
			if a.modelID != "gpt-5.3-codex" || a.modelMenu {
				t.Fatal("search selection did not commit and dismiss")
			}
		})
	}
}

func TestModelPickerProviderScrollKeepsSettingsFixed(t *testing.T) {
	a := testApp(t, "default")
	for index := 0; index < 30; index++ {
		id := fmt.Sprintf("provider-%02d", index)
		a.state.Providers = append(a.state.Providers, contracts.Provider{ID: id, Name: id})
		a.state.Integrations.Providers = append(a.state.Integrations.Providers, contracts.ProviderDetails{ProviderID: id, TargetID: "local", Installed: true, Enabled: true})
		a.state.Models = append(a.state.Models, contracts.Model{ID: id + "/model", ProviderID: id, Label: id + " model"})
	}
	tt := ui.NewTester(a.View, 900, 600)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	settings, _ := tt.Find("Model management")
	panel, _ := tt.Find("Model picker")
	first, _ := tt.Find("provider-00")
	tt.Scroll(first.X+first.W/2, first.Y+first.H/2, 0, 2000)
	last, ok := tt.Find("provider-29")
	if !ok || last.Y < panel.Y || last.Y+last.H > settings.Y {
		t.Fatalf("last provider did not scroll into the middle rail: %+v", last)
	}
	if got, _ := tt.Find("Model management"); got != settings {
		t.Fatal("provider scroll moved settings entry")
	}
}

func TestSmallModelMenuKeyboardSelection(t *testing.T) {
	a := modelListSurfaceTestApp(t, "light")
	tt := ui.NewTester(a.View, 900, 600)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "gpt-5.3-codex" || a.modelMenu {
		t.Fatalf("plain menu keyboard selection failed: model=%s open=%v", a.modelID, a.modelMenu)
	}
}

func TestModelMenuTracksButtonAcrossComposerLayouts(t *testing.T) {
	for _, hero := range []bool{true, false} {
		for _, draft := range []string{"", strings.Repeat("A longer draft\n", 20)} {
			a := modelListSurfaceTestApp(t, "light")
			a.state.Models = a.state.Models[:1]
			a.draft = draft
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				ui.Column(c).Fill().Padding(20).Justify(ui.End).Children(func() {
					a.nativeComposerSurface(c, composerSurfaceProps{
						draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
						controls: a.centralControls(), hero: hero, widthHint: 736,
						inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
						onSend: func(string) bool { return false }, onChooseAttach: func() {},
					})
				})
			}, 900, 700)
			tt.Advance(time.Second)
			panel, ok := tt.Find("Model picker")
			provider, _ := tt.Find("OpenAI")
			model, _ := tt.Find("Select model GPT-5.4")
			trigger, triggerOK := tt.Find("Select model")
			if !ok || !triggerOK || math.Abs(float64(trigger.Y-panel.Y-panel.H-6)) > 1 ||
				math.Abs(float64(model.Y-provider.Y)) > 1 {
				t.Fatalf("hero=%t draft=%d: menu=%+v trigger=%+v provider=%+v model=%+v", hero, len(draft), panel, trigger, provider, model)
			}
		}
	}
}

func TestModelSubmenuUsesAvailableSideAndKeepsSupplierCentered(t *testing.T) {
	for _, x := range []float32{380, 900} {
		a := modelListSurfaceTestApp(t, "light")
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(a.palette.UI())
			ui.Row(c).Absolute().Left(x).Top(450).Children(func() {
				a.centralControls().modelTrigger(c)
			})
		}, 1100, 600)
		tt.Advance(time.Second)
		panel, _ := tt.Find("Model picker")
		trigger, _ := tt.Find("Select model")
		provider, _ := tt.Find("OpenAI")
		model, _ := tt.Find("Select model GPT-5.4")
		providerCenter := provider.X + provider.W/2
		providerWidth := provider.W + 8 // the card's 4 DIP padding on each side
		wantLeft := trigger.X+trigger.W/2+panel.W-providerWidth/2 > 1100-8
		if (model.X < provider.X) != wantLeft {
			t.Fatalf("wrong submenu side at x=%g: provider=%+v model=%+v", x, provider, model)
		}
		if math.Abs(float64(providerCenter-trigger.X-trigger.W/2)) > 1 {
			t.Fatalf("supplier menu is not centered on trigger at x=%g: panel=%+v trigger=%+v", x, panel, trigger)
		}
	}
}

func TestModelSubmenuHeightDoesNotMoveProviders(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, x := range []float32{40, 680} {
			t.Run(fmt.Sprintf("%s/x-%g", appearance, x), func(t *testing.T) {
				a := modelListSurfaceTestApp(t, appearance)
				a.state.Providers = nil
				a.state.Models = nil
				for index, count := range []int{1, 2, 20} {
					id := fmt.Sprintf("provider-%d", index)
					a.state.Providers = append(a.state.Providers, contracts.Provider{ID: id, Name: id, Enabled: true})
					a.state.Integrations.Providers = append(a.state.Integrations.Providers, contracts.ProviderDetails{ProviderID: id, TargetID: "local", Installed: true, Enabled: true})
					for model := 0; model < count; model++ {
						name := fmt.Sprintf("%s/model-%02d", id, model)
						a.state.Models = append(a.state.Models, contracts.Model{ID: name, ProviderID: id, Label: name})
					}
				}
				a = modelSnapshotApp(t, a.state)
				a.modelID = "provider-0/model-00"
				a.modelMenu = true
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(a.palette.UI())
					c.DisableAnimations()
					c.Root().Background(a.palette.Shell)
					ui.Row(c).Absolute().Left(x).Top(300).Children(func() {
						a.centralControls().modelTrigger(c)
					})
				}, 900, 380)
				tt.Advance(time.Second)
				first, _ := tt.Find("provider-0")
				settings, _ := tt.Find("Model management")
				for _, index := range []int{2, 1, 0, 2} {
					label := fmt.Sprintf("provider-%d", index)
					row, ok := tt.Find(label)
					if !ok {
						t.Fatalf("missing provider %s", label)
					}
					tt.Move(row.X+row.W/2, row.Y+row.H/2)
					tt.Advance(100 * time.Millisecond)
					if got, _ := tt.Find("provider-0"); got != first {
						t.Fatalf("browsing %s moved providers: before=%+v after=%+v", label, first, got)
					}
					if got, _ := tt.Find("Model management"); got != settings {
						t.Fatalf("browsing %s moved management: before=%+v after=%+v", label, settings, got)
					}
					model, ok := tt.Find("Select model " + label + "/model-00")
					trigger, _ := tt.Find("Select model")
					if !ok || model.Y < 8 || model.Y+model.H > trigger.Y-6 || !(model.X > row.X+row.W || model.X+model.W < row.X) {
						t.Fatalf("invalid independent submenu: model=%+v provider=%+v trigger=%+v", model, row, trigger)
					}
					if index == 1 && math.Abs(float64(model.Y-row.Y)) > 1 {
						t.Fatalf("submenu did not align to browsed provider: model=%+v provider=%+v", model, row)
					}
					if index == 2 {
						writeModelSurfaceEvidence(t, fmt.Sprintf("independent-submenu-%s-x-%g", appearance, x), tt, map[string]any{"provider": first, "settings": settings, "firstModel": model, "trigger": trigger})
					}
				}
				tt.Key(0, ui.KeyTab)
				tt.Key(0, ui.KeyEnd)
				tt.Key(0, ui.KeyEnter)
				if a.modelID != "provider-2/model-19" || a.modelMenu {
					t.Fatal("scrolled submenu keyboard selection failed")
				}
				a.modelMenu = true
				a.modelList.ScrollIntoView(0)
				tt.Frame()
				tt.Advance(100 * time.Millisecond)
				model, ok := tt.Find("Select model provider-2/model-00")
				panel, _ := tt.Find("Model picker")
				if !ok || model.Y >= panel.Y {
					t.Fatalf("fixture must exercise the submenu outside the provider overlay bounds: model=%+v overlay=%+v", model, panel)
				}
				tt.Move(model.X+model.W/2, model.Y+model.H/2)
				tt.ClickAt(model.X+model.W/2, model.Y+model.H/2)
				if a.modelID != "provider-2/model-00" || a.modelMenu {
					t.Fatal("pointer click outside the provider overlay bounds did not select the submenu model")
				}
			})
		}
	}
}
