package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/plugins"
)

const maxRunningSubagents = 8

func enabledSubagentProfiles(profiles []contracts.SubagentProfile) []contracts.SubagentProfile {
	return contracts.EnabledSubagentProfiles(profiles)
}

func filterSubagentTools(tools []agent.Tool, profile contracts.SubagentProfile) []agent.Tool {
	if contracts.IsReadOnlySubagent(profile.ID) {
		tools = slices.DeleteFunc(tools, func(tool agent.Tool) bool {
			return !slices.Contains([]string{"Read", "ListDirectory", "Glob", "Grep"}, tool.Definition().Name)
		})
	}
	if len(profile.Tools) == 0 {
		return slices.DeleteFunc(tools, func(tool agent.Tool) bool {
			return deniedSubagentTool(profile.DisallowedTools, tool.Definition().Name)
		})
	}
	return slices.DeleteFunc(tools, func(tool agent.Tool) bool {
		name := tool.Definition().Name
		return !matchesSubagentTool(profile.Tools, name) || deniedSubagentTool(profile.DisallowedTools, name)
	})
}

func matchesSubagentTool(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if match, _ := filepath.Match(pattern, name); match {
			return true
		}
	}
	return false
}

func deniedSubagentTool(patterns []string, name string) bool {
	for _, pattern := range patterns {
		// Never broaden access by dropping an argument-constrained denial.
		if i := strings.IndexByte(pattern, '('); i >= 0 {
			pattern = pattern[:i]
		}
		if match, _ := filepath.Match(pattern, name); match {
			return true
		}
	}
	return false
}

type subagentExecution struct {
	startHook       func(context.Context, agent.HookInput) (agent.HookResult, error)
	startContext    context.Context
	started         chan struct{}
	startSignal     sync.Once
	skillScope      forkSkillScope
	plugins         []plugins.Bundle
	parentSessionID string
	parentBlockID   string
	profile         contracts.SubagentProfile
	background      bool
	providers       []model.Provider
	goal            goalScope
}

type delegationTool struct {
	definition model.Tool
	execute    func(context.Context, json.RawMessage) (model.ToolOutput, error)
}

func (t delegationTool) Definition() model.Tool { return t.definition }
func (t delegationTool) Concurrent() bool       { return false }
func (t delegationTool) Execute(
	ctx context.Context,
	_ string,
	args json.RawMessage,
) (model.ToolOutput, error) {
	return t.execute(ctx, args)
}

