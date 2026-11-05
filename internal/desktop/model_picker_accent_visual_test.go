package desktop

import (
	"fmt"
	"image/color"
	"testing"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestModelAccentInsetOverlayUsesOuterPlateBounds(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, box := range []ui.Rect{{X: 20, Y: 20, W: 72, H: 28}, {X: 23.5, Y: 17.25, W: 186.625, H: 28}} {
			t.Run(fmt.Sprintf("%gx-%g", scale, box.W), func(t *testing.T) {
				var outer, inset ui.Handle
				var outerBox, insetBox ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					outerBox, insetBox = outer.Bounds(c), inset.Bounds(c)
					ui.Box(c).Absolute().Left(box.X).Top(box.Y).Size(box.W, box.H).Border(1, ui.Transparent).Bind(&outer).Children(func() {
						modelAccentInsetOverlay(c).Bind(&inset)
					})
				}, 240, 80)
				tt.SetScale(scale)
				tt.Frame()
				if outerBox != box || insetBox != outerBox {
					t.Fatalf("inset must cover the original border bounds without changing layout: outer=%+v inset=%+v want=%+v", outerBox, insetBox, box)
				}
			})
		}
	}
}

func TestModelAccentPlateSourceOuterRimAtTwoX(t *testing.T) {
	for _, tc := range []struct {
		appearance         string
		plane              ui.Color
		top, inner, bottom color.RGBA
	}{
		{"light", ui.RGB(255, 255, 255), color.RGBA{161, 150, 226, 255}, color.RGBA{127, 108, 239, 255}, color.RGBA{117, 101, 212, 255}},
		{"dark", ui.RGB(39, 39, 42), color.RGBA{170, 158, 248, 255}, color.RGBA{148, 133, 248, 255}, color.RGBA{153, 140, 245, 255}},
	} {
		t.Run(tc.appearance, func(t *testing.T) {
			palette := design.Palette("rcode-"+tc.appearance, tc.appearance == "dark")
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(palette.UI())
				c.DisableAnimations()
				c.Root().Background(tc.plane)
				modelAccentPlate(c, palette, 0).Absolute().Left(20).Top(20).Size(200, 28).Label("Accent plate")
			}, 240, 80)
			tt.SetScale(2)
			box, ok := tt.Find("Accent plate")
			if !ok || box != (ui.Rect{X: 20, Y: 20, W: 200, H: 28}) {
				t.Fatalf("the material changed its casting box: %+v", box)
			}
			frame := tt.Image()
			// These are source R7 native2x samples at x100 and y888/890/942.
			// Here the same points are 28 DIPs from this plate's left edge.
			// The 3-channel allowance covers source gradient quantization.
			var samples []map[string]any
			for _, point := range []struct {
				y    int
				want color.RGBA
			}{{40, tc.top}, {42, tc.inner}, {94, tc.bottom}} {
				got := frame.RGBAAt(96, point.y)
				if absChannel(got.R, point.want.R) > 3 || absChannel(got.G, point.want.G) > 3 || absChannel(got.B, point.want.B) > 3 {
					t.Fatalf("source outer rim at y%d=%v, want %v; inset must overlay the border rather than add a second inner ring", point.y, got, point.want)
				}
				samples = append(samples, map[string]any{"headlessDevicePx": [2]int{96, point.y}, "actual": got, "source": point.want})
			}
			writeModelSurfaceEvidence(t, "r8-accent-"+tc.appearance+"-2x", tt, map[string]any{"scope": "headless isolated production accent plate regression, not native WGC acceptance", "scale": 2, "plate": box, "samples": samples})
		})
	}
}

func TestModelAccentOverlayKeepsSliderHitFocusAndKeyboard(t *testing.T) {
	a := modelSurfaceTestApp(t, "light")
	tt := ui.NewTester(a.View, 840, 560)
	tt.SetScale(2)
	tt.Advance(20 * time.Millisecond)
	slider, ok := tt.Find("Reasoning effort")
	if !ok || slider.H != 36 || slider.W != 230 {
		t.Fatalf("production effort slider lost its source box: %+v", slider)
	}
	levels := a.centralControls().reasoningChoice().levels
	y := slider.Y + slider.H/2
	for i, want := range levels {
		x := slider.X + 22 + (slider.W-44)*float32(i)/float32(len(levels)-1)
		tt.ClickAt(x, y)
		current, present := tt.Find("Reasoning effort")
		if a.reasoning != want || !present || !nearRect(current, slider) || !tt.Focused("Reasoning effort") {
			t.Fatalf("paint overlay intercepted stop %d: reasoning=%s want=%s slider=%+v focused=%v", i, a.reasoning, want, current, tt.Focused("Reasoning effort"))
		}
	}
	tt.Press(slider.X+22, y)
	tt.Move(slider.X+slider.W-22, y)
	tt.Release(slider.X+slider.W-22, y)
	if a.reasoning != levels[len(levels)-1] {
		t.Fatal("paint overlay intercepted the native slider drag")
	}
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyRight)
	if a.reasoning != levels[1] {
		t.Fatal("paint overlay changed the native panel's effort keys")
	}
	if a.modelMenu || !a.effortMenu {
		t.Fatal("effort interaction opened the model list")
	}
}
