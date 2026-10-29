//go:build uifixture

package workbench

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTabStripStyleFixtureObservedProductionState(t *testing.T) {
	for _, appearance := range []string{"dark", "light"} {
		for _, style := range []string{"inactive-hover", "dirty-file", "file-hover", "file-hover-edge"} {
			for _, scale := range []float32{1, 2} {
				fixture, err := NewTabStripFixture(TabStripFixtureScene{appearance, 400, 2, 0})
				if err != nil {
					t.Fatal(err)
				}
				if err := fixture.SetStyleCase(style); err != nil {
					t.Fatal(err)
				}
				tt := ui.NewTester(fixture.View, 840, 560)
				tt.SetScale(scale)
				tt.Advance(time.Second)
				state := fixture.State()
				x, y, valid := TabStripFixturePointerTarget(state)
				if !valid || !state.Ready() {
					t.Fatalf("style layout did not settle: %+v", state)
				}
				tt.Move(x, y)
				tt.Advance(time.Second)
				state = fixture.State()
				if !state.Ready() || state.StyleCase != style || !TabStripFixturePointerReady(state) {
					t.Fatalf("%s/%s/scale%g state not observed: %+v", appearance, style, scale, state)
				}
				if tabStripStyleUsesFiles(style) {
					if len(fixture.workspace.tabs) != 2 || state.Geometry.Chips[0].SessionID != "file-01" || state.Geometry.Chips[0].Kind != "file" || state.Geometry.Chips[0].Title != "Tab01.txt" || !fixture.workspace.documents["file-01"].editor.Buffer.Modified() || fixture.workspace.documents["file-02"].editor.Buffer.Modified() {
						t.Fatal("dirty supplement did not use real file documents and buffer modification")
					}
					state.StyleEvidence.DirtyFileIDs[0] = "mutated"
					if fixture.State().StyleEvidence.DirtyFileIDs[0] != "file-01" {
						t.Fatal("style evidence shared mutable slices")
					}
				}
				if style != "dirty-file" {
					tt.Move(10, 10)
					tt.Advance(time.Second)
					if TabStripFixturePointerReady(fixture.State()) {
						t.Fatal("hover supplement accepted pointer outside target")
					}
				}
				fixture.Close()
			}
		}
	}
}

func TestTabStripEdgeFixtureUsesMeasuredHostAndTypedLongPaths(t *testing.T) {
	const cardObservation = "edge fixture tooltip card observation"
	for _, appearance := range []string{"dark", "light"} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s-scale%g", appearance, scale), func(t *testing.T) {
				f, err := NewTabStripFixture(TabStripFixtureScene{appearance, 400, 2, 0})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(f.Close)
				if err := f.SetStyleCase("file-hover-edge"); err != nil {
					t.Fatal(err)
				}
				tt := ui.NewTester(func(c *ui.Context) {
					f.View(c)
					// Resolve the production card by its durable identity. This
					// label exists only in this test, so Find can still observe
					// the visible clipped box independently of committed bounds.
					if index := f.workspace.tabTooltip.shown; index >= 0 && index < len(f.workspace.tabTooltip.owners) {
						c.Resolve(f.workspace.tabTooltip.owners[index].card).Label(cardObservation)
					}
				}, 840, 560)
				tt.SetScale(scale)
				tt.Advance(time.Second)
				state := f.State()
				if !state.Ready() || state.Geometry.Host != (TabStripFixtureBounds{400, 510, 400, 38}) {
					t.Fatalf("edge host not committed inside client: %+v", state)
				}
				for i, chip := range state.Geometry.Chips {
					name := fmt.Sprintf("Tab%02d.txt", i+1)
					wantPath := "workspace/long-component-path/long-component-path/long-component-path/" + name
					doc := f.workspace.documents[chip.SessionID]
					if doc == nil || doc.file.Path != wantPath || chip.Title != name || chip.Kind != "file" || f.workspace.tabDetail(f.workspace.tabs[i]) != wantPath || doc.editor.Buffer.Modified() != (i == 0) {
						t.Fatalf("edge did not retain actual file/title/detail/dirty state: %+v", chip)
					}
				}
				x, y, valid := TabStripFixturePointerTarget(state)
				chip := state.Geometry.Chips[1].Bounds
				if !valid || x != chip.X+chip.Width/2 || y != chip.Y+chip.Height/2 || x != 580 || y != 531 {
					t.Fatalf("target is not measured second chip center: %g,%g %+v", x, y, chip)
				}
				tt.Move(x, y)
				tt.Advance(time.Second)
				if state = f.State(); !state.Ready() || !TabStripFixturePointerReady(state) || state.StyleEvidence.HoveredTabID != "file-02" {
					t.Fatalf("edge actual production hover not observed: %+v", state)
				}
				edge := state.StyleEvidence.Edge
				if edge == nil || len(edge.FilePaths) != 2 || edge.FilePaths[1] != f.workspace.documents["file-02"].file.Path {
					t.Fatal("edge metadata did not observe typed file paths")
				}
				tip := edge.Tooltip
				if !tip.ManagerVisible || tip.ShownOwnerIndex != 2 || tip.SlotIndex != 1 || tip.Close || tip.Phase != "visible" || tip.Text != edge.FilePaths[1] || tip.ShowPoint != [2]float32{x, y} {
					t.Fatalf("edge metadata did not observe the real detail winner: %+v", tip)
				}
				bounds := tip.CommittedBounds
				if bounds.Width <= 220 || bounds.Width > 380 || bounds.Height <= 0 || bounds.X >= x || bounds.Y >= y || bounds.X+bounds.Width > 840 || bounds.Y+bounds.Height > 560 {
					t.Fatalf("long tooltip did not flip both axes inside client: %+v point %g,%g", bounds, x, y)
				}
				actual, ok := tt.Find(cardObservation)
				// Find returns a clipped rectangle, recomputing its height by
				// subtracting its two float32 edges; Handle.Bounds keeps the
				// committed height. Preserve both observations without rounding.
				if !ok || actual.X != bounds.X || actual.Y != bounds.Y || actual.W != bounds.Width || math.Abs(float64(actual.H-bounds.Height)) > .0001 || tip.ObservedAtBuildPass <= 0 || tip.ObservedAtBuildPass > state.BuildPasses || tip.ObservedAt.IsZero() || !strings.Contains(tip.BoundsSource, "previous committed layout") {
					t.Fatalf("tooltip committed-bound observation is missing provenance: %+v actual %+v", tip, actual)
				}
				edge.FilePaths[1] = "mutated"
				edge.Tooltip.Text = "mutated"
				if f.State().StyleEvidence.Edge.FilePaths[1] == "mutated" || f.State().StyleEvidence.Edge.Tooltip.Text == "mutated" {
					t.Fatal("edge metadata shared mutable returned state")
				}
				closeBounds, ok := tt.Find("Close tab Tab02.txt")
				if !ok {
					t.Fatal("production close control not found")
				}
				tt.Move(closeBounds.X+closeBounds.W/2, closeBounds.Y+closeBounds.H/2)
				tt.Advance(time.Second)
				winner := f.State().StyleEvidence.Edge.Tooltip
				if !winner.ManagerVisible || winner.ShownOwnerIndex != 3 || !winner.Close || winner.Text != "Close tab" || f.workspace.tabTooltip.owners[2].phase != tabTooltipVisible {
					t.Fatalf("observer confused visible parent with actual close overlay winner: %+v", winner)
				}
			})
		}
	}
}

