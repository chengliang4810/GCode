package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"gcode/internal/contracts"
)

type hookJSONOutput struct {
	Continue         *bool               `json:"continue"`
	StopReason       string              `json:"stopReason"`
	SuppressOutput   bool                `json:"suppressOutput"`
	SystemMessage    string              `json:"systemMessage"`
	TerminalSequence string              `json:"terminalSequence"`
	Decision         string              `json:"decision"`
	Reason           string              `json:"reason"`
	Specific         *hookSpecificOutput `json:"hookSpecificOutput"`
	WatchPaths       []string            `json:"watchPaths"`
}

type hookSpecificOutput struct {
	Event                contracts.HookEvent   `json:"hookEventName"`
	Context              string                `json:"additionalContext"`
	PermissionDecision   string                `json:"permissionDecision"`
	PermissionReason     string                `json:"permissionDecisionReason"`
	UpdatedInput         json.RawMessage       `json:"updatedInput"`
	UpdatedToolOutput    json.RawMessage       `json:"updatedToolOutput"`
	UpdatedMCPToolOutput json.RawMessage       `json:"updatedMCPToolOutput"`
	Decision             *hookPermissionOutput `json:"decision"`
	InitialUserMessage   string                `json:"initialUserMessage"`
	SessionTitle         string                `json:"sessionTitle"`
	WatchPaths           []string              `json:"watchPaths"`
	ReloadSkills         bool                  `json:"reloadSkills"`
	Retry                bool                  `json:"retry"`
	DisplayContent       *string               `json:"displayContent"`
	WorktreePath         string                `json:"worktreePath"`
	Action               string                `json:"action"`
	Content              json.RawMessage       `json:"content"`
}

type hookPermissionOutput struct {
	Behavior           string          `json:"behavior"`
	Message            string          `json:"message"`
	Interrupt          bool            `json:"interrupt"`
	UpdatedInput       json.RawMessage `json:"updatedInput"`
	UpdatedPermissions json.RawMessage `json:"updatedPermissions"`
}

