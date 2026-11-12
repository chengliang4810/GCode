package engine

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func (e *Engine) runHookRunner(session contracts.Session, run *activeRun, rules []contracts.HookRule, runtimeOptions ...hookRuntimeOptions) (*agent.HookRunner, func(context.Context, agent.HookInput) (agent.HookResult, error)) {
	options := hookRuntimeOptions{}
	if len(runtimeOptions) > 0 {
		options = runtimeOptions[len(runtimeOptions)-1]
	}
	options.Tools = slices.Clone(options.Tools)
	options.Warnings = slices.Clone(options.Warnings)
	processEnv, processEnvErr := e.processEnvironment(session.StorageRootID, session.ID)
	var redactionsMu sync.Mutex
	redactions := map[string]func(string) string{}
	redactAll := func(text string) string {
		text = e.redactHookText(contracts.HookRule{}, text)
		redactionsMu.Lock()
		current := make([]func(string) string, 0, len(redactions))
		for _, redact := range redactions {
			current = append(current, redact)
		}
		redactionsMu.Unlock()
		for _, redact := range current {
			text = redact(text)
		}
		return text
	}
	runner := agent.NewHookRunner(rules, func(execution agent.HookExecution) error {
		redact := e.hookTextRedactor(execution.Rule)
		redactionsMu.Lock()
		redactions[execution.Rule.ID] = redact
		redactionsMu.Unlock()
		name := execution.Rule.Name
		if name == "" {
			name = execution.Rule.ID
		}
		text := "Hook " + name + " · " + string(execution.Input.Event)
		if execution.Err != nil {
			text += "\n" + execution.Err.Error()
		} else if execution.Rule.Async || execution.Rule.AsyncRewake {
			text += "\nNotification completed."
		} else if execution.Result.Deny {
			text += "\nBlocked: " + execution.Result.Reason
		} else {
			text += "\nCompleted."
		}
		if fields := unsupportedHookOutputs(execution.Result); len(fields) > 0 {
			text += "\nHost action is not implemented: " + strings.Join(fields, ", ") + "."
		}
		if raw := execution.Result.UpdatedToolOutput; raw != nil {
			if err := agent.ValidateHookToolOutput(execution.Input.ToolName, raw, execution.Input.ToolOutput); err != nil {
				text += "\nIgnored invalid updatedToolOutput: " + err.Error()
			}
		}
		if execution.Result.UpdatedMCPToolOutput != nil && !strings.HasPrefix(execution.Input.ToolName, "mcp__") {
			text += "\nIgnored updatedMCPToolOutput for a builtin tool."
		}
		if execution.Result.SystemMessage != "" {
			text += "\n" + execution.Result.SystemMessage
		}
		if output := strings.TrimSpace(execution.Output); output != "" && !execution.Result.SuppressOutput {
			text += "\n" + output
		}
		if stderr := strings.TrimSpace(execution.Stderr); stderr != "" {
			text += "\n" + stderr
		}
		text = redact(text)
		title := redact(execution.Result.SessionTitle)
		if execution.Input.Event == contracts.HookSessionStart && !slices.Contains([]string{"startup", "resume", "fork"}, execution.Input.Source) {
			title = ""
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		goalCleared := false
		if !execution.Rule.Async && !execution.Rule.AsyncRewake && execution.Input.Event == contracts.HookUserPromptSubmit &&
			(execution.Result.Deny || execution.Result.Continue != nil && !*execution.Result.Continue) {
			goalCleared = e.rejectHookInputLocked(session.ID, execution.Input.UserInputID, run)
		}
		if title != "" {
			if index := e.sessionIndex(session.ID); index >= 0 {
				e.state.Sessions[index].Title = title
			}
		}
		message := contracts.Message{
			ID: id("message"), SessionID: session.ID, TurnID: run.id,
			Role: "assistant", CreatedAt: time.Now(),
			Blocks: []contracts.Block{{ID: id("block"), Kind: "text", Text: text}},
		}
		e.state.Messages[session.ID] = append(e.state.Messages[session.ID], message)
		if err := e.persistLocked(session.ID); err != nil {
			return err
		}
		if title != "" {
			e.publishLocked(contracts.Event{Kind: "session.updated", SessionID: session.ID})
		}
		if goalCleared {
			e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: session.ID, RunID: run.id})
		}
		var exitCode *int
		if execution.ExitCodeKnown {
			value := execution.ExitCode
			exitCode = &value
		}
		failure := ""
		if execution.Err != nil {
			failure = redact(execution.Err.Error())
		}
		e.publishLocked(contracts.Event{
			Kind: "hook.finished", SessionID: session.ID, RunID: run.id, Message: &message,
			PluginID: execution.Rule.PluginID, ComponentID: execution.Rule.ID, ResourceID: execution.Rule.ID,
			Text: redact(execution.Output), Error: failure, ExitCode: exitCode,
		})
		return nil
	}, agent.WithHookEvaluator(hookRuntimeEvaluator(options, session)), agent.WithHookOnceState(options.OnceState),
		agent.WithHookBackgroundContext(e.background), agent.WithHookBackgroundTracker(e),
		agent.WithHookPermissionRequest(func(ctx context.Context, input agent.HookInput, reason string) (bool, error) {
			reason = redactAll(reason)
			if len(input.ToolInput) > 0 {
				var value any
				if json.Unmarshal(input.ToolInput, &value) == nil {
					input.ToolInput, _ = json.Marshal(redactHookTranscriptValue(value, redactAll))
				} else {
					input.ToolInput = json.RawMessage(redactAll(string(input.ToolInput)))
				}
			}
			return hookRuntimePermission(options)(ctx, input, reason)
		}),
		agent.WithHookAsyncResult(func(_ context.Context, execution agent.HookExecution) error {
			return e.queueHookResult(session.ID, execution, false)
		}),
		agent.WithHookRewake(func(_ context.Context, execution agent.HookExecution) error {
			return e.queueHookResult(session.ID, execution, true)
		}))
	invoke := func(ctx context.Context, input agent.HookInput) (agent.HookResult, error) {
		if processEnvErr != nil {
			return agent.HookResult{}, processEnvErr
		}
		input.SessionID, input.RunID, input.CWD = session.ID, run.id, session.CWD
		input.EnvFile = options.EnvFile
		input.Environment = processEnv
		if input.SubagentType == "" {
			input.SubagentType = session.SubagentType
		}
		input.PermissionMode = "bypassPermissions"
		if input.Event == contracts.HookSessionStart {
			_, input.Model, _ = strings.Cut(run.selection.ModelID, "/")
		}
		if run.selection.Reasoning != "" {
			input.Effort = &agent.HookEffort{Level: run.selection.Reasoning}
		}
		if run.subagent != nil {
			input.ParentSessionID = run.subagent.parentSessionID
			if input.Event == contracts.HookStop {
				input.Event = contracts.HookSubagentStop
			}
		}
		if run.subagent != nil {
			input.AgentID = session.ID
		}
		hasEvent := runner.HasEvent(input.Event)
		if !hasEvent && input.Event != contracts.HookPermissionRequest {
			return agent.HookResult{}, nil
		}
		if hasEvent {
			transcriptSession := session.ID
			if run.subagent != nil && (input.Event == contracts.HookSubagentStart || input.Event == contracts.HookSubagentStop) {
				transcriptSession = run.subagent.parentSessionID
				input.SessionID = transcriptSession
			}
			path, err := e.writeHookTranscript(transcriptSession)
			if err != nil {
				return agent.HookResult{}, err
			}
			input.TranscriptPath = path
			if input.Event == contracts.HookSubagentStop {
				childPath, err := e.writeHookTranscript(session.ID)
				if err != nil {
					return agent.HookResult{}, err
				}
				input.AgentTranscriptPath = childPath
			}
		}
		result, err := runner.Run(ctx, input)
		result = e.redactHookResult(contracts.HookRule{}, result)
		redactionsMu.Lock()
		currentRedactions := make([]func(string) string, 0, len(redactions))
		for _, redact := range redactions {
			currentRedactions = append(currentRedactions, redact)
		}
		redactionsMu.Unlock()
		for _, redact := range currentRedactions {
			result = redactHookResultWith(result, redact)
		}
		return result, err
	}
	return runner, invoke
}

