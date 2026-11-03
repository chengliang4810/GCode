package engine

import (
	"context"
	"errors"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

// meteredModel sees every attempt, including failed streams, retries, hooks
// and compaction. It never reconstructs usage from visible text.
type meteredModel struct {
	inner             agent.Model
	core              *Engine
	sessionID, turnID string
	provider          model.Provider
	modelID           string
}

func (m meteredModel) Stream(ctx context.Context, request model.Request, emit func(model.Delta) error) (model.Result, error) {
	if err := ctx.Err(); err != nil {
		return model.Result{}, err
	}
	result, callErr := m.inner.Stream(ctx, request, emit)
	m.core.mu.Lock()
	defer m.core.mu.Unlock()
	if m.core.sessionIndex(m.sessionID) < 0 {
		return result, callErr
	}
	m.core.state.ModelRequests = append(m.core.state.ModelRequests, contracts.ModelRequest{
		ID: id("request"), SessionID: m.sessionID, TurnID: m.turnID,
		ProviderID: m.provider.ID, ProviderName: m.provider.Name, ModelID: m.modelID,
		Usage: result.Usage, DecodeDuration: result.DecodeDuration,
		RequestDuration:   result.RequestDuration,
		FirstTokenLatency: result.FirstTokenLatency, FirstTokenObserved: result.FirstTokenObserved,
	})
	// Accounting survives an error or cancellation even when no assistant
	// execution message can safely be committed.
	err := m.core.persistLocked(m.sessionID)
	m.core.publishLocked(contracts.Event{Kind: "model.request.recorded", SessionID: m.sessionID, RunID: m.turnID})
	return result, errors.Join(callErr, err)
}
