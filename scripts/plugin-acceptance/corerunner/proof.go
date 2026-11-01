package corerunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/mcp"
	"gcode/scripts/plugin-acceptance/acceptance"
)

func completedTools(state contracts.Snapshot) []contracts.ToolCall {
	var result []contracts.ToolCall
	for _, messages := range state.Messages {
		for _, message := range messages {
			for _, block := range message.Blocks {
				if block.Tool != nil && block.Tool.Status == "completed" && !block.Tool.IsError {
					result = append(result, *block.Tool)
					continue
				}
				// The engine replaces a finished Agent call with a child-link
				// block, so the result the parent model actually received lives
				// on Subagent in the exact subagentResult wire format.
				if block.Subagent == nil || block.Subagent.Status == "" || block.Subagent.Status == "running" {
					continue
				}
				sessionID := block.Subagent.SessionID
				if sessionID == "" {
					sessionID = block.Subagent.ID
				}
				result = append(result, contracts.ToolCall{
					ID:     block.Subagent.ID,
					Name:   "Agent",
					Status: "completed",
					Output: fmt.Sprintf("child_session_id: %s\nstatus: %s\n%s", sessionID, block.Subagent.Status, block.Subagent.Summary),
				})
			}
		}
	}
	return result
}

func hookRuleID(pluginID string, component acceptance.Component) string {
	name := component.Name
	if strings.HasPrefix(name, "skill/") || strings.HasPrefix(name, "command/") {
		_, name, _ = strings.Cut(name, "/")
		return name
	}
	return pluginID + "/" + name
}

func triggerEvidence(request acceptance.RunRequest, component acceptance.Component, plan ProbePlan, seen observation) (map[string]any, error) {
	switch component.Kind {
	case "skill", "command":
		for _, event := range seen.events {
			if event.Kind == "skill.invoked" && event.ResourceID == component.Name && (event.PluginID == "" || event.PluginID == request.PluginID) {
				if _, err := actualModelResponse(seen, request); err != nil {
					return nil, err
				}
				return map[string]any{"eventId": event.EventID, "resourceId": event.ResourceID, "runId": event.RunID}, nil
			}
		}
		return nil, fmt.Errorf("no actual skill.invoked event for %s", component.Name)
	case "agent":
		for _, session := range seen.state.Sessions {
			if session.SubagentType == component.Name && session.ParentSessionID != "" && session.LastOutcome == "completed" {
				if _, err := actualModelResponse(seen, request); err != nil {
					return nil, err
				}
				return map[string]any{"childSessionId": session.ID, "subagentType": session.SubagentType, "outcome": session.LastOutcome}, nil
			}
		}
		return nil, fmt.Errorf("no completed core child agent for %s", component.Name)
	case "hook":
		expected := hookRuleID(request.PluginID, component)
		for _, event := range seen.events {
			if event.Kind != "hook.finished" || event.ComponentID != expected || (event.PluginID != "" && event.PluginID != request.PluginID) {
				continue
			}
			if event.Error != "" {
				return nil, fmt.Errorf("hook %s failed: %s", expected, event.Error)
			}
			if event.ExitCode == nil {
				return nil, errors.New("hook event has no actual exit code")
			}
			return map[string]any{"eventId": event.EventID, "ruleId": event.ComponentID, "exitCode": *event.ExitCode, "output": event.Text}, nil
		}
		return nil, fmt.Errorf("no exact hook.finished event for %s", expected)
	case "mcp":
		if plan.MCPTool == "" {
			return nil, errors.New("MCP probe must select an explicit server tool")
		}
		name, err := mcp.QualifiedToolName(mcp.Config{Name: component.Name, Plugin: request.Bundle.Manifest.Name}, plan.MCPTool)
		if err != nil {
			return nil, err
		}
		for _, tool := range completedTools(seen.state) {
			if tool.Name == name {
				return map[string]any{"toolId": tool.ID, "qualifiedName": tool.Name, "args": tool.Args, "output": tool.Output}, nil
			}
		}
		return nil, fmt.Errorf("no successful tools/call for exact MCP server/tool %s", name)
	}
	return nil, errors.New("runtime component is unsupported")
}

