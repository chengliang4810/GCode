package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestHookTimeoutPreservesFractionalSeconds(t *testing.T) {
	for _, test := range []struct {
		name string
		rule contracts.HookRule
		want time.Duration
	}{
		{"fractional", contracts.HookRule{TimeoutSeconds: 0.125}, 125 * time.Millisecond},
		{"session end fractional", contracts.HookRule{Event: contracts.HookSessionEnd, TimeoutSeconds: 0.125}, 125 * time.Millisecond},
		{"session end default", contracts.HookRule{Event: contracts.HookSessionEnd}, 1500 * time.Millisecond},
		{"session end cap", contracts.HookRule{Event: contracts.HookSessionEnd, TimeoutSeconds: 60.125}, time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := hookTimeout(test.rule); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestHookHTTPTransmitsJSONAndRestrictsEnvironment(t *testing.T) {
	t.Setenv("GCODE_HOOK_ALLOWED", "token")
	t.Setenv("GCODE_HOOK_PRIVATE", "secret")
	var received HookInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid HTTP hook request")
		}
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-Private") != "" {
			t.Errorf("header expansion: %v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"http policy"}}`))
	}))
	defer server.Close()
	runner := NewHookRunner([]contracts.HookRule{{ID: "http", Event: contracts.HookPreToolUse, Type: "http", URL: server.URL, Enabled: true,
		Headers:        map[string]string{"Authorization": "Bearer $GCODE_HOOK_ALLOWED", "X-Private": "${GCODE_HOOK_PRIVATE}"},
		AllowedEnvVars: []string{"GCODE_HOOK_ALLOWED"}}}, nil)
	result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookPreToolUse, SessionID: "session", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"pwd"}`)})
	if err != nil || !result.Deny || received.SessionID != "session" || received.ToolName != "Bash" {
		t.Fatalf("%+v %+v %v", result, received, err)
	}
}

func TestHookHTTPFailuresAndFailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "failed", http.StatusServiceUnavailable) }))
	defer server.Close()
	for _, test := range []struct {
		name, onFailure string
		deny            bool
	}{
		{"default nonblocking", "", false}, {"closed", "block", true}, {"explicit continue", "continue", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var execution HookExecution
			runner := NewHookRunner([]contracts.HookRule{{ID: "http", Event: contracts.HookPreToolUse, Type: "http", URL: server.URL, Enabled: true, OnFailure: test.onFailure}},
				func(value HookExecution) error { execution = value; return nil })
			result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookPreToolUse})
			if err != nil || result.Deny != test.deny || execution.Err == nil {
				t.Fatalf("%+v %+v %v", result, execution, err)
			}
		})
	}
}

func TestHookInjectedEvaluatorsAndSkillOnce(t *testing.T) {
	var calls atomic.Int32
	rule := contracts.HookRule{ID: "prompt", Event: contracts.HookStop, Type: "prompt", Prompt: "Check $ARGUMENTS", Enabled: true, Once: true, Source: "skill"}
	runner := NewHookRunner([]contracts.HookRule{rule}, nil, WithHookEvaluator(func(_ context.Context, handler contracts.HookRule, input HookInput) (HookEvaluation, error) {
		if !strings.Contains(handler.Prompt, `"hook_event_name":"Stop"`) || strings.Contains(handler.Prompt, "$ARGUMENTS") {
			t.Error("prompt was not expanded")
		}
		if calls.Add(1) == 1 {
			return HookEvaluation{Output: `{"ok":false,"reason":"check tests"}`}, nil
		}
		return HookEvaluation{Output: `{"ok":true}`}, nil
	}))
	for i := 0; i < 3; i++ {
		result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookStop})
		if err != nil || result.Deny != (i == 0) {
			t.Fatalf("run %d %+v %v", i, result, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("blocked attempt consumed once hook: %d", calls.Load())
	}
}

func TestHookScopedRulesAndOnceAcrossRuns(t *testing.T) {
	state := &HookOnceState{}
	var calls atomic.Int32
	rule := contracts.HookRule{ID: "skill-once", Event: contracts.HookStop, Type: "prompt", Prompt: "check", Enabled: true, Source: "skill", Once: true}
	evaluator := WithHookEvaluator(func(context.Context, contracts.HookRule, HookInput) (HookEvaluation, error) {
		calls.Add(1)
		return HookEvaluation{Output: `{"ok":true}`}, nil
	})
	for i := 0; i < 2; i++ {
		runner := NewHookRunner(nil, nil, evaluator, WithHookOnceState(state))
		cleanup, err := runner.PushRules([]contracts.HookRule{rule})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runner.Run(context.Background(), HookInput{Event: contracts.HookStop}); err != nil {
			t.Fatal(err)
		}
		cleanup()
		cleanup()
		if _, err := runner.Run(context.Background(), HookInput{Event: contracts.HookStop}); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("once state did not survive runs: %d", calls.Load())
	}
}

func TestHookMissingEvaluatorReportsUnsupportedRuntime(t *testing.T) {
	var execution HookExecution
	runner := NewHookRunner([]contracts.HookRule{{ID: "prompt", Event: contracts.HookStop, Type: "prompt", Prompt: "check", Enabled: true}},
		func(value HookExecution) error { execution = value; return nil })
	result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookStop})
	if err != nil || result.Deny || execution.Err == nil || !strings.Contains(execution.Err.Error(), "connected prompt evaluator") {
		t.Fatalf("%+v %+v %v", result, execution, err)
	}
}

func TestHookAsyncRewakeAndNotification(t *testing.T) {
	rule := testHookRule(t, contracts.HookStop, "deny", "background policy")
	rule.AsyncRewake = true
	var wakes atomic.Int32
	runner := NewHookRunner([]contracts.HookRule{rule}, nil, WithHookRewake(func(_ context.Context, execution HookExecution) error {
		if execution.ExitCode != 2 || execution.Stderr != "background policy" || execution.Result.Deny {
			t.Error("invalid background decision")
		}
		wakes.Add(1)
		return nil
	}))
	result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookStop, CWD: t.TempDir()})
	if err != nil || result.Deny {
		t.Fatalf("%+v %v", result, err)
	}
	if err := runner.WaitBackground(); err != nil {
		t.Fatal(err)
	}
	if wakes.Load() != 1 {
		t.Fatal("asyncRewake did not notify session owner")
	}

	rule = testHookRule(t, contracts.HookPostToolUse, "output", `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"background facts"},"systemMessage":"done"}`)
	rule.Async = true
	var notifications atomic.Int32
	runner = NewHookRunner([]contracts.HookRule{rule}, nil, WithHookAsyncResult(func(_ context.Context, execution HookExecution) error {
		if execution.Result.Context != "background facts" || execution.Result.SystemMessage != "done" {
			t.Errorf("%+v", execution.Result)
		}
		notifications.Add(1)
		return nil
	}))
	result, err = runner.Run(context.Background(), HookInput{Event: contracts.HookPostToolUse, CWD: t.TempDir()})
	if err != nil || result.Context != "" {
		t.Fatalf("late context injected synchronously: %+v %v", result, err)
	}
	if err := runner.WaitBackground(); err != nil {
		t.Fatal(err)
	}
	if notifications.Load() != 1 {
		t.Fatal("async result not delivered")
	}
}

func TestHookCommandExecFormEnvironmentAndEmptyArgs(t *testing.T) {
	directory := t.TempDir()
	input := HookInput{Event: contracts.HookSessionStart, CWD: directory, Effort: &HookEffort{Level: "high"}, EnvFile: filepath.Join(directory, "env")}
	rule := contracts.HookRule{Command: "${CLAUDE_PLUGIN_ROOT}/helper", Args: []string{}, PluginRoot: directory, PluginData: filepath.Join(directory, "data"),
		UserConfig: map[string]string{"key": "value"}}
	program, args, err := hookCommand(rule, input)
	if err != nil || program != directory+"/helper" || args == nil || len(args) != 0 {
		t.Fatalf("%s %#v %v", program, args, err)
	}
	env := strings.Join(hookEnvironment(rule, input), "\n")
	for _, value := range []string{"CLAUDE_PROJECT_DIR=" + directory, "CLAUDE_PLUGIN_ROOT=" + directory, "CLAUDE_PLUGIN_DATA=" + rule.PluginData,
		"CLAUDE_PLUGIN_OPTION_KEY=value", "CLAUDE_EFFORT=high", "CLAUDE_ENV_FILE=" + input.EnvFile} {
		if !strings.Contains(env, value) {
			t.Errorf("missing %s", value)
		}
	}
	if _, _, err := hookCommand(contracts.HookRule{Command: "echo ${user_config.key}"}, input); err == nil {
		t.Fatal("shell form interpolated user config")
	}
}

type hookEchoTool struct{ executed *json.RawMessage }

func (t hookEchoTool) Definition() model.Tool { return model.Tool{Name: "Bash"} }
func (t hookEchoTool) Concurrent() bool       { return false }
func (t hookEchoTool) Execute(_ context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	*t.executed = append(json.RawMessage(nil), raw...)
	return model.ToolOutput{Text: "done"}, nil
}

func TestHookToolParameterUpdateAndPermissionRequest(t *testing.T) {
	for _, test := range []struct {
		name    string
		approve bool
	}{{"approved", true}, {"denied", false}} {
		t.Run(test.name, func(t *testing.T) {
			var executed, updated json.RawMessage
			var events []contracts.HookEvent
			tool := hookedTool{Tool: hookEchoTool{executed: &executed}, inputUpdated: func(raw json.RawMessage) { updated = raw },
				hook: func(_ context.Context, input HookInput) (HookResult, error) {
					events = append(events, input.Event)
					switch input.Event {
					case contracts.HookPreToolUse:
						return HookResult{PermissionDecision: "ask", UpdatedInput: json.RawMessage(`{"command":"pwd"}`)}, nil
					case contracts.HookPermissionRequest:
						if string(input.ToolInput) != `{"command":"pwd"}` {
							t.Errorf("permission saw stale arguments: %s", input.ToolInput)
						}
						if test.approve {
							return HookResult{PermissionDecision: "allow"}, nil
						}
						return HookResult{Deny: true, Reason: "denied"}, nil
					}
					return HookResult{}, nil
				}}
			_, err := tool.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"command":"unsafe"}`))
			if (err == nil) != test.approve || string(updated) != `{"command":"pwd"}` {
				t.Fatalf("updated=%s error=%v", updated, err)
			}
			if test.approve && string(executed) != `{"command":"pwd"}` || !test.approve && executed != nil {
				t.Fatalf("wrong executable parameters: %s", executed)
			}
			if len(events) < 2 || events[1] != contracts.HookPermissionRequest {
				t.Fatalf("%v", events)
			}
		})
	}
}