func (e *Engine) delegationTool(
	parentID string,
	run *activeRun,
	profiles []contracts.SubagentProfile,
) agent.Tool {
	types := make([]string, 0, len(profiles))
	var description strings.Builder
	description.WriteString("Delegate an independent task to a subagent with its own context. ")
	description.WriteString("Models and reasoning are selected in Settings, not in the task prompt. ")
	description.WriteString("Background execution returns a child session ID and sends a completion notification. ")
	description.WriteString("Do not poll or sleep waiting for a background child. Available types:\n")
	for _, profile := range profiles {
		if profile.Disabled {
			continue
		}
		types = append(types, profile.ID)
		encoded, _ := json.Marshal(struct{ ID, Name, Description string }{
			profile.ID, profile.Name, profile.Description,
		})
		description.Write(encoded)
		description.WriteByte('\n')
	}
	return delegationTool{
		definition: model.Tool{
			Name: "Agent", Description: description.String(),
			Parameters: map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"description", "prompt"},
				"properties": map[string]any{
					"description": map[string]any{"type": "string", "description": "Short task title"},
					"prompt": map[string]any{
						"type": "string", "description": "Complete task instructions; parent history is not shared",
					},
					"subagent_type":     map[string]any{"type": "string", "enum": types},
					"run_in_background": map[string]any{"type": "boolean"},
					"model":             map[string]any{"type": "string", "enum": []string{"sonnet", "opus", "haiku", "inherit"}, "description": "Claude model role. This host resolves roles to the inherited model; a user-configured subagent model takes precedence."},
				},
			},
		},
		execute: func(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
			var args struct {
				Description string `json:"description"`
				Prompt      string `json:"prompt"`
				Type        string `json:"subagent_type"`
				Background  bool   `json:"run_in_background"`
				Model       string `json:"model"`
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&args); err != nil {
				return model.ToolOutput{}, err
			}
			if strings.TrimSpace(args.Prompt) == "" || strings.TrimSpace(args.Description) == "" {
				return model.ToolOutput{}, errors.New("a nonempty prompt and description are required")
			}
			if args.Type == "" {
				args.Type = "general-purpose"
			}
			if args.Model != "" && !slices.Contains([]string{"sonnet", "opus", "haiku", "inherit"}, args.Model) {
				return model.ToolOutput{}, errors.New("unsupported model role")
			}
			var profile *contracts.SubagentProfile
			for _, candidate := range profiles {
				if candidate.ID == args.Type && !candidate.Disabled {
					profile = &candidate
					break
				}
			}
			if profile == nil {
				return model.ToolOutput{}, fmt.Errorf("subagent type %q is not configured", args.Type)
			}
			e.mu.Lock()
			child, active, err := e.spawnSubagentLocked(
				ctx, parentID, run, *profile, args.Description, args.Prompt, args.Background,
			)
			e.mu.Unlock()
			if err != nil {
				return model.ToolOutput{}, err
			}
			e.waitSubagentStarted(active)
			if args.Background {
				return model.ToolOutput{Text: fmt.Sprintf(
					"Background subagent started. child_session_id: %s\nCompletion will be delivered automatically.",
					child.ID,
				)}, nil
			}
			// Drain accepted child work even after cancellation; its completed
			// tools and terminal state must be recorded before the parent retires.
			<-active.done
			e.mu.Lock()
			defer e.mu.Unlock()
			return model.ToolOutput{Text: subagentResult(child.ID, active.output, active.err)}, active.err
		},
	}
}