func TestTabStripEdgeMetadataDoesNotChangeOlderStyles(t *testing.T) {
	for _, style := range []string{"", "inactive-hover", "dirty-file", "file-hover"} {
		t.Run("style-"+style, func(t *testing.T) {
			f, err := NewTabStripFixture(TabStripFixtureScene{"dark", 400, 2, 0})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(f.Close)
			if err := f.SetStyleCase(style); err != nil {
				t.Fatal(err)
			}
			tt := ui.NewTester(f.View, 840, 560)
			tt.Advance(time.Second)
			state := f.State()
			data, err := json.Marshal(state)
			if err != nil || strings.Contains(string(data), `"edge"`) || state.Geometry.Host != (TabStripFixtureBounds{40, 40, 400, 38}) {
				t.Fatalf("older style acquired edge metadata or host: %s %v", data, err)
			}
			if style == "dirty-file" || style == "file-hover" {
				for _, document := range f.workspace.documents {
					if document.file.Path != document.file.Name {
						t.Fatal("older file style acquired long edge paths")
					}
				}
			}
		})
	}
}

func TestTabStripEdgeTooltipMaturationPreservesStripReadiness(t *testing.T) {
	for _, appearance := range []string{"dark", "light"} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s-scale%g", appearance, scale), func(t *testing.T) {
				f, err := NewTabStripFixture(TabStripFixtureScene{appearance, 400, 2, 0})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(f.Close)
				if err := f.SetStyleCase("file-hover-edge"); err != nil {
					t.Fatal(err)
				}
				type observation struct {
					at    time.Time
					state TabStripFixtureState
				}
				var samples []observation
				collect := false
				tt := ui.NewTester(func(c *ui.Context) {
					f.View(c)
					if collect {
						samples = append(samples, observation{c.Now(), f.State()})
					}
				}, 840, 560)
				tt.SetScale(scale)
				tt.Advance(time.Second)
				x, y, valid := TabStripFixturePointerTarget(f.State())
				if !valid {
					t.Fatal("production chip not committed")
				}
				hoverAt := tt.Now()
				tt.Move(x, y)
				tt.Advance(349 * time.Millisecond)
				before := f.State()
				if !before.Ready() || !TabStripFixturePointerReady(before) || before.StyleEvidence.Edge.Tooltip.ManagerVisible {
					t.Fatalf("pre-deadline hover was not a settled strip without overlay: %+v", before)
				}
				collect = true
				tt.Advance(6 * time.Millisecond)
				shownWithoutCommittedBounds, committed := false, false
				for _, sample := range samples {
					state := sample.state
					tip := state.StyleEvidence.Edge.Tooltip
					if !reflect.DeepEqual(state.Geometry, before.Geometry) || state.Pointer != before.Pointer || !reflect.DeepEqual(state.StyleEvidence.Edge.FilePaths, before.StyleEvidence.Edge.FilePaths) {
						t.Fatal("test changed actual strip geometry, input or typed paths")
					}
					if tip.ManagerVisible && tip.CommittedBounds.Width == 0 {
						shownWithoutCommittedBounds = true
					}
					if tip.ManagerVisible && tip.CommittedBounds.Width > 0 {
						committed = true
					}
					if !state.Ready() || !state.GeometryStable || state.StableFrames != 2 {
						t.Errorf("tooltip-only transition reset strip readiness at %s: shown=%v bounds=%+v stable=%v/%d", sample.at.Sub(hoverAt), tip.ManagerVisible, tip.CommittedBounds, state.GeometryStable, state.StableFrames)
					}
				}
				if len(samples) == 0 || !shownWithoutCommittedBounds || !committed {
					t.Fatalf("did not observe actual shown/previous-commit-zero/committed transitions: samples=%d shownWithoutBounds=%v committed=%v", len(samples), shownWithoutCommittedBounds, committed)
				}
				t.Logf("actual tooltip transitions retained across %d build observations; strip pointer stayed at %g,%g", len(samples), x, y)
			})
		}
	}
}

