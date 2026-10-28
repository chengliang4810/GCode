package desktop

import (
	"image"
	"image/color"
	"math"
	"testing"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

func TestBackgroundShaderMatchesSourceAlphaMask(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			source.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	bitmap := ui.NewBitmap(source)
	hero, composer := ui.Rect{W: 80, H: 64}, ui.Rect{X: 22, Y: 24, W: 36, H: 20}
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Size(80, 64).Draw(func(p *ui.Painter, r ui.Rect) {
			conversationBackgroundShader().Draw(p, r, shader.Options{Images: [4]*ui.Bitmap{bitmap}, Uniforms: map[string]any{"Fit": [4]float32{0, 0, 80, 64}, "Composer": [4]float32{22, 24, 36, 20}, "Opacity": float32(.84)}})
		})
	}, 80, 64)
	pixels := tt.Image()
	for y := 2; y < 62; y += 7 {
		for x := 2; x < 78; x += 7 {
			expected := float64(design.BackgroundCompositeAlpha(hero, composer, float32(x)+.5, float32(y)+.5, 1) * .84 * 255)
			if math.Abs(float64(pixels.RGBAAt(x, y).A)-expected) > 2 {
				t.Fatalf("source mask mismatch at %d,%d: %d != %g", x, y, pixels.RGBAAt(x, y).A, expected)
			}
		}
	}
}
