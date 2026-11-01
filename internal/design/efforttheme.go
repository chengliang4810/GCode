package design

import (
	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

// EffortPalette is the reasoning slider's three-stop gradient and the ink that
// reads on it. The preset is device-local and independent of model options.
type EffortPalette struct {
	Colors [3]ui.Color
	Light  bool
}

// EffortPaletteFor resolves a persisted preset against the active theme.
// Follow theme tracks the accent; every other preset keeps its own colors.
func EffortPaletteFor(theme contracts.EffortTheme, tokens Tokens) EffortPalette {
	preset := contracts.NormalizeEffortTheme(theme)
	rgb := func(hex string) ui.Color { return ui.Hex(hex) }
	var colors [3]ui.Color
	switch preset {
	case contracts.EffortThemeAccent:
		colors = [3]ui.Color{tokens.AccentStrong, tokens.Accent, tokens.AccentStrong}
	case contracts.EffortThemeTerracotta:
		colors = [3]ui.Color{rgb("#b95a3e"), rgb("#d7805e"), rgb("#ca6947")}
	case contracts.EffortThemeDeepBlue:
		colors = [3]ui.Color{rgb("#3152d9"), rgb("#637dfa"), rgb("#394bcb")}
	case contracts.EffortThemeSkyBlue:
		colors = [3]ui.Color{rgb("#006bd9"), rgb("#168df0"), rgb("#3aa4f2")}
	case contracts.EffortThemeAurora:
		colors = [3]ui.Color{rgb("#4285f4"), rgb("#9b72cc"), rgb("#d96670")}
	case contracts.EffortThemeSilver:
		colors = [3]ui.Color{rgb("#ffffff"), rgb("#e8e8ef"), rgb("#c9ccd8")}
	case contracts.EffortThemeGold:
		colors = [3]ui.Color{rgb("#c78c18"), rgb("#f1bc45"), rgb("#e3a324")}
	default: // Purple
		colors = [3]ui.Color{rgb("#8050ed"), rgb("#9b6df5"), rgb("#7143da")}
	}
	return EffortPalette{
		Colors: colors,
		Light:  preset == contracts.EffortThemeSilver || preset == contracts.EffortThemeAccent && luminance(colors[1]) > .65,
	}
}

// EffortThemeLabel is the interface message key of a preset.
func EffortThemeLabel(theme contracts.EffortTheme) string {
	switch contracts.NormalizeEffortTheme(theme) {
	case contracts.EffortThemeAccent:
		return "Follow theme"
	case contracts.EffortThemeTerracotta:
		return "Terracotta"
	case contracts.EffortThemeDeepBlue:
		return "Deep blue"
	case contracts.EffortThemeSkyBlue:
		return "Sky blue"
	case contracts.EffortThemeAurora:
		return "Aurora"
	case contracts.EffortThemeSilver:
		return "Silver"
	case contracts.EffortThemeGold:
		return "Gold"
	default:
		return "Purple"
	}
}

// ColorAt samples the palette's first stop, middle and last by fraction.
func (p EffortPalette) ColorAt(fraction float32) ui.Color {
	position := max(0, min(fraction, 1)) * 2
	if position <= 1 {
		return p.Colors[0].Mix(p.Colors[1], position)
	}
	return p.Colors[1].Mix(p.Colors[2], position-1)
}

// Spark is the ink the fill's stops use: dark on pale palettes, light on the
// rest.
func (p EffortPalette) Spark() ui.Color {
	if p.Light {
		return ui.Hex("#000000")
	}
	return ui.Hex("#ffffff")
}
