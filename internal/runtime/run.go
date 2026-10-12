package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gcode/internal/agent"
	"gcode/internal/hooks"
	"gcode/internal/sandbox"
	"gcode/internal/sessions"
	"gcode/internal/tools"
)

func (engine *Engine) runAgent(
	ctx context.Context,
	input Input,
	provider agent.Provider,
	key string,
	registry *tools.Registry,
	sandboxEngine *sandbox.Engine,
	hookDispatcher *hooks.Dispatcher,
	permissionMode agent.PermissionMode,
	specialists []agent.SpecialistInfo,
	skillInfos []agent.SkillInfo,
	initialMessages []agent.Message,
	callbacks Callbacks,
) (agent.Result, error) {
	runID := input.RunID
	appendEvent := func(kind sessions.EventType, payload any) {
		_ = engine.appendEvent(input.SessionID, kind, payload, key)
	}
	appendEvent(sessions.EventMessage, map[string]any{"runId": runID, "role": "user", "content": redact(input.Prompt, key)})
	options := agent.Options{
		MaxTurns:                engine.maxTurns,
		DeferThreshold:          engine.deferTools,
		Specialists:             specialists,
		Skills:                  skillInfos,
		SessionID:               input.SessionID,
		CallingSessionID:        input.CallingSessionID,
		CallingToolUseID:        input.CallingToolUseID,
		Depth:                   input.Depth,
		SessionTitle:            input.Title,
		ProviderName:            input.Provider.ID,
		Model:                   input.Provider.Model,
		Cwd:                     input.Cwd,
		Registry:                registry,
		PermissionMode:          permissionMode,
		Autonomy:                input.Autonomy,
		Sandbox:                 sandboxEngine,
		FileTracker:             tools.NewFileTracker(),
		Hooks:                   hookDispatcher,
		RequireCompletionSignal: false,
		EnabledTools:            append([]string(nil), input.EnabledTools...),
		DisabledTools:           append([]string(nil), input.DisabledTools...),
		InitialMessages:         append([]agent.Message(nil), initialMessages...),
	}
	options.OnAssistantMessage = func(message agent.Message) {
		payload := map[string]any{
			"runId":   runID,
			"role":    "assistant",
			"content": redact(message.Content, key),
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{
					"id":             call.ID,
					"providerCallId": call.ProviderCallID,
					"name":           call.Name,
					"arguments":      redact(call.Arguments, key),
					"freeform":       call.Freeform,
					"signature":      redact(call.Signature, key),
				})
			}
			payload["toolCalls"] = calls
		}
		if len(message.Reasoning) > 0 {
			blocks := make([]map[string]any, 0, len(message.Reasoning))
			for _, block := range message.Reasoning {
				blocks = append(blocks, map[string]any{
					"provider":  block.Provider,
					"type":      block.Type,
					"text":      redact(block.Text, key),
					"signature": redact(block.Signature, key),
					"data":      redact(block.Data, key),
				})
			}
			payload["reasoning"] = blocks
		}
		appendEvent(sessions.EventMessage, payload)
	}
	options.OnText = func(text string) {
		text = redact(text, key)
		if callbacks.OnText != nil {
			callbacks.OnText(text)
		}
	}
	options.OnReasoning = func(text string) {
		text = redact(text, key)
		if callbacks.OnReasoning != nil {
			callbacks.OnReasoning(text)
		}
	}
	options.OnToolCall = func(call agent.ToolCall) {
		mapped := ToolCall{
			ID:             call.ID,
			ProviderCallID: call.ProviderCallID,
			Name:           call.Name,
			Arguments:      redact(call.Arguments, key),
			Freeform:       call.Freeform,
			Signature:      redact(call.Signature, key),
		}
		appendEvent(sessions.EventToolCall, map[string]any{
			"runId": runID, "id": mapped.ID, "providerCallId": mapped.ProviderCallID,
			"name": mapped.Name, "arguments": mapped.Arguments, "freeform": mapped.Freeform,
			"signature": mapped.Signature,
		})
		if callbacks.OnToolCall != nil {
			callbacks.OnToolCall(mapped)
		}
	}
	options.OnPermissionRequest = func(requestContext context.Context, request agent.PermissionRequest) (agent.PermissionDecision, error) {
		mapped := PermissionRequest{
			ID:        request.ToolCallID,
			Tool:      request.ToolName,
			Arguments: redact(encode(request.Args), key),
			Options:   permissionOptions(request.AvailableDecisions),
		}
		appendEvent(sessions.EventPermissionRequest, map[string]any{
			"runId": runID, "id": mapped.ID, "tool": mapped.Tool, "arguments": mapped.Arguments,
			"options": mapped.Options,
		})
		if callbacks.OnPermissionRequest == nil {
			return agent.PermissionDecision{Action: agent.PermissionDecisionDeny, Reason: "permission callback unavailable"}, nil
		}
		action, err := callbacks.OnPermissionRequest(requestContext, mapped)
		if err != nil {
			return agent.PermissionDecision{}, err
		}
		appendEvent(sessions.EventPermissionDecision, map[string]any{"runId": runID, "id": mapped.ID, "action": action})
		return agent.PermissionDecision{Action: agent.PermissionDecisionAction(action)}, nil
	}
	options.OnPermission = func(event agent.PermissionEvent) {
		mapped := PermissionEvent{ID: event.ToolCallID, Tool: event.ToolName, Action: string(event.Action), Reason: redact(event.Reason, key)}
		appendEvent(sessions.EventPermission, map[string]any{
			"runId": runID, "id": mapped.ID, "tool": mapped.Tool, "action": mapped.Action, "reason": mapped.Reason,
		})
		if callbacks.OnPermission != nil {
			callbacks.OnPermission(mapped)
		}
	}
	options.OnAskUser = func(requestContext context.Context, request agent.AskUserRequest) (agent.AskUserResponse, error) {
		mapped := AskRequest{ID: request.ToolCallID, Questions: make([]Question, 0, len(request.Questions))}
		for _, question := range request.Questions {
			mappedQuestion := Question{Header: question.Header, Text: question.Question}
			for index, label := range question.Options {
				description := ""
				if index < len(question.OptionDescriptions) {
					description = question.OptionDescriptions[index]
				}
				mappedQuestion.Options = append(mappedQuestion.Options, QuestionOption{Label: redact(label, key), Description: redact(description, key)})
			}
			mapped.Questions = append(mapped.Questions, mappedQuestion)
		}
		appendEvent(sessions.EventMessage, map[string]any{"runId": runID, "role": "ask", "request": mapped})
		if callbacks.OnAskUser == nil {
			return agent.AskUserResponse{}, errors.New("ask callback unavailable")
		}
		answers, err := callbacks.OnAskUser(requestContext, mapped)
		if err != nil {
			return agent.AskUserResponse{}, err
		}
		appendEvent(sessions.EventMessage, map[string]any{"runId": runID, "role": "ask_user_answers", "answers": answers})
		return agent.AskUserResponse{Answers: answers}, nil
	}
	options.OnToolResult = func(result agent.ToolResult) {
		mapped := ToolResult{
			ID:             result.ToolCallID,
			ProviderCallID: result.ToolCallProviderID,
			Name:           result.Name,
			Status:         string(result.Status),
			Output:         redact(result.ModelOutput(), key),
			ChangedFiles:   append([]string(nil), result.ChangedFiles...),
		}
		appendEvent(sessions.EventToolResult, map[string]any{
			"runId": runID, "id": mapped.ID, "providerCallId": mapped.ProviderCallID,
			"name": mapped.Name, "status": mapped.Status,
			"output": mapped.Output, "changedFiles": mapped.ChangedFiles, "redacted": result.Redacted,
		})
		if callbacks.OnToolResult != nil {
			callbacks.OnToolResult(mapped)
		}
	}
	options.OnUsage = func(usage agent.Usage) {
		mapped := Usage{PromptTokens: usage.EffectiveInputTokens(), CompletionTokens: usage.EffectiveOutputTokens(), TotalTokens: usage.TotalTokens()}
		appendEvent(sessions.EventUsage, map[string]any{"runId": runID, "promptTokens": mapped.PromptTokens, "completionTokens": mapped.CompletionTokens, "totalTokens": mapped.TotalTokens})
		if callbacks.OnUsage != nil {
			callbacks.OnUsage(mapped)
		}
	}
	options.OnToolProgress = callbacks.OnToolProgress
	result, err := agent.Run(ctx, input.Prompt, provider, options)
	if err != nil {
		appendEvent(sessions.EventError, map[string]any{"runId": runID, "message": redact(err.Error(), key)})
		if callbacks.OnError != nil {
			callbacks.OnError(err)
		}
		return result, err
	}
	appendEvent(sessions.EventMessage, map[string]any{"runId": runID, "role": "run_end", "turns": result.Turns})
	return result, nil
}

