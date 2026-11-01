package design

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

// ApplyAccent applies the original AccentPreset/AccentRoles algorithm. Syntax,
// terminal ANSI, status and diff roles remain owned by the chosen theme.
func ApplyAccent(t Tokens, selection string) Tokens {
	presets := map[string][2]string{
		"rcode": {"#8b7cf6", "#5b43e8"}, "orange": {"#fb923c", "#c2410c"}, "amber": {"#fbbf24", "#a16207"},
		"green": {"#4ade80", "#15803d"}, "cyan": {"#22d3ee", "#0e7490"}, "blue": {"#60a5fa", "#2563eb"}, "pink": {"#f472b6", "#be185d"},
	}
	values, ok := presets[strings.ToLower(selection)]
	if !ok {
		return t
	}
	index := 1
	if t.Dark {
		index = 0
	}
	primary := ensureContrast(ui.Hex(values[index]), t.Background, 3)
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	on := black
	if contrast(white, primary) >= contrast(black, primary) {
		on = white
	}
	strong := ensureContrast(primary, on, 4.5)
	lightMix, deepMix := float32(.18), float32(.26)
	lightBase := t.Background
	selectionAlpha := float32(.24)
	if t.Dark {
		lightMix = .28
		deepMix = .18
		lightBase = white
		selectionAlpha = .35
	}
	t.Accent = primary
	t.AccentStrong = strong
	t.AccentOn = on
	t.Selection = primary.Alpha(selectionAlpha)
	t.Caret = primary
	t.Glyph = [3]ui.Color{sourceMix(primary, lightBase, lightMix), primary, sourceMix(primary, black, deepMix)}
	return t
}

func sourceMix(a, b ui.Color, amount float32) ui.Color {
	mix := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*amount + .5) }
	return ui.Color{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}

func luminance(c ui.Color) float32 {
	linear := func(channel uint8) float32 {
		x := float32(channel) / 255
		if x <= .04045 {
			return x / 12.92
		}
		return float32(math.Pow(float64((x+.055)/1.055), 2.4))
	}
	return .2126*linear(c.R) + .7152*linear(c.G) + .0722*linear(c.B)
}

func contrast(a, b ui.Color) float32 {
	if a.A < 255 {
		a = a.Over(b)
	}
	x, y := luminance(a), luminance(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}

func ensureContrast(color, background ui.Color, minimum float32) ui.Color {
	if contrast(color, background) >= minimum {
		return color
	}
	black, white := ui.Hex("#000000"), ui.Hex("#ffffff")
	target := white
	if contrast(black, background) >= contrast(white, background) {
		target = black
	}
	for step := 1; step <= 20; step++ {
		candidate := sourceMix(color, target, float32(step)/20)
		if contrast(candidate, background) >= minimum {
			return candidate
		}
	}
	return target
}
