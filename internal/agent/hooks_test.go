package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestHookProcessHelper(t *testing.T) {
	at := slices.Index(os.Args, "--")
	if at < 0 || len(os.Args) <= at+3 || os.Args[at+1] != "hook-helper" {
		return
	}
	mode, payload := os.Args[at+2], os.Args[at+3]
	data, _ := io.ReadAll(os.Stdin)
	switch mode {
	case "capture":
		_ = os.WriteFile(payload, data, 0600)
	case "output":
		fmt.Print(payload)
	case "deny":
		fmt.Fprint(os.Stderr, payload)
		os.Exit(2)
	case "deny-json":
		fmt.Print(payload)
		fmt.Fprint(os.Stderr, "policy")
		os.Exit(2)
	case "fail":
		fmt.Fprint(os.Stderr, payload)
		os.Exit(3)
	case "wait":
		_ = os.WriteFile(payload, []byte("started"), 0600)
		time.Sleep(30 * time.Second)
	}
	os.Exit(0)
}

func testHookRule(t *testing.T, event contracts.HookEvent, mode, payload string) contracts.HookRule {
	t.Helper()
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.HookRule{
		ID: string(event), Event: event, Type: "command", Command: program,
		Args:    []string{"-test.run=^TestHookProcessHelper$", "--", "hook-helper", mode, payload},
		Enabled: true, TimeoutSeconds: 5,
	}
}

func testHookShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return ""
}

func TestHookRunnerSendsJSONWithoutShellInterpolation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "input.json")
	rule := testHookRule(t, contracts.HookPreToolUse, "capture", path)
	rule.Matcher = "^Write$"
	runner := NewHookRunner([]contracts.HookRule{rule}, nil)
	input := HookInput{Event: contracts.HookPreToolUse, CWD: directory, SessionID: "s", ToolName: "Read"}
	if _, err := runner.Run(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("nonmatching hook executed")
	}
	input.ToolName, input.ToolInput = "Write", json.RawMessage(`{"text":"$(echo private) ; & exit 9"}`)
	if _, err := runner.Run(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var received HookInput
	if err := json.Unmarshal(data, &received); err != nil {
		t.Fatal(err)
	}
	var toolInput map[string]string
	if err := json.Unmarshal(received.ToolInput, &toolInput); err != nil {
		t.Fatal(err)
	}
	if received.SessionID != "s" || toolInput["text"] != "$(echo private) ; & exit 9" {
		t.Fatalf("hook input changed: %s", data)
	}
}

func TestHookRunnerBlockingAndInvalidOutput(t *testing.T) {
	for _, test := range []struct {
		name, mode, output string
		denied             bool
		wantErr            bool
	}{
		{"json deny", "output", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"policy"}}`, true, false},
		{"exit deny", "deny", "policy", true, false},
		{"context", "output", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"run tests"}}`, false, false},
		{"invalid json", "output", `{"decision":`, false, true},
		{"invalid decision", "output", `{"decision":"maybe"}`, false, true},
		{"unknown field ignored", "output", `{"shell":"ignored"}`, false, false},
		{"trailing json", "output", `{} {}`, false, true},
		{"process failed", "fail", "failure", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule := testHookRule(t, contracts.HookPreToolUse, test.mode, test.output)
			var reported HookExecution
			runner := NewHookRunner([]contracts.HookRule{rule}, func(execution HookExecution) error {
				reported = execution
				return nil
			})
			result, err := runner.Run(context.Background(), HookInput{
				Event: contracts.HookPreToolUse, CWD: t.TempDir(), ToolName: "Write",
			})
			if err != nil || result.Deny != test.denied {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if test.wantErr && reported.Err == nil {
				t.Fatal("failed hook was reported as completed")
			}
			if test.name == "context" && result.Context != "run tests" {
				t.Fatal("context was lost")
			}
		})
	}
}