func TestHookPermissionAskRequiresConnectedDecision(t *testing.T) {
	runner := NewHookRunner(nil, nil, WithHookPermissionRequest(func(_ context.Context, input HookInput, reason string) (bool, error) {
		if reason != "guard request" {
			t.Errorf("reason=%q", reason)
		}
		return true, nil
	}))
	result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookPermissionRequest, PermissionReason: "guard request"})
	if err != nil || result.Deny || result.PermissionDecision != "allow" {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = NewHookRunner(nil, nil).Run(context.Background(), HookInput{Event: contracts.HookPermissionRequest})
	if err != nil || !result.Deny {
		t.Fatalf("unconnected permission request silently allowed: %+v %v", result, err)
	}
}

func TestHookRulesExecuteInParallelWithStableAggregation(t *testing.T) {
	var entered atomic.Int32
	allEntered := make(chan struct{})
	rules := []contracts.HookRule{
		{ID: "first", Event: contracts.HookStop, Type: "mcp_tool", MCPServer: "fixture", ToolName: "first-hook", Enabled: true},
		{ID: "second", Event: contracts.HookStop, Type: "mcp_tool", MCPServer: "fixture", ToolName: "second-hook", Enabled: true},
	}
	runner := NewHookRunner(rules, nil, WithHookEvaluator(func(ctx context.Context, rule contracts.HookRule, input HookInput) (HookEvaluation, error) {
		if entered.Add(1) == 2 {
			close(allEntered)
		}
		select {
		case <-allEntered:
		case <-ctx.Done():
			return HookEvaluation{}, ctx.Err()
		}
		return HookEvaluation{Output: `{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"` + rule.ID + `"}}`}, nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := runner.Run(ctx, HookInput{Event: contracts.HookStop})
	if err != nil || result.Context != "first\nsecond" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestHookRunnerReportFailurePropagates(t *testing.T) {
	expected := errors.New("audit unavailable")
	var mu sync.Mutex
	var reports int
	runner := NewHookRunner([]contracts.HookRule{testHookRule(t, contracts.HookSessionStart, "output", "facts")},
		func(HookExecution) error { mu.Lock(); reports++; mu.Unlock(); return expected })
	_, err := runner.Run(context.Background(), HookInput{Event: contracts.HookSessionStart, CWD: t.TempDir()})
	if !errors.Is(err, expected) || reports != 1 {
		t.Fatalf("reports=%d err=%v", reports, err)
	}
}

func TestHookRunnerDeduplicatesMatchingHandlers(t *testing.T) {
	first := contracts.HookRule{ID: "one", Event: contracts.HookPreToolUse, Type: "mcp_tool", MCPServer: "fixture", ToolName: "hook", Enabled: true, Matcher: "Bash"}
	second := first
	second.ID, second.Matcher = "two", "Bash|Read"
	var calls atomic.Int32
	runner := NewHookRunner([]contracts.HookRule{first, second}, nil, WithHookEvaluator(func(context.Context, contracts.HookRule, HookInput) (HookEvaluation, error) {
		calls.Add(1)
		return HookEvaluation{Output: `{}`}, nil
	}))
	if _, err := runner.Run(context.Background(), HookInput{Event: contracts.HookPreToolUse, ToolName: "Bash"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate command ran %d times", calls.Load())
	}
}

func TestHookExitTwoRetainsJSONFieldsAndBlockingDecision(t *testing.T) {
	rule := testHookRule(t, contracts.HookPreToolUse, "deny-json", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","additionalContext":"facts"}}`)
	result, err := NewHookRunner([]contracts.HookRule{rule}, nil).Run(context.Background(), HookInput{Event: contracts.HookPreToolUse, CWD: t.TempDir()})
	if err != nil || !result.Deny || result.Context != "facts" || result.Reason != "policy" {
		t.Fatalf("%+v %v", result, err)
	}
}
