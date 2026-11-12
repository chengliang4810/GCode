package contracts

import "time"

// ModelFetch is a transient result for an explicitly requested provider model
// listing. ProviderID is the configured provider the request was sent to.
type ModelFetch struct {
	ProviderID string     `json:"providerId"`
	State      string     `json:"state"`
	Models     []string   `json:"models,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

func CloneModelFetch(value *ModelFetch) *ModelFetch {
	if value == nil {
		return nil
	}
	result := *value
	result.Models = append([]string(nil), value.Models...)
	result.FinishedAt = copyPointer(value.FinishedAt)
	return &result
}
