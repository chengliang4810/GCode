//go:build uifixture

package workbench

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestTabStripFixtureProductionMatrix(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		for _, width := range []int{400, 220} {
			for _, count := range []int{0, 2, 6} {
				for _, fraction := range []float32{0, .5, 1} {
					for _, scale := range []float32{1, 2} {
						t.Run(fmt.Sprintf("%s-w%d-n%d-f%g-s%g", mode, width, count, fraction, scale), func(t *testing.T) {
							scene := TabStripFixtureScene{mode, width, count, fraction}
							fixture, err := NewTabStripFixture(scene)
							if err != nil {
								t.Fatal(err)
							}
							t.Cleanup(fixture.Close)
							tt := ui.NewTester(fixture.View, 840, 560)
							tt.SetScale(scale)
							tt.Advance(time.Second)
							state := fixture.State()
							if !state.Ready() || state.Scene != scene || state.NativeWindow {
								t.Fatalf("unsettled or mismatched controlled state: %+v", state)
							}
							if state.BuildPasses < state.FrameSamples || state.FrameSamples < 4 {
								t.Fatalf("stable samples do not represent distinct completed frames: %+v", state)
							}
							if state.Geometry.Host != (TabStripFixtureBounds{40, 40, float32(width), 38}) || state.Geometry.ScrollViewport != (TabStripFixtureBounds{48, 44, float32(width - 44), 34}) {
								t.Fatalf("measured host/scroller differs from production layout: %+v", state.Geometry)
							}
							if state.Geometry.Expand != (TabStripFixtureBounds{float32(40 + width - 32), 47, 28, 28}) {
								t.Fatalf("expand did not keep its production slot: %+v", state.Geometry.Expand)
							}
							for i, chip := range state.Geometry.Chips {
								if chip.SessionID != fmt.Sprintf("side-%02d", i+1) || chip.Title != fmt.Sprintf("Tab%02d", i+1) || chip.Kind != "session" || chip.Icon != "chat-round-line" || !chip.Idle || chip.Bounds.Width != 112 || chip.Bounds.Height != 24 {
									t.Fatalf("typed side-session/production icon or bounds changed: %+v", chip)
								}
								actual, ok := tt.Find(chip.Title)
								visible := tabStripFixtureVisible(chip.Bounds, state.Geometry.ScrollViewport)
								if !ok || actual.W != visible.Width || actual.H != visible.Height || actual.X != visible.X || actual.Y != visible.Y {
									t.Fatalf("committed box is not the rendered production chip: bounds=%+v clipped=%+v tester=%+v", chip.Bounds, visible, actual)
								}
							}
							before := state.BuildPasses
							tt.Advance(time.Second)
							if tt.FramePending() || fixture.State().BuildPasses != before {
								t.Fatal("ready fixture continued forcing frames")
							}
						})
					}
				}
			}
		}
	}
}

func tabStripFixtureVisible(bounds, clip TabStripFixtureBounds) TabStripFixtureBounds {
	x, y := max(bounds.X, clip.X), max(bounds.Y, clip.Y)
	w, h := min(bounds.X+bounds.Width, clip.X+clip.Width)-x, min(bounds.Y+bounds.Height, clip.Y+clip.Height)-y
	if w <= 0 || h <= 0 {
		return TabStripFixtureBounds{x, y, 0, 0}
	}
	return TabStripFixtureBounds{x, y, w, h}
}

func TestTabStripFixtureObserverDoesNotChangeRendering(t *testing.T) {
	for _, mode := range []string{"dark", "light"} {
		for _, width := range []int{400, 220} {
			for _, fraction := range []float32{0, .5, 1} {
				t.Run(fmt.Sprintf("%s-w%d-f%g", mode, width, fraction), func(t *testing.T) {
					var testers []*ui.Tester
					for _, bind := range []bool{false, true} {
						fixture, err := NewTabStripFixture(TabStripFixtureScene{mode, width, 6, fraction})
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(fixture.Close)
						if !bind {
							fixture.workspace.tabObserver = tabStripObserver{}
						}
						tt := ui.NewTester(fixture.View, 840, 560)
						tt.Advance(time.Second)
						testers = append(testers, tt)
					}
					if !bytes.Equal(testers[0].Image().Pix, testers[1].Image().Pix) {
						t.Fatal("binding observation handles changed rendered pixels")
					}
					for _, label := range []string{"Fixture host", "Workspace tabs", "Scroll workspace tabs", "Tab01", "Tab06", "New surface", "Expand workspace"} {
						a, okA := testers[0].Find(label)
						b, okB := testers[1].Find(label)
						if a != b || okA != okB {
							t.Fatalf("observer changed %q geometry: %+v %+v", label, a, b)
						}
					}
				})
			}
		}
	}
}