func (e *Engine) spawnSubagentLocked(
	ctx context.Context,
	parentID string,
	parentRun *activeRun,
	profile contracts.SubagentProfile,
	description string,
	prompt string,
	background bool,
) (contracts.Session, *activeRun, error) {
	if err := e.checkLocked(ctx); err != nil {
		return contracts.Session{}, nil, err
	}
	parentIndex := e.sessionIndex(parentID)
	if parentIndex < 0 || e.runs[parentID] != parentRun || parentRun.subagent != nil {
		return contracts.Session{}, nil, errors.New("parent execution is no longer available")
	}
	running := 0
	for _, run := range e.runs {
		if run.subagent != nil {
			running++
		}
	}
	if running >= maxRunningSubagents {
		return contracts.Session{}, nil, fmt.Errorf("at most %d subagents can run at once", maxRunningSubagents)
	}
	parent := e.state.Sessions[parentIndex]
	selectedModel, reasoning := parentRun.selection.ModelID, parentRun.selection.Reasoning
	if profile.Model != nil {
		selectedModel, reasoning = profile.Model.ModelID, profile.Model.Reasoning
	}
	_, selected, err := resolveConfiguredModel(parentRun.providers, selectedModel)
	if err != nil {
		return contracts.Session{}, nil, fmt.Errorf("subagent %q model is unavailable: %w", profile.Name, err)
	}
	if profile.Model != nil && len(selected.ReasoningLevels) > 0 && strings.TrimSpace(reasoning) == "" {
		return contracts.Session{}, nil, fmt.Errorf("choose a reasoning level for subagent %q in Settings", profile.Name)
	}
	effort, err := selected.ResolveEffort(reasoning)
	if err != nil {
		return contracts.Session{}, nil, fmt.Errorf("subagent %q: %w", profile.Name, err)
	}
	// agent is a serial tool, so exactly one parent invocation is in flight.
	// Keep its stable call ID when replacing the generic row with a child link.
	var parentBlock *contracts.Block
	for i, message := range e.state.Messages[parentID] {
		if message.TurnID != parentRun.id {
			continue
		}
		for j, block := range message.Blocks {
			if block.Tool != nil && (block.Tool.Name == "Agent" || block.Tool.Name == "Skill") && block.Tool.Status == "running" {
				parentBlock = &e.state.Messages[parentID][i].Blocks[j]
			}
		}
	}
	if parentBlock == nil {
		return contracts.Session{}, nil, errors.New("parent agent invocation is missing")
	}
	blockID := parentBlock.ID
	priorBlock := *parentBlock
	childID, _, err := e.reserveSessionLocked(parent.StorageRootID)
	if err != nil {
		return contracts.Session{}, nil, err
	}
	now := time.Now()
	child := contracts.Session{
		ID: childID, StorageRootID: parent.StorageRootID, ParentSessionID: parentID, SubagentType: profile.ID, SideChat: true,
		Title: description, ProjectID: parent.ProjectID, DeviceID: parent.DeviceID,
		CWD: parent.CWD, Branch: parent.Branch, WorktreeID: parent.WorktreeID,
		ModelID: selectedModel, ReasoningLevel: model.ReasoningLabel(effort),
		Status: contracts.Idle, CreatedAt: now, UpdatedAt: now,
	}
	execution := &subagentExecution{
		startHook: parentRun.hook, startContext: ctx, started: make(chan struct{}),
		plugins:         slices.Clone(parentRun.plugins),
		parentSessionID: parentID, parentBlockID: blockID, profile: profile, background: background,
		providers: parentRun.providers,
		goal:      parentRun.goal,
	}
	if scope, ok := ctx.Value(forkSkillScopeKey{}).(forkSkillScope); ok {
		execution.skillScope = forkSkillScope{
			hooks: contracts.CloneHookRules(scope.hooks), disallowed: slices.Clone(scope.disallowed),
		}
	}
	e.state.Sessions = append([]contracts.Session{child}, e.state.Sessions...)
	e.loadedSessions[child.ID] = true
	e.history[child.ID] = []model.Message{{Role: "user", Text: prompt}}
	e.context[child.ID] = model.CloneMessages(e.history[child.ID])
	e.appendUserLocked(child.ID, prompt)
	parentBlock.Kind, parentBlock.Tool = "subagent", nil
	parentBlock.Subagent = &contracts.Subagent{
		ID: child.ID, SessionID: child.ID, Name: profile.Name, Task: description,
		Status: "running", ModelID: child.ModelID, ReasoningLevel: child.ReasoningLevel,
	}
	parentIndex = e.sessionIndex(parentID)
	priorCount := e.state.Sessions[parentIndex].RunningSubagents
	e.state.Sessions[parentIndex].RunningSubagents++
	if background {
		ctx = context.WithoutCancel(ctx)
	}
	if err = e.startRunLocked(ctx, child, "", execution); err != nil {
		*parentBlock = priorBlock
		e.state.Sessions = e.state.Sessions[1:]
		e.state.Sessions[e.sessionIndex(parentID)].RunningSubagents = priorCount
		delete(e.history, child.ID)
		delete(e.context, child.ID)
		delete(e.state.Messages, child.ID)
		delete(e.loadedSessions, child.ID)
		delete(e.sessionHashes, child.ID)
		delete(e.sessionRecords, child.ID)
		e.releaseEmptySessionDirectories(child)
		return contracts.Session{}, nil, err
	}
	e.updateRunningSubagentsLocked(parentID)
	e.publishLocked(contracts.Event{Kind: "subagent.started", SessionID: parentID})
	return child, e.runs[child.ID], nil
}

func readOnlyTools() []agent.Tool {
	tools := []agent.Tool{}
	for _, tool := range agent.CoreTools() {
		switch tool.Definition().Name {
		case "Read", "ListDirectory", "Glob", "Grep":
			tools = append(tools, tool)
		}
	}
	return tools
}

func finalReply(history []model.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" && len(history[i].Calls) == 0 && history[i].Text != "" {
			return history[i].Text
		}
	}
	return ""
}

