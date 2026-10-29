//go:build uifixture

package main

import (
	"context"
	"errors"
	"time"

	"gcode/internal/workbench"
	"github.com/egoist/mygo"
)

type hoverDwellObservation struct {
	ObservedAt         time.Time `json:"observedAt"`
	ElapsedNanoseconds int64     `json:"elapsedNanoseconds"`
	PointerMoveCount   int       `json:"pointerMoveCount"`
	GeometryStable     bool      `json:"geometryStable"`
	StableFrames       int       `json:"stableFrames"`
	HoveredTabID       string    `json:"hoveredTabId"`
	NativeWindowActive bool      `json:"nativeWindowActive"`
	ForegroundHWND     uint64    `json:"foregroundHWND"`
}

// Dwell begins with an actual UI-thread observation of received input and
// committed production hover. These samples do not establish the exact first
// OS motion time, GPU presentation time or continuous state between samples.
type hoverDwellEvidence struct {
	RequestedNanoseconds int64                   `json:"requestedNanoseconds"`
	ElapsedNanoseconds   int64                   `json:"elapsedNanoseconds"`
	StartedAt            time.Time               `json:"startedAt"`
	EndedAt              time.Time               `json:"endedAt"`
	ObservationCount     int                     `json:"observationCount"`
	StartSnapshot        resultSnapshot          `json:"startSnapshot"`
	EndSnapshot          resultSnapshot          `json:"endSnapshot"`
	Observations         []hoverDwellObservation `json:"observations"`
	TimingSource         string                  `json:"timingSource"`
}

type timedHoverSnapshot struct {
	Snapshot   resultSnapshot
	ObservedAt time.Time
}

func captureHoverDwell(ctx context.Context, window *mygo.Window, fixture *workbench.TabStripFixture, expected resultSnapshot, requested time.Duration) (resultSnapshot, *hoverDwellEvidence, error) {
	return monitorHoverDwell(ctx, expected, requested, func(ctx context.Context) (timedHoverSnapshot, error) {
		type result struct {
			sample timedHoverSnapshot
			err    error
		}
		ch := make(chan result, 1)
		window.Update(func() {
			native, err := measureNativeWindow(window, expected.Native.ActualScale, expected.Native.PointerMessagesSent)
			snapshot := resultSnapshot{fixture.State(), native}
			ch <- result{timedHoverSnapshot{snapshot, time.Now()}, err}
		})
		select {
		case r := <-ch:
			return r.sample, r.err
		case <-ctx.Done():
			return timedHoverSnapshot{}, ctx.Err()
		}
	})
}

