package design

import "github.com/egoist/mygo/ui"

// MenuBlur is the backdrop blur of floating glass surfaces, in DIPs: the
// reference's frost::MENU_BLUR, shared by the composer pill, popovers and
// dialogs.
const MenuBlur = float32(16)

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

// Frost reports whether the shell paints translucent glass over the window's
// material — the reference's Theme::is_glass. Floating surfaces gate their
// backdrop blur and translucent tints on it, not on the surface preference
// alone: an opaque shell has nothing to show through.
func Frost(t Tokens, preference string, supported bool) bool {
	if !supported {
		return false
	}
	glass := Glass(t, preference, supported)
	return glass.A > 0 && glass.A < 255
}

// PanelBackground follows Theme::panel_bg: editor hosts and Files share the
// theme background at 40% coverage when their shell has translucent glass.
func PanelBackground(t Tokens, preference string) ui.Color {
	if Glass(t, preference, true).A < 255 {
		return t.Background.Alpha(.4)
	}
	return t.Background
}

// ComposerSurface follows Theme::composer_surface_bg: the composer pill's
// fill. The dark frosted pill takes composer_sidebar_tint, which solves a tint
// that carries the panel tone over the shell glass at the reference's 15%
// scene coverage; light frost and opaque surfaces composite the input tint
// onto the page instead.
func ComposerSurface(t Tokens, preference string, supported bool) ui.Color {
	frost := Frost(t, preference, supported)
	if frost && t.Dark {
		return t.composerTint(preference, supported)
	}
	tint := t.Input.Over(t.Background)
	if frost {
		return tint.Alpha(.35)
	}
	return tint
}

// composerTint ports Theme::composer_sidebar_tint: the target is the panel
// tone at 40% over the glass, the canvas the glass itself; the returned color
// is solved for full coverage of the target at the reference's minimum
// alpha, then painted at 15% — the dark pill reads as glass tinted toward
// the panel, not as a raised slab.
func (t Tokens) composerTint(preference string, supported bool) ui.Color {
	canvas := Glass(t, preference, supported).Over(t.Background)
	target := t.Background.Alpha(.4).Over(canvas)
	alpha := float32(.60)
	for _, c := range [3][2]uint8{{canvas.R, target.R}, {canvas.G, target.G}, {canvas.B, target.B}} {
		b, d := float32(c[0])/255, float32(c[1])/255
		needed := (b - d) / max(b, .000001)
		if d > b {
			needed = (d - b) / max(1-b, .000001)
		}
		alpha = max(alpha, needed)
	}
	solve := func(base, desired uint8) uint8 {
		b, d := float32(base)/255, float32(desired)/255
		v := (d - b*(1-alpha)) / alpha
		return uint8(min(max(v, 0), 1)*255 + .5)
	}
	return ui.RGBA(solve(canvas.R, target.R), solve(canvas.G, target.G), solve(canvas.B, target.B), .15)
}

// ComposerBorder follows Theme::composer_surface_border: the frost's cool
// hairline over the pill, the plain border on an opaque surface.
func ComposerBorder(t Tokens, preference string, supported bool) ui.Color {
	if !Frost(t, preference, supported) {
		return t.Border
	}
	if t.Dark {
		return ui.Hex("#bdc7d1").Alpha(.09)
	}
	return ui.Hex("#435260").Alpha(.10)
}

// SidebarTone follows the reference's sidebar_tone: the wash the sidebar
// column paints over the shell glass, so the column reads as its own
// surface without a card of its own (crates/ui/src/shell.rs).
func SidebarTone(t Tokens) ui.Color {
	if t.Dark {
		return ui.Hex("#ebebeb").Alpha(.05)
	}
	return ui.Hex("#1a1a1a").Alpha(.05)
}

// FloatingSurface follows popover.rs::surface_bg: the tint a popover, a
// dialog or any other floating card paints over its backdrop blur. The dark
// frosted card shares the composer pill's tint; light frost takes the glass
// overlay, and an opaque surface flattens the input tint onto the page.
func FloatingSurface(t Tokens, preference string, supported bool) ui.Color {
	if !Frost(t, preference, supported) {
		return t.Input.Over(t.Background)
	}
	if t.Dark {
		return t.composerTint(preference, supported)
	}
	return GlassOverlay(t, preference, supported)
}

// GlassOverlay follows Theme::glass_overlay: the tint a floating card paints
// over its backdrop blur. Light frost keeps fixed coverage so the blurred
// scene stays visible; dark frost raises it until shell text keeps its
// contrast over the brightest backdrop.
func GlassOverlay(t Tokens, preference string, supported bool) ui.Color {
	if !Frost(t, preference, supported) {
		return t.Overlay
	}
	if !t.Dark {
		return t.Overlay.Alpha(.45)
	}
	adverse := ui.Hex("#ffffff")
	return t.Overlay.Alpha(t.contrastTintAlpha(t.Overlay, .50, adverse))
}

// contrastTintAlpha raises a tint's coverage until primary and muted text
// keep their contrast over backdrop, as Theme::contrast_checked_tint_alpha.
func (t Tokens) contrastTintAlpha(tint ui.Color, base float32, backdrop ui.Color) float32 {
	for step := 0; step <= 20; step++ {
		alpha := base + (1-base)*float32(step)/20
		composite := tint.Alpha(alpha).Over(backdrop)
		if contrast(t.Text, composite) >= 4.5 && contrast(t.Muted, composite) >= 3 {
			return alpha
		}
	}
	return 1
}
