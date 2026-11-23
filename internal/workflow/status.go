// Package workflow holds the pure rules of the dynamic workflow feature: run
// state, phase and instance states, timeline geometry, the roster that keeps
// large casts bounded, settings consequences and artifact indexing.
//
// The package never talks to a model, a file system or a UI. The engine owns
// execution and the desktop package owns projection, so both can share one
// set of rules instead of computing their own.
package workflow

import "gcode/internal/contracts"

// IsTerminal reports whether a run can no longer change on its own.
func IsTerminal(status string) bool {
	switch status {
	case contracts.WorkflowStatusCompleted, contracts.WorkflowStatusErrored, contracts.WorkflowStatusStopped:
		return true
	}
	return false
}

// IsActive reports whether a run still owns work: it is running, or it is
// waiting for a question or a hole that only the core can settle.
func IsActive(status string) bool {
	return status == contracts.WorkflowStatusPending || status == contracts.WorkflowStatusRunning
}

// CanConfigure reports whether a run still accepts a settings change.
// Completed runs do not, and a stopped run that was superseded by a successor
// does not either.
func CanConfigure(run contracts.WorkflowRun) bool {
	switch run.Status {
	case contracts.WorkflowStatusPending, contracts.WorkflowStatusRunning, contracts.WorkflowStatusErrored:
		return true
	}
	return false
}

// CanResume reports whether a stopped run may be resumed. Seeing "stopped" is
// not enough: a superseded run was replaced by a successor.
func CanResume(run contracts.WorkflowRun) bool {
	return run.Status == contracts.WorkflowStatusStopped &&
		run.StopReason != contracts.WorkflowStopSuperseded &&
		run.Resumable
}

// StopReasonText returns the reason a run stopped, or an empty string when the
// run did not stop.
func StopReasonText(run contracts.WorkflowRun) string {
	if run.Status != contracts.WorkflowStatusStopped {
		return ""
	}
	return run.StopReason
}

// ActorState folds the executor's finer states into the four states the
// timeline shows. Executing, repairing and nudged are running; queued,
// dispatched and waiting are pending.
func ActorState(state string) string {
	switch state {
	case contracts.WorkflowActorRunning, "executing", "repairing", "nudged":
		return contracts.WorkflowActorRunning
	case contracts.WorkflowActorAsking, "asked", "escalated":
		return contracts.WorkflowActorAsking
	case contracts.WorkflowActorDone, "settled", "succeeded":
		return contracts.WorkflowActorDone
	case contracts.WorkflowActorFailed, "errored", "cancelled":
		return contracts.WorkflowActorFailed
	default:
		return contracts.WorkflowActorPending
	}
}

// ActorPriority orders the fixed roster slots. Asking first, then running,
// then failed, then the original participant order fills the rest.
func ActorPriority(state string) int {
	switch state {
	case contracts.WorkflowActorAsking:
		return 0
	case contracts.WorkflowActorRunning:
		return 1
	case contracts.WorkflowActorFailed:
		return 2
	default:
		return 3
	}
}

// PhaseState derives a phase state from its instances. A phase that still
// holds running work is running even when other instances already settled.
func PhaseState(actors []contracts.WorkflowActor) string {
	if len(actors) == 0 {
		return contracts.WorkflowPhasePending
	}
	state := contracts.WorkflowPhaseDone
	for _, actor := range actors {
		switch ActorState(actor.Status) {
		case contracts.WorkflowActorRunning, contracts.WorkflowActorAsking:
			return contracts.WorkflowPhaseRunning
		case contracts.WorkflowActorPending:
			state = contracts.WorkflowPhasePending
		case contracts.WorkflowActorFailed:
			if state != contracts.WorkflowPhasePending {
				state = contracts.WorkflowPhaseFailed
			}
		}
	}
	return state
}

// PhaseCounts reports how many instances a phase observed and how many of them
// settled. The observed count is what the run really saw, not the static
// definition total.
func PhaseCounts(actors []contracts.WorkflowActor) (observed, settled int) {
	for _, actor := range actors {
		observed++
		switch ActorState(actor.Status) {
		case contracts.WorkflowActorDone, contracts.WorkflowActorFailed:
			settled++
		}
	}
	return observed, settled
}
