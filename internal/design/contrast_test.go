package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A semantic color is tuned for marks, where 3:1 is enough. When it carries
// text it has to stay readable on every surface a page reads on, and the
// correction must reach 4.5:1 without changing the hue's role.
func TestReadableTonesReachTextContrastInEveryTheme(t *testing.T) {
	for _, variant := range Themes() {
		palette := Palette(variant.ID, variant.Appearance == "dark")
		for name, role := range map[string]ui.Color{
			"danger": palette.Danger, "warning": palette.Warning,
			"success": palette.Success, "accent": palette.Accent,
		} {
			readable := palette.Readable(role)
			for surfaceName, surface := range map[string]ui.Color{
				"background": palette.Background, "card": palette.Card, "dialog": palette.Dialog,
			} {
				if got := contrast(readable, surface); got < 4.5 {
					t.Errorf("%s %s on %s: %g, want at least 4.5", variant.ID, name, surfaceName, got)
				}
			}
			// The correction may only move the tone toward black or white; the
			// role itself never changes.
			if readable == role {
				continue
			}
			if distance(readable, role) > distance(ui.Hex("#ffffff"), role) && distance(readable, role) > distance(ui.Hex("#000000"), role) {
				t.Errorf("%s %s was corrected away from its hue", variant.ID, name)
			}
		}
	}
}

// distance is how far a color sits from another, channel by channel.
func distance(a, b ui.Color) int {
	sum := 0
	for _, pair := range [][2]int{{int(a.R), int(b.R)}, {int(a.G), int(b.G)}, {int(a.B), int(b.B)}} {
		sum += abs(pair[0] - pair[1])
	}
	return sum
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
