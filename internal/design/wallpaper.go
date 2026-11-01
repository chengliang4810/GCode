package design

import (
	"image/color"

	"github.com/egoist/mygo/ui"
)

// ApplyWallpaperColors gives the desktop's sampled color the same contrast
// guarantees as a preset accent, then subtly tints surfaces. Status, syntax,
// terminal and diff colors stay owned by the selected theme.
func ApplyWallpaperColors(t Tokens, sample color.NRGBA) Tokens {
	if sample.A == 0 {
		return t
	}
	primary := ensureContrast(ui.Color{R: sample.R, G: sample.G, B: sample.B, A: 255}, t.Background, 3)
	t.Background = sourceMix(t.Background, primary, .025)
	t.Shell = sourceMix(t.Shell, primary, .035)
	t.Raised = sourceMix(t.Raised, primary, .025)
	t.Card = sourceMix(t.Card, primary, .025)
	t.Dialog = sourceMix(t.Dialog, primary, .025)
	t.Input = sourceMix(t.Input, primary, .025)
	primary = ensureContrast(primary, t.Background, 3)
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	on := black
	if contrast(white, primary) >= contrast(black, primary) {
		on = white
	}
	t.Accent, t.AccentOn = primary, on
	t.AccentStrong = ensureContrast(primary, on, 4.5)
	t.Caret = primary
	alpha := float32(.24)
	if t.Dark {
		alpha = .35
	}
	t.Selection = primary.Alpha(alpha)
	t.Glyph = [3]ui.Color{sourceMix(primary, white, .18), primary, sourceMix(primary, black, .18)}
	return t
}
