package desktop

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func dockFrameAt(t *testing.T, d *composerDockState, now time.Time, docked, reduced bool) composerDockFrame {
	t.Helper()
	return d.tick(now, docked, reduced)
}

func TestComposerDockGlideSettlesWithoutOvershoot(t *testing.T) {
	var dock composerDockState
	now := time.Unix(100, 0)
	if frame := dockFrameAt(t, &dock, now, false, false); frame.Amount != 0 || frame.Active {
		t.Fatalf("canvas did not start at rest: %+v", frame)
	}
	previous := float32(0)
	for i := 0; i < 80; i++ {
		now = now.Add(16 * time.Millisecond)
		frame := dockFrameAt(t, &dock, now, true, false)
		if frame.Amount < previous {
			t.Fatalf("critically damped dock overshoot at frame %d: %v < %v", i, frame.Amount, previous)
		}
		if frame.Amount > 1 {
			t.Fatalf("dock left its range: %v", frame.Amount)
		}
		previous = frame.Amount
	}
	final := dockFrameAt(t, &dock, now.Add(time.Second), true, false)
	if final.Active || math.Abs(float64(final.Amount-1)) > .001 {
		t.Fatalf("dock did not settle: %+v", final)
	}
}

func TestComposerDockReversalKeepsItsPosition(t *testing.T) {
	var dock composerDockState
	now := time.Unix(100, 0)
	dockFrameAt(t, &dock, now, false, false)
	previous := float32(0)
	before := composerDockFrame{}
	for i := 0; i < 12; i++ {
		now = now.Add(16 * time.Millisecond)
		before = dockFrameAt(t, &dock, now, true, false)
		previous = before.Amount
	}
	if previous <= 0 || previous >= 1 {
		t.Fatalf("dock did not travel before the reversal: %v", previous)
	}
	// Reversing keeps the painted position and chrome; velocity carries it
	// back instead of restarting at the old endpoint.
	reversed := dockFrameAt(t, &dock, now, false, false)
	if math.Abs(float64(reversed.Amount-previous)) > .02 {
		t.Fatalf("reversal jumped: %v -> %v", previous, reversed.Amount)
	}
	if math.Abs(float64(reversed.Selectors-before.Selectors)) > .02 || math.Abs(float64(reversed.Footer-before.Footer)) > .02 {
		t.Fatalf("reversal revived old chrome: %+v -> %+v", before, reversed)
	}
	for i := 0; i < 40; i++ {
		now = now.Add(16 * time.Millisecond)
		dockFrameAt(t, &dock, now, false, false)
	}
	if frame := dockFrameAt(t, &dock, now, false, false); math.Abs(float64(frame.Amount)) > .001 || frame.Active {
		t.Fatalf("reversed dock did not return to the canvas: %+v", frame)
	}
}

func TestComposerDockStagesChromeOnOneTimeline(t *testing.T) {
	var dock composerDockState
	now := time.Unix(100, 0)
	dockFrameAt(t, &dock, now, false, false)
	start := now
	flip := dockFrameAt(t, &dock, now, true, false)
	if flip.Selectors < .99 || flip.Footer > .01 || flip.Transcript > .05 {
		t.Fatalf("canvas chrome did not lead the docking: %+v", flip)
	}
	early := dockFrameAt(t, &dock, start.Add(composerDockDuration/4), true, false)
	if early.Selectors >= flip.Selectors {
		t.Fatalf("selectors did not leave first: %v", early.Selectors)
	}
	middle := dockFrameAt(t, &dock, start.Add(composerDockDuration/2), true, false)
	if middle.Selectors > .05 {
		t.Fatalf("selectors still painted after a quarter of the handoff: %v", middle.Selectors)
	}
	if middle.Footer <= early.Footer {
		t.Fatalf("footer did not arrive during travel: early %v middle %v", early.Footer, middle.Footer)
	}
	done := dockFrameAt(t, &dock, start.Add(composerDockDuration), true, false)
	if done.Selectors != 0 || done.Footer != 1 || done.Transcript != 1 || done.Dissolve != 1 {
		t.Fatalf("docked chrome did not settle: %+v", done)
	}
}

func TestComposerDockSnapsOnPauseAndReducedMotion(t *testing.T) {
	var dock composerDockState
	now := time.Unix(100, 0)
	dockFrameAt(t, &dock, now, false, false)
	// A frame gap longer than the animation is a pause, not travel.
	paused := dockFrameAt(t, &dock, now.Add(time.Second), true, false)
	if !paused.Flipped || paused.Active || paused.Amount != 1 {
		t.Fatalf("paused route did not snap docked: %+v", paused)
	}
	reduced := dockFrameAt(t, &dock, now.Add(time.Second+16*time.Millisecond), false, true)
	if reduced.Active || reduced.Amount != 0 || reduced.Selectors != 1 {
		t.Fatalf("reduced motion did not snap the canvas: %+v", reduced)
	}
}

