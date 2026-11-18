package contracts

import "time"

// Automation is a host-owned prompt schedule. Interval runs wait after completion.
type Automation struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Prompt          string    `json:"prompt"`
	ProjectID       string    `json:"projectId,omitempty"`
	ModelID         string    `json:"modelId"`
	ReasoningLevel  string    `json:"reasoningLevel,omitempty"`
	Mode            string    `json:"mode"` // cron, interval, manual
	Cron            string    `json:"cron,omitempty"`
	Timezone        string    `json:"timezone"`
	IntervalSeconds int64     `json:"intervalSeconds,omitempty"`
	Once            bool      `json:"once"`
	Enabled         bool      `json:"enabled"`
	Continuous      bool      `json:"continuous"`
	TargetSessionID string    `json:"targetSessionId,omitempty"`
	SessionID       string    `json:"sessionId,omitempty"`
	NextAt          time.Time `json:"nextAt"`
	CreatedAt       time.Time `json:"createdAt"`
}

type AutomationRun struct {
	ID           string    `json:"id"`
	AutomationID string    `json:"automationId"`
	SessionID    string    `json:"sessionId,omitempty"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
}
