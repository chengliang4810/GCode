package contracts

import (
	"context"

	"gcode/internal/model"
)

type ModelInvocation struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId,omitempty"`
	Source    string `json:"source"`
	model.Invocation
}

type ModelTrajectory struct {
	SessionID string            `json:"sessionId"`
	Records   []ModelInvocation `json:"records"`
	Truncated bool              `json:"truncated"`
}

// ModelTrajectoryService reads full diagnostics on demand, outside snapshots.
type ModelTrajectoryService interface {
	ReadModelTrajectory(context.Context, string) (ModelTrajectory, error)
}
