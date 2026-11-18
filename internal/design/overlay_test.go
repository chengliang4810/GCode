package design

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// OverlayShadow lifts a whole surface: the wrapper that casts it shows the
// shadow outside its own box, and the clipped card inside it does not stop it.
// That is what the floating surfaces of the interface rely on, so a wrapper
// that clips its own content must never be what casts the overlay shadow.
func TestOverlayShadowPaintsOutsideTheSurfaceItLifts(t *testing.T) {
	const box = 120
	const left, top = 40, 40
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Hex("#ffffff"))
		wrapper := OverlayShadow(ui.Box(c).Absolute().Left(left).Top(top).Width(box).Height(80).Radius(12))
		wrapper.Children(func() {
			ui.Box(c).Fill().Radius(12).Clip().Background(ui.Hex("#ffffff"))
		})
	}, 320, 240)
	image := tt.Image()
	at := func(x, y int) uint8 { return image.RGBAAt(x, y).R }
	if far := at(300, 220); far != 255 {
		t.Fatalf("the page behind the surface is not plain: %d", far)
	}
	// The shadow is offset by 8 DIPs, so it reaches furthest below the surface,
	// past the card's own bottom edge.
	if below := at(left+box/2, top+80+6); below > 250 {
		t.Fatalf("no overlay shadow below the surface: pixel %d, page 255", below)
	}
	// The card itself stays opaque: the shadow shows around it, not through it.
	if inside := at(left+box/2, top+40); inside != 255 {
		t.Fatalf("the overlay shadow shows through the card: pixel %d", inside)
	}
}

// OverlayShadow is the whole floating-surface shadow language, so the surfaces
// that use it take it from here. A desktop file that also clips the element it
// shadows brings back the cut the overlay shadow had: the audit's GEO-1.
func TestDesktopOverlaySurfacesTakeTheirShadowFromHere(t *testing.T) {
	for _, path := range []string{"../desktop/model_picker.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "Clip()") && strings.Contains(line, "Shadow(") {
				t.Errorf("%s:%d: one element clips its content and casts a shadow: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
