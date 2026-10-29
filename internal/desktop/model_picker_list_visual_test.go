package desktop

import (
	"fmt"
	"image/color"
	"math"
	"runtime"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestCompactModelSearchFontScalesWhileCustomLineHeightStaysFixed(t *testing.T) {
	for _, size := range []float32{13, 16, 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			a := modelListSurfaceTestApp(t, "dark")
			var expectedAdvance float32
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("zeron-dark", true)
				theme := a.palette.UI()
				theme.FontSize = 13 * size / 16
				c.SetTheme(theme)
				c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
				c.DisableAnimations()
				c.Root().Background(a.palette.Shell).Font("Geist").FontSize(theme.FontSize).TextColor(a.palette.Text)
				design.ApplyReferenceTypography(c)
				features := ""
				if runtime.GOOS == "windows" {
					features = "kern=0"
				}
				expectedAdvance, _ = c.MeasureText(0, ui.Span{Text: "MMMM", Font: "Geist", Size: 14 * size / 16, Weight: 500, Features: features})
				ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
					ui.Row(c).Height(48).FillWidth().AlignItems(ui.Center).Children(func() { a.centralControls().modelTrigger(c) })
				})
			}, 840, 560)
			tt.Advance(time.Second)
			search, ok := tt.Find("Search models")
			start, focused := tt.TextCaret()
			if !ok || !focused || search.H != 22.75 || start.H != 22.75 {
				t.Fatalf("ComposerTextElement's measured px line changed with UIFont %g: search=%+v caret=%+v", size, search, start)
			}
			tt.Type("MMMM")
			end, focused := tt.TextCaret()
			if !focused || a.modelQuery != "MMMM" || math.Abs(float64(end.X-start.X-expectedAdvance)) > 1 {
				t.Fatalf("native search did not use the source's scaled 14px font: UIFont=%g start=%+v end=%+v advance=%g query=%q", size, start, end, expectedAdvance, a.modelQuery)
			}
			writeModelSurfaceEvidence(t, fmt.Sprintf("search-uifont-%g", size), tt, map[string]any{"scope": "headless native editor regression; source ComposerTextElement uses px(configured_line_height), independently of its inherited rem line style", "uiFontSize": size, "search": search, "caretBeforeTyping": start, "caretAfterTyping": end, "expectedTypedAdvance": expectedAdvance})
		})
	}
}

func modelListSurfaceTestApp(t *testing.T, appearance string) *App {
	t.Helper()
	a := modelSurfaceTestApp(t, appearance, true)
	a.state.Providers = []contracts.Provider{{ID: "openai", Name: "OpenAI", Icon: "openai-mark"}}
	a.state.Models = []contracts.Model{
		{ID: "gpt-5.4", ProviderID: "openai", Label: "GPT-5.4", ReasoningLevels: []string{"Low", "Medium", "High", "Extra High"}},
		{ID: "gpt-5.3-codex", ProviderID: "openai", Label: "GPT-5.3 Codex"},
		{ID: "gpt-5.2", ProviderID: "openai", Label: "GPT-5.2"},
		{ID: "gpt-5.1-codex-mini", ProviderID: "openai", Label: "GPT-5.1 Codex Mini"},
	}
	a.state.Settings.ModelPicker.Favorites = nil
	a = modelSnapshotApp(t, a.state)
	a.modelMenu, a.modelListPage = true, true
	a.modelCursor = 0
	return a
}

