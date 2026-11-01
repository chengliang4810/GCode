package desktop

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// Panels open and close over a short eased width tween instead of appearing
// in one frame: the reference shell's PANEL_RESIZE (180ms, CSS ease), stretched
// by the square root of the distance so a wide column travels at about the
// pace of the sidebar, up to 240ms. A live drag never eases — it follows the
// pointer — and reduced motion snaps.
const (
	panelTweenBase      = 180 * time.Millisecond
	panelTweenReference = float32(256) // the sidebar's default width
	panelTweenMaxScale  = float32(4) / 3
	panelTweenFrame     = 16 * time.Millisecond
)

// panelEase is the reference's PANEL_RESIZE curve (CSS ease).
var panelEase = ui.CubicBezier(0.25, 0.1, 0.25, 1)

// panelWidthTween eases one column between two widths. The endpoint is read
// from the live target every frame, so a column lands where its layout puts it
// even if a neighbour changed the target mid-flight.
type panelWidthTween struct {
	running  bool
	from     float32
	started  time.Time
	duration time.Duration
}

func (t *panelWidthTween) begin(now time.Time, from, to float32, reduced bool) {
	if reduced || math.Abs(float64(to-from)) < 0.5 {
		t.running = false
		return
	}
	distance := float32(math.Abs(float64(to - from)))
	scale := min(max(float32(math.Sqrt(float64(distance/panelTweenReference))), 1), panelTweenMaxScale)
	t.running = true
	t.from = from
	t.started = now
	t.duration = time.Duration(float64(panelTweenBase) * float64(scale))
}

// value is the width to paint this frame. target is the width the layout asks
// for now; a finished or absent tween is exactly the target.
func (t *panelWidthTween) value(c *ui.Context, target float32) float32 {
	if !t.running {
		return target
	}
	elapsed := c.Now().Sub(t.started)
	if elapsed >= t.duration {
		t.running = false
		return target
	}
	c.After(panelTweenFrame)
	progress := min(max(float32(elapsed)/float32(t.duration), 0), 1)
	return t.from + (target-t.from)*panelEase(progress)
}

// panelContentWidth is the width a pane lays its content out at while its
// column eases: opening lays out at the destination and closing keeps the
// current layout, so the moving edge reveals the pane instead of reflowing it
// on every frame. A resize between two open widths follows the destination.
func panelContentWidth(current, from, to float32) float32 {
	switch {
	case from < 0.5:
		return to
	case to < 0.5:
		return max(current, from)
	default:
		return to
	}
}

// paintPanels is the allocation the frame draws: the widths being tweened
// take the place of the settled ones, and the conversation keeps the rest.
func paintPanels(viewport float32, painted [3]float32, fit panelAllocation) panelWidths {
	var width panelWidths
	if fit.Sidebar {
		width.Sidebar = min(max(painted[0], 0), max(0, fit.SidebarLimit))
	}
	available := max(0, viewport-width.Sidebar)
	if fit.Files {
		width.Files = min(max(painted[2], 0), max(0, available))
	}
	available -= width.Files
	if fit.Right {
		width.Right = min(max(painted[1], 0), max(0, available))
	}
	width.Chat = max(0, available-width.Right)
	return width
}

// layoutPanels advances the panel tweens and records this frame's columns.
// Resting targets come from the settled allocation; a panel the fit hides
// settles at zero. A panel that is still closing stays in the fit until its
// column reaches zero, so its neighbours expand as it leaves.
func (a *App) layoutPanels(c *ui.Context, viewport float32, open [3]bool, reduced bool) {
	resting := allocatePanels(viewport, a.sidebarWidth, a.filesWidth, a.rightWidth, a.fit, a.rightExpanded)
	var targets [3]float32
	if open[0] && a.fit.Sidebar {
		targets[0] = resting.Sidebar
	}
	if open[1] && a.fit.Right {
		targets[1] = resting.Right
	}
	if open[2] && a.fit.Files {
		targets[2] = resting.Files
	}
	if !a.panelReady {
		// The first frame has no earlier width to ease from: a restored
		// layout appears settled, as the window opens.
		a.panelReady = true
		a.panelPainted, a.panelMotion = targets, [3]bool{}
	}
	now := c.Now()
	for i := range a.panelTween {
		if a.panelMotion[i] {
			a.panelMotion[i] = false
			a.panelTween[i].begin(now, a.panelPainted[i], targets[i], reduced)
			a.panelContent[i] = panelContentWidth(a.panelPainted[i], a.panelPainted[i], targets[i])
		}
		a.panelPainted[i] = a.panelTween[i].value(c, targets[i])
		if a.panelTween[i].running {
			// Keep the column in the layout until it has finished leaving.
			switch i {
			case 0:
				a.fit.Sidebar = true
			case 1:
				a.fit.Right = true
			case 2:
				a.fit.Files = true
			}
		}
	}
	a.frameColumns = paintPanels(viewport, a.panelPainted, a.fit)
	a.visibleSidebarWidth = a.panelPainted[0]
}

// workspaceTakeover reports the workspace owning the whole column: not
// while its widening tween still shows the conversation leaving.
func (a *App) workspaceTakeover() bool {
	return a.fit.Right && a.rightExpanded && !a.panelAnimating(1)
}

// panelAnimating reports whether the column is easing this frame; a drag's
// own width bookkeeping stands aside while it does.
func (a *App) panelAnimating(which int) bool { return a.panelTween[which].running }

// panelContentBox lays a moving pane's content out at the width it held when
// the tween began, so the column's clipped edge reveals it instead of
// reflowing it every frame. A resting pane builds its content directly.
func (a *App) panelContentBox(c *ui.Context, which int, build func()) {
	if !a.panelAnimating(which) {
		build()
		return
	}
	width := a.panelContent[which]
	if which == 0 {
		width = max(0, width-1)
	}
	ui.Box(c).Width(width).Shrink(0).Clip().Children(build)
}
