package desktop

import (
	"math"
	"time"
)

// The blank-canvas handoff, ported from composer_dock.rs: the composer keeps
// one layout slot at the bottom of the conversation and only its painted
// origin follows a retargetable, critically damped clock. Docking takes 420ms
// and undocking 470ms (the source's `duration(docked)`), and the chrome swaps
// its pieces on staged windows of the same timeline instead of on one frame.
const (
	composerDockDuration   = 420 * time.Millisecond
	composerUndockDuration = 470 * time.Millisecond
	// A frame gap longer than this is a route/window pause, not animation
	// time: the handoff snaps instead of replaying a stale timeline.
	composerDockFrameLimit = 250 * time.Millisecond
)

// glide is composer_dock.rs's critically damped spring. It never oscillates,
// and a target that changes mid-flight keeps both position and velocity, so a
// reversal (a failed send, a quick new-chat navigation) continues from where
// the composer shows instead of restarting.
type glide struct {
	value, velocity, target float32
}

func (g *glide) advance(target, seconds, duration float32) {
	g.target = target
	omega := 12 / max(duration, .001)
	displacement := g.value - target
	c := g.velocity + omega*displacement
	decay := float32(math.Exp(float64(-omega * seconds)))
	g.value = target + (displacement+c*seconds)*decay
	g.velocity = (g.velocity - omega*c*seconds) * decay
	if !g.moving() {
		*g = glide{value: target, target: target}
	}
}

func (g glide) moving() bool {
	return math.Abs(float64(g.value-g.target)) > .0005 || math.Abs(float64(g.velocity)) > .005
}

// composerDockVisuals are the staged opacities that ride beside the geometry:
// the blank canvas's selectors leave first, the session footer arrives with
// the input, and the canvas/transcript dissolve spans the whole handoff.
type composerDockVisuals struct {
	transcript, selectors, footer, dissolve float32
}

func composerSettledVisuals(docked bool) composerDockVisuals {
	value := float32(0)
	if docked {
		value = 1
	}
	return composerDockVisuals{
		transcript: value,
		selectors:  1 - value,
		footer:     value,
		dissolve:   value,
	}
}

// stage eases value along smoothstep between start and end of the handoff.
func dockStage(value, start, end float32) float32 {
	t := max(0, min((value-start)/(end-start), 1))
	return t * t * (3 - 2*t)
}

func (v composerDockVisuals) advance(docked bool, time float32) composerDockVisuals {
	target := composerSettledVisuals(docked)
	blend := func(from, to, start, end float32) float32 {
		return from + (to-from)*dockStage(time, start, end)
	}
	if docked {
		return composerDockVisuals{
			transcript: blend(v.transcript, target.transcript, .20, .65),
			// The spring has covered most of its travel by a quarter of the
			// clock: release the departing selectors during it.
			selectors: blend(v.selectors, target.selectors, 0, .25),
			footer:    blend(v.footer, target.footer, .25, .55),
			dissolve:  blend(v.dissolve, target.dissolve, .06, .88),
		}
	}
	// On return, release the thread chrome first and restore the canvas's
	// selectors near arrival, behind the rising input.
	return composerDockVisuals{
		transcript: blend(v.transcript, target.transcript, 0, .25),
		selectors:  blend(v.selectors, target.selectors, .50, .95),
		footer:     blend(v.footer, target.footer, 0, .18),
		dissolve:   blend(v.dissolve, target.dissolve, .08, .85),
	}
}

type composerDockChoreography struct {
	started time.Time
	from    composerDockVisuals
	docked  bool
}

type composerDockState struct {
	ready  bool
	docked bool
	phase  glide
	last   time.Time
	choreo *composerDockChoreography
	visual composerDockVisuals
}

// composerDockFrame is one frame of the handoff. Amount is 0 on the centered
// blank canvas and 1 on the docked session composer.
type composerDockFrame struct {
	Amount   float32
	Active   bool
	Flipped  bool
	Previous bool
	Selectors,
	Footer, Transcript, Dissolve float32
}

// snap puts the dock at rest, for reduced motion, a first paint and route
// pauses. The returned frame is what that route paints.
func (d *composerDockState) snap(docked bool, now time.Time) composerDockFrame {
	previous := d.docked
	d.ready, d.docked, d.last = true, docked, now
	value := float32(0)
	if docked {
		value = 1
	}
	d.phase = glide{value: value, target: value}
	d.choreo = nil
	d.visual = composerSettledVisuals(docked)
	return d.frame(value, false, previous, previous != docked)
}