func TestCompactModelListSourceMaterialAndNativeSearch(t *testing.T) {
	for _, tc := range []struct {
		appearance                      string
		band, selected, ring, tab, line color.RGBA
	}{
		{"dark", color.RGBA{43, 43, 46, 255}, color.RGBA{64, 64, 67, 255}, color.RGBA{81, 81, 84, 255}, color.RGBA{56, 56, 59, 255}, color.RGBA{56, 56, 59, 255}},
		{"light", color.RGBA{250, 250, 250, 255}, color.RGBA{237, 237, 237, 255}, color.RGBA{220, 220, 220, 255}, color.RGBA{235, 235, 235, 255}, color.RGBA{227, 227, 227, 255}},
	} {
		t.Run(tc.appearance, func(t *testing.T) {
			a := modelListSurfaceTestApp(t, tc.appearance)
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("zeron-"+tc.appearance, tc.appearance == "dark")
				c.SetTheme(a.palette.UI())
				c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
				c.DisableAnimations()
				c.Root().Background(a.palette.Shell).Font("Geist").FontSize(13).TextColor(a.palette.Text)
				design.ApplyReferenceTypography(c)
				ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
					ui.Row(c).Height(48).FillWidth().AlignItems(ui.Center).Children(func() { a.centralControls().modelTrigger(c) })
				})
				if c.Theme().TextMuted != a.palette.Muted {
					t.Fatal("the search placeholder's faint color must not alter the frame's muted role")
				}
			}, 840, 560)
			tt.Advance(time.Second)
			panel, panelOK := tt.Find("Model picker")
			star, starOK := tt.Find("Add GPT-5.4 to favorites")
			if !panelOK || !starOK || panel != (ui.Rect{X: 8, Y: 264, W: 256, H: 226}) || star != (ui.Rect{X: 231, Y: 353, W: 24, H: 24}) {
				t.Fatalf("source popup/list/favorite slot geometry changed: popup=%+v star=%+v", panel, star)
			}
			frame := tt.Image()
			for _, point := range []struct {
				name string
				x, y int
				want color.RGBA
			}{
				{"unselected band", 150, 399, tc.band}, {"bottom inset", 150, 486, tc.band},
				{"selected row", 150, 365, tc.selected}, {"selected top inset ring", 150, 349, tc.ring},
				{"viewed provider", 53, 289, tc.tab}, {"search hairline", 150, 344, tc.line},
			} {
				if got := frame.RGBAAt(point.x, point.y); got != point.want {
					t.Fatalf("%s pixel=%v, want independently sampled source %v", point.name, got, point.want)
				}
			}
			search, searchOK := tt.Find("Search models")
			if !searchOK || search.H != 22.75 || !tt.Focused("Search models") {
				t.Fatalf("source ComposerInput's own 22.75px line must remain a focused native editor: %+v", search)
			}
			// Peak placeholder ink identifies the faint role without depending on
			// glyph antialiasing or a caret blink phase.
			faint := color.RGBA{a.palette.Faint.R, a.palette.Faint.G, a.palette.Faint.B, a.palette.Faint.A}
			foundFaint := false
			for y := int(search.Y); y < int(search.Y+search.H); y++ {
				for x := int(search.X) + 3; x < int(search.X)+115; x++ {
					if frame.RGBAAt(x, y) == faint {
						foundFaint = true
					}
				}
			}
			if !foundFaint {
				t.Fatal("empty native search did not paint the source faint placeholder")
			}
			if runtime.GOOS == "windows" {
				// The original 1x Windows WGC image has this caret-excluded
				// placeholder extent. At 13px its last ink ends 8px earlier.
				// Other platform rasterizers have no independent native evidence.
				left, top, right, bottom := 163, 338, 0, 0
				for y := 310; y < 338; y++ {
					for x := 44; x < 163; x++ {
						p := frame.RGBAAt(x, y)
						if tc.appearance == "dark" && min(p.R, p.G, p.B) > 80 || tc.appearance == "light" && max(p.R, p.G, p.B) < 200 {
							left, top, right, bottom = min(left, x), min(top, y), max(right, x), max(bottom, y)
						}
					}
				}
				if left != 44 || top != 319 || right != 147 || bottom != 328 {
					t.Fatalf("native search's 14px faint placeholder extent=%v, want source [44 319 147 328]", []int{left, top, right, bottom})
				}
			}
			// Advance through the native blink cycle; do not replace its timing
			// or input-method caret geometry with a painted stand-in.
			tt.Advance(100 * time.Millisecond)
			caret, caretActive := tt.TextCaret()
			caretFrame := tt.Image()
			cx, cy := int(caret.X), int(caret.Y+caret.H/2)
			accent := color.RGBA{a.palette.Accent.R, a.palette.Accent.G, a.palette.Accent.B, a.palette.Accent.A}
			if !caretActive || caretFrame.RGBAAt(cx, cy) != accent || caretFrame.RGBAAt(cx+1, cy) != accent || caretFrame.RGBAAt(cx+2, cy) == accent {
				t.Fatalf("native search did not paint the source's 2 DIP caret: caret=%+v active=%v", caret, caretActive)
			}
			writeModelSurfaceEvidence(t, tc.appearance+"-model-list", tt, map[string]any{"scope": "headless implementation regression, not native WGC acceptance", "popup": panel, "selectedFavoriteSlot": star, "search": search, "band": tc.band, "selectedFill": tc.selected, "ring": tc.ring, "placeholder": faint})
			tt.Type("5.3")
			if a.modelQuery != "5.3" || !tt.HasText("GPT-5.3 Codex") || tt.HasText("GPT-5.2") {
				t.Fatal("the styled native search lost filtering")
			}
			tt.Key(0, ui.KeyEnter)
			if a.modelID != "gpt-5.3-codex" || a.modelListPage || !a.modelMenu {
				t.Fatal("native Enter must activate the filtered model and return to the compact panel")
			}
		})
	}
}
