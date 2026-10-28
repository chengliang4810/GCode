package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPanelBackgroundFromFrozenSurfaceRecipes(t *testing.T) {
	for _, tc := range []struct {
		name, theme, surface string
		dark                 bool
		want                 ui.Color
	}{
		{"light glass", "zeron-light", "theme", false, ui.Color{R: 255, G: 255, B: 255, A: 102}},
		{"dark glass", "zeron-dark", "theme", true, ui.Color{R: 6, G: 6, B: 6, A: 102}},
		{"light opaque", "zeron-light", "opaque", false, ui.Color{R: 255, G: 255, B: 255, A: 255}},
		{"dark opaque", "zeron-dark", "opaque", true, ui.Color{R: 6, G: 6, B: 6, A: 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PanelBackground(Palette(tc.theme, tc.dark), tc.surface); got != tc.want {
				t.Fatalf("panel paint=%+v, source recipe wants %+v", got, tc.want)
			}
		})
	}
}
