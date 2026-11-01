package desktop

import (
	"encoding/json"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestIndependentReasoningPopupGeometry(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			a := modelSurfaceTestApp(t, appearance)
			tt := ui.NewTester(a.View, 900, 600)
			panel, ok := tt.Find("Reasoning picker")
			if !ok || panel.X < 0 || panel.Y < 0 || panel.X+panel.W > 900 || panel.Y+panel.H > 600 {
				t.Fatalf("effort popup bounds %+v", panel)
			}
			if _, ok := tt.Find("Model picker"); ok {
				t.Fatal("effort popup contains model list")
			}
			if _, ok := tt.Find("Context window"); ok {
				t.Fatal("reasoning duplicated model options")
			}
			if _, ok := tt.Find("Reasoning effort"); !ok {
				t.Fatal("independent reasoning slider missing")
			}
			writeModelSurfaceEvidence(t, "effort-"+appearance, tt, map[string]any{"scope": "implementation layout", "popup": panel})
		})
	}
}

func TestModelTriggerOpenAndHoverWash(t *testing.T) {
	for _, tc := range []struct {
		appearance string
		idle, wash color.RGBA
	}{
		{"dark", color.RGBA{13, 13, 13, 255}, color.RGBA{40, 40, 40, 255}},
		{"light", color.RGBA{243, 243, 245, 255}, color.RGBA{229, 229, 231, 255}},
	} {
		t.Run(tc.appearance, func(t *testing.T) {
			a := modelSurfaceTestApp(t, tc.appearance)
			a.modelMenu, a.effortMenu = true, false
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("rcode-"+tc.appearance, tc.appearance == "dark")
				c.SetTheme(a.palette.UI())
				c.DisableAnimations()
				c.Root().Background(a.palette.Shell).Font("Geist").FontSize(13)
				design.ApplyReferenceTypography(c)
				ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() { a.centralControls().modelTrigger(c) })
			}, 840, 560)
			anchor, ok := tt.Find("Select model")
			if !ok {
				t.Fatal("production trigger missing")
			}
			x, y := int(anchor.X+3), int(anchor.Y+anchor.H/2)
			check := func(state string, want color.RGBA) {
				t.Helper()
				pixel := tt.Image().RGBAAt(x, y)
				// The open card's soft shadow can reach the neighboring chip.
				if absChannel(pixel.R, want.R) > 2 || absChannel(pixel.G, want.G) > 2 || absChannel(pixel.B, want.B) > 2 {
					t.Fatalf("%s trigger wash = %v, want %v", state, pixel, want)
				}
			}
			check("open without pointer", tc.wash)
			a.modelMenu = false
			tt.Frame()
			check("closed without pointer", tc.idle)
			tt.Move(float32(x), float32(y))
			tt.Advance(200 * time.Millisecond)
			check("closed with pointer", tc.wash)
			tt.Move(800, 40)
			tt.Advance(200 * time.Millisecond)
			check("pointer left", tc.idle)
		})
	}
}

func TestModelPopupRetainsTriggerTypographyAcrossOverlayRoot(t *testing.T) {
	a := modelSurfaceTestApp(t, "dark")
	var outsideWidth float32
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		c.Root().Font("Geist").FontSize(13).FontWeight(400)
		outsideWidth, _ = c.MeasureText(0, ui.Span{Text: "Outside", Font: "Geist", Size: 13, Weight: 400})
		ui.Row(c).Children(func() { ui.Text(c, "Outside").FontSize(13) })
		ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() { a.centralControls().reasoningTrigger(c) })
	}, 840, 560)
	box, ok := tt.Find("Outside")
	if !ok || math.Abs(float64(box.W-outsideWidth)) > 1 {
		t.Fatal("popup altered typography outside its overlay")
	}
	if _, ok := tt.Find("Reasoning picker"); !ok {
		t.Fatal("independent reasoning popup is missing")
	}
}

func absChannel(a, b uint8) int { return int(math.Abs(float64(a) - float64(b))) }

func modelSurfaceTestApp(t *testing.T, appearance string) *App {
	t.Helper()
	a := testApp(t, "default")
	a.state.Settings.Appearance.Surface = "opaque"
	a.state.Settings.Appearance.Mode = appearance
	a.state.Settings.Appearance.ThemeID = "rcode-" + appearance
	a.state.Settings.ModelPicker.ProviderID = "openai"
	a.state.Settings.ModelPicker.Selections = map[string]contracts.ModelSelection{"openai": {ModelID: "gpt-5.4", Reasoning: "High"}}
	for i := range a.state.Models {
		if a.state.Models[i].ID == "gpt-5.4" {
			a.state.Models[i].Options = []contracts.ModelOption{
				{ID: "serviceTier", Label: "Service tier", Choices: []string{"standard", "fast"}, DefaultChoice: "standard", ChoiceLabels: map[string]string{"standard": "Standard", "fast": "Fast"}},
				{ID: "contextWindow", Label: "Context window", Choices: []string{"standard", "1m"}, DefaultChoice: "standard", ChoiceLabels: map[string]string{"standard": "Standard", "1m": "1M tokens"}},
			}
		}
	}
	a = modelSnapshotApp(t, a.state)
	a.modelMenu, a.effortMenu = false, true
	return a
}

func imagePoint(x, y float32) [2]int {
	return [2]int{int(math.Round(float64(x))), int(math.Round(float64(y)))}
}

func writeModelSurfaceEvidence(t *testing.T, name string, tt *ui.Tester, evidence map[string]any) {
	t.Helper()
	dir := os.Getenv("RCODE_MODEL_SURFACE_EVIDENCE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, tt.Image()); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
