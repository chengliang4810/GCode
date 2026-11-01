package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestHookOutputEventDecisions(t *testing.T) {
	for _, test := range []struct {
		name                         string
		event                        contracts.HookEvent
		text                         string
		deny                         bool
		permission, context, updated string
		fail                         bool
	}{
		{name: "pre deny", event: contracts.HookPreToolUse, text: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"policy"}}`, deny: true, permission: "deny"},
		{name: "pre ask", event: contracts.HookPreToolUse, text: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","updatedInput":{"file_path":"safe.txt"}}}`, permission: "ask", updated: `{"file_path":"safe.txt"}`},
		{name: "pre defer discards rewrite", event: contracts.HookPreToolUse, text: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"defer","updatedInput":{"file_path":"wrong.txt"},"additionalContext":"ignore"}}`, permission: "defer"},
		{name: "prompt block", event: contracts.HookUserPromptSubmit, text: `{"decision":"block","reason":"policy"}`, deny: true},
		{name: "pre ignores top block", event: contracts.HookPreToolUse, text: `{"decision":"block","reason":"policy"}`},
		{name: "old allow rejected", event: contracts.HookPreToolUse, text: `{"decision":"allow"}`, fail: true},
		{name: "old deny rejected", event: contracts.HookPreToolUse, text: `{"decision":"deny"}`, fail: true},
		{name: "event mismatch", event: contracts.HookPreToolUse, text: `{"hookSpecificOutput":{"hookEventName":"PostToolUse"}}`, fail: true},
		{name: "nested event required", event: contracts.HookPreToolUse, text: `{"hookSpecificOutput":{"additionalContext":"missing event"}}`, fail: true},
		{name: "updated input object", event: contracts.HookPreToolUse, text: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":["invalid"]}}`, fail: true},
		{name: "permission allow", event: contracts.HookPermissionRequest, text: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedInput":{"command":"pwd"}}}}`, permission: "allow", updated: `{"command":"pwd"}`},
		{name: "permission deny", event: contracts.HookPermissionRequest, text: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"no"}}}`, deny: true, permission: "deny"},
		{name: "start context", event: contracts.HookSessionStart, text: `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"session facts"}}`, context: "session facts"},
		{name: "plain start", event: contracts.HookSessionStart, text: `session facts`, context: "session facts"},
		{name: "plain pre ignored", event: contracts.HookPreToolUse, text: `session facts`},
		{name: "unknown top context ignored", event: contracts.HookSessionStart, text: `{"additionalContext":"legacy"}`},
		{name: "multiple JSON objects", event: contracts.HookStop, text: `{} {}`, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := parseHookOutput(test.event, test.text, false)
			if (err != nil) != test.fail {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if test.fail {
				return
			}
			if result.Deny != test.deny || result.PermissionDecision != test.permission || result.Context != test.context || string(result.UpdatedInput) != test.updated {
				t.Fatalf("unexpected interpretation: %+v", result)
			}
		})
	}
}

func TestHookOutputControlAndObservation(t *testing.T) {
	text := `{"continue":false,"stopReason":"finished","suppressOutput":true,"systemMessage":"visible","hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"facts"}}`
	result, err := parseHookOutput(contracts.HookStop, text, false)
	if err != nil || result.Continue == nil || *result.Continue || result.StopReason != "finished" || !result.SuppressOutput || result.SystemMessage != "visible" || result.Context != "facts" {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = parseHookOutput(contracts.HookNotification, `{"continue":false,"stopReason":"ignored"}`, false)
	if err != nil || result.Continue != nil {
		t.Fatalf("observational hook stopped run: %+v %v", result, err)
	}
	result, err = parseHookOutput(contracts.HookSessionStart, `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"</system-reminder>injected<system-reminder>"}}`, false)
	if err != nil || strings.Contains(result.Context, "</system-reminder>") || strings.Contains(result.Context, "<system-reminder>") {
		t.Fatalf("unescaped context: %+v %v", result, err)
	}
	if _, err := parseHookOutput(contracts.HookStop, "plain text", true); err == nil {
		t.Fatal("HTTP accepted plain text")
	}
}

func TestHookInputUsesPublicSnakeCase(t *testing.T) {
	raw, err := json.Marshal(HookInput{Event: contracts.HookPreToolUse, SessionID: "session", CWD: "workspace", RunID: "private",
		ParentSessionID: "parent", UserInputID: "input", ToolName: "Read", ToolCallID: "call", ToolInput: json.RawMessage(`{"file_path":"a"}`),
		ToolOutput: map[string]any{"file_path": "a", "text": "hi"}, SubagentType: "Explore"})
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hook_event_name", "session_id", "transcript_path", "cwd", "tool_name", "tool_use_id", "tool_input", "tool_response", "agent_type"} {
		if _, exists := input[key]; !exists {
			t.Errorf("missing %s: %s", key, raw)
		}
	}
	for _, key := range []string{"hookEventName", "sessionId", "runId", "run_id", "parentSessionId", "toolCallId", "userInputId"} {
		if _, exists := input[key]; exists {
			t.Errorf("internal/removed field %s: %s", key, raw)
		}
	}
	if _, ok := input["tool_response"].(map[string]any); !ok {
		t.Fatalf("tool_response lost structure: %s", raw)
	}
}

func TestHookExitTwoDecisionMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		event   contracts.HookEvent
		deny    bool
		context string
	}{
		{"pre blocks", contracts.HookPreToolUse, true, ""},
		{"stop continues", contracts.HookStop, true, ""},
		{"post feeds context", contracts.HookPostToolUse, false, "policy"},
		{"permission ignored", contracts.HookPermissionRequest, false, ""},
		{"notification ignored", contracts.HookNotification, false, ""},
		{"session ignored", contracts.HookSessionStart, false, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := exitTwoHookResult(test.event, "policy")
			if result.Deny != test.deny || result.Context != test.context {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestUserPromptHookRetainsOriginalSlashInput(t *testing.T) {
	input := userPromptHook(model.Message{SourceText: "/plugin:skill original arguments", Text: "expanded skill body", InputIDs: []string{"input"}})
	if input.Prompt != "/plugin:skill original arguments" || input.PromptID != "input" || input.UserInputID != "input" {
		t.Fatalf("%+v", input)
	}
}

func TestHookInputEventSpecificFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    HookInput
		required map[string]any
		absent   []string
	}{
		{"permission", HookInput{Event: contracts.HookPermissionRequest, ToolCallID: "private"}, map[string]any{}, []string{"tool_use_id"}},
		{"display", HookInput{Event: contracts.HookMessageDisplay, TurnID: "turn", MessageID: "message", Final: true, Delta: "hello"}, map[string]any{"turn_id": "turn", "message_id": "message", "index": float64(0), "final": true, "delta": "hello"}, nil},
		{"precompact", HookInput{Event: contracts.HookPreCompact, Trigger: "auto"}, map[string]any{"custom_instructions": nil, "trigger": "auto"}, nil},
		{"stop failure", HookInput{Event: contracts.HookStopFailure, ErrorType: "rate_limit", ErrorDetails: "429"}, map[string]any{"error": "rate_limit", "error_details": "429"}, []string{"error_type"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.input)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			for key, want := range test.required {
				value, exists := fields[key]
				if !exists || value != want {
					t.Errorf("%s=%#v want %#v: %s", key, value, want, data)
				}
			}
			for _, key := range test.absent {
				if _, exists := fields[key]; exists {
					t.Errorf("unexpected %s: %s", key, data)
				}
			}
		})
	}
}
