//go:build uifixture

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
)

func TestTabStripFixtureHoverDwellFlags(t *testing.T) {
	for _, test := range []struct {
		name  string
		args  []string
		valid bool
		want  time.Duration
	}{
		{"default baseline", nil, true, 0},
		{"explicit zero baseline", []string{"--hover-dwell=0"}, true, 0},
		{"session hover", []string{"--style-case=inactive-hover", "--hover-dwell=1500ms"}, true, 1500 * time.Millisecond},
		{"file hover", []string{"--style-case=file-hover", "--hover-dwell=1ns"}, true, time.Nanosecond},
		{"edge file hover", []string{"--style-case=file-hover-edge", "--hover-dwell=1s"}, true, time.Second},
		{"maximum", []string{"--style-case=file-hover", "--hover-dwell=10s"}, true, 10 * time.Second},
		{"file hover zero", []string{"--style-case=file-hover"}, true, 0},
		{"baseline positive", []string{"--hover-dwell=1s"}, false, 0},
		{"dirty positive", []string{"--style-case=dirty-file", "--hover-dwell=1s"}, false, 0},
		{"negative", []string{"--style-case=inactive-hover", "--hover-dwell=-1ns"}, false, 0},
		{"over maximum", []string{"--style-case=file-hover", "--hover-dwell=10.000000001s"}, false, 0},
		{"invalid duration", []string{"--style-case=file-hover", "--hover-dwell=NaN"}, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--out-dir", filepath.Join(t.TempDir(), "fresh")}, test.args...)
			o, err := parseOptions(args, &bytes.Buffer{})
			if (err == nil) != test.valid || test.valid && o.hoverDwell != test.want {
				t.Fatalf("flags: %+v %v", o, err)
			}
			if test.valid && o.styleCase == "" && len(scenes(o)) != 36 {
				t.Fatal("dwell default changed baseline matrix")
			}
		})
	}
}

func TestTabStripFixtureDwellMetadataIsOptIn(t *testing.T) {
	state := workbench.TabStripFixtureState{Scene: workbench.TabStripFixtureScene{Appearance: "dark", HostWidth: 400, Count: 0}}
	native := windowMetrics{PID: 1, HWND: 2}
	baseline, err := attachFixtureMetadata(filepath.Join(t.TempDir(), "baseline.json"), state, native, native, nil, "client")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(baseline, &fields); err != nil || len(fields) != 3 || fields["fixtureHoverDwell"] != nil {
		t.Fatal("baseline transport acquired dwell metadata")
	}
	record, err := json.Marshal(captureRecord{State: state})
	if err != nil || strings.Contains(string(record), "hoverDwell") {
		t.Fatal("baseline manifest acquired dwell metadata")
	}
	path := filepath.Join(t.TempDir(), "dwell.json")
	original := []byte("{\"pixelWidth\":1684,\"opaqueTransportField\":[true,1]}\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	dwell := &hoverDwellEvidence{RequestedNanoseconds: int64(time.Second), ObservationCount: 2}
	data, err := attachFixtureMetadata(path, state, native, native, dwell, "wgc")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fields); err != nil || fields["fixtureHoverDwell"] == nil {
		t.Fatal("dwell metadata replaced transport fields")
	}
	var transport []any
	if err := json.Unmarshal(fields["opaqueTransportField"], &transport); err != nil || !reflect.DeepEqual(transport, []any{true, float64(1)}) {
		t.Fatal("dwell metadata changed an original transport value")
	}
	var saved hoverDwellEvidence
	if err := json.Unmarshal(fields["fixtureHoverDwell"], &saved); err != nil || !reflect.DeepEqual(saved, *dwell) {
		t.Fatal("dwell metadata did not round trip")
	}
	backup, err := os.ReadFile(path + ".transport-original.json")
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatal("raw transport bytes changed")
	}
	path = filepath.Join(t.TempDir(), "conflicting.json")
	conflicting := []byte(`{"fixtureHoverDwell":{"external":true}}`)
	if err := os.WriteFile(path, conflicting, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := attachFixtureMetadata(path, state, native, native, dwell, "wgc"); err == nil {
		t.Fatal("preexisting dwell field accepted")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, conflicting) {
		t.Fatal("rejected transport changed")
	}
}