func parseHookOutput(event contracts.HookEvent, text string, jsonOnly bool) (HookResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return HookResult{}, nil
	}
	if !strings.HasPrefix(text, "{") {
		if jsonOnly {
			return HookResult{}, errors.New("expected a JSON object")
		}
		if event == contracts.HookWorktreeCreate {
			return HookResult{WorktreePath: text}, nil
		}
		if slices.Contains([]contracts.HookEvent{contracts.HookSessionStart, contracts.HookUserPromptSubmit}, event) {
			return HookResult{Context: escapeHookContext(text)}, nil
		}
		return HookResult{}, nil
	}
	var output hookJSONOutput
	decoder := json.NewDecoder(strings.NewReader(text))
	if err := decoder.Decode(&output); err != nil {
		return HookResult{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return HookResult{}, errors.New("expected one JSON object")
	}
	if output.Decision != "" && output.Decision != "block" {
		return HookResult{}, errors.New("decision must be block or absent")
	}
	result := HookResult{Continue: output.Continue, StopReason: output.StopReason, SuppressOutput: output.SuppressOutput,
		SystemMessage: output.SystemMessage, TerminalSequence: output.TerminalSequence}
	if event == contracts.HookCwdChanged || event == contracts.HookFileChanged {
		result.WatchPaths = slices.Clone(output.WatchPaths)
	}
	if output.Decision == "block" && topLevelHookDecision(event) {
		result.Deny, result.Reason = true, firstHookText(output.Reason, "Hook blocked execution.")
	}
	if specific := output.Specific; specific != nil {
		if specific.Event == "" || specific.Event != event {
			return HookResult{}, fmt.Errorf("hookSpecificOutput.hookEventName must be %q", event)
		}
		if hookContextEvent(event) {
			result.Context = escapeHookContext(specific.Context)
		}
		switch event {
		case contracts.HookPreToolUse, contracts.HookPreModelSwitch:
			if !slices.Contains([]string{"", "allow", "deny", "ask", "defer"}, specific.PermissionDecision) ||
				event == contracts.HookPreModelSwitch && specific.PermissionDecision == "defer" {
				return HookResult{}, errors.New("invalid permissionDecision")
			}
			result.PermissionDecision = specific.PermissionDecision
			result.Reason = firstHookText(specific.PermissionReason, result.Reason)
			if specific.PermissionDecision == "deny" {
				result.Deny = true
			}
			if specific.PermissionDecision == "defer" {
				result.Context = ""
			} else if specific.UpdatedInput != nil {
				if !hookJSONObject(specific.UpdatedInput) {
					return HookResult{}, errors.New("updatedInput must be an object")
				}
				result.UpdatedInput = slices.Clone(specific.UpdatedInput)
			}
		case contracts.HookPermissionRequest:
			if decision := specific.Decision; decision != nil {
				if decision.Behavior != "allow" && decision.Behavior != "deny" {
					return HookResult{}, errors.New("decision.behavior must be allow or deny")
				}
				result.PermissionDecision, result.Reason = decision.Behavior, decision.Message
				result.Deny = decision.Behavior == "deny"
				if decision.Interrupt {
					value := false
					result.Continue = &value
					result.StopReason = decision.Message
				}
				if decision.Behavior == "allow" {
					if decision.UpdatedInput != nil && !hookJSONObject(decision.UpdatedInput) {
						return HookResult{}, errors.New("decision.updatedInput must be an object")
					}
					result.UpdatedInput = slices.Clone(decision.UpdatedInput)
					result.UpdatedPermissions = slices.Clone(decision.UpdatedPermissions)
				}
			}
		case contracts.HookPostToolUse:
			result.UpdatedToolOutput = slices.Clone(specific.UpdatedToolOutput)
			result.UpdatedMCPToolOutput = slices.Clone(specific.UpdatedMCPToolOutput)
		case contracts.HookSessionStart:
			result.InitialUserMessage, result.SessionTitle = specific.InitialUserMessage, specific.SessionTitle
			result.WatchPaths, result.ReloadSkills = slices.Clone(specific.WatchPaths), specific.ReloadSkills
		case contracts.HookPermissionDenied:
			result.Retry = specific.Retry
		case contracts.HookMessageDisplay:
			result.DisplayContent = specific.DisplayContent
		case contracts.HookWorktreeCreate:
			result.WorktreePath = specific.WorktreePath
		case contracts.HookElicitation, contracts.HookElicitationResult:
			if !slices.Contains([]string{"", "accept", "decline", "cancel"}, specific.Action) {
				return HookResult{}, errors.New("invalid elicitation action")
			}
			result.Action, result.Content = specific.Action, slices.Clone(specific.Content)
		}
		if event == contracts.HookUserPromptSubmit {
			result.SessionTitle = specific.SessionTitle
		}
	}
	if event == contracts.HookElicitation || event == contracts.HookElicitationResult {
		if result.Deny {
			result.Action = "decline"
		}
	}
	// Observational/display events cannot terminate or redirect processing.
	if hookObservationalEvent(event) {
		result.Continue, result.StopReason = nil, ""
	}
	if slices.Contains([]contracts.HookEvent{contracts.HookPreCompact, contracts.HookConfigChange, contracts.HookElicitation, contracts.HookElicitationResult, contracts.HookWorktreeCreate}, event) {
		result.Continue, result.StopReason = nil, ""
	}
	if slices.Contains([]contracts.HookEvent{contracts.HookSetup, contracts.HookInstructionsLoaded, contracts.HookMessageDisplay,
		contracts.HookNotification, contracts.HookConfigChange, contracts.HookWorktreeCreate, contracts.HookWorktreeRemove,
		contracts.HookPreCompact, contracts.HookPostCompact, contracts.HookSessionEnd, contracts.HookStopFailure,
		contracts.HookElicitation, contracts.HookElicitationResult}, event) {
		result.SystemMessage = ""
	}
	if event == contracts.HookTaskCreated {
		result.Continue, result.StopReason = nil, ""
	}
	if event == contracts.HookWorktreeRemove {
		return HookResult{}, nil
	}
	return result, nil
}