func (d *composerDockState) frame(amount float32, active, previous, flipped bool) composerDockFrame {
	return composerDockFrame{
		Amount:     max(0, min(amount, 1)),
		Active:     active,
		Flipped:    flipped,
		Previous:   previous,
		Selectors:  d.visual.selectors,
		Footer:     d.visual.footer,
		Transcript: d.visual.transcript,
		Dissolve:   d.visual.dissolve,
	}
}

// tick advances the handoff to now. It is pure apart from the state it keeps:
// the same call order is what unit tests replay to prove continuity.
func (d *composerDockState) tick(now time.Time, docked, reduced bool) composerDockFrame {
	if !d.ready || reduced || (!d.last.IsZero() && now.Sub(d.last) > composerDockFrameLimit) {
		return d.snap(docked, now)
	}
	previous := d.docked
	flipped := previous != docked
	dt := float32(0)
	if !flipped && !d.last.IsZero() {
		dt = float32(now.Sub(d.last).Seconds())
	}
	d.last = now
	if flipped {
		// A click after idle is the start of the new motion, not elapsed
		// animation time; the last painted velocity survives the retarget.
		d.choreo = &composerDockChoreography{started: now, from: d.visual, docked: docked}
		d.docked = docked
	}
	duration := composerDockDuration
	if !docked {
		duration = composerUndockDuration
	}
	target := float32(0)
	if docked {
		target = 1
	}
	d.phase.advance(target, dt, float32(duration.Seconds()))
	if d.choreo != nil {
		time := float32(now.Sub(d.choreo.started).Seconds()) / float32(duration.Seconds())
		if time >= 1 {
			d.choreo = nil
			d.visual = composerSettledVisuals(docked)
		} else {
			d.visual = d.choreo.from.advance(docked, time)
		}
	} else {
		d.visual = composerSettledVisuals(docked)
	}
	return d.frame(d.phase.value, d.choreo != nil || d.phase.moving(), previous, flipped)
}

// composerHandoff is the route that the composer is leaving, captured when
// the dock clock starts: the previous pill height plus the presentation mode
// that produced it. The surfacing pill morphs from these instead of jumping
// between the blank canvas's tall input and the thread's compact row.
type composerHandoff struct {
	Amount   float32
	Height   float32
	Expanded bool
	Hero     bool
}

// composerRouteStyle names one end of a handoff.
type composerRouteStyle struct {
	hero, expanded bool
	height         float32
}

// composerMorphGeometry is the pill mid-handoff: the committed route's layout
// eased toward the previous route's look. Controls already pin themselves to
// the pill's bottom, so only the box, radius, insets and text padding glide.
type composerMorphGeometry struct {
	radius, inset, actionBottom float32
	height, input               float32
	left, right                 float32
	padding, content            [4]float32
}

func composerMorphGeometryOf(committed, previous composerRouteStyle, amount, natural, compactPaddingY, controls float32) composerMorphGeometry {
	amount = max(0, min(amount, 1))
	lerp := func(a, b float32) float32 { return a + (b-a)*amount }
	radius := func(hero bool) float32 {
		if hero {
			return 26
		}
		return 22
	}
	inset := func(expanded bool) float32 {
		if expanded {
			return 12
		}
		return 8
	}
	actionBottom := func(expanded bool) float32 {
		if expanded {
			return 8
		}
		return 7.5
	}
	padding := func(expanded bool) [4]float32 {
		if expanded {
			return [4]float32{16, 16, 4, 16}
		}
		return [4]float32{compactPaddingY, 8, compactPaddingY, 8}
	}
	x := func(expanded bool) (float32, float32) {
		if expanded {
			return 16, 16
		}
		return 36, 44 + controls
	}
	committedPad, previousPad := padding(committed.expanded), padding(previous.expanded)
	committedLeft, committedRight := x(committed.expanded)
	previousLeft, previousRight := x(previous.expanded)
	height := lerp(natural, previous.height)
	var pad, content [4]float32
	for i := range pad {
		pad[i] = lerp(committedPad[i], previousPad[i])
		content[i] = pad[i]
	}
	return composerMorphGeometry{
		radius:       lerp(radius(committed.hero), radius(previous.hero)),
		inset:        lerp(inset(committed.expanded), inset(previous.expanded)),
		actionBottom: lerp(actionBottom(committed.expanded), actionBottom(previous.expanded)),
		height:       height,
		input:        max(0, height-2),
		left:         lerp(committedLeft, previousLeft),
		right:        lerp(committedRight, previousRight),
		padding:      pad,
		content:      content,
	}
}