func actualModelResponse(seen observation, request acceptance.RunRequest) (string, error) {
	expected := strings.TrimPrefix(request.Provider.ModelID, request.Provider.Provider.ID+"/")
	for i := len(seen.wire) - 1; i >= 0; i-- {
		call := seen.wire[i]
		if call.Status >= 200 && call.Status < 300 && call.Model == expected && call.ResponseSHA256 != "" && call.Error == "" {
			if call.ResponseID != "" {
				return call.ResponseID, nil
			}
			return "sha256:" + call.ResponseSHA256, nil
		}
	}
	return "", errors.New("no actual successful HTTP model response for the configured test model")
}

func assertBusiness(component acceptance.Component, plan ProbePlan, seen observation, pluginID string) ([]AssertionResult, error) {
	var results []AssertionResult
	for _, assertion := range plan.Assertions {
		result := AssertionResult{Kind: assertion.Kind, Path: assertion.Path}
		var err error
		switch assertion.Kind {
		case "file", "file-exists", "file-absent":
			path, pathErr := workspacePath(seen.workspace, assertion.Path)
			if pathErr != nil {
				return results, pathErr
			}
			info, statErr := os.Lstat(path)
			if assertion.Kind == "file-absent" {
				result.Matched = errors.Is(statErr, os.ErrNotExist)
				break
			}
			if statErr != nil {
				return results, statErr
			}
			if !info.Mode().IsRegular() || info.Size() > 8<<20 {
				return results, errors.New("assertion target is not a bounded regular fixture file")
			}
			if assertion.Kind == "file-exists" {
				result.Matched = true
				break
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return results, readErr
			}
			result.Actual = string(data)
			result.Matched, err = matches(result.Actual, assertion)
		case "text":
			result.Actual = seen.assistant
			if assertion.Source == "tool" {
				var output strings.Builder
				for _, tool := range completedTools(seen.state) {
					if assertion.ToolName == "" || tool.Name == assertion.ToolName {
						output.WriteString(tool.Output)
						output.WriteByte('\n')
					}
				}
				result.Actual = output.String()
			}
			if assertion.Source == "hook" {
				if component.Kind != "hook" {
					return results, errors.New("hook output assertion requires a hook component")
				}
				expected := hookRuleID(pluginID, component)
				found := false
				for _, event := range seen.events {
					if event.Kind == "hook.finished" && event.ComponentID == expected && event.ExitCode != nil {
						found = true
						result.Actual = event.Text
					}
				}
				if !found {
					return results, fmt.Errorf("hook output assertion has no finished event for %s", expected)
				}
			}
			result.Matched, err = matches(result.Actual, assertion)
		case "exit":
			found := false
			if component.Kind == "hook" {
				expected := hookRuleID(pluginID, component)
				for _, event := range seen.events {
					if event.Kind == "hook.finished" && event.ComponentID == expected && event.ExitCode != nil {
						found = true
						result.Actual = fmt.Sprint(*event.ExitCode)
						result.Matched = *event.ExitCode == *assertion.ExitCode
					}
				}
			} else {
				for _, tool := range completedTools(seen.state) {
					if tool.Name == assertion.ToolName && tool.ExitCode != nil {
						found = true
						result.Actual = fmt.Sprint(*tool.ExitCode)
						result.Matched = *tool.ExitCode == *assertion.ExitCode
					}
				}
			}
			if !found {
				return results, errors.New("exit assertion has no exact observed process exit code; nil is not success")
			}
		default:
			return results, errors.New("unsupported assertion")
		}
		if err != nil {
			return results, err
		}
		results = append(results, result)
		if !result.Matched {
			return results, fmt.Errorf("%s business assertion failed for %s", assertion.Kind, assertion.Path)
		}
	}
	return results, nil
}