// Production hover/layout comes from Tester. The native identity values below
// are explicit unit-test inputs, not evidence of a native foreground window.
func dwellTestSnapshot(t *testing.T, style string) resultSnapshot {
	t.Helper()
	f, err := workbench.NewTabStripFixture(workbench.TabStripFixtureScene{Appearance: "dark", HostWidth: 400, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	if err := f.SetStyleCase(style); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(f.View, 840, 560)
	tt.Advance(time.Second)
	x, y, valid := workbench.TabStripFixturePointerTarget(f.State())
	if !valid {
		t.Fatal("production chip was not laid out")
	}
	tt.Move(x, y)
	tt.Advance(time.Second)
	state := f.State()
	if !state.Ready() || !workbench.TabStripFixturePointerReady(state) {
		t.Fatal("production hover did not settle")
	}
	state.NativeWindow = true
	return resultSnapshot{state, windowMetrics{PID: 1, HWND: 2, DPI: 192, ActualScale: 2, PointerTargetHWND: 3, ForegroundHWND: 2, NativeWindowActive: true, PointerMessagesSent: true}}
}

func TestHoverDwellRequiresStableTargetAndNativeIdentity(t *testing.T) {
	expected := dwellTestSnapshot(t, "file-hover")
	for _, test := range []struct {
		name   string
		change func(*resultSnapshot)
		valid  bool
	}{
		{"unchanged", func(*resultSnapshot) {}, true},
		{"duplicate received position", func(r *resultSnapshot) {
			r.State.Pointer.MoveCount++
			r.State.GeometryStable = false
			r.State.StableFrames = 0
		}, true},
		{"no committed readiness without duplicate", func(r *resultSnapshot) { r.State.GeometryStable = false }, false},
		{"pointer left", func(r *resultSnapshot) { r.State.Pointer.LastMoveX++ }, false},
		{"hover lost", func(r *resultSnapshot) { r.State.Pointer.AnyControlHovered = false }, false},
		{"hover changed", func(r *resultSnapshot) { r.State.StyleEvidence.HoveredTabID = "file-01" }, false},
		{"dirty data changed", func(r *resultSnapshot) { r.State.StyleEvidence.DirtyFileIDs = nil }, false},
		{"geometry changed", func(r *resultSnapshot) { r.State.Geometry.Host.Width-- }, false},
		{"scene changed", func(r *resultSnapshot) { r.State.Scene.Appearance = "light" }, false},
		{"input count rolled back", func(r *resultSnapshot) { r.State.Pointer.MoveCount-- }, false},
		{"foreground changed", func(r *resultSnapshot) { r.Native.ForegroundHWND = 4 }, false},
		{"window moved", func(r *resultSnapshot) { r.Native.WindowBounds.X++ }, false},
		{"native target changed", func(r *resultSnapshot) { r.Native.PointerTargetHWND++ }, false},
		{"native detached", func(r *resultSnapshot) { r.State.NativeWindow = false }, false},
		{"native messages unavailable", func(r *resultSnapshot) { r.Native.PointerMessagesSent = false }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := expected
			style := *expected.State.StyleEvidence
			current.State.StyleEvidence = &style
			test.change(&current)
			if err := validateHoverDwellSnapshot(current, expected, expected); (err == nil) != test.valid {
				t.Fatalf("dwell gate: %v", err)
			}
		})
	}
}

func TestHoverDwellDoesNotReuseOldDuplicateInput(t *testing.T) {
	expected := dwellTestSnapshot(t, "file-hover")
	previous := expected
	previous.State.Pointer.MoveCount++
	current := previous
	current.State.GeometryStable = false
	if validateHoverDwellSnapshot(current, expected, previous) == nil {
		t.Fatal("old duplicate input concealed a new loss of committed readiness")
	}
}

func TestHoverDwellAllowsActualEdgeTooltipMaturationOnly(t *testing.T) {
	actual := dwellTestSnapshot(t, "file-hover-edge")
	expected := actual
	style := *expected.State.StyleEvidence
	edge := *style.Edge
	edge.Tooltip = workbench.TabStripFixtureTooltipEvidence{ShownOwnerIndex: -1, SlotIndex: -1, Phase: "none"}
	style.Edge = &edge
	expected.State.StyleEvidence = &style
	if err := validateHoverDwellSnapshot(actual, expected, expected); err != nil {
		t.Fatalf("natural tooltip maturation was mistaken for input/scene drift: %v", err)
	}
	if validateCapturedState(actual.State, expected.State, expected.Native, actual.Native) == nil {
		t.Fatal("capture pre/post gate ignored changed actual tooltip observation")
	}
	changed := actual
	changedStyle := *actual.State.StyleEvidence
	changedEdge := *changedStyle.Edge
	changedEdge.FilePaths = []string{"different path"}
	changedStyle.Edge = &changedEdge
	changed.State.StyleEvidence = &changedStyle
	if validateHoverDwellSnapshot(changed, expected, expected) == nil {
		t.Fatal("tooltip maturation exception concealed changed typed file data")
	}
}

func TestHoverDwellActualTooltipFramesKeepReadyAndCaptureGateStrict(t *testing.T) {
	f, err := workbench.NewTabStripFixture(workbench.TabStripFixtureScene{Appearance: "light", HostWidth: 400, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	if err := f.SetStyleCase("file-hover-edge"); err != nil {
		t.Fatal(err)
	}
	var frames []workbench.TabStripFixtureState
	collect := false
	tt := ui.NewTester(func(c *ui.Context) {
		f.View(c)
		if collect {
			frames = append(frames, f.State())
		}
	}, 840, 560)
	tt.Advance(time.Second)
	x, y, valid := workbench.TabStripFixturePointerTarget(f.State())
	if !valid {
		t.Fatal("actual production chip was not committed")
	}
	tt.Move(x, y)
	tt.Advance(349 * time.Millisecond)
	initial := f.State()
	if !initial.Ready() || initial.StyleEvidence.Edge.Tooltip.ManagerVisible {
		t.Fatal("initial hover did not settle before the production tooltip deadline")
	}
	// Only native identity is explicit unit input here. Readiness, strip
	// geometry, actual hover and complete tooltip observations come unmodified
	// from the production fixture and its Tester's advanced frame clock.
	native := windowMetrics{PID: 1, HWND: 2, DPI: 192, ActualScale: 2, PointerTargetHWND: 3, ForegroundHWND: 2, NativeWindowActive: true, PointerMessagesSent: true}
	initial.NativeWindow = true
	expected := resultSnapshot{initial, native}
	previous := expected
	collect = true
	tt.Advance(6 * time.Millisecond)
	shownWithoutBounds, committed := false, false
	for _, state := range frames {
		state.NativeWindow = true
		current := resultSnapshot{state, native}
		if !state.Ready() {
			t.Fatal("actual tooltip maturation invalidated strip readiness")
		}
		if err := validateHoverDwellSnapshot(current, expected, previous); err != nil {
			t.Fatalf("actual tooltip maturation failed the unchanged dwell input gate: %v", err)
		}
		tip := state.StyleEvidence.Edge.Tooltip
		shownWithoutBounds = shownWithoutBounds || tip.ManagerVisible && tip.CommittedBounds.Width == 0
		committed = committed || tip.ManagerVisible && tip.CommittedBounds.Width > 0
		previous = current
	}
	if !shownWithoutBounds || !committed {
		t.Fatal("test did not retain the actual previous-commit-zero and committed tooltip frames")
	}
	if validateCapturedState(previous.State, expected.State, native, native) == nil {
		t.Fatal("capture pre/post gate ignored the complete changed tooltip snapshot")
	}
	if err := validateCapturedState(previous.State, previous.State, native, native); err != nil {
		t.Fatalf("final unchanged actual capture snapshot was rejected: %v", err)
	}
}

func TestHoverDwellUsesObservedIntervalAndFinalCommittedState(t *testing.T) {
	expected := dwellTestSnapshot(t, "inactive-hover")
	base := time.Now()
	duplicate := expected
	duplicate.State.Pointer.MoveCount++
	duplicate.State.GeometryStable, duplicate.State.StableFrames = false, 0
	committed := duplicate
	committed.State.GeometryStable, committed.State.StableFrames = true, 2
	duplicateAgain := duplicate
	duplicateAgain.State.Pointer.MoveCount++
	committedAgain := duplicateAgain
	committedAgain.State.GeometryStable, committedAgain.State.StableFrames = true, 2
	samples := []timedHoverSnapshot{
		{duplicate, base}, // No dwell credit before this input's committed frames.
		{committed, base.Add(100 * time.Millisecond)},
		{duplicateAgain, base.Add(250 * time.Millisecond)}, // Elapsed, but not Ready.
		{committedAgain, base.Add(260 * time.Millisecond)},
	}
	i := 0
	actual, evidence, err := monitorHoverDwell(context.Background(), expected, 150*time.Millisecond, func(context.Context) (timedHoverSnapshot, error) {
		if i >= len(samples) {
			t.Fatal("dwell failed to accept final committed sample")
		}
		sample := samples[i]
		i++
		return sample, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if i != 4 || evidence.ObservationCount != 3 || evidence.ElapsedNanoseconds != int64(160*time.Millisecond) || !evidence.StartedAt.Equal(samples[1].ObservedAt) || !evidence.EndedAt.Equal(samples[3].ObservedAt) || !actual.State.Ready() || actual.State.Pointer.MoveCount != committedAgain.State.Pointer.MoveCount || !reflect.DeepEqual(evidence.EndSnapshot, actual) {
		t.Fatalf("incorrect observed dwell: %+v", evidence)
	}
	if evidence.Observations[1].GeometryStable || evidence.Observations[1].PointerMoveCount != committedAgain.State.Pointer.MoveCount {
		t.Fatal("duplicate input's transient readiness was hidden")
	}
}

func TestHoverDwellRetainsFailureAndCancellationEvidence(t *testing.T) {
	expected := dwellTestSnapshot(t, "file-hover")
	base := time.Now()
	changed := expected
	changed.Native.ForegroundHWND = 4
	i := 0
	actual, evidence, err := monitorHoverDwell(context.Background(), expected, time.Second, func(context.Context) (timedHoverSnapshot, error) {
		i++
		if i == 1 {
			return timedHoverSnapshot{expected, base}, nil
		}
		return timedHoverSnapshot{changed, base.Add(time.Millisecond)}, nil
	})
	if err == nil || actual.Native.ForegroundHWND != 4 || evidence == nil || evidence.ObservationCount != 2 || evidence.EndSnapshot.Native.ForegroundHWND != 4 {
		t.Fatal("failure lost the actual final observation")
	}
	t.Run("cancel before observation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, evidence, err := monitorHoverDwell(ctx, expected, time.Second, func(context.Context) (timedHoverSnapshot, error) {
			t.Fatal("cancelled monitor requested a UI observation")
			return timedHoverSnapshot{}, nil
		})
		if !errors.Is(err, context.Canceled) || evidence != nil {
			t.Fatal("cancelled monitor continued")
		}
	})
	t.Run("cancel after settled observation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		actual, evidence, err := monitorHoverDwell(ctx, expected, time.Second, func(context.Context) (timedHoverSnapshot, error) {
			cancel()
			return timedHoverSnapshot{expected, base}, nil
		})
		if !errors.Is(err, context.Canceled) || evidence == nil || evidence.ObservationCount != 1 || !reflect.DeepEqual(actual, expected) {
			t.Fatal("cancellation lost the last actual observation")
		}
	})
}
