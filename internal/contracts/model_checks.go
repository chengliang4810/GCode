package contracts

import (
	"time"

	"gcode/internal/model"
)

// ModelCheck is a transient result for an explicitly requested connection test.
// ModelID is the same provider/model identifier used by the model picker.
type ModelCheck struct {
	ProviderID string       `json:"providerId"`
	ModelID    string       `json:"modelId"`
	State      string       `json:"state"`
	Error      string       `json:"error,omitempty"`
	Usage      *model.Usage `json:"usage,omitempty"`
	StartedAt  time.Time    `json:"startedAt"`
	FinishedAt *time.Time   `json:"finishedAt,omitempty"`
}

func CloneModelChecks(values []ModelCheck) []ModelCheck {
	if values == nil {
		return nil
	}
	result := make([]ModelCheck, len(values))
	copy(result, values)
	for i := range result {
		result[i].Usage = copyPointer(result[i].Usage)
		result[i].FinishedAt = copyPointer(result[i].FinishedAt)
	}
	return result
}
