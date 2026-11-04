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
	defer m.updateRetry(nil)
	project := func(delta model.Delta) error {
		if delta.Retry != nil {
			m.updateRetry(delta.Retry)
		} else if delta.Text != "" || delta.Reasoning != "" || delta.Output {
			m.updateRetry(nil)
		}
		if emit != nil {
			return emit(delta)
		}
		return nil
	}
	if inner, ok := m.inner.(interface {
		StreamWithAttempts(context.Context, model.Request, func(model.Delta) error, func(model.Result) error) (model.Result, error)
	}); ok {
		return inner.StreamWithAttempts(ctx, request, project, m.recordAttempt)
	}
	// Injected models without a physical transport represent one attempt.
	result, callErr := m.inner.Stream(ctx, request, project)
	return result, errors.Join(callErr, m.recordAttempt(result))
}

func (m meteredModel) recordAttempt(result model.Result) error {
	m.core.mu.Lock()
	defer m.core.mu.Unlock()
	if m.core.sessionIndex(m.sessionID) < 0 {
		return nil
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
	return err
}

func (m meteredModel) updateRetry(status *model.RetryStatus) {
	// Naming has no active turn and must not replace the main run's status.
	if m.turnID == "" {
		return
	}
	m.core.mu.Lock()
	defer m.core.mu.Unlock()
	run := m.core.runs[m.sessionID]
	index := m.core.sessionIndex(m.sessionID)
	if m.core.closed || run == nil || run.id != m.turnID || index < 0 {
		return
	}
	session := &m.core.state.Sessions[index]
	if status == nil && session.ModelRetry == nil {
		return
	}
	session.ModelRetry = nil
	if status != nil {
		copy := *status
		session.ModelRetry = &copy
	}
	m.core.publishLocked(contracts.Event{Kind: "model.retry.updated", SessionID: m.sessionID, RunID: m.turnID})
}
