package workbench

import "github.com/egoist/mygo/ui"

// tabTooltipTestWindow observes committed card geometry through the same
// durable Handles that the input path uses. Find(text) cannot identify a card:
// its text may also belong to an earlier anchor or an independent control.
type tabTooltipTestWindow struct {
	*ui.Tester
	cards map[*tabTooltipOwner]ui.Rect
}

func newTabTooltipTestWindow(s *State, view func(*ui.Context), width, height int) *tabTooltipTestWindow {
	window := &tabTooltipTestWindow{cards: make(map[*tabTooltipOwner]ui.Rect)}
	window.Tester = ui.NewTester(func(c *ui.Context) {
		view(c)
		clear(window.cards)
		for _, owner := range s.tabTooltip.owners {
			if bounds := owner.card.Bounds(c); bounds.W > 0 && bounds.H > 0 {
				window.cards[owner] = bounds
			}
		}
	}, width, height)
	return window
}

func tabTooltipStripTester(s *State, width int, expanded bool) *tabTooltipTestWindow {
	return newTabTooltipTestWindow(s, func(c *ui.Context) {
		c.SetTheme(s.palette().UI())
		c.DisableAnimations()
		c.Root().Background(ui.RGB(70, 80, 90))
		ui.Column(c).Fill().Children(func() { s.TabStrip(c, expanded) })
	}, width, 80)
}

func (window *tabTooltipTestWindow) card(close bool) (ui.Rect, bool) {
	// Bounds reads the previous committed pass. Request one build after the
	// tested event, without advancing time or sending input, to observe it.
	window.Frame()
	for owner, bounds := range window.cards {
		if owner.close == close {
			return bounds, true
		}
	}
	return ui.Rect{}, false
}
