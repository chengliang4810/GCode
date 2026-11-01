package design

import (
	"testing"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

func TestEffortThemesKeepTheirSourcePalette(t *testing.T) {
	theme := Palette("rcode-dark", true)
	cases := []struct {
		theme  contracts.EffortTheme
		colors [3]ui.Color
		light  bool
	}{
		{contracts.EffortThemeAccent, [3]ui.Color{theme.AccentStrong, theme.Accent, theme.AccentStrong}, false},
		{contracts.EffortThemePurple, [3]ui.Color{ui.Hex("#8050ed"), ui.Hex("#9b6df5"), ui.Hex("#7143da")}, false},
		{contracts.EffortThemeTerracotta, [3]ui.Color{ui.Hex("#b95a3e"), ui.Hex("#d7805e"), ui.Hex("#ca6947")}, false},
		{contracts.EffortThemeDeepBlue, [3]ui.Color{ui.Hex("#3152d9"), ui.Hex("#637dfa"), ui.Hex("#394bcb")}, false},
		{contracts.EffortThemeSkyBlue, [3]ui.Color{ui.Hex("#006bd9"), ui.Hex("#168df0"), ui.Hex("#3aa4f2")}, false},
		{contracts.EffortThemeAurora, [3]ui.Color{ui.Hex("#4285f4"), ui.Hex("#9b72cc"), ui.Hex("#d96670")}, false},
		{contracts.EffortThemeSilver, [3]ui.Color{ui.Hex("#ffffff"), ui.Hex("#e8e8ef"), ui.Hex("#c9ccd8")}, true},
		{contracts.EffortThemeGold, [3]ui.Color{ui.Hex("#c78c18"), ui.Hex("#f1bc45"), ui.Hex("#e3a324")}, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.theme), func(t *testing.T) {
			got := EffortPaletteFor(tc.theme, theme)
			if got.Colors != tc.colors {
				t.Fatalf("palette = %v, want %v", got.Colors, tc.colors)
			}
			if got.Light != tc.light {
				t.Fatalf("light spark = %v, want %v", got.Light, tc.light)
			}
			if got.Spark() != map[bool]ui.Color{true: ui.Hex("#000000"), false: ui.Hex("#ffffff")}[tc.light] {
				t.Fatal("spark does not contrast with the preset")
			}
		})
	}
}

func TestEffortPaletteSamplesThreeStopsAndTracksTheAccent(t *testing.T) {
	palette := EffortPaletteFor(contracts.EffortThemeAurora, Palette("rcode-dark", true))
	first, middle, last := palette.Colors[0], palette.Colors[1], palette.Colors[2]
	if palette.ColorAt(-1) != first || palette.ColorAt(0) != first || palette.ColorAt(.5) != middle || palette.ColorAt(1) != last || palette.ColorAt(2) != last {
		t.Fatal("gradient sampling left its three stops")
	}
	if palette.ColorAt(.25) == first || palette.ColorAt(.75) == last {
		t.Fatal("gradient steps instead of mixing between stops")
	}

	theme := Palette("rcode-dark", true)
	recolored := theme
	recolored.Accent, recolored.AccentStrong = ui.Hex("#ff0000"), ui.Hex("#ff0000")
	if EffortPaletteFor(contracts.EffortThemePurple, theme).Colors != EffortPaletteFor(contracts.EffortThemePurple, recolored).Colors {
		t.Fatal("a fixed preset followed the theme accent")
	}
	if EffortPaletteFor(contracts.EffortThemeAccent, theme).Colors == EffortPaletteFor(contracts.EffortThemeAccent, recolored).Colors {
		t.Fatal("follow theme stopped tracking the accent")
	}
}

func TestEffortThemeValuesAndLabelsAreComplete(t *testing.T) {
	values := contracts.EffortThemeValues()
	if len(values) != 8 || values[0] != contracts.EffortThemeAccent || values[7] != contracts.EffortThemeGold {
		t.Fatalf("preset order differs from the appearance page: %v", values)
	}
	seen := map[string]bool{}
	for _, value := range values {
		label := EffortThemeLabel(value)
		if label == "" || seen[label] {
			t.Fatalf("missing or duplicate label for %q", value)
		}
		seen[label] = true
	}
	if contracts.NormalizeEffortTheme("") != contracts.EffortThemePurple || contracts.NormalizeEffortTheme("bogus") != contracts.EffortThemePurple {
		t.Fatal("unsupported values must default to the source preset")
	}
}
