package desktop

import (
	"math"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// The reasoning slider's motion follows the reference pickers::compact. The
// fill and the thumb ease to a chosen level over the reference's RESIZE
// curve, the thumb's press eases in and out, and the filled rail carries a
// light that flows toward the thumb while the accent breathes. The reference
// holds that light at its reduced-motion frame; this port keeps the accent
// alive whenever the desktop allows motion, and paints the accepted
// three-stop ramp when it does not.
const (
	// effortTransitionDuration is the reference's motion::RESIZE.
	effortTransitionDuration = 200 * time.Millisecond
	// effortTransitionTick and effortShimmerTick pace the frames that carry
	// the level transition and the flowing light.
	effortTransitionTick = 16 * time.Millisecond
	effortShimmerTick    = 33 * time.Millisecond
	// effortPhaseRate is the reference's 0.32 + 0.22*intensity, at the
	// intensity the reference always draws the accent with.
	effortPhaseRate = 0.54
	// effortPhaseCap bounds one phase step after a stall, as the reference does.
	effortPhaseCap = 0.05
	// effortThumbInset is the reference's THUMB_INSET: the travel of the
	// thumb's centre stops that far from each end of the rail.
	effortThumbInset = 22
	// effortFillPast is the reference's FILL_PAST: how far the fill reaches
	// past the thumb's centre.
	effortFillPast = 14
	// effortTau is a full turn, as the reference's waves use.
	effortTau = 2 * math.Pi
)

// effortTransitionEase is the reference's motion::EASE_OUT.
var effortTransitionEase = ui.CubicBezier(0, 0, .58, 1)

// effortTransition eases one value to a target over the reference's RESIZE
// curve. It mirrors the reference's ScalarTransition: a target that changes
// mid-flight restarts the curve from the value shown, and a still frame lands
// at once.
type effortTransition struct {
	from, to float32
	started  time.Time
	seeded   bool
}

// sample returns the value this frame shows and whether it is still moving.
func (t *effortTransition) sample(now time.Time, target float32, still bool) (value float32, moving bool) {
	if !t.seeded {
		t.from, t.to, t.started, t.seeded = target, target, now, true
		return target, false
	}
	if still {
		t.from, t.to = target, target
		return target, false
	}
	if t.to != target {
		t.from, t.to, t.started = t.value(now), target, now
	}
	value = t.value(now)
	return value, math.Abs(float64(value-target)) > .001
}

// jump sets the value at once, as a drag moves it under the pointer, and
// re-roots the curve there so that letting go eases from that point.
func (t *effortTransition) jump(now time.Time, value float32) {
	t.from, t.to, t.started, t.seeded = value, value, now, true
}

// value returns where the curve stands at now.
func (t *effortTransition) value(now time.Time) float32 {
	elapsed := now.Sub(t.started)
	if elapsed >= effortTransitionDuration {
		return t.to
	}
	progress := float32(0)
	if elapsed > 0 {
		progress = float32(elapsed) / float32(effortTransitionDuration)
	}
	return t.from + (t.to-t.from)*effortTransitionEase(progress)
}

// effortMotion is the reasoning slider's animation: the front the fill and
// the thumb travel to, the thumb's press, and the phase of the flowing light.
type effortMotion struct {
	front effortTransition
	press effortTransition
	phase float32
	last  time.Time
}

// effortSliderPaint is one frame of the reasoning slider's motion.
type effortSliderPaint struct {
	// fraction is where the fill's front and the thumb's centre stand.
	fraction float32
	// press is how far the thumb's press shadow has eased in.
	press float32
	// breath is the accent's slow rise and fall, .5 at rest.
	breath float32
	// phase is the flowing light's position in its cycle.
	phase float32
	// count is how many stops the ladder has.
	count int
	// live says the accent moves this frame.
	live bool
	// maximum says the topmost stop is chosen, where the reference adds
	// its drifting particles.
	maximum bool
}

// paint samples the slider's motion for this frame. level is the chosen
// stop, count how many the ladder has, and pressed whether the pointer holds
// the slider. A still frame lands every transition at once; a live one lets
// the accent's light flow.
func (m *effortMotion) paint(c *ui.Context, level, count int, pressed, still, live bool) effortSliderPaint {
	now := c.Now()
	target := float32(0)
	if count > 1 {
		target = float32(level) / float32(count-1)
	}
	fraction, moving := m.front.sample(now, target, still)
	press, pressing := m.press.sample(now, effortAmount(pressed), still)
	m.advance(now, live)
	breath := float32(.5)
	if live {
		breath = .5 + .5*float32(math.Sin(effortTau*float64(m.phase)))
	}
	switch {
	case moving || pressing:
		c.After(effortTransitionTick)
	case live:
		c.After(effortShimmerTick)
	}
	return effortSliderPaint{
		fraction: fraction,
		press:    press,
		breath:   breath,
		phase:    m.phase,
		count:    count,
		live:     live,
		maximum:  count > 1 && level == count-1,
	}
}

// advance moves the flowing light's phase while the accent is alive, and
// holds it while nothing moves, as the reference does.
func (m *effortMotion) advance(now time.Time, live bool) {
	if !live {
		m.last = now
		return
	}
	if m.last.IsZero() {
		m.last = now
		return
	}
	step := float32(now.Sub(m.last).Seconds())
	if step < 0 {
		step = 0
	}
	if step > effortPhaseCap {
		step = effortPhaseCap
	}
	m.last = now
	m.phase = effortFract(m.phase + step*effortPhaseRate)
}

// effortAmount is 1 for a held control and 0 for a released one.
func effortAmount(held bool) float32 {
	if held {
		return 1
	}
	return 0
}

// effortFract is the fraction of a turn, as the reference's phases use.
func effortFract(value float32) float32 {
	return value - float32(math.Floor(float64(value)))
}

// effortFraction maps x DIPs across the rail to the stop it lands between,
// as the reference's effort_fraction does: the travel begins THUMB_INSET
// from each end.
func effortFraction(x, width float32) float32 {
	if width <= 2*effortThumbInset {
		return 0
	}
	return min(max((x-effortThumbInset)/(width-2*effortThumbInset), 0), 1)
}

// effortPointerFraction maps the pointer to the slider's travel.
func effortPointerFraction(slider ui.Element) (float32, bool) {
	bounds := slider.Bounds()
	if bounds.W <= 2*effortThumbInset {
		return 0, false
	}
	x, _, _ := slider.PointerPosition()
	return effortFraction(x, bounds.W), true
}

// effortLive reports whether the slider's accent may move. The reference
// holds its light still while the window is not in front, and reduced motion
// or a fixture frame holds it still too.
func (a *composerController) effortLive(c *ui.Context) bool {
	if a.reduceMotion(c) || c.Instant() {
		return false
	}
	return a.win == nil || a.win.IsFocused()
}

// effortGlow is the accent's coloured halo, the reference's
// intensity*(0.2+0.25*breath).
func effortGlow(breath float32) float32 {
	return .2 + .25*breath
}

// effortHalo is the light under the thumb, the reference's
// intensity*(0.55+0.45*breath).
func effortHalo(breath float32) float32 {
	return .55 + .45*breath
}

// effortFlow paints the reference's travelling light over the filled rail:
// two slow waves drift across it at different speeds, the light gathers just
// before the thumb, and it fades in from the rail's start. Short streaks, the
// topmost stop's drifting particles and the periodic lightning ride the same
// phase.
func effortFlow(p *ui.Painter, effort design.EffortPalette, paint effortSliderPaint, plate ui.Rect) {
	inner := ui.Rect{X: plate.X + 1, Y: plate.Y + 1, W: max(0, plate.W-2), H: max(0, plate.H-2)}
	if inner.W <= 2 || inner.H <= 2 {
		return
	}
	// The rail's own coordinates: the fill starts at the rail's start, so
	// the thumb's centre sits FILL_PAST inside the fill's far end. plate.Y
	// is RAIL_TOP, the rail's own y origin.
	railTop := plate.Y
	fillEnd := inner.W + 2
	center := fillEnd - effortFillPast
	flow := func(x float32) ui.Color {
		base := effort.ColorAt((x - 1) / max(1, inner.W))
		slow := .5 + .5*float32(math.Sin(effortTau*float64(x/150-2*paint.phase)))
		quick := .5 + .5*float32(math.Sin(effortTau*float64(x/64-3*paint.phase)+1.7))
		glow := .65*slow + .35*quick
		glow *= glow
		charge := float32(math.Exp(-float64(max(0, center-x))/40)) * (.6 + .4*paint.breath)
		lead := min(max((x-1)/28, 0), 1)
		alpha := (.08+.34*glow)*lead + .22*charge
		return base.Mix(effort.Spark(), min(max(alpha, 0), .55))
	}
	stops := make([]ui.GradientStop, 0, 32)
	for i := range 32 {
		at := float32(i) / 31
		stops = append(stops, ui.GradientStop{Color: flow(1 + at*inner.W), At: at})
	}
	p.FillGradient(inner, ui.LinearGradient{Angle: 90, Stops: stops}, inner.H/2)

	start := float32(effortFillPast)
	span := center - start
	if span <= 10 {
		return
	}
	p.Clip(inner, inner.H/2, func() {
		streaks := int(min(max(span/32, 2), 8))
		for i := range streaks {
			seed := float32(i)
			progress := effortFract(paint.phase*(3+float32(i%3)) + seed*.381)
			head := start + span*progress
			length := min(10+float32(i%3)*4, head-start)
			fade := min(max(min(progress*8, (1-progress)*8), 0), 1)
			if length <= 1 {
				continue
			}
			p.FillGradient(ui.Rect{
				X: plate.X + head - length,
				Y: railTop + 7 + float32(math.Mod(float64(i)*7, 15)),
				W: length, H: 1.2,
			}, ui.LinearGradient{
				Angle: 90,
				From:  effort.Spark().Alpha(0),
				To:    effort.Spark().Alpha(.4 * fade),
			}, .6)
		}
		if paint.maximum {
			particles := int(min(max(span/14, 4), 18))
			for i := range particles {
				seed := float32(i)
				progress := effortFract(paint.phase*(1+float32(i%2)) + seed*.618)
				fade := min(max(min(progress*10, (1-progress)*10), 0), 1)
				opacity := (.18 + .28*float32(math.Abs(math.Sin(float64(seed*1.7+paint.phase*effortTau))))) * fade
				p.Fill(ui.Rect{
					X: plate.X + start + span*progress,
					Y: railTop + 5 + float32(math.Mod(float64(i)*11.7, 18)),
					W: 2, H: 2,
				}, effort.Spark().Alpha(opacity), 1)
			}
		}
		flash := effortFract(paint.phase * 2)
		if flash >= .16 {
			return
		}
		alpha := float32(math.Sin(float64(flash/.16)*math.Pi)) * .65
		x := plate.X + start + span*.62
		var bolt ui.Path
		bolt.MoveTo(x+3, railTop+6)
		bolt.LineTo(x-2, railTop+14)
		bolt.LineTo(x+2, railTop+14)
		bolt.LineTo(x-3, railTop+22)
		p.StrokePath(&bolt, 1.2, effort.Spark().Alpha(alpha))
	})
}
