package workflow

import "gcode/internal/contracts"

// DefaultConcurrency is the starting concurrency of a run. It is a starting
// point, not a ceiling.
const DefaultConcurrency = 1

// NormalizeConcurrency applies the only rule the reference applies: the value
// must be an integer of at least one. It never clamps to a maximum.
func NormalizeConcurrency(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

// Outcome tells the caller what a settings change means for the run.
type Outcome int

const (
	// OutcomeNone means the change does not touch the run.
	OutcomeNone Outcome = iota
	// OutcomeRetune means only the concurrency changed: the same run is
	// retuned in place. No successor run appears.
	OutcomeRetune
	// OutcomeSuccessor means the model changed: the predecessor stops and a
	// successor run carries the work forward.
	OutcomeSuccessor
)

// ChangeOutcome classifies a settings change. A change that only lowers the
// concurrency retunes the running run in place.
func ChangeOutcome(change contracts.WorkflowSettingsChange) Outcome {
	if change.HasModel {
		return OutcomeSuccessor
	}
	if change.HasConc {
		return OutcomeRetune
	}
	return OutcomeNone
}

// ResolveModel resolves the model a run uses after a change. An absent model
// means the session model; a missing setting never swaps the provider.
func ResolveModel(change contracts.WorkflowSettingsChange, sessionModel string) (string, bool) {
	if !change.HasModel {
		return "", false
	}
	if change.ModelID == "" {
		return sessionModel, true
	}
	return change.ModelID, true
}

// ModelChanged reports whether the change asks for a different model than the
// run currently uses.
func ModelChanged(change contracts.WorkflowSettingsChange, current, sessionModel string) bool {
	if !change.HasModel {
		return false
	}
	target := change.ModelID
	if target == "" {
		target = sessionModel
	}
	return target != current
}

// ConcurrencyChanged reports whether the change asks for a different
// concurrency than the run currently uses.
func ConcurrencyChanged(change contracts.WorkflowSettingsChange, current int) bool {
	if !change.HasConc {
		return false
	}
	return NormalizeConcurrency(change.Concurrency) != current
}
