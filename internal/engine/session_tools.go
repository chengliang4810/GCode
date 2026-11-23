package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

type sessionToolSet struct {
	tools   []agent.Tool
	cleanup func()
	tasks   agent.BackgroundTasks
}

// Background commands belong to the host session, so a completed model turn or
// a disconnected client cannot accidentally terminate accepted work.
func (e *Engine) sessionRuntimeTools(session contracts.Session, envFile string) ([]agent.Tool, error) {
	base, err := e.sessionBaseTools(session, envFile)
	if err != nil {
		return nil, err
	}
	return e.workflowSessionTools(session, base), nil
}

// sessionBaseTools 是会话的常驻工具集。它按会话缓存：后台任务跟着会话走。
func (e *Engine) sessionBaseTools(session contracts.Session, envFile string) ([]agent.Tool, error) {
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
	tools, cleanup, tasks := agent.RunToolSetWithTasks(context.Background(), session.CWD, agent.RunToolOptions{Env: environment, EnvFile: envFile,
		OpenOutput: func() (io.WriteCloser, string, error) { return e.openToolOutput(session) },
		TaskChanged: func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			if !e.closed && e.sessionTools[session.ID] != nil {
				e.publishLocked(contracts.Event{Kind: "task.changed", SessionID: session.ID})
			}
		},
	})
	for i, inner := range tools {
		if inner.Definition().Name == "TaskOutput" || inner.Definition().Name == "TaskStop" {
			tools[i] = e.sessionTaskTool(session.ID, inner)
		}
	}
	e.sessionTools[session.ID] = &sessionToolSet{tools: tools, cleanup: cleanup, tasks: tasks}
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
			session := e.state.Sessions[index]
			status := session.LastOutcome
			if child != nil {
				child.cancel()
				status = "stopping"
			}
			e.mu.Unlock()
			return model.ToolOutput{Text: "Subagent task stop requested: " + input.TaskID, Data: map[string]any{
				"task_id": input.TaskID, "description": session.Title, "status": status,
			}}, nil
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
		if err := e.ensureSessionDataLocked(input.TaskID); err != nil {
			return model.ToolOutput{}, err
		}
		defer e.evictIdleSessionsLocked()
		session := e.state.Sessions[index]
		status := "running"
		if e.runs[input.TaskID] == nil {
			status = session.LastOutcome
		}
		output := model.Message{Text: finalReply(e.history[session.ID])}
		var latest time.Time
		for _, run := range e.state.Runs {
			if run.SessionID == session.ID && !run.StartedAt.Before(latest) {
				latest, output.OutputFile = run.StartedAt, run.OutputFile
			}
		}
		output, err := agent.PrepareOutput(ctx, nil, output)
		if err != nil {
			return model.ToolOutput{}, err
		}
		text := fmt.Sprintf("task_id: %s\nstatus: %s\n%s", session.ID, status, output.Text)
		data := map[string]any{
			"task_id": session.ID, "description": session.Title, "status": status,
			"output": output.Text,
		}
		if output.OutputFile != "" {
			data["output_file"], data["output_truncated"] = output.OutputFile, true
		}
		return model.ToolOutput{Text: text, Data: data}, nil
	}}
}
