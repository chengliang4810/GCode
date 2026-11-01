package design

import (
	"image/color"
	"testing"
)

func TestWallpaperPaletteKeepsReadableAccentAndRestoresWithoutSample(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			base := ApplyAccent(Palette("rcode", dark), "blue")
			unchanged := ApplyWallpaperColors(base, color.NRGBA{})
			if unchanged.Accent != base.Accent || unchanged.Background != base.Background {
				t.Fatal("missing wallpaper changed selected theme")
			}
			for _, sample := range []color.NRGBA{{R: 250, G: 250, B: 240, A: 255}, {R: 4, G: 4, B: 8, A: 255}, {R: 220, G: 40, B: 30, A: 255}} {
				palette := ApplyWallpaperColors(base, sample)
				if contrast(palette.Accent, palette.Background) < 3 || contrast(palette.Accent, palette.AccentOn) < 4.5 {
					t.Fatalf("unreadable wallpaper accent: %+v", sample)
				}
				if palette.Danger != base.Danger || palette.Success != base.Success || palette.ANSI != base.ANSI {
					t.Fatal("wallpaper changed status or terminal colors")
				}
			}
		})
	}
}
