package desktop

import (
	"image/color"
	"testing"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// The reasoning slider's motion mirrors the reference pickers::compact: a
// transition starts settled, retargets without jumping, and lands at once
// when motion is off.
func TestEffortTransitionStartsSettledAndRetargetsWithoutJumping(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var transition effortTransition
	value, moving := transition.sample(start, .5, false)
	if value != .5 || moving {
		t.Fatalf("first sample = %v, %v; want the target at rest", value, moving)
	}
	if value, _ = transition.sample(start, 1, false); value != .5 {
		t.Fatalf("retarget jumped to %v; it must start from the value shown", value)
	}
	middle := start.Add(effortTransitionDuration / 2)
	before := transition.value(middle)
	retargeted, _ := transition.sample(middle, 0, false)
	if diff := before - retargeted; diff > .0001 || diff < -.0001 {
		t.Fatalf("retarget moved from %v to %v; it must restart from the value shown", before, retargeted)
	}
	if value, moving = transition.sample(middle, 0, true); value != 0 || moving {
		t.Fatalf("still frame = %v, %v; want the target at once", value, moving)
	}
}

// The transition lands where the reference's RESIZE curve puts it.
func TestEffortTransitionLandsOnTheReferenceCurve(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var transition effortTransition
	transition.sample(start, 0, false)
	transition.sample(start, 1, false)
	middle := transition.value(start.Add(effortTransitionDuration / 2))
	if middle <= 0 || middle >= 1 {
		t.Fatalf("half way = %v; the front must travel between the stops", middle)
	}
	if got := transition.value(start.Add(effortTransitionDuration)); got != 1 {
		t.Fatalf("landed at %v; want the target", got)
	}
	if got := transition.value(start.Add(2 * effortTransitionDuration)); got != 1 {
		t.Fatalf("overshot to %v; the curve must rest at the target", got)
	}
}

// A jump re-roots the curve, as a drag moving the fill under the pointer
// does, so letting go eases from where the pointer left it.
func TestEffortTransitionJumpReRootsTheCurve(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var transition effortTransition
	transition.sample(start, 0, false)
	transition.jump(start.Add(50*time.Millisecond), .4)
	if got := transition.value(start.Add(50 * time.Millisecond)); got != .4 {
		t.Fatalf("jump landed at %v; want the pointer's position", got)
	}
	transition.sample(start.Add(50*time.Millisecond), 1, false)
	if got := transition.value(start.Add(50 * time.Millisecond)); got != .4 {
		t.Fatalf("release left %v; the curve must ease from the pointer", got)
	}
}

// The stops and a continuous drag share the rail's inset geometry.
func TestEffortStopsShareTheInsetGeometry(t *testing.T) {
	width := float32(228)
	for count := 2; count <= 8; count++ {
		for index := range count {
			fraction := float32(index) / float32(count-1)
			x := effortThumbInset + fraction*(width-2*effortThumbInset)
			if diff := effortFraction(x, width) - fraction; diff > .00001 || diff < -.00001 {
				t.Fatalf("stop %d of %d maps to %v; want %v", index, count, effortFraction(x, width), fraction)
			}
			if x-2 < 0 || x+2 > width {
				t.Fatalf("stop %d of %d falls outside the rail", index, count)
			}
		}
	}
	if diff := effortFraction(68, width) - .25; diff > .00001 || diff < -.00001 {
		t.Fatalf("a quarter across = %v; want .25", effortFraction(68, width))
	}
	if effortFraction(-100, width) != 0 || effortFraction(500, width) != 1 {
		t.Fatal("the travel must clamp to the rail's ends")
	}
}

// The flowing light's phase advances only while the accent lives.
func TestEffortShimmerPhaseHoldsStillUnlessTheAccentLives(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var motion effortMotion
	motion.advance(start, true)
	motion.advance(start.Add(100*time.Millisecond), true)
	if motion.phase <= 0 {
		t.Fatal("the light did not move while the accent lived")
	}
	held := motion.phase
	motion.advance(start.Add(200*time.Millisecond), false)
	motion.advance(start.Add(300*time.Millisecond), false)
	if motion.phase != held {
		t.Fatalf("the light moved to %v while the accent was still", motion.phase)
	}
}

// The chosen stop reaches the slider's motion state, which then eases to it.
func TestReasoningSliderEasesToTheChosenStop(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select reasoning effort"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Reasoning effort"); err != nil {
		t.Fatal(err)
	}
	levels := a.centralControls().reasoningChoice().levels
	if len(levels) < 2 {
		t.Fatal("the scenario's model has no effort ladder to move along")
	}
	tt.Key(0, ui.KeyHome)
	tt.Advance(2 * effortTransitionDuration)
	motion := &a.centralControls().effortMotion
	if motion.front.to != 0 {
		t.Fatalf("the front rests at %v; want the first stop", motion.front.to)
	}
	tt.Key(0, ui.KeyEnd)
	if motion.front.to != 1 {
		t.Fatalf("the front headed for %v; want the last stop", motion.front.to)
	}
	if got := motion.front.value(motion.front.started); got != 0 {
		t.Fatalf("the front left %v at once; it must travel to the stop", got)
	}
	middle := motion.front.value(motion.front.started.Add(effortTransitionDuration / 2))
	if middle <= 0 || middle >= 1 {
		t.Fatalf("half way = %v; the front must travel between the stops", middle)
	}
	tt.Advance(2 * effortTransitionDuration)
	if got := motion.front.value(motion.front.started.Add(effortTransitionDuration)); got != 1 {
		t.Fatalf("the front rested at %v; want the chosen stop", got)
	}
}