func TestComposerMorphGeometryMeetsBothRoutes(t *testing.T) {
	committed := composerRouteStyle{hero: false, expanded: false}
	previous := composerRouteStyle{hero: true, expanded: true, height: 120}
	compactPad := float32(12)
	atRest := composerMorphGeometryOf(committed, previous, 0, 49, compactPad, 200)
	if atRest.height != 49 || atRest.radius != 22 || atRest.inset != 8 {
		t.Fatalf("docked endpoint changed: %+v", atRest)
	}
	if atRest.left != 36 || atRest.right != 244 || atRest.input != 47 {
		t.Fatalf("docked input inset changed: %+v", atRest)
	}
	atCanvas := composerMorphGeometryOf(committed, previous, 1, 49, compactPad, 200)
	if atCanvas.height != 120 || atCanvas.radius != 26 || atCanvas.inset != 12 {
		t.Fatalf("canvas endpoint changed: %+v", atCanvas)
	}
	if atCanvas.left != 16 || atCanvas.right != 16 || atCanvas.input != 118 {
		t.Fatalf("canvas input inset changed: %+v", atCanvas)
	}
	if atCanvas.padding != [4]float32{16, 16, 4, 16} || atCanvas.content != atCanvas.padding {
		t.Fatalf("canvas text padding changed: %+v", atCanvas)
	}
	half := composerMorphGeometryOf(committed, previous, .5, 49, compactPad, 200)
	if math.Abs(float64(half.height-84.5)) > .01 {
		t.Fatalf("halfway height = %v, want 84.5", half.height)
	}
}

// The blank canvas send must start travelling on the key, not when the host
// answers, and the composer must cross every frame's position without a jump.
func TestComposerDocksWithoutJumpingOnSend(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	before, _ := tt.Find("Message")
	tt.Type("Dock this")
	tt.Key(0, ui.KeyEnter)
	if a.errorText != "" {
		t.Fatal(a.errorText)
	}
	type sample struct {
		top    float32
		height float32
	}
	var samples []sample
	for i := 0; i < 40; i++ {
		tt.Advance(16 * time.Millisecond)
		box, ok := tt.Find("Message")
		if !ok {
			t.Fatalf("composer vanished at frame %d", i)
		}
		samples = append(samples, sample{top: box.Y, height: box.H})
	}
	last := samples[len(samples)-1]
	if last.top <= before.Y {
		t.Fatalf("composer did not travel down: %v -> %v", before.Y, last.top)
	}
	if last.height != 47 {
		t.Fatalf("docked input did not settle compact: %v", last.height)
	}
	previous := before.Y
	for i, s := range samples {
		if s.top < previous-.5 {
			t.Fatalf("frame %d moved the composer up: %v -> %v", i, previous, s.top)
		}
		previous = s.top
	}
	if samples[0].top-before.Y > 60 {
		t.Fatalf("dock jumped on its first frame: %v -> %v", before.Y, samples[0].top)
	}
	// The same input entity crosses the handoff: the writer keeps typing
	// without clicking back into the composer.
	tt.Type("still focused")
	tt.Frame()
	if a.draft != "still focused" {
		t.Fatalf("focus did not survive the dock: draft %q", a.draft)
	}
}

// The same handoff must run without motion when the desktop asks for less.
func TestComposerDockHonoursReducedMotion(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.ReduceMotion = true
	a.state.Settings.Appearance.MotionPreference = "reduced"
	// Persist through the service: the send's snapshot would otherwise
	// restore the fixture's motion settings.
	a.saveSettings()
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("No motion")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(16 * time.Millisecond)
	box, ok := tt.Find("Message")
	if !ok {
		t.Fatal("composer missing")
	}
	if box.H != 47 {
		t.Fatalf("reduced motion morphed the pill: height %v", box.H)
	}
}

// A tall blank-canvas draft must hand its height to the dock instead of
// collapsing to the compact row on the send frame.
func TestComposerDockCarriesTheTallDraftHeight(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("line one\nline two\nline three\nline four\nline five")
	canvas, _ := tt.Find("Message")
	if canvas.H <= 120 {
		t.Fatalf("canvas draft did not expand: %v", canvas.H)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Millisecond)
	first, _ := tt.Find("Message")
	if first.H < canvas.H-8 {
		t.Fatalf("dock dropped the tall height on its first frame: %v -> %v", canvas.H, first.H)
	}
	tt.Advance(600 * time.Millisecond)
	settled, _ := tt.Find("Message")
	if settled.H != 47 {
		t.Fatalf("dock did not settle compact: %v", settled.H)
	}
}