func hookJSONObject(raw json.RawMessage) bool {
	data := strings.TrimSpace(string(raw))
	return strings.HasPrefix(data, "{") && json.Valid(raw)
}

func escapeHookContext(text string) string {
	text = strings.ReplaceAll(text, "<system-reminder>", "&lt;system-reminder>")
	return strings.ReplaceAll(text, "</system-reminder>", "&lt;/system-reminder>")
}

func topLevelHookDecision(event contracts.HookEvent) bool {
	return slices.Contains([]contracts.HookEvent{contracts.HookUserPromptSubmit, contracts.HookUserPromptExpansion,
		contracts.HookPostToolUse, contracts.HookPostToolUseFailure, contracts.HookPostToolBatch,
		contracts.HookStop, contracts.HookSubagentStop, contracts.HookConfigChange, contracts.HookPreCompact,
		contracts.HookTaskCreated, contracts.HookPreModelSwitch, contracts.HookElicitation, contracts.HookElicitationResult}, event)
}

func hookContextEvent(event contracts.HookEvent) bool {
	return slices.Contains([]contracts.HookEvent{contracts.HookSessionStart, contracts.HookSubagentStart,
		contracts.HookUserPromptSubmit, contracts.HookUserPromptExpansion, contracts.HookPreToolUse,
		contracts.HookPostToolUse, contracts.HookPostToolUseFailure, contracts.HookPostToolBatch,
		contracts.HookStop, contracts.HookSubagentStop, contracts.HookPostModelSwitch}, event)
}

func hookObservationalEvent(event contracts.HookEvent) bool {
	return slices.Contains([]contracts.HookEvent{contracts.HookSetup, contracts.HookNotification,
		contracts.HookSessionEnd, contracts.HookPostCompact, contracts.HookInstructionsLoaded,
		contracts.HookStopFailure, contracts.HookCwdChanged, contracts.HookDirectoryAdded,
		contracts.HookFileChanged, contracts.HookMessageDisplay, contracts.HookPostModelSwitch}, event)
}

func exitTwoHookResult(event contracts.HookEvent, reason string) HookResult {
	if event == contracts.HookPermissionRequest || event == contracts.HookPermissionDenied || hookObservationalEvent(event) {
		return HookResult{}
	}
	if event == contracts.HookPostToolUse || event == contracts.HookPostToolUseFailure {
		return HookResult{Context: escapeHookContext(reason)}
	}
	if event == contracts.HookSessionStart || event == contracts.HookSubagentStart {
		return HookResult{SystemMessage: reason}
	}
	result := HookResult{Deny: true, Reason: reason}
	if event == contracts.HookElicitation || event == contracts.HookElicitationResult {
		result.Action = "decline"
	}
	return result
}

func parseHookEvaluation(rule contracts.HookRule, output string) (HookResult, error) {
	var evaluation struct {
		OK         *bool  `json:"ok"`
		Reason     string `json:"reason"`
		Impossible bool   `json:"impossible"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &evaluation); err != nil {
		return HookResult{}, err
	}
	if evaluation.OK == nil {
		return HookResult{}, errors.New("hook evaluator must return ok")
	}
	if *evaluation.OK {
		return HookResult{}, nil
	}
	if strings.TrimSpace(evaluation.Reason) == "" {
		return HookResult{}, errors.New("hook evaluator requires reason when ok is false")
	}
	if rule.Event == contracts.HookPermissionRequest || rule.Event == contracts.HookPermissionDenied {
		return HookResult{}, nil
	}
	if rule.Type == "prompt" && evaluation.Impossible && (rule.Event == contracts.HookStop || rule.Event == contracts.HookSubagentStop) {
		return HookResult{}, nil
	}
	result := HookResult{Deny: true, Reason: evaluation.Reason}
	if rule.Type == "prompt" && !rule.ContinueOnBlock && slices.Contains([]contracts.HookEvent{contracts.HookPreToolUse,
		contracts.HookPostToolUse, contracts.HookTeammateIdle}, rule.Event) {
		value := false
		result.Continue, result.StopReason = &value, evaluation.Reason
	}
	return result, nil
}