// The filled rail carries the reference's travelling light: two phases of it
// paint different light.
func TestEffortFlowMovesTheLightWithThePhase(t *testing.T) {
	theme := design.Palette("rcode-dark", true)
	effort := design.EffortPaletteFor("aurora", theme)
	paint := func(phase float32) *color.RGBA {
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.UI())
			c.DisableAnimations()
			c.Root().Background(theme.Background)
			modelEffortPlate(c, theme, effort, effortSliderPaint{
				fraction: 1, breath: .5, phase: phase, count: 5, live: true,
			}).Absolute().Left(20).Top(20).Size(200, 28).Label("Effort plate")
		}, 240, 80)
		got := tt.Image().RGBAAt(120, 34)
		return &got
	}
	first, second := paint(0), paint(.5)
	if first == second {
		t.Fatal("the filled rail looks the same in both phases; the light does not move")
	}
}

// effortHandleLeft returns the x of the handle's left edge, which the
// reference paints as an opaque light plate over the rail.
func effortHandleLeft(tt *ui.Tester, slider ui.Rect) int {
	frame := tt.Image()
	y := int(slider.Y + 12)
	for x := int(slider.X); x < int(slider.X+slider.W); x++ {
		got := frame.RGBAAt(x, y)
		if got.R > 230 && got.G > 230 && got.B > 230 && maxChannel(got)-minChannel(got) < 12 {
			return x
		}
	}
	return -1
}

func maxChannel(c color.RGBA) int {
	return max(int(c.R), max(int(c.G), int(c.B)))
}

func minChannel(c color.RGBA) int {
	return min(int(c.R), min(int(c.G), int(c.B)))
}

// The handle travels the rail as the front eases, instead of jumping to the
// chosen stop.
func TestReasoningSliderHandleTravelsTheRail(t *testing.T) {
	a := modelSurfaceTestApp(t, "dark")
	tt := ui.NewTester(a.View, 840, 560)
	tt.Advance(20 * time.Millisecond)
	slider, ok := tt.Find("Reasoning effort")
	if !ok {
		t.Fatal("the reasoning slider is not on screen")
	}
	if levels := a.centralControls().reasoningChoice().levels; len(levels) < 2 {
		t.Fatalf("the surface's model offers %d effort levels; the rail needs a ladder", len(levels))
	}
	tt.Key(0, ui.KeyHome)
	tt.Advance(2 * effortTransitionDuration)
	start := effortHandleLeft(tt, slider)
	if start < 0 {
		t.Fatal("the handle is not painted over the rail")
	}
	tt.Key(0, ui.KeyEnd)
	tt.Advance(effortTransitionDuration / 2)
	middle := effortHandleLeft(tt, slider)
	tt.Advance(effortTransitionDuration)
	end := effortHandleLeft(tt, slider)
	if middle <= start || end <= middle {
		t.Fatalf("the handle stood at %d, %d and %d; it must travel the rail", start, middle, end)
	}
	if end-start < int(slider.W)-44 {
		t.Fatalf("the handle travelled %d DIPs of a %d DIP rail", end-start, int(slider.W)-44)
	}
}
