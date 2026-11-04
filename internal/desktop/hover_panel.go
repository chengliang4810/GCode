package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type hoverPanelState struct {
	open, suppressed, keyboard bool
	leaveAt                    time.Time
}

func (state *hoverPanelState) finish(c *ui.Context, hovered bool) {
	if hovered || state.keyboard {
		state.leaveAt = time.Time{}
		return
	}
	state.suppressed = false
	if !state.open {
		return
	}
	if state.leaveAt.IsZero() {
		// Allow the pointer to cross the gap between the trigger and panel.
		state.leaveAt = c.Now().Add(180 * time.Millisecond)
	}
	if delay := state.leaveAt.Sub(c.Now()); delay > 0 {
		c.After(delay)
	} else {
		state.open = false
		c.Invalidate()
	}
}
