package engine

import (
	"context"
	"errors"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

type taskModel struct {
	selection contracts.ModelSelection
	providers []model.Provider
}

// Automatic results resume the task's accepted model, even if the picker now
// displays a different selection for the next user input.
func (e *Engine) startAutomaticLocked(session contracts.Session, requestID string) error {
	accepted, ok := e.taskModels[session.ID]
	if !ok {
		return e.startLocked(session, requestID)
	}
	session.ModelID, session.ReasoningLevel = accepted.selection.ModelID, accepted.selection.Reasoning
	return e.startRunWithProvidersLocked(context.Background(), session, requestID, nil, accepted.providers)
}

// setSessionModelLocked validates the next selection without changing an
// already admitted request. An explicit invalid effort remains an error.
func (e *Engine) setSessionModelLocked(session *contracts.Session, modelID, reasoning *string) error {
	if modelID != nil && *modelID != session.ModelID {
		session.ModelID = *modelID
		session.ReasoningLevel = ""
	}
	if reasoning != nil {
		session.ReasoningLevel = *reasoning
	}
	_, selected, err := e.resolveModel(session.ModelID)
	if err != nil {
		return err
	}
	effort, err := selected.ResolveEffort(session.ReasoningLevel)
	if err != nil {
		return err
	}
	session.ReasoningLevel = model.ReasoningLabel(effort)
	return nil
}

// sessionModel uses the configuration accepted with the user input. A later
// picker/settings edit changes the next task, never this task's tool rounds,
// retries, hooks or completion checks.
type sessionModel struct {
	core       *Engine
	sessionID  string
	run        *activeRun
	trackReply bool
}

func (m sessionModel) selectionLocked() (contracts.Session, model.Provider, model.Model, error) {
	e := m.core
	index := e.sessionIndex(m.sessionID)
	if index < 0 {
		return contracts.Session{}, model.Provider{}, model.Model{}, errors.New("session not found")
	}
	session := e.state.Sessions[index]
	session.ModelID, session.ReasoningLevel = m.run.selection.ModelID, m.run.selection.Reasoning
	provider, selected, err := resolveConfiguredModel(m.run.providers, session.ModelID)
	return session, provider, selected, err
}

func (m sessionModel) Stream(ctx context.Context, request model.Request, emit func(model.Delta) error) (model.Result, error) {
	if err := ctx.Err(); err != nil {
		return model.Result{}, err
	}
	e := m.core
	e.mu.Lock()
	session, provider, selected, err := m.selectionLocked()
	if err != nil {
		e.mu.Unlock()
		return model.Result{}, err
	}
	effort, err := selected.ResolveEffort(session.ReasoningLevel)
	if err != nil {
		e.mu.Unlock()
		return model.Result{}, err
	}
	gateway, err := e.client.Bind(provider, selected)
	if err != nil {
		e.mu.Unlock()
		return model.Result{}, err
	}
	source := "main"
	if m.run.subagent != nil {
		source = "subagent"
	} else if m.run.compacting {
		source = "compact"
	}
	gateway.Observe = e.trajectoryRecorder(session, m.run.id, source)
	if m.trackReply && emit != nil {
		messages := e.state.Messages[session.ID]
		if index := m.run.messageIndex; index < len(messages) && messages[index].ID == m.run.messageID && messages[index].ModelID != session.ModelID {
			messages[index].ModelID = session.ModelID
			e.publishLocked(contracts.Event{Kind: "message.changed", SessionID: session.ID, RunID: m.run.id, Message: &messages[index]})
		}
	}
	e.mu.Unlock()
	request.Effort = effort
	// Keep all connected tools in the loop. Each protocol receives only the
	// definitions it can encode, including after a cross-protocol switch.
	tools := make([]model.Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if warning := mcpToolProtocolWarning(tool.Name, provider.Protocol); warning != "" {
			request.System += "\n" + warning
			continue
		}
		tools = append(tools, tool)
	}
	request.Tools = tools
	bound := meteredModel{inner: gateway, core: e, sessionID: session.ID, turnID: m.run.id, provider: provider, modelID: selected.ID}
	return bound.Stream(ctx, request, emit)
}
