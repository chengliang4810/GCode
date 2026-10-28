package design

import "github.com/egoist/mygo/ui"

// Glass applies the reference Theme::glass tint-coverage contrast guard.
func Glass(t Tokens, preference string, supported bool) ui.Color {
	if !supported || preference == "opaque" || preference == "Opaque" || (!t.Frosted && preference != "frosted" && preference != "Frosted") {
		return t.Shell
	}
	adverse := ui.Hex("#000000")
	if t.Dark {
		adverse = ui.Hex("#ffffff")
	}
	for step := 0; step <= 20; step++ {
		alpha := float32(.8 + .2*float64(step)/20)
		tint := t.Shell.Alpha(alpha)
		composite := tint.Over(adverse)
		if contrast(t.Text, composite) >= 4.5 && contrast(t.Muted, composite) >= 3 {
			return tint
		}
	}
	return t.Shell
}

// PanelBackground follows Theme::panel_bg: editor hosts and Files share the
// theme background at 40% coverage when their shell has translucent glass.
func PanelBackground(t Tokens, preference string) ui.Color {
	if Glass(t, preference, true).A < 255 {
		return t.Background.Alpha(.4)
	}
	return t.Background
}
