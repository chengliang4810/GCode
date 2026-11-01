package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The frosted surfaces the reference derives from its theme: the composer
// pill's tint, the floating card's tint, the sidebar's wash, and the
// hairline the pill carries instead of the plain border.
func TestFrostedSurfacesFollowTheFrozenRecipes(t *testing.T) {
	dark := Palette("rcode-dark", true)
	light := Palette("rcode-light", false)
	if Frost(dark, "theme", false) || Frost(dark, "opaque", true) {
		t.Fatal("frost must need both the preference and the window material")
	}
	if !Frost(dark, "theme", true) {
		t.Fatal("the dark theme recommends frost")
	}
	// composer_sidebar_tint: #090909 at the reference's 15% scene coverage,
	// solved over the shell glass the pill sits on.
	if got := ComposerSurface(dark, "theme", true); got != ui.RGBA(9, 9, 9, .15) {
		t.Fatalf("dark frosted pill tint=%+v, source recipe wants %+v", got, ui.RGBA(9, 9, 9, .15))
	}
	if got := ComposerSurface(dark, "opaque", true); got != dark.Input.Over(dark.Background) {
		t.Fatalf("opaque pill must flatten the input tint: %+v", got)
	}
	if got := ComposerSurface(light, "theme", true); got.A != 89 || got.R != 255 {
		t.Fatalf("light frosted pill keeps 35%% white, got %+v", got)
	}
	if got := ComposerBorder(dark, "theme", true); got != ui.Hex("#bdc7d1").Alpha(.09) {
		t.Fatalf("dark frosted hairline=%+v", got)
	}
	if got := ComposerBorder(dark, "opaque", true); got != dark.Border {
		t.Fatalf("opaque pill border=%+v", got)
	}
	// A floating card shares the dark pill's tint; light frost keeps the
	// reference's fixed 45% overlay.
	if got := FloatingSurface(dark, "theme", true); got != ComposerSurface(dark, "theme", true) {
		t.Fatalf("dark floating card=%+v, want the pill's tint", got)
	}
	if got := FloatingSurface(light, "theme", true); got != light.Overlay.Alpha(.45) {
		t.Fatalf("light floating card=%+v", got)
	}
	if got := FloatingSurface(dark, "opaque", true); got != dark.Input.Over(dark.Background) {
		t.Fatalf("opaque floating card must flatten the input tint: %+v", got)
	}
	if got := GlassOverlay(dark, "theme", true); got.A < 128 || got.A >= 255 {
		t.Fatalf("dark glass overlay must keep the scene visible, got %+v", got)
	}
	if got := SidebarTone(dark); got != ui.Hex("#ebebeb").Alpha(.05) {
		t.Fatalf("dark sidebar tone=%+v", got)
	}
	if got := SidebarTone(light); got != ui.Hex("#1a1a1a").Alpha(.05) {
		t.Fatalf("light sidebar tone=%+v", got)
	}
}

func TestPanelBackgroundFromFrozenSurfaceRecipes(t *testing.T) {
	for _, tc := range []struct {
		name, theme, surface string
		dark                 bool
		want                 ui.Color
	}{
		{"light glass", "rcode-light", "theme", false, ui.Color{R: 255, G: 255, B: 255, A: 102}},
		{"dark glass", "rcode-dark", "theme", true, ui.Color{R: 6, G: 6, B: 6, A: 102}},
		{"light opaque", "rcode-light", "opaque", false, ui.Color{R: 255, G: 255, B: 255, A: 255}},
		{"dark opaque", "rcode-dark", "opaque", true, ui.Color{R: 6, G: 6, B: 6, A: 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PanelBackground(Palette(tc.theme, tc.dark), tc.surface); got != tc.want {
				t.Fatalf("panel paint=%+v, source recipe wants %+v", got, tc.want)
			}
		})
	}
}