func monitorHoverDwell(ctx context.Context, expected resultSnapshot, requested time.Duration, observe func(context.Context) (timedHoverSnapshot, error)) (resultSnapshot, *hoverDwellEvidence, error) {
	if requested <= 0 || requested > 10*time.Second {
		return expected, nil, errors.New("hover dwell must be positive and at most10s")
	}
	if expected.State.StyleCase != "inactive-hover" && expected.State.StyleCase != "file-hover" && expected.State.StyleCase != "file-hover-edge" {
		return expected, nil, errors.New("hover dwell requires a hover supplement")
	}
	if err := validateHoverDwellSnapshot(expected, expected, expected); err != nil || !expected.State.Ready() {
		return expected, nil, errors.Join(errors.New("hover dwell requires a settled initial snapshot"), err)
	}
	ctx, cancel := context.WithTimeout(ctx, requested+2*time.Second)
	defer cancel()
	last := expected
	previous := expected
	var evidence *hoverDwellEvidence
	var startedAt, previousAt time.Time
	for {
		if err := ctx.Err(); err != nil {
			return last, evidence, err
		}
		sample, err := observe(ctx)
		if !sample.ObservedAt.IsZero() {
			last = sample.Snapshot
		}
		if err != nil {
			return last, evidence, err
		}
		if sample.ObservedAt.IsZero() || !previousAt.IsZero() && sample.ObservedAt.Before(previousAt) {
			return last, evidence, errors.New("hover dwell observation time is missing or went backwards")
		}
		previousAt = sample.ObservedAt
		validationErr := validateHoverDwellSnapshot(last, expected, previous)
		previous = last
		if evidence == nil && validationErr == nil && last.State.Ready() {
			startedAt = sample.ObservedAt
			evidence = &hoverDwellEvidence{
				RequestedNanoseconds: int64(requested), StartedAt: startedAt.UTC(), StartSnapshot: last,
				TimingSource: "Go monotonic time between UI-thread observations after received input and committed hover; not exact first OS motion or GPU presentation time, and not proof of continuous state between polling samples",
			}
		}
		if evidence != nil {
			elapsed := sample.ObservedAt.Sub(startedAt)
			hovered := ""
			if last.State.StyleEvidence != nil {
				hovered = last.State.StyleEvidence.HoveredTabID
			}
			evidence.ElapsedNanoseconds = int64(elapsed)
			evidence.EndedAt, evidence.EndSnapshot = sample.ObservedAt.UTC(), last
			evidence.Observations = append(evidence.Observations, hoverDwellObservation{
				ObservedAt: sample.ObservedAt.UTC(), ElapsedNanoseconds: int64(elapsed),
				PointerMoveCount: last.State.Pointer.MoveCount, GeometryStable: last.State.GeometryStable,
				StableFrames: last.State.StableFrames, HoveredTabID: hovered,
				NativeWindowActive: last.Native.NativeWindowActive, ForegroundHWND: last.Native.ForegroundHWND,
			})
			evidence.ObservationCount = len(evidence.Observations)
		}
		if validationErr != nil {
			return last, evidence, validationErr
		}
		if evidence != nil && time.Duration(evidence.ElapsedNanoseconds) >= requested && last.State.Ready() {
			return last, evidence, nil
		}
		if err := pause(ctx, 16*time.Millisecond); err != nil {
			return last, evidence, err
		}
	}
}

func validateHoverDwellSnapshot(current, expected, previous resultSnapshot) error {
	if !current.State.NativeWindow || current.Native.PID <= 0 || current.Native.HWND == 0 || current.Native.PointerTargetHWND == 0 || !current.Native.PointerMessagesSent || current.Native != expected.Native || current.State.Expanded != expected.State.Expanded || current.State.Pointer.Source != expected.State.Pointer.Source || current.State.Pointer.MoveCount < previous.State.Pointer.MoveCount || current.State.Pointer.MoveCount < 1 {
		return errors.New("hover dwell native identity, foreground, scene or received input changed")
	}
	state := current.State
	if current.State.StyleCase == "file-hover-edge" && state.StyleEvidence != nil && expected.State.StyleEvidence != nil && state.StyleEvidence.Edge != nil && expected.State.StyleEvidence.Edge != nil {
		// This diagnostic observation may naturally mature from no selected
		// overlay to Visible while the same input target/scene stays fixed.
		// Preserve actual snapshots in the evidence; only the dwell input gate
		// ignores tooltip lifecycle. File paths and every other style field
		// remain strict. The capture pre/post gate uses the final real snapshot.
		style := *state.StyleEvidence
		edge := *style.Edge
		edge.Tooltip = expected.State.StyleEvidence.Edge.Tooltip
		style.Edge = &edge
		state.StyleEvidence = &style
	}
	// Identical-position messages increment the observed count and invalidate
	// old stable frames until the next committed frames. Permit that transient
	// state only after additional received input; the final sample must be Ready.
	duplicatePending := state.Pointer.MoveCount > expected.State.Pointer.MoveCount && (state.Pointer.MoveCount > previous.State.Pointer.MoveCount || !previous.State.Ready())
	if !state.Ready() && duplicatePending {
		state.GeometryStable, state.StableFrames = expected.State.GeometryStable, expected.State.StableFrames
	}
	return validateCapturedState(state, expected.State, expected.Native, current.Native)
}
