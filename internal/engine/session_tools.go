package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

type sessionToolSet struct {
	tools   []agent.Tool
	cleanup func()
}

// Background commands belong to the host session, so a completed model turn or
// a disconnected client cannot accidentally terminate accepted work.
func (e *Engine) sessionRuntimeTools(session contracts.Session, envFile string) ([]agent.Tool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errors.New("host is closed")
	}
	if e.sessionIndex(session.ID) < 0 {
		return nil, errors.New("session not found")
	}
	if e.sessionTools == nil {
		e.sessionTools = map[string]*sessionToolSet{}
	}
	if owner := e.sessionTools[session.ID]; owner != nil {
		return slices.Clone(owner.tools), nil
	}
	environment, err := e.processEnvironment(session.StorageRootID, session.ID)
	if err != nil {
		return nil, err
	}
	tools, cleanup := agent.RunToolSet(context.Background(), session.CWD, agent.RunToolOptions{Env: environment, EnvFile: envFile})
	for i, inner := range tools {
		if inner.Definition().Name == "TaskOutput" || inner.Definition().Name == "TaskStop" {
			tools[i] = e.sessionTaskTool(session.ID, inner)
		}
	}
	e.sessionTools[session.ID] = &sessionToolSet{tools: tools, cleanup: cleanup}
	return slices.Clone(tools), nil
}

func (e *Engine) sessionTaskTool(parentID string, inner agent.Tool) agent.Tool {
	return delegationTool{definition: inner.Definition(), execute: func(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
		var input struct {
			TaskID  string `json:"task_id"`
			Block   *bool  `json:"block"`
			Timeout *int   `json:"timeout"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return model.ToolOutput{}, err
		}
		e.mu.Lock()
		index := e.sessionIndex(input.TaskID)
		owned := index >= 0 && e.state.Sessions[index].ParentSessionID == parentID && e.state.Sessions[index].SubagentType != ""
		if !owned {
			e.mu.Unlock()
			return inner.Execute(ctx, "", raw)
		}
		child := e.runs[input.TaskID]
		if inner.Definition().Name == "TaskStop" {
			if child != nil {
				child.cancel()
			}
			e.mu.Unlock()
			return model.ToolOutput{Text: "Subagent task stop requested: " + input.TaskID}, nil
		}
		block, timeout := true, 30000
		if input.Block != nil {
			block = *input.Block
		}
		if input.Timeout != nil {
			timeout = *input.Timeout
		}
		if timeout < 0 || timeout > 600000 {
			e.mu.Unlock()
			return model.ToolOutput{}, errors.New("timeout must be 0..600000 milliseconds")
		}
		e.mu.Unlock()
		if block && child != nil && timeout > 0 {
			timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return model.ToolOutput{}, ctx.Err()
			case <-child.done:
			case <-timer.C:
			}
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		index = e.sessionIndex(input.TaskID)
		if index < 0 {
			return model.ToolOutput{}, errors.New("subagent task is no longer available")
		}
		session := e.state.Sessions[index]
		status := "running"
		if e.runs[input.TaskID] == nil {
			status = session.LastOutcome
		}
		text := fmt.Sprintf("task_id: %s\nstatus: %s\n%s", session.ID, status, finalReply(e.history[session.ID]))
		return model.ToolOutput{Text: text, Data: map[string]any{"task_id": session.ID, "status": status, "output": finalReply(e.history[session.ID])}}, nil
	}}
}