func matches(actual string, assertion Assertion) (bool, error) {
	if assertion.Equals != nil && actual != *assertion.Equals {
		return false, nil
	}
	if assertion.Contains != "" && !strings.Contains(actual, assertion.Contains) {
		return false, nil
	}
	if assertion.Regex != "" {
		return regexp.MatchString(assertion.Regex, actual)
	}
	return true, nil
}

func probePayload(component acceptance.Component, plan ProbePlan, seen observation, matched map[string]any, results []AssertionResult) (map[string]any, map[string]any) {
	output := map[string]any{"response": seen.assistant, "observed": matched, "assertions": results}
	input := map[string]any{"prompt": plan.Prompt, "componentId": component.ID}
	budgets := make([]map[string]any, 0, len(seen.wire))
	for _, call := range seen.wire {
		budgets = append(budgets, map[string]any{"model": call.Model, "maxOutputTokens": call.MaxOutputTokens,
			"reasoningEffort": call.ReasoningEffort, "thinkingType": call.ThinkingType, "thinkingBudgetTokens": call.ThinkingBudgetTokens,
			"requestSha256": call.RequestSHA256})
	}
	input["modelRequests"] = budgets
	switch component.Kind {
	case "hook":
		event := component.Name
		parts := strings.Split(event, "/")
		if len(parts) >= 3 {
			event = parts[len(parts)-3]
		}
		input["hook_event_name"] = event
		output["stdout"] = matched["output"]
		output["exitCode"] = matched["exitCode"]
		output["transport"] = "core-hook-runner"
	case "mcp":
		input["method"] = "tools/call"
		input["params"] = map[string]any{"name": plan.MCPTool, "arguments": matched["args"]}
		output["content"] = []any{map[string]any{"type": "text", "text": matched["output"]}}
	default:
		for i := len(seen.wire) - 1; i >= 0; i-- {
			if seen.wire[i].ResponseSHA256 != "" {
				output["modelResponseId"] = "sha256:" + seen.wire[i].ResponseSHA256
				break
			}
		}
	}
	return input, output
}

func writeBoundTrace(request acceptance.RunRequest, component acceptance.Component, probe acceptance.Probe, root string, start, finish time.Time, input, output any, result *acceptance.Probe, secrets []string) error {
	var log []byte
	for _, item := range []struct {
		phase   string
		at      time.Time
		payload any
	}{{"invoke", start, input}, {"result", finish, output}} {
		payload, err := redactedJSON(item.payload, secrets)
		if err != nil {
			return err
		}
		record := acceptance.TraceRecord{RunID: request.RunID, PluginID: request.PluginID, MarketplaceCommit: request.MarketplaceCommit, SourceCommit: request.SourceCommit,
			ArtifactDigest: request.ArtifactDigest, Platform: request.Platform, Model: request.Model, ProbeID: probe.ID, ComponentID: component.ID, ComponentDigest: component.Digest,
			Operation: probe.Operation, Phase: item.phase, At: item.at, Payload: payload}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		log = append(log, data...)
		log = append(log, '\n')
	}
	path := filepath.Join(root, "probe.jsonl")
	if err := os.WriteFile(path, log, 0600); err != nil {
		return err
	}
	result.Log = acceptance.EvidenceFile{Path: path, SHA256: digest(log)}
	return nil
}

func saveRedacted(path string, value any, secrets []string) error {
	data, err := redactedJSON(value, secrets)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func redactedJSON(value any, secrets []string) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	var redact func(any) any
	redact = func(value any) any {
		switch current := value.(type) {
		case string:
			for _, secret := range secrets {
				if secret != "" {
					current = strings.ReplaceAll(current, secret, "[REDACTED]")
				}
			}
			return current
		case []any:
			for i := range current {
				current[i] = redact(current[i])
			}
			return current
		case map[string]any:
			for key, v := range current {
				current[key] = redact(v)
			}
			return current
		default:
			return value
		}
	}
	return json.MarshalIndent(redact(tree), "", "  ")
}