func subagentResult(sessionID, reply string, err error) string {
	status := "completed"
	if err != nil {
		status = "failed"
		reply = err.Error()
		if errors.Is(err, context.Canceled) {
			status = "interrupted"
		}
	}
	return fmt.Sprintf("child_session_id: %s\nstatus: %s\n%s", sessionID, status, reply)
}

func (e *Engine) updateRunningSubagentsLocked(parentID string) {
	if index := e.sessionIndex(parentID); index >= 0 {
		e.state.Sessions[index].RunningSubagents = 0
		for _, run := range e.runs {
			if run.subagent != nil && run.subagent.parentSessionID == parentID {
				e.state.Sessions[index].RunningSubagents++
			}
		}
	}
}

func (e *Engine) finishSubagentLocked(session contracts.Session, run *activeRun, runErr error) {
	child := run.subagent
	status, summary := "completed", run.output
	if runErr != nil {
		status, summary = "failed", runErr.Error()
		if errors.Is(runErr, context.Canceled) {
			status = "interrupted"
		}
	}
	for i := range e.state.Messages[child.parentSessionID] {
		for j := range e.state.Messages[child.parentSessionID][i].Blocks {
			block := &e.state.Messages[child.parentSessionID][i].Blocks[j]
			if block.ID == child.parentBlockID && block.Subagent != nil {
				block.Subagent.Status = status
				block.Subagent.Summary = summary
			}
		}
	}
	e.updateRunningSubagentsLocked(child.parentSessionID)
	index := e.sessionIndex(child.parentSessionID)
	goalStillExists := run.goal.id == "" || e.goalLocked(run.goal) != nil
	if child.background && index >= 0 && !e.state.Sessions[index].Archived && goalStillExists {
		result := contracts.QueuedMessage{
			ID: "subagent-result-" + run.id, SessionID: child.parentSessionID, Source: "subagent", Delivery: "steer",
			Text:      "Background subagent result:\n" + subagentResult(session.ID, summary, runErr),
			CreatedAt: time.Now(),
		}
		for i := range e.state.Queue {
			if e.state.Queue[i].ID == result.ID {
				e.state.Queue[i] = result
				return
			}
		}
		e.state.Queue = append(e.state.Queue, result)
		e.wakeRunLocked(child.parentSessionID)
	}
}

func (e *Engine) resumeSubagentParentLocked(run *activeRun) {
	child := run.subagent
	index := e.sessionIndex(child.parentSessionID)
	if !child.background || index < 0 || e.closed || e.runs[child.parentSessionID] != nil ||
		e.state.Sessions[index].Archived || e.state.Sessions[index].LastOutcome != "completed" {
		return
	}
	if run.goal.id != "" {
		if goal := e.goalLocked(run.goal); goal == nil || goal.Status != contracts.GoalActive {
			return
		}
	}
	if err := e.startLocked(e.state.Sessions[index], ""); err != nil {
		e.failSubagentContinuationLocked(child.parentSessionID, err)
	}
}

func (e *Engine) failSubagentContinuationLocked(parentID string, err error) {
	index := e.sessionIndex(parentID)
	if index < 0 {
		return
	}
	parent := &e.state.Sessions[index]
	parent.Status, parent.LastOutcome = contracts.Errored, "failed"
	text := fmt.Sprintf("Cannot continue after a background subagent: %s. The result is saved in the queue; fix the model configuration and retry.", err)
	if e.state.Settings.General.Language == "zh-CN" {
		text = fmt.Sprintf("后台子 Agent 已返回，但父会话无法继续：%s。结果保留在队列中，请修正模型配置后重试。", err)
	}
	e.state.Messages[parentID] = append(e.state.Messages[parentID], contracts.Message{
		ID: id("message"), SessionID: parentID, Role: "assistant", CreatedAt: time.Now(),
		Blocks: []contracts.Block{{ID: id("block"), Kind: "text", Text: text}},
	})
	_ = e.persistLocked(parentID)
	e.publishLocked(contracts.Event{Kind: "run.failed", SessionID: parentID})
}