func unsupportedHookOutputs(result agent.HookResult) []string {
	var fields []string
	if result.UpdatedPermissions != nil {
		fields = append(fields, "updatedPermissions")
	}
	if result.InitialUserMessage != "" {
		fields = append(fields, "initialUserMessage")
	}
	if result.WatchPaths != nil {
		fields = append(fields, "watchPaths")
	}
	if result.ReloadSkills {
		fields = append(fields, "reloadSkills")
	}
	if result.Retry {
		fields = append(fields, "retry")
	}
	if result.WorktreePath != "" {
		fields = append(fields, "worktreePath")
	}
	if result.Action != "" || result.Content != nil {
		fields = append(fields, "elicitation action/content")
	}
	if result.TerminalSequence != "" {
		fields = append(fields, "terminalSequence")
	}
	return fields
}
func (e *Engine) rejectHookInputLocked(sessionID, inputID string, run *activeRun) bool {
	if inputID == "" {
		return false
	}
	e.history[sessionID] = slices.DeleteFunc(e.history[sessionID], func(message model.Message) bool {
		return slices.Contains(message.InputIDs, inputID)
	})
	// Rebuild after rejection so compacted context cannot retain denied input.
	e.context[sessionID] = model.CloneMessages(e.history[sessionID])
	delete(e.contextMemories, sessionID)
	delete(run.pending, inputID)
	if goal := e.goalLocked(run.goal); goal != nil && goal.InitialInputID == inputID {
		e.state.Sessions[e.sessionIndex(sessionID)].Goal = nil
		return true
	}
	return false
}

// BackgroundStarted and BackgroundFinished count running asynchronous hooks so
// shutdown can join them.
func (e *Engine) BackgroundStarted()  { e.hookBackground.Add(1) }
func (e *Engine) BackgroundFinished() { e.hookBackground.Done() }
