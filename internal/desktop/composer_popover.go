package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type composerMenuMeasurement struct {
	anchor ui.Rect
	height float32
}

// composerMenuPlacement follows Pickers::menu_geometry / measure_trigger.
// Bounds is committed geometry; a cold opening uses the source's 320px,
// above-trigger default until the next measured frame. Keep only scalar
// measurements in local state and request a frame when they change.
func composerMenuPlacement(c *ui.Context, panel, anchor ui.Element, newThread, compact bool) (budget float32, below bool) {
	bounds, height := measureComposerMenu(c, panel, anchor)
	if bounds.W <= 0 || bounds.H <= 0 {
		return 320, false
	}
	above := max(float32(0), bounds.Y-14)
	under := max(float32(0), height-bounds.Y-bounds.H-14)
	preferBelow := newThread && under >= 180
	below = preferBelow || !compact && above < 180 && under > above
	budget = above
	if below {
		budget = under
	}
	return min(float32(640), budget), below
}

// Prompt completions always open above the composer, independently of the
// model picker's preferred direction. A short viewport shrinks the list.
func promptMenuHeight(c *ui.Context, panel, anchor ui.Element) float32 {
	bounds, _ := measureComposerMenu(c, panel, anchor)
	if bounds.W <= 0 || bounds.H <= 0 {
		return 320
	}
	return min(float32(320), max(float32(0), bounds.Y-14))
}

func measureComposerMenu(c *ui.Context, panel, anchor ui.Element) (ui.Rect, float32) {
	_, height := c.Size()
	bounds := anchor.Bounds()
	measurement := ui.Local(panel, "composer-menu-measurement", func() composerMenuMeasurement { return composerMenuMeasurement{} })
	next := composerMenuMeasurement{anchor: bounds, height: height}
	if *measurement != next {
		*measurement = next
		c.After(time.Millisecond)
	}
	return bounds, height
}
