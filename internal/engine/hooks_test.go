package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestEngineHookProcessHelper(t *testing.T) {
	at := slices.Index(os.Args, "--")
	if at < 0 || len(os.Args) <= at+2 || os.Args[at+1] != "engine-hook-helper" {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	_ = os.WriteFile(os.Args[at+2], data, 0600)
	if len(os.Args) > at+3 {
		if len(os.Args) > at+4 {
			var input struct {
				Prompt string `json:"prompt"`
			}
			_ = json.Unmarshal(data, &input)
			if !strings.HasPrefix(input.Prompt, os.Args[at+4]) {
				os.Exit(0)
			}
		}
		fmt.Print(os.Args[at+3])
	}
	os.Exit(0)
}

func TestHooksExecuteFromUserSettingsInRealEngineLifecycle(t *testing.T) {
	var requests atomic.Int32
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		switch requests.Add(1) {
		case 1:
			goalResponse(w, model.Chat, "", &model.Call{ID: "write", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"hooked"}`}, true)
		case 2:
			goalResponse(w, model.Chat, "", &model.Call{ID: "read", Name: "Read", Arguments: `{"file_path":"missing.txt"}`}, true)
		default:
			goalResponse(w, model.Chat, "Finished.", nil, true)
		}
	}))
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	observedEvents := []contracts.HookEvent{contracts.HookSessionStart, contracts.HookUserPromptSubmit, contracts.HookPreToolUse, contracts.HookPostToolUse, contracts.HookPostToolUseFailure, contracts.HookStop}
	for _, event := range append(slices.Clone(observedEvents), contracts.HookPermissionRequest) {
		settings.Hooks = append(settings.Hooks, contracts.HookRule{
			ID: string(event), Event: event, Type: "command", Command: program, Enabled: true,
			Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper",
				filepath.Join(workspace, string(event)+".json")},
		})
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's slice must not change the accepted configuration.
	settings.Hooks[0].Args[0] = "caller mutation"
	sendGoal(t, core, "Write proof.txt, then inspect missing.txt")
	summary, err := core.Snapshot(context.Background())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("goal session was not created: %+v, %v", summary.Sessions, err)
	}
	// Hooks launch several real helper processes. Wait for the authoritative
	// completion snapshot so Windows race builds are not bound to the short
	// subagent polling deadline.
	state = waitIdle(t, core, summary.Sessions[0].ID)
	if state.Runs[0].Status != "completed" || requests.Load() != 3 {
		t.Fatalf("run failed: %+v requests=%d", state.Runs, requests.Load())
	}
	for _, event := range observedEvents {
		data, err := os.ReadFile(filepath.Join(workspace, string(event)+".json"))
		if err != nil {
			t.Fatalf("hook event %s did not run: %v", event, err)
		}
		var input agent.HookInput
		if err = json.Unmarshal(data, &input); err != nil {
			t.Fatal(err)
		}
		if input.Event != event || input.SessionID != state.Sessions[0].ID || input.RunID != "" || input.CWD != workspace {
			t.Fatalf("hook lost session ownership or leaked internal run: %+v", input)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["hook_event_name"] != string(event) || payload["session_id"] != state.Sessions[0].ID || payload["run_id"] != nil || payload["runId"] != nil {
			t.Fatalf("invalid public hook payload: %s", data)
		}
		if event == contracts.HookPostToolUseFailure && !strings.Contains(input.ToolError, "missing.txt") {
			t.Fatalf("failure hook did not receive real tool error: %+v", input)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, string(contracts.HookPermissionRequest)+".json")); !os.IsNotExist(err) {
		t.Fatalf("PermissionRequest fired without PreToolUse asking: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "proof.txt")); err != nil || string(data) != "hooked" {
		t.Fatalf("actual tool was not exercised: %q %v", data, err)
	}
	visibleHooks := 0
	for _, message := range state.Messages[state.Sessions[0].ID] {
		for _, block := range message.Blocks {
			if strings.HasPrefix(block.Text, "Hook ") {
				visibleHooks++
			}
		}
	}
	if visibleHooks < 7 {
		t.Fatal("hook outcomes were hidden from transcript")
	}
}

func TestUserPromptHookDenialStopsBeforeModelRequest(t *testing.T) {
	var requests atomic.Int32
	var leaked atomic.Bool
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		data, _ := json.Marshal(body)
		if strings.Contains(string(data), "blocked request") {
			leaked.Store(true)
		}
		requests.Add(1)
		goalResponse(w, model.Chat, "Allowed request complete.", nil, true)
	}))
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.Hooks = []contracts.HookRule{{
		ID: "deny", Event: contracts.HookUserPromptSubmit, Type: "command", Command: program, Enabled: true,
		Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper",
			filepath.Join(workspace, "denied.json"), `{"decision":"block","reason":"local policy"}`, "blocked request"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "blocked request")
	summary, err := core.Snapshot(context.Background())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("denied conversation was not created: %+v, %v", summary.Sessions, err)
	}
	sessionID := summary.Sessions[0].ID
	releaseView := pinSessionView(t, core, sessionID)
	defer releaseView()
	state = waitSubagentState(t, core, runsFinished, sessionID)
	if requests.Load() != 0 || state.Runs[0].Status != "failed" ||
		!strings.Contains(state.Runs[0].Error, "local policy") {
		t.Fatalf("hook denial was accepted as success: requests=%d runs=%+v", requests.Load(), state.Runs)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: "allowed request"},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, func(snapshot contracts.Snapshot) bool { return len(snapshot.Runs) == 2 && runsFinished(snapshot) }, sessionID)
	if leaked.Load() || requests.Load() != 1 || state.Runs[1].Status != "completed" {
		t.Fatalf("denied input leaked into a later model call: leaked=%v requests=%d runs=%+v", leaked.Load(), requests.Load(), state.Runs)
	}
}

func TestQueuedHookDenialsDoNotLeakUncheckedBatchInputs(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	var leaked atomic.Bool
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		data, _ := json.Marshal(body)
		if strings.Contains(string(data), "DENIED QUEUED") {
			leaked.Store(true)
		}
		if requests.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-request.Context().Done():
				return
			}
		}
		goalResponse(w, model.Chat, "Completed.", nil, true)
	}))
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.Hooks = []contracts.HookRule{{
		ID: "deny", Event: contracts.HookUserPromptSubmit, Type: "command", Command: program, Enabled: true,
		Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper",
			filepath.Join(workspace, "denied.json"), `{"decision":"block","reason":"local policy"}`, "DENIED"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "initial request")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("initial request did not reach the model")
	}
	state, _ = core.Snapshot(context.Background())
	sessionID := state.Sessions[0].ID
	releaseView := pinSessionView(t, core, sessionID)
	defer releaseView()
	for _, text := range []string{"DENIED QUEUED", "APPROVED QUEUED"} {
		if err := core.Execute(context.Background(), contracts.Command{
			Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: text},
		}); err != nil {
			t.Fatal(err)
		}
	}
	close(release)
	state = waitSubagentState(t, core, runsFinished, sessionID)
	if len(state.Runs) != 2 || state.Runs[0].Status != "completed" || state.Runs[1].Status != "failed" {
		t.Fatalf("queued denial did not fail only its own user task: %+v", state.Runs)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: "continue allowed work"},
	}); err != nil {
		t.Fatal(err)
	}
	// The denied input owns its failed task; the approved row remains queued
	// until another send resumes execution, then opens its own task too.
	state = waitSubagentState(t, core, func(snapshot contracts.Snapshot) bool { return len(snapshot.Runs) == 4 && runsFinished(snapshot) }, sessionID)
	if leaked.Load() || requests.Load() != 3 || state.Runs[2].Status != "completed" || state.Runs[3].Status != "completed" {
		t.Fatalf("queued denial leaked into future execution: leaked=%v requests=%d runs=%+v", leaked.Load(), requests.Load(), state.Runs)
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	data, _ := json.Marshal(core.history[sessionID])
	if strings.Contains(string(data), "DENIED QUEUED") || !strings.Contains(string(data), "APPROVED QUEUED") {
		t.Fatalf("batch admission lost accepted input or retained denial: %s", data)
	}
}

func TestGoalHookRejectionClearsOnlyTheInitialGoalInput(t *testing.T) {
	for _, initial := range []bool{true, false} {
		name := "additional input preserves goal"
		if initial {
			name = "initial input clears goal"
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				close(entered)
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
				goalResponse(w, model.Chat, "Work remains.", nil, true)
			}))
			program, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			state, _ := core.Snapshot(context.Background())
			settings := state.Settings
			settings.Hooks = []contracts.HookRule{{
				ID: "deny", Event: contracts.HookUserPromptSubmit, Type: "command", Command: program, Enabled: true,
				Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper",
					filepath.Join(workspace, "denied.json"), `{"decision":"block","reason":"local policy"}`, "DENIED"},
			}}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			events, err := core.Subscribe(ctx)
			if err != nil {
				t.Fatal(err)
			}
			text := "/goal valid objective"
			if initial {
				text = "/goal DENIED INITIAL"
			}
			sendGoal(t, core, text)
			var goalID, sessionID string
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for goalID == "" {
				select {
				case event := <-events:
					if event.Kind != "run.started" || event.Snapshot == nil {
						continue
					}
					scoped, err := core.Snapshot(context.Background(), event.SessionID)
					if err != nil || len(scoped.Sessions) == 0 {
						t.Fatalf("goal session was not available: %+v, %v", scoped.Sessions, err)
					}
					goal := scoped.Sessions[0].Goal
					if goal == nil || goal.InitialInputID == "" {
						t.Fatal("goal initial input has no explicit identity")
					}
					goalID, sessionID = goal.ID, event.SessionID
				case <-deadline.C:
					t.Fatal("goal did not start")
				}
			}
			if !initial {
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("valid goal did not reach the model")
				}
				if err := core.Execute(context.Background(), contracts.Command{
					Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: "DENIED ADDITIONAL"},
				}); err != nil {
					t.Fatal(err)
				}
				close(release)
			}
			state = waitSubagentState(t, core, runsFinished, sessionID)
			goal := state.Sessions[0].Goal
			if initial {
				if goal != nil || requests.Load() != 0 {
					t.Fatalf("denied goal remained executable: goal=%+v requests=%d", goal, requests.Load())
				}
				if err := core.Execute(context.Background(), contracts.Command{
					Kind: "goal.resume", SessionID: sessionID, ID: goalID,
				}); err == nil {
					t.Fatal("resume bypassed rejected initial objective")
				}
			} else if goal == nil || goal.ID != goalID || goal.Status != contracts.GoalPaused || requests.Load() != 1 {
				t.Fatalf("rejecting additional input cleared the admitted goal: goal=%+v requests=%d", goal, requests.Load())
			}
		})
	}
}
