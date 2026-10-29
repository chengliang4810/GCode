package desktop

import (
	"encoding/json"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestModelPopupSourceGeometryAndMaterial(t *testing.T) {
	for _, appearance := range []string{"dark", "light"} {
		for _, compact := range []bool{true, false} {
			name := appearance + "-compact"
			if !compact {
				name = appearance + "-standard"
			}
			t.Run(name, func(t *testing.T) {
				a := modelSurfaceTestApp(t, appearance, compact)
				tt := ui.NewTester(func(c *ui.Context) {
					a.palette = design.Palette("zeron-"+appearance, appearance == "dark")
					c.SetTheme(a.palette.UI())
					c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
					c.DisableAnimations()
					c.Root().Background(a.palette.Shell).Font("Geist").FontSize(13).TextColor(a.palette.Text)
					design.ApplyReferenceTypography(c)
					ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
						ui.Row(c).FillWidth().Height(48).AlignItems(ui.Center).Children(func() { a.centralControls().modelTrigger(c) })
					})
				}, 840, 560)
				tt.Advance(time.Second)
				panel, ok := tt.Find("Model picker")
				if !ok {
					t.Fatal("production popup missing")
				}
				width := float32(256)
				if !compact {
					width = 304
				}
				if panel.X != 8 || panel.W != width {
					t.Fatalf("source end alignment must snap to 8px without flipping: %+v", panel)
				}
				row, ok := tt.Find("Context window")
				if !ok {
					t.Fatal("flat option row missing")
				}
				wantRowHeight := float32(30)
				if compact {
					wantRowHeight = 26
				}
				if row.H != wantRowHeight || row.X != panel.X+5 || row.W != panel.W-10 {
					t.Fatalf("option row does not use the source card inset: panel=%+v row=%+v", panel, row)
				}
				wantColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
				if appearance == "dark" {
					wantColor = color.RGBA{R: 39, G: 39, B: 42, A: 255}
				}
				point := imagePoint(panel.X+120, panel.Y+18)
				painted := tt.Image().RGBAAt(point[0], point[1])
				if painted != wantColor {
					t.Fatalf("popup painted the wrong plane: %v, want %v", painted, wantColor)
				}
				evidence := map[string]any{"renderer": "headless MyGo implementation evidence; not a reference golden", "appearance": appearance, "compact": compact, "popup": panel, "optionRow": row, "popupColor": painted}
				if compact {
					slider, ok := tt.Find("Reasoning effort")
					if !ok || panel.H != 134 || slider.X != panel.X+13 || slider.Y != panel.Y+55 || slider.W != 230 || slider.H != 36 || row.Y != panel.Y+103 {
						t.Fatalf("source panel/effort/option geometry differs: panel=%+v slider=%+v row=%+v", panel, slider, row)
					}
					if runtime.GOOS == "windows" {
						value, ok := tt.Find("Standard")
						if !ok || value != (ui.Rect{X: 171, Y: 461, W: 58, H: 21}) {
							t.Fatalf("option value differs from the native GPUI inspector's 1x bounds: %+v", value)
						}
						evidence["optionValue"] = value
					}
					railTop := tt.Image().RGBAAt(int(slider.X+210), int(slider.Y+8))
					railBottom := tt.Image().RGBAAt(int(slider.X+210), int(slider.Y+26))
					if appearance == "dark" && railTop.R <= railBottom.R || appearance == "light" && railTop.R >= railBottom.R {
						t.Fatalf("rail lost the source vertical lighting: top=%v bottom=%v", railTop, railBottom)
					}
					fillTop := tt.Image().RGBAAt(int(slider.X+50), int(slider.Y+8))
					fillBottom := tt.Image().RGBAAt(int(slider.X+50), int(slider.Y+26))
					if fillTop.R <= fillBottom.R {
						t.Fatalf("accent plate is not lit from above: top=%v bottom=%v", fillTop, fillBottom)
					}
					var stops []map[string]any
					for _, index := range []int{0, 1, 3} {
						x := slider.X + 22 + (slider.W-44)*float32(index)/3
						pixel := tt.Image().RGBAAt(int(x), int(slider.Y+18))
						neighbor := tt.Image().RGBAAt(int(x+5), int(slider.Y+18))
						brighter := index < 3 || appearance == "dark"
						if brighter && pixel.R <= neighbor.R || !brighter && pixel.R >= neighbor.R {
							t.Fatalf("source stop %d is missing from the 22px-inset ladder: center=%v nearby=%v", index, pixel, neighbor)
						}
						stops = append(stops, map[string]any{"center": [2]float32{x, slider.Y + 18}, "rgba": pixel})
					}
					evidence["slider"], evidence["visibleStops"] = slider, stops
					evidence["railTop"], evidence["railBottom"], evidence["fillTop"], evidence["fillBottom"] = railTop, railBottom, fillTop, fillBottom
				}
				t.Logf("source geometry/material evidence: %v", evidence)
				writeModelSurfaceEvidence(t, name, tt, evidence)
			})
		}
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
			a := modelSurfaceTestApp(t, tc.appearance, true)
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("zeron-"+tc.appearance, tc.appearance == "dark")
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
	for _, compact := range []bool{true, false} {
		name := "standard"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			a := modelSurfaceTestApp(t, "dark", compact)
			var mediumValue, mediumModel, normalOutside float32
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("zeron-dark", true)
				c.SetTheme(a.palette.UI())
				c.DisableAnimations()
				c.Root().Background(a.palette.Shell).Font("Geist").FontSize(13).FontWeight(400)
				design.ApplyReferenceTypography(c)
				features := ""
				if runtime.GOOS == "windows" {
					features = "kern=0"
				}
				mediumValue, _ = c.MeasureText(0, ui.Span{Text: "Standard", Font: "Geist", Size: 13, Weight: 500, Features: features})
				if runtime.GOOS == "windows" {
					mediumValue = float32(math.Ceil(float64(mediumValue)))
				}
				mediumModel, _ = c.MeasureText(0, ui.Span{Text: "GPT-5.4", Font: "Geist", Size: 12, Weight: 500, Features: features})
				normalOutside, _ = c.MeasureText(0, ui.Span{Text: "Outside", Font: "Geist", Size: 13, Weight: 400, Features: features})
				ui.Row(c).Children(func() { ui.Text(c, "Outside").FontSize(13) })
				ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
					anchor := ui.ButtonBase(c).Size(114, 32).Label("Anchor")
					a.centralControls().modelPopover(c, anchor)
				})
			}, 840, 560)
			for _, tc := range []struct {
				text string
				want float32
			}{{"Standard", mediumValue}, {"Outside", normalOutside}} {
				box, ok := tt.Find(tc.text)
				if !ok || math.Abs(float64(box.W-tc.want)) > .01 {
					t.Fatalf("%s text width = %+v, want %.4f; overlay weight must be scoped to its inherited medium text", tc.text, box, tc.want)
				}
			}
			if compact {
				box, ok := tt.Find("GPT-5.4")
				if !ok || math.Abs(float64(box.W-mediumModel)) > .01 {
					t.Fatalf("compact model subtitle lost the inherited medium weight: %+v, want width %.4f", box, mediumModel)
				}
			}
		})
	}
}

func absChannel(a, b uint8) int { return int(math.Abs(float64(a) - float64(b))) }

func modelSurfaceTestApp(t *testing.T, appearance string, compact bool) *App {
	t.Helper()
	a := testApp(t, "default")
	a.state.Settings.Appearance.Surface = "opaque"
	a.state.Settings.Appearance.Mode = appearance
	a.state.Settings.Appearance.ThemeID = "zeron-" + appearance
	a.state.Settings.General.CompactModelPicker = compact
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
	a.modelMenu = true
	return a
}

func imagePoint(x, y float32) [2]int {
	return [2]int{int(math.Round(float64(x))), int(math.Round(float64(y)))}
}

func writeModelSurfaceEvidence(t *testing.T, name string, tt *ui.Tester, evidence map[string]any) {
	t.Helper()
	dir := os.Getenv("ZERON_MODEL_SURFACE_EVIDENCE")
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