func TestTabStripEdgeActualGeometryAndHoverStillResetReadiness(t *testing.T) {
	for _, change := range []string{"host position", "actual hover"} {
		t.Run(change, func(t *testing.T) {
			f, err := NewTabStripFixture(TabStripFixtureScene{"light", 400, 2, 0})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(f.Close)
			if err := f.SetStyleCase("file-hover-edge"); err != nil {
				t.Fatal(err)
			}
			shift := false
			tt := ui.NewTester(func(c *ui.Context) {
				f.View(c)
				if shift {
					// Change the actual current host element before layout. No
					// recorded bounds or observation fields are edited by the test.
					c.Resolve(f.host).Left(405)
				}
			}, 840, 560)
			tt.Advance(time.Second)
			x, y, _ := TabStripFixturePointerTarget(f.State())
			tt.Move(x, y)
			tt.Advance(100 * time.Millisecond)
			before := f.State()
			if !before.Ready() || !TabStripFixturePointerReady(before) {
				t.Fatal("strip did not settle before the actual change")
			}
			if change == "host position" {
				shift = true
				tt.Frame() // commits the actual new position
				tt.Advance(time.Millisecond)
				tt.Frame() // reads that committed position at a distinct time
				state := f.State()
				if state.Geometry.Host.X != 405 || state.Ready() || state.GeometryStable || state.StableFrames != 0 {
					t.Fatalf("actual host movement was treated as tooltip-only diagnostic change: %+v", state)
				}
			} else {
				tt.Move(10, 10)
				state := f.State()
				if !reflect.DeepEqual(state.Geometry, before.Geometry) || state.Pointer.AnyControlHovered || TabStripFixturePointerReady(state) || state.Ready() || state.GeometryStable {
					t.Fatalf("actual hover loss was hidden by tooltip diagnostic exception: %+v", state)
				}
			}
		})
	}
}

func TestTabStripStyleFixtureKeepsBaselineSeparate(t *testing.T) {
	f, err := NewTabStripFixture(TabStripFixtureScene{"dark", 400, 2, 0})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tt := ui.NewTester(f.View, 840, 560)
	tt.Advance(time.Second)
	b, err := json.Marshal(f.State())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "styleCase") || strings.Contains(string(b), "styleEvidence") || len(f.workspace.snapshot.Files) != 0 {
		t.Fatal("baseline acquired supplement fields or file data")
	}
	if f.SetStyleCase("unrecognized") == nil {
		t.Fatal("unknown supplement accepted")
	}
	if err := f.SetStyleCase("dirty-file"); err != nil {
		t.Fatal(err)
	}
	if f.SetScene(TabStripFixtureScene{"dark", 220, 6, .5}) == nil {
		t.Fatal("supplement accepted baseline matrix dimensions")
	}
}

func TestTabStripFixtureNewPointerCannotReuseOldStableFrames(t *testing.T) {
	f, err := NewTabStripFixture(TabStripFixtureScene{"dark", 400, 2, 0})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tt := ui.NewTester(f.View, 840, 560)
	tt.Move(10, 10)
	tt.Advance(time.Second)
	if !f.State().Ready() {
		t.Fatal("baseline did not settle")
	}
	// Model a message received between the last committed frame and the CLI
	// UI-thread read. Old geometry remains valid; input readiness does not.
	f.pointer.MoveCount++
	if f.State().Ready() {
		t.Fatal("new input reused two old stable frames")
	}
	tt.Frame()
	tt.Advance(time.Second)
	if !f.State().Ready() {
		t.Fatal("new input failed to obtain its own committed stable frames")
	}
}
