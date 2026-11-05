package desktop

import (
	"runtime"
	"strings"

	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// NativeVibrancy is the window material the shell paints its glass over.
// The reference's shell frost sits over a blurred desktop on macOS and
// Windows (crates/ui/src/theme.rs::window_background_appearance), so
// Windows takes Acrylic: Mica, MyGo's default there, tints with the
// wallpaper but never blurs what is behind the window.
func NativeVibrancy(surface string) mygo.Vibrancy {
	if strings.EqualFold(surface, "opaque") {
		return mygo.VibrancyNone
	}
	if runtime.GOOS == "windows" {
		return mygo.VibrancyAcrylic
	}
	return mygo.VibrancySidebar
}

// NativeVibrancy is this window's material, from the surface preference.
func (a *App) NativeVibrancy() mygo.Vibrancy {
	return NativeVibrancy(a.state.Settings.Appearance.Surface)
}

// frosted reports whether floating surfaces paint their backdrop blur: the
// window shows a material and the theme asks for frost.
func (a *App) frosted(c *ui.Context) bool {
	return design.Frost(a.palette, a.state.Settings.Appearance.Surface, c.Vibrancy())
}

// frostedSurface paints a floating surface the way the reference's frost
// does: a backdrop blur under the surface's translucent tint, then its
// hairline (crates/ui/src/frost.rs, crates/ui/src/popover.rs). The blur is
// what the surface is for — the transcript under the composer, the page under
// a dialog — so the tint stays light enough to show it.
//
// Without a window material there is nothing to blur: the surface keeps the
// opaque fill and border the reference falls back to.
func (a *App) frostedSurface(c *ui.Context, el ui.Element, radius float32, tint, border ui.Color) ui.Element {
	if !a.frosted(c) {
		return el.Background(tint).Border(1, border)
	}
	// The hairline keeps the border's layout inset but paints with the
	// tint, above it, the way the reference's card draws its own edge.
	el = el.Border(1, ui.Transparent).Material(glass.Blur{Radius: design.MenuBlur})
	return el.Draw(func(p *ui.Painter, bounds ui.Rect) {
		p.Fill(bounds, tint, radius)
		p.Stroke(bounds, border, radius, 1)
	})
}

// frostedComposer is the composer pill's frosted fill and hairline
// (Theme::composer_surface_bg and composer_surface_border).
func (a *App) frostedComposer(c *ui.Context, el ui.Element, radius float32) ui.Element {
	surface := a.state.Settings.Appearance.Surface
	return a.frostedSurface(c, el, radius,
		design.ComposerSurface(a.palette, surface, c.Vibrancy()),
		design.ComposerBorder(a.palette, surface, c.Vibrancy()))
}

// Reference attached headers use the input glass tint with rounded top corners,
// rather than adding a second independently blurred pill behind the composer.
func (a *App) attachedComposerSurface(c *ui.Context, el ui.Element) ui.Element {
	s := design.AttachedPanels()
	return el.Radius(s.Radius, s.Radius, 0, 0).
		Background(design.ComposerSurface(a.palette, a.state.Settings.Appearance.Surface, c.Vibrancy())).
		Border(1, design.ComposerBorder(a.palette, a.state.Settings.Appearance.Surface, c.Vibrancy()))
}

// floatingSurface is a popover's or dialog's frosted card: the shared popover
// tint over the backdrop blur, with the theme border.
func (a *App) floatingSurface(c *ui.Context, el ui.Element, radius float32) ui.Element {
	surface := a.state.Settings.Appearance.Surface
	return a.frostedSurface(c, el, radius,
		design.FloatingSurface(a.palette, surface, c.Vibrancy()),
		a.palette.Border)
}
