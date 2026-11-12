package contracts

import (
	"maps"
	"time"
)

// ApplicationUsageSummary is the persisted lifetime summary for application
// usage. The fields are deliberately typed instead of storing an encoded
// presentation payload so the report can be cloned and evolved with the
// snapshot contract.
type ApplicationUsageSummary struct {
	Total              int64 `json:"total"`
	Input              int64 `json:"input"`
	Output             int64 `json:"output"`
	Cached             int64 `json:"cached"`
	Peak               int64 `json:"peak"`
	LongestSessionMS   int64 `json:"longestSessionMs"`
	CurrentStreak      int   `json:"currentStreak"`
	LongestStreak      int   `json:"longestStreak"`
	UnknownModelTokens int64 `json:"unknownModelTokens"`
}

// ApplicationUsageDay is the persisted per-calendar-day usage projection.
type ApplicationUsageDay struct {
	Date   time.Time        `json:"date"`
	Tokens int64            `json:"tokens"`
	Turns  int              `json:"turns"`
	Tools  int              `json:"tools"`
	Models map[string]int64 `json:"models,omitempty"`
}

// ApplicationUsageReport is the durable report consumed by the desktop usage
// page. Date keys are local calendar dates in YYYY-MM-DD form and each value
// retains its midnight time for calendar-window rendering.
type ApplicationUsageReport struct {
	Summary ApplicationUsageSummary        `json:"summary"`
	Days    map[string]ApplicationUsageDay `json:"days"`
}

// Clone returns an owned copy of the report, including all nested model maps.
func (r ApplicationUsageReport) Clone() ApplicationUsageReport {
	clone := r
	if r.Days != nil {
		clone.Days = make(map[string]ApplicationUsageDay, len(r.Days))
		for key, day := range r.Days {
			day.Models = maps.Clone(day.Models)
			clone.Days[key] = day
		}
	}
	return clone
}
