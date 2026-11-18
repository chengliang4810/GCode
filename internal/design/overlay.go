package design

import "github.com/egoist/mygo/ui"

// The overlay shadow is the one shadow of a floating surface: a popover, a
// menu or a dialog above the page. Cards lying in the page stay flat, so a
// shadow this soft belongs to an overlay and nowhere else.
const (
	OverlayShadowX       = float32(0)
	OverlayShadowY       = float32(8)
	OverlayShadowBlur    = float32(24)
	OverlayShadowSpread  = float32(-3)
	OverlayShadowOpacity = float32(.12)
)

// OverlayShadow attaches the overlay shadow to a floating surface's wrapper.
//
// The wrapper is what casts the shadow: it carries the surface's rounding, and
// it clips nothing, so nothing cuts the shadow at the surface's own edge. The
// element inside the wrapper keeps the surface's fill, padding and clip, which
// is what the content needs and the shadow does not: leave the clip there, and
// never on the element that carries this.
func OverlayShadow(element ui.Element) ui.Element {
	return element.Shadow(OverlayShadowX, OverlayShadowY, OverlayShadowBlur, OverlayShadowSpread, ui.RGBA(0, 0, 0, OverlayShadowOpacity))
}
