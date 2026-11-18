package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// A split divider follows the pointer directly: the width the pointer asks
// for is the width the pane shows. Pushing the seam past one of its limits
// plays a single short bounce, unless the caller closes a collapsible pane
// after the pointer crosses its minimum width.
const (
	resizeBounceDuration = 220 * time.Millisecond
	resizeBounceTravel   = float32(5)
	resizeBounceOutShare = float32(0.32)
	resizeBounceFrame    = 16 * time.Millisecond
)

// resizeEdge is the limit a drag pushed past.
type resizeEdge int

const (
	resizeEdgeNone resizeEdge = iota
	resizeEdgeMin
	resizeEdgeMax
)

// splitResize carries the resize feel of one split divider. held is the limit
// the pointer pushes past now, which latches so a held edge bounces once.
// A pulse, once started, plays to the end even after the pointer comes back
// inside: cutting it short would snap the seam by the travel it had left.
// painted is the bounce the last frame added; a drag sample takes it back out,
// so the bounce stays a picture and never enters the width the app keeps.
type splitResize struct {
	held    resizeEdge
	bounced resizeEdge
	started time.Time
	painted float32
	// dragging is true on a frame whose pointer move changed the size, so
	// Escape knows a drag is in flight.
	dragging bool
}

// paneDragOrigin is where the persisted pane sizes were when the drag that is
// in flight started. Escape puts them back, so a drag that crossed a limit
// leaves no trace on the pane or on the settings it saves.
type paneDragOrigin struct {
	sidebar, right, terminal, files float32
	known                           bool
}

// beginPaneDrag records the sizes a drag starts from, once.
func (a *App) beginPaneDrag() {
	a.dragMovedFrame = true
	if a.dragOrigin.known {
		return
	}
	a.dragOrigin = paneDragOrigin{
		sidebar:  float32(a.state.Settings.SidebarWidth),
		right:    float32(a.state.Settings.RightPaneWidth),
		terminal: float32(a.state.Settings.TerminalHeight),
		files:    float32(a.state.Settings.FilesWidth),
		known:    true,
	}
}

// endPaneDragFrame clears a drag whose pointer stopped moving: Escape restores
// the drag still in flight, never a size from an earlier one.
func (a *App) endPaneDragFrame(moved bool) {
	if !moved {
		a.dragOrigin = paneDragOrigin{}
	}
}

// cancelPaneDrag puts every pane a live drag moved back where that drag
// started, for Escape. It reports whether any pane had a drag to cancel.
func (a *App) cancelPaneDrag(c *ui.Context) bool {
	origin := a.dragOrigin
	if !origin.known {
		return false
	}
	a.dragOrigin = paneDragOrigin{}
	a.sidebarResize.dragging = false
	a.rightResize.dragging = false
	a.terminalResize.dragging = false
	a.filesResize.dragging = false
	a.sidebarWidth, a.sidebarContentWidth = origin.sidebar, origin.sidebar
	a.rightWidth = origin.right
	a.terminalHeight = origin.terminal
	a.filesWidth = origin.files
	a.state.Settings.SidebarWidth = float64(origin.sidebar)
	a.state.Settings.RightPaneWidth = float64(origin.right)
	a.state.Settings.TerminalHeight = float64(origin.terminal)
	a.state.Settings.FilesWidth = float64(origin.files)
	if c != nil {
		a.schedulePaneSave(c, paneSidebar|paneRight|paneTerminal|paneFiles)
	}
	return true
}

// paint is the size the divider shows this frame: the size the app keeps,
// plus the bounce still playing at the limit the pointer pushed past.
func (r *splitResize) paint(c *ui.Context, size float32, reduced bool) float32 {
	r.painted = 0
	if r.started.IsZero() || reduced {
		return size
	}
	elapsed := c.Now().Sub(r.started)
	if elapsed >= resizeBounceDuration {
		r.started = time.Time{}
		return size
	}
	r.painted = resizeBounceOffset(r.bounced, float32(elapsed)/float32(resizeBounceDuration))
	c.After(resizeBounceFrame)
	return size + r.painted
}

// observe takes one drag sample: requested is the size the pointer asked for,
// with the bounce of the last frame taken back out. It keeps the size inside
// its limits and starts a bounce when the pointer newly pushes past one.
func (r *splitResize) observe(c *ui.Context, requested, minimum, maximum float32, reduced bool) float32 {
	edge := resizeEdgeNone
	switch {
	case requested < minimum:
		edge = resizeEdgeMin
	case requested > maximum:
		edge = resizeEdgeMax
	}
	r.dragging = true
	previous := r.held
	r.held = edge
	if edge != resizeEdgeNone && edge != previous && !reduced {
		r.bounced, r.started = edge, c.Now()
		c.After(resizeBounceFrame)
	}
	return min(max(requested, minimum), maximum)
}

// reset forgets a bounce whose pane is not on screen: a pane hidden and
// shown again must bounce at its limit the next time the pointer pushes
// there.
func (r *splitResize) reset() { *r = splitResize{} }

// resizeBounceOffset is the bounce at an edge: a rounded pulse that eases out
// to its travel and home again, with no jolt at either join.
func resizeBounceOffset(edge resizeEdge, progress float32) float32 {
	progress = min(max(progress, 0), 1)
	magnitude := float32(0)
	if progress < resizeBounceOutShare {
		magnitude = smoothstep(progress / resizeBounceOutShare)
	} else {
		magnitude = 1 - smoothstep((progress-resizeBounceOutShare)/(1-resizeBounceOutShare))
	}
	magnitude *= resizeBounceTravel
	if edge == resizeEdgeMin {
		return -magnitude
	}
	return magnitude
}

// smoothstep eases t from 0 to 1 with no velocity at either end.
func smoothstep(t float32) float32 {
	t = min(max(t, 0), 1)
	return t * t * (3 - 2*t)
}