func permissionOptions(actions []agent.PermissionDecisionAction) []PermissionOption {
	result := make([]PermissionOption, 0, len(actions))
	for _, action := range actions {
		label := string(action)
		switch action {
		case agent.PermissionDecisionAllow:
			label = "允许本次"
		case agent.PermissionDecisionAllowStrict:
			label = "审查后允许"
		case agent.PermissionDecisionAllowForSession:
			label = "会话内允许"
		case agent.PermissionDecisionAllowPrefix:
			label = "会话内允许前缀"
		case agent.PermissionDecisionAlwaysAllowPrefix:
			label = "始终允许前缀"
		case agent.PermissionDecisionDeny:
			label = "拒绝"
		case agent.PermissionDecisionAlwaysAllow:
			label = "始终允许"
		case agent.PermissionDecisionCancel:
			label = "取消"
		}
		result = append(result, PermissionOption{ID: string(action), Label: label})
	}
	return result
}

func (engine *Engine) appendEvent(sessionID string, kind sessions.EventType, payload any, key string) error {
	if engine == nil || engine.store == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	raw := redact(encode(payload), key)
	if raw == "" {
		raw = "{}"
	}
	return func() error {
		_, err := engine.store.AppendEvent(sessionID, sessions.AppendEventInput{Type: kind, Payload: json.RawMessage(raw)})
		return err
	}()
}