func TestHookRunnerIgnoresUnsupportedMatchers(t *testing.T) {
	for _, event := range []contracts.HookEvent{contracts.HookUserPromptSubmit, contracts.HookStop} {
		t.Run(string(event), func(t *testing.T) {
			rule := testHookRule(t, event, "output", `{"decision":"block","reason":"matched"}`)
			rule.Matcher = "^MATCH ME$"
			runner := NewHookRunner([]contracts.HookRule{rule}, nil)
			input := HookInput{Event: event, CWD: t.TempDir(), Prompt: "MATCH ME", Response: "MATCH ME"}
			result, err := runner.Run(context.Background(), input)
			if err != nil || !result.Deny {
				t.Fatalf("unsupported matcher prevented hook: result=%+v err=%v", result, err)
			}
			input.Prompt, input.Response = "other", "other"
			result, err = runner.Run(context.Background(), input)
			if err != nil || !result.Deny {
				t.Fatalf("unsupported matcher was used: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestHookRunnerExecutesShellCommand(t *testing.T) {
	command := "printf 'shell context'"
	if runtime.GOOS == "windows" {
		command = "Write-Output 'shell context'"
	}
	runner := NewHookRunner([]contracts.HookRule{{
		ID: "shell", Event: contracts.HookSessionStart, Type: "command", Command: command, Shell: testHookShell(), Enabled: true,
	}}, nil)
	result, err := runner.Run(context.Background(), HookInput{Event: contracts.HookSessionStart, CWD: t.TempDir()})
	if err != nil || result.Context != "shell context" {
		t.Fatalf("shell hook failed: result=%+v err=%v", result, err)
	}
}

func TestHookRunnerEnforcesTimeout(t *testing.T) {
	directory := t.TempDir()
	rule := testHookRule(t, contracts.HookSessionStart, "wait", filepath.Join(directory, "started"))
	rule.TimeoutSeconds = 0.2
	var report HookExecution
	runner := NewHookRunner([]contracts.HookRule{rule}, func(execution HookExecution) error { report = execution; return nil })
	started := time.Now()
	_, err := runner.Run(context.Background(), HookInput{Event: contracts.HookSessionStart, CWD: directory})
	if err != nil || !errors.Is(report.Err, context.DeadlineExceeded) {
		t.Fatalf("hook timeout not reported as nonblocking diagnostic: %v %+v", err, report)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("fractional timeout was truncated or ignored: %v", elapsed)
	}
}

func TestHookRunnerAsyncCancellationAndWait(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "started")
	rule := testHookRule(t, contracts.HookStop, "wait", path)
	rule.Async = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := NewHookRunner([]contracts.HookRule{rule}, nil)
	if _, err := runner.Run(ctx, HookInput{Event: contracts.HookStop, CWD: directory}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background hook did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	waited := make(chan error, 1)
	go func() { waited <- runner.Wait() }()
	select {
	case err := <-waited:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled process reported success: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled hook did not terminate")
	}
}

func TestRunTriggersImplementedHookEvents(t *testing.T) {
	directory := t.TempDir()
	var mu sync.Mutex
	var seen []contracts.HookEvent
	steps := 0
	backend := testModel{stream: func(request model.Request) (model.Result, error) {
		steps++
		if steps == 1 {
			if !strings.Contains(request.History[len(request.History)-1].Text, "policy context") {
				t.Fatal("hook context did not reach model")
			}
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{
				{ID: "write", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"done"}`},
				{ID: "read", Name: "Read", Arguments: `{"file_path":"missing.txt"}`},
			}}}, nil
		}
		return model.Result{Message: model.Message{Role: "assistant", Text: "complete"}}, nil
	}}
	err := Run(context.Background(), Options{
		Model: backend, CWD: directory, Tools: CoreTools(), HookSource: "startup",
		Request: model.Request{History: []model.Message{{Role: "user", Text: "task", UserInput: true}}},
		Record:  func(model.Message) error { return nil },
		Emit:    func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			mu.Lock()
			seen = append(seen, input.Event)
			mu.Unlock()
			if input.Event == contracts.HookUserPromptSubmit {
				return HookResult{Context: "policy context"}, nil
			}
			return HookResult{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []contracts.HookEvent{contracts.HookSessionStart, contracts.HookUserPromptSubmit,
		contracts.HookPreToolUse, contracts.HookPostToolUse, contracts.HookPostToolUseFailure, contracts.HookStop} {
		if !slices.Contains(seen, event) {
			t.Fatalf("event %s was not triggered: %v", event, seen)
		}
	}
	if slices.Contains(seen, contracts.HookPermissionRequest) {
		t.Fatal("default allowed tool emitted a permission request")
	}
	if data, err := os.ReadFile(filepath.Join(directory, "proof.txt")); err != nil || string(data) != "done" {
		t.Fatalf("real tool did not execute: %q %v", data, err)
	}
}

func TestHookDeniesToolAndStopCanContinue(t *testing.T) {
	directory := t.TempDir()
	steps, stops := 0, 0
	backend := testModel{stream: func(request model.Request) (model.Result, error) {
		steps++
		switch steps {
		case 1:
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{
				{ID: "write", Name: "Write", Arguments: `{"file_path":"denied.txt","content":"no"}`},
			}}}, nil
		case 2:
			if last := request.History[len(request.History)-1]; !last.IsError || !strings.Contains(last.Text, "blocked") {
				t.Fatalf("denial was not returned to model: %+v", last)
			}
		case 3:
			if !strings.Contains(request.History[len(request.History)-1].Text, "verify first") {
				t.Fatal("stop feedback missing")
			}
		default:
			t.Fatal("stop hook continued unexpectedly")
		}
		return model.Result{Message: model.Message{Role: "assistant", Text: "finished"}}, nil
	}}
	err := Run(context.Background(), Options{
		Model: backend, CWD: directory, Tools: CoreTools(),
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookPreToolUse {
				return HookResult{Deny: true, Reason: "blocked"}, nil
			}
			if input.Event == contracts.HookStop {
				stops++
				if stops == 1 {
					return HookResult{Deny: true, Reason: "verify first"}, nil
				}
			}
			return HookResult{}, nil
		},
	})
	if err != nil || steps != 3 {
		t.Fatalf("steps=%d error=%v", steps, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "denied.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("denied tool executed")
	}
}

func TestUserPromptHooksOnlyRunForExplicitHumanInputs(t *testing.T) {
	var prompts []string
	step, idle, drained := 0, false, false
	backend := testModel{stream: func(model.Request) (model.Result, error) {
		step++
		return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
	}}
	err := Run(context.Background(), Options{
		Model: backend,
		Request: model.Request{History: []model.Message{
			{Role: "user", Text: "earlier human input", UserInput: true},
			{Role: "user", Text: "subagent completion notification"},
		}},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
		Pending: func(bool) []model.Message {
			if drained {
				return nil
			}
			drained = true
			return []model.Message{
				{Role: "user", Text: "queued human input", UserInput: true},
				{Role: "user", Text: "queued subagent notification"},
			}
		},
		Idle: func(context.Context, model.Request) ([]model.Message, error) {
			if idle {
				return nil, nil
			}
			idle = true
			return []model.Message{{Role: "user", Text: "automatic goal continuation"}}, nil
		},
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookUserPromptSubmit {
				prompts = append(prompts, input.Prompt)
			}
			return HookResult{}, nil
		},
	})
	if err != nil || step != 2 || !slices.Equal(prompts, []string{"queued human input"}) {
		t.Fatalf("internal continuation was treated as human input: step=%d prompts=%v err=%v", step, prompts, err)
	}
}

func TestStopHookContextContinuesWithFeedback(t *testing.T) {
	calls := 0
	err := Run(context.Background(), Options{
		Model: testModel{stream: func(model.Request) (model.Result, error) {
			calls++
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookStop && calls == 1 {
				return HookResult{Context: "Notification recorded."}, nil
			}
			return HookResult{}, nil
		},
	})
	if err != nil || calls != 2 {
		t.Fatalf("Stop feedback did not continue execution once: calls=%d err=%v", calls, err)
	}
}