func TestTabStripFixtureRequiresDistinctCommittedFrames(t *testing.T) {
	fixture, err := NewTabStripFixture(TabStripFixtureScene{"light", 220, 6, .5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 840, 560)
	for i := 0; i < 6; i++ {
		tt.Frame()
	}
	if fixture.State().Ready() || fixture.State().FrameSamples != 1 {
		t.Fatal("same-clock builds were counted as distinct stable frames")
	}
	tt.Advance(time.Second)
	if !fixture.State().Ready() {
		t.Fatalf("actual scheduled frames did not settle: %+v", fixture.State())
	}
	if err := fixture.SetScene(TabStripFixtureScene{"dark", 400, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if fixture.State().Ready() {
		t.Fatal("previous scene remained ready after changing scene")
	}
	tt.Frame()
	tt.Advance(time.Second)
	state := fixture.State()
	if !state.Ready() || state.ActiveIndex != nil || state.Geometry.Plus != nil || len(state.Geometry.Chips) != 0 || state.MaxX != 0 {
		t.Fatalf("empty scene leaked old state: %+v", state)
	}
}

func TestTabStripFixtureReportsActualPointerAndHover(t *testing.T) {
	fixture, err := NewTabStripFixture(TabStripFixtureScene{"light", 400, 2, 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 840, 560)
	tt.Move(10, 10)
	tt.Advance(time.Second)
	pointer := fixture.State().Pointer
	if !pointer.MoveSeen || pointer.LastMoveX != 10 || pointer.LastMoveY != 10 || pointer.AnyControlHovered {
		t.Fatalf("outside-host input was not observed: %+v", pointer)
	}
	chip, _ := tt.Find("Tab01")
	tt.Move(chip.X+chip.W/2, chip.Y+chip.H/2)
	tt.Advance(time.Second)
	if !fixture.State().Pointer.AnyControlHovered {
		t.Fatal("production hover was hidden by fixture metadata")
	}
	tt.Move(10, 10)
	tt.Advance(time.Second)
	if fixture.State().Pointer.AnyControlHovered {
		t.Fatal("production hover did not clear")
	}
}

func TestTabStripFixtureTypedServiceAndValidation(t *testing.T) {
	valid := TabStripFixtureScene{"dark", 400, 6, 0}
	fixture, err := NewTabStripFixture(valid)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	snap, err := fixture.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 7 || len(snap.Providers) != 0 || len(snap.Models) != 0 {
		t.Fatal("fixture did not use isolated side-session data")
	}
	for i, session := range snap.Sessions {
		if session.Status != contracts.Idle || i > 0 && (!session.SideChat || session.ParentSessionID != "parent") {
			t.Fatalf("wrong session relationship: %+v", session)
		}
	}
	snap.Sessions[1].Title = "mutated"
	again, _ := fixture.service.Snapshot(context.Background())
	if again.Sessions[1].Title != "Tab01" {
		t.Fatal("typed snapshot shares mutable data")
	}
	if fixture.service.Execute(context.Background(), contracts.Command{Kind: "session.send"}) == nil {
		t.Fatal("read-only fixture executed a backend mutation")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.service.Snapshot(cancelled); err == nil {
		t.Fatal("cancelled service read succeeded")
	}
	for _, bad := range []TabStripFixtureScene{{"system", 400, 6, 0}, {"dark", 300, 6, 0}, {"dark", 400, 4, 0}, {"dark", 400, 6, -1}, {"dark", 400, 6, float32(math.NaN())}} {
		if err := fixture.SetScene(bad); err == nil {
			t.Fatalf("invalid scene accepted: %+v", bad)
		}
		if fixture.scene != valid {
			t.Fatal("invalid scene changed current data")
		}
	}
}
