package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAccentOverlayPreservesThemeOwnedRoles(t *testing.T) {
	for _, variant := range Themes() {
		for _, selection := range []string{"zeron", "orange", "amber", "green", "cyan", "blue", "pink"} {
			t.Run(variant.ID+"/"+selection, func(t *testing.T) {
				original := Palette(variant.ID, variant.Appearance == "dark")
				changed := ApplyAccent(original, selection)
				if contrast(changed.Accent, changed.Background) < 3 || contrast(changed.AccentOn, changed.AccentStrong) < 4.5 {
					t.Fatal("source contrast hardening failed")
				}
				if changed.ANSI != original.ANSI || changed.DiffAdd != original.DiffAdd || changed.DiffDelete != original.DiffDelete || changed.Success != original.Success {
					t.Fatal("accent recolored a theme-owned semantic role")
				}
				for key, color := range original.Syntax {
					if changed.Syntax[key] != color {
						t.Fatal("syntax changed")
					}
				}
			})
		}
	}
}

func TestOriginalAccentPresetPairs(t *testing.T) {
	if ApplyAccent(Palette("zeron-dark", true), "orange").Accent != ui.Hex("#fb923c") {
		t.Fatal("wrong dark preset")
	}
	if ApplyAccent(Palette("zeron-light", false), "orange").Accent != ui.Hex("#c2410c") {
		t.Fatal("wrong light preset")
	}
	original := Palette("dracula", true)
	if ApplyAccent(original, "theme").Accent != original.Accent {
		t.Fatal("theme default accent changed")
	}
}
