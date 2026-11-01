package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const activityFoldDuration = 140 * time.Millisecond

// BoardUI task-tree cadence: a slow light sweep keeps the active group's
// summary legible, while each newly appended row reveals quickly enough to
// read as a continuous log rather than a stack of discrete pop-ins.
const (
	activityRowRevealDuration  = 360 * time.Millisecond
	activityConnectorDuration  = 480 * time.Millisecond
	activityFirstRowDelay      = 90 * time.Millisecond
	activityRowStagger         = 65 * time.Millisecond
	activityShimmerPeriod      = 3400 * time.Millisecond
	activityShimmerFrame       = 33 * time.Millisecond
	activityShimmerHalfWidth   = float32(0.36)
	activityShimmerStrip       = float32(2)
	activityRevealLift         = float32(4)
	activityMotionFrame        = 16 * time.Millisecond
	activityRevealSettleBuffer = 2 * time.Second
)

var (
	activityRowRevealEase = ui.CubicBezier(0.16, 1, 0.3, 1)
	activityConnectorEase = ui.CubicBezier(0.22, 1, 0.36, 1)
)

var activityFoldAnimation = ui.Animation{
	Duration: activityFoldDuration,
	Ease:     ui.CubicBezier(0, 0, .58, 1),
}

// Only the owning pane's manual fold suppresses its follow glide. Content
// growth from streaming continues to use the normal list following policy.
type activityFoldMotion struct {
	scope string
	until time.Time
	list  *ui.ListState
}

func (a *App) transcriptListScope(c *ui.Context, root ui.Element, list *ui.ListState, scope string, build func()) {
	motion := ui.Local(root, "activity-fold-motion", func() activityFoldMotion { return activityFoldMotion{} })
	if motion.scope != scope {
		*motion = activityFoldMotion{scope: scope}
	}
	motion.list = list
	list.FollowAnimation = nil
	now := c.Now()
	// Content that grows under the reader, as a streamed reply does, glides
	// the tail into view; a wheel notch is never glided, so the two no longer
	// share a preference.
	if !a.reduceMotion(c) {
		if !now.Before(motion.until) {
			list.FollowAnimation = &ui.Smooth
		} else {
			// Restore normal stream following even if no further fold frame
			// is needed after its final layout.
			c.After(motion.until.Sub(now))
		}
	}
	previous := a.activityMotion
	a.activityMotion = motion
	defer func() { a.activityMotion = previous }()
	root.Children(build)
}

func (a *App) manualActivityFold(c *ui.Context, id string, open *bool, parent string) func() {
	motion, now, reduced := a.activityMotion, c.Now(), a.reduceMotion(c)
	return func() {
		a.rememberActivityFold(id, *open)
		if *open && parent != "" {
			a.rememberActivityFold(parent, true)
		}
		if motion != nil {
			motion.until = time.Time{}
			if !reduced {
				motion.until = now.Add(activityFoldDuration)
			}
			// Actions run before layout. Clear an existing lag in this very
			// frame, rather than let the click start one more follow tween.
			motion.list.FollowAnimation = nil
		}
	}
}

// activityRowMotion carries one activity row's arrival animation: the row
// grows in over activityRowRevealDuration while its content fades and lifts
// with the branch leg of the slower connector draw.
type activityRowMotion struct {
	progress     float32
	content      float32
	incoming     float32
	continuation float32
	settled      bool
}

func activityRevealProgress(start time.Time, now time.Time, reduce bool, total time.Duration, ease ui.Easing) float32 {
	if start.IsZero() || reduce {
		return 1
	}
	elapsed := now.Sub(start)
	if elapsed <= 0 {
		return 0
	}
	if elapsed >= total {
		return 1
	}
	return ease(float32(elapsed) / float32(total))
}

// activityConnectorParts splits one arrival into the trunk's incoming draw
// and the branch leg that carries the row's content; the branch starts
// slightly before the incoming leg ends so the bend has no dead frame.
func activityConnectorParts(progress float32, hasPredecessor bool) (incoming, branch float32) {
	clamp01 := func(v float32) float32 { return max(0, min(v, 1)) }
	incomingStart, incomingEnd, branchStart := float32(0.45), float32(0.72), float32(0.68)
	if !hasPredecessor {
		incomingStart, incomingEnd, branchStart = 0, 0.62, 0.58
	}
	progress = clamp01(progress)
	incoming = clamp01((progress - incomingStart) / (incomingEnd - incomingStart))
	branch = clamp01((progress - branchStart) / (1 - branchStart))
	return incoming, branch
}

// activityContinuation lets the previous row's trunk grow toward a new
// arrival; the extension belongs to the next row's timing.
func activityContinuation(nextProgress float32) float32 {
	return max(0, min(nextProgress/0.45, 1))
}

// activityShimmerPhase folds the elapsed time into the sweep's cycle.
func activityShimmerPhase(start time.Time, now time.Time) float32 {
	if start.IsZero() {
		return 0
	}
	elapsed := now.Sub(start)
	if elapsed < 0 {
		return 0
	}
	f := float32(elapsed) / float32(activityShimmerPeriod)
	return f - float32(int(f))
}

// activityShimmerAmount samples the sweeping highlight at x, in units of the
// title's width: a triangular peak half a shoulder wide, repeated every
// three widths, as the board's measured recipe.
func activityShimmerAmount(x float32, phase float32) float32 {
	center := -2.5 + max(0, min(phase, 1))*6
	amount := float32(0)
	for step := -2; step <= 2; step++ {
		at := center + float32(step)*3
		distance := x - at
		if distance < 0 {
			distance = -distance
		}
		amount = max(amount, max(0, min(1-distance/activityShimmerHalfWidth, 1)))
	}
	return amount
}
