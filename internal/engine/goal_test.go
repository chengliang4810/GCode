package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestGoalExecutesVerifiesAndContinuesAcrossProtocols(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var workCalls, checks atomic.Int32
			core, workspace := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if isGoalVerification(body) {
					if tools, _ := body["tools"].([]any); len(tools) != 0 {
						t.Error("completion verifier was given tools")
					}
					if checks.Add(1) == 1 {
						goalResponse(w, protocol, `{"passed":false,"reason":"The second file is missing.","nextAction":"Write second.txt and verify both files."}`, nil, true)
					} else {
						goalResponse(w, protocol, `{"passed":true,"reason":"Both requested files were written and verified."}`, nil, true)
					}
					return
				}
				switch workCalls.Add(1) {
				case 1:
					goalResponse(w, protocol, "", &model.Call{ID: "write-first", Name: "Write", Arguments: `{"file_path":"first.txt","content":"one"}`}, true)
				case 2:
					goalResponse(w, protocol, "The first file is ready.", nil, true)
				case 3:
					encoded, _ := json.Marshal(body)
					if !strings.Contains(string(encoded), "Write second.txt") {
						t.Error("verifier gap did not reach the next executor iteration")
					}
					goalResponse(w, protocol, "", &model.Call{ID: "write-second", Name: "Write", Arguments: `{"file_path":"second.txt","content":"two"}`}, true)
				default:
					goalResponse(w, protocol, "Both files are ready.", nil, true)
				}
			}))
			sendGoal(t, core, "/goal Create first.txt and second.txt")
			state := waitSubagentState(t, core, func(s contracts.Snapshot) bool {
				return runsFinished(s) && len(s.Sessions) == 1 && s.Sessions[0].Goal != nil &&
					s.Sessions[0].Goal.Status == contracts.GoalComplete
			})
			goal := state.Sessions[0].Goal
			if workCalls.Load() != 4 || checks.Load() != 2 || goal.Iteration != 2 ||
				goal.TokensUsed != 48 || goal.UnknownUsageCount != 0 || len(goal.Verifications) != 2 ||
				goal.Verifications[0].Passed || !goal.Verifications[1].Passed {
				t.Fatalf("goal lifecycle or accounting is incorrect: %+v work=%d checks=%d", goal, workCalls.Load(), checks.Load())
			}
			for _, path := range []string{"first.txt", "second.txt"} {
				if _, err := os.Stat(filepath.Join(workspace, path)); err != nil {
					t.Fatal("goal did not execute real filesystem tools:", err)
				}
			}
			users := 0
			for _, message := range state.Messages[state.Sessions[0].ID] {
				if message.Role == "user" {
					users++
				}
			}
			if users != 1 {
				t.Fatal("runtime continuations were displayed as human messages")
			}
		})
	}
}

func TestGoalVerifierErrorsPauseInsteadOfCompleting(t *testing.T) {
	for _, kind := range []string{"invalid-json", "incomplete-verdict", "trailing", "tool-call", "http-error"} {
		t.Run(kind, func(t *testing.T) {
			core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if !isGoalVerification(body) {
					goalResponse(w, model.Chat, "I think I am done.", nil, true)
					return
				}
				switch kind {
				case "invalid-json":
					goalResponse(w, model.Chat, "done", nil, true)
				case "incomplete-verdict":
					goalResponse(w, model.Chat, `{"passed":false,"reason":"Missing evidence"}`, nil, true)
				case "trailing":
					goalResponse(w, model.Chat, `{"passed":true,"reason":"Done"} additional text`, nil, true)
				case "tool-call":
					goalResponse(w, model.Chat, "", &model.Call{ID: "must-not-run", Name: "Write", Arguments: `{"file_path":"verifier.txt","content":"bad"}`}, true)
				case "http-error":
					http.Error(w, "unavailable", http.StatusUnauthorized)
				}
			}))
			sendGoal(t, core, "/goal Verify the implementation")
			state := waitSubagentState(t, core, runsFinished)
			goal := state.Sessions[0].Goal
			if goal == nil || goal.Status != contracts.GoalPaused || !strings.Contains(goal.Reason, "verification failed") {
				t.Fatalf("unreliable verification was accepted: %+v", goal)
			}
			if _, err := os.Stat(filepath.Join(workspace, "verifier.txt")); !os.IsNotExist(err) {
				t.Fatal("completion verifier executed a tool")
			}
		})
	}
}

func TestGoalBudgetIncludesVerificationAndRejectsBypassResume(t *testing.T) {
	var calls atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		calls.Add(1)
		if isGoalVerification(body) {
			goalResponse(w, model.Chat, `{"passed":false,"reason":"More work remains","nextAction":"Continue inspection"}`, nil, true)
		} else {
			goalResponse(w, model.Chat, "Partial result", nil, true)
		}
	}))
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Budgeted goal"}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	budget := int64(16)
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "goal.set", SessionID: state.Sessions[0].ID,
		Goal: &contracts.GoalInput{Objective: "Complete a budgeted investigation", TokenBudget: &budget},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	goal := state.Sessions[0].Goal
	if calls.Load() != 2 || goal.TokensUsed != 16 || goal.Status != contracts.GoalBudgetLimited {
		t.Fatalf("verifier did not consume the goal budget: calls=%d goal=%+v", calls.Load(), goal)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "goal.resume", SessionID: state.Sessions[0].ID, ID: goal.ID,
	}); err == nil {
		t.Fatal("resume bypassed the exhausted budget")
	}
}

func TestGoalUnknownUsageDoesNotBecomeZeroBudgetCost(t *testing.T) {
	for _, budgeted := range []bool{true, false} {
		t.Run(fmt.Sprint(budgeted), func(t *testing.T) {
			var calls atomic.Int32
			core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				calls.Add(1)
				if isGoalVerification(body) {
					goalResponse(w, model.Chat, `{"passed":true,"reason":"Evidence is sufficient"}`, nil, false)
				} else {
					goalResponse(w, model.Chat, "Completed task", nil, false)
				}
			}))
			if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
				t.Fatal(err)
			}
			state, _ := core.Snapshot(context.Background())
			input := &contracts.GoalInput{Objective: "Inspect a task"}
			budget := int64(100)
			if budgeted {
				input.TokenBudget = &budget
			}
			if err := core.Execute(context.Background(), contracts.Command{
				Kind: "goal.set", SessionID: state.Sessions[0].ID, Goal: input,
			}); err != nil {
				t.Fatal(err)
			}
			state = waitSubagentState(t, core, runsFinished)
			goal := state.Sessions[0].Goal
			if budgeted && (goal.Status != contracts.GoalBudgetLimited || calls.Load() != 1) {
				t.Fatalf("unknown usage allowed additional budgeted calls: %+v calls=%d", goal, calls.Load())
			}
			if !budgeted && (goal.Status != contracts.GoalComplete || calls.Load() != 2) {
				t.Fatalf("unbudgeted goal lost explicit unknown-usage state: %+v", goal)
			}
			if goal.UnknownUsageCount != int(calls.Load()) {
				t.Fatal("missing usage was silently reported as exact zero")
			}
		})
	}
}

func TestGoalNewInputDuringVerificationTakesPriority(t *testing.T) {
	verifyStarted, releaseVerify := make(chan struct{}), make(chan struct{})
	var checks, work atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if isGoalVerification(body) {
			if checks.Add(1) == 1 {
				close(verifyStarted)
				select {
				case <-releaseVerify:
				case <-r.Context().Done():
					return
				}
			}
			goalResponse(w, model.Chat, `{"passed":true,"reason":"Reviewed all evidence"}`, nil, true)
			return
		}
		if work.Add(1) == 2 {
			encoded, _ := json.Marshal(body)
			if !strings.Contains(string(encoded), "NEW REQUIREMENT") {
				t.Error("new input was not processed before completion")
			}
		}
		goalResponse(w, model.Chat, "Work result", nil, true)
	}))
	sendGoal(t, core, "/goal Inspect the project")
	select {
	case <-verifyStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("verification never started")
	}
	state, _ := core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			SessionID: state.Sessions[0].ID, Text: "NEW REQUIREMENT: check the extra case",
		},
	}); err != nil {
		t.Fatal(err)
	}
	close(releaseVerify)
	state = waitSubagentState(t, core, runsFinished)
	if checks.Load() != 2 || work.Load() != 2 || state.Sessions[0].Goal.Status != contracts.GoalComplete {
		t.Fatalf("stale completion verdict took priority: work=%d verify=%d %+v", work.Load(), checks.Load(), state.Sessions[0].Goal)
	}
}

func TestGoalClearCannotBeCompletedByAnInFlightVerifier(t *testing.T) {
	verifyStarted := make(chan struct{})
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isGoalVerification(subagentRequest(t, r)) {
			goalResponse(w, model.Chat, "Work done", nil, true)
			return
		}
		close(verifyStarted)
		<-r.Context().Done()
	}))
	sendGoal(t, core, "/goal Verify this task")
	select {
	case <-verifyStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("verification never started")
	}
	state, _ := core.Snapshot(context.Background())
	session := state.Sessions[0]
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "goal.clear", SessionID: session.ID, ID: session.Goal.ID,
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	if state.Sessions[0].Goal != nil {
		t.Fatal("cancelled verifier recreated or completed the cleared goal")
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "goal.resume", SessionID: session.ID, ID: session.Goal.ID,
	}); err == nil {
		t.Fatal("stale goal action was accepted")
	}
}

func TestGoalRestartPausesAndSettlesToLastHeartbeat(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	session := newTestSession(t, core, "", "Goal restart")
	start := time.Now().Add(-time.Minute)
	core.state.Sessions[0].Goal = &contracts.SessionGoal{
		ID: "persisted-goal", Objective: "Keep the objective", Status: contracts.GoalVerifying,
		Iteration: 2, TimeUsedMS: 1000, TokensUsed: 27, ActiveSince: &start,
		LastSeenAt: start.Add(15 * time.Second), CreatedAt: start, UpdatedAt: start,
	}
	core.state.Sessions[0].Status = contracts.Working
	if err = core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	core.mu.Unlock()
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	state, _ := core.Snapshot(context.Background())
	goal := state.Sessions[0].Goal
	if goal.Status != contracts.GoalPaused || goal.ActiveSince != nil ||
		goal.TimeUsedMS != 16000 || goal.TokensUsed != 27 || goal.Iteration != 2 || len(state.Runs) != 0 {
		t.Fatalf("restart inferred completion or resumed execution: %+v", goal)
	}
	if err = core.Execute(context.Background(), contracts.Command{Kind: "session.fork", SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	if state.Sessions[0].Goal != nil {
		t.Fatal("fork inherited authoritative goal state")
	}
}

func TestGoalBudgetPreservesUnexecutedUserInput(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		close(started)
		select {
		case <-release:
			goalResponse(w, model.Chat, "Work finished.", nil, true)
		case <-r.Context().Done():
		}
	}))
	state, _ := core.Snapshot(context.Background())
	sessionID := ""
	core.mu.Lock()
	sessionID = newTestSession(t, core, state.Projects[0].ID, "Budget queue").ID
	core.state.Sessions[0].ModelID = "goal/model"
	core.mu.Unlock()
	budget := int64(8)
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "goal.set", SessionID: sessionID,
		Goal: &contracts.GoalInput{Objective: "First task", TokenBudget: &budget},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not start")
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: "UNEXECUTED FOLLOWUP"},
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	state = waitSubagentState(t, core, runsFinished)
	if state.Sessions[0].Goal.Status != contracts.GoalBudgetLimited ||
		len(state.Queue) != 1 || state.Queue[0].Text != "UNEXECUTED FOLLOWUP" || len(state.Runs) != 1 {
		t.Fatalf("budget stop lost input or started an unauthorized continuation: %+v", state.Queue)
	}
	for _, message := range state.Messages[sessionID] {
		for _, block := range message.Blocks {
			if block.Text == "UNEXECUTED FOLLOWUP" {
				t.Fatal("unexecuted queued input was displayed as already consumed")
			}
		}
	}
}

func TestGoalRestoresInputWithoutLosingCompactedWork(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("restoring unexecuted input must not issue a request")
	}))
	core.mu.Lock()
	defer core.mu.Unlock()
	session := newTestSession(t, core, "", "Restore inputs")
	input := contracts.QueuedMessage{ID: "pending", SessionID: session.ID, Text: "NEW INPUT", CreatedAt: time.Now()}
	core.history[session.ID] = []model.Message{
		{Role: "user", Text: "EARLIER REQUIREMENT"},
		{Role: "assistant", Text: "EARLIER VERIFIED WORK"},
		{Role: "user", Text: input.Text, InputIDs: []string{input.ID}},
	}
	core.context[session.ID] = []model.Message{{
		Role: "user", Text: "Summary of earlier verified work and new input", InputIDs: []string{input.ID},
	}}
	core.state.Messages[session.ID] = []contracts.Message{{ID: input.ID, SessionID: session.ID, Role: "user"}}
	run := &activeRun{pending: map[string]contracts.QueuedMessage{input.ID: input}}
	core.restoreGoalInputsLocked(session.ID, run)
	if len(core.state.Queue) != 1 || len(core.context[session.ID]) != 2 ||
		core.context[session.ID][1].Text != "EARLIER VERIFIED WORK" || len(core.state.Messages[session.ID]) != 0 {
		t.Fatal("restoring queued input lost earlier work or retained the unexecuted message")
	}
}

func TestGoalWaitsForChildAndAcceptsNewInputBeforeChildFinishes(t *testing.T) {
	var parentCalls, checks atomic.Int32
	childStarted, releaseChild, steered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if isGoalVerification(body) {
			checks.Add(1)
			goalResponse(w, model.Chat, `{"passed":true,"reason":"Child results and user requirements were processed."}`, nil, true)
			return
		}
		switch parentCalls.Add(1) {
		case 1:
			goalResponse(w, model.Chat, "", &model.Call{ID: "delegate", Name: "Agent", Arguments: `{"description":"Lookup","prompt":"lookup","subagent_type":"worker","run_in_background":true}`}, true)
		case 2:
			goalResponse(w, model.Chat, "Waiting for the child.", nil, true)
		case 3:
			data, _ := json.Marshal(body)
			if !strings.Contains(string(data), "PRIORITY INPUT") {
				t.Error("waiting parent did not receive new input")
			}
			close(steered)
			goalResponse(w, model.Chat, "New requirement accepted.", nil, true)
		default:
			data, _ := json.Marshal(body)
			if !strings.Contains(string(data), "CHILD EVIDENCE") {
				t.Error("parent did not incorporate child evidence before verification")
			}
			goalResponse(w, model.Chat, "All evidence incorporated.", nil, true)
		}
	}))
	defer parent.Close()
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		close(childStarted)
		select {
		case <-releaseChild:
			goalResponse(w, model.Chat, "CHILD EVIDENCE", nil, true)
		case <-r.Context().Done():
		}
	}))
	defer child.Close()
	core, _ := subagentCore(t, parent.URL, child.URL, model.Chat)
	state, _ := core.Snapshot(context.Background())
	state.Settings.Subagents = []contracts.SubagentProfile{{
		ID: "worker", Name: "Worker", Description: "Lookup",
		Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		Text: "/goal Complete the task with child evidence", ProjectID: state.Projects[0].ID, ModelID: "parent/parent-model",
	}}); err != nil {
		t.Fatal(err)
	}
	waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return parentCalls.Load() >= 2 && len(s.Sessions) == 2
	})
	select {
	case <-childStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not start")
	}
	state, _ = core.Snapshot(context.Background())
	var parentID string
	for _, session := range state.Sessions {
		if session.SubagentType == "" {
			parentID = session.ID
		}
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: parentID, Text: "PRIORITY INPUT",
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-steered:
	case <-time.After(5 * time.Second):
		t.Fatal("new input waited for the still-running child")
	}
	if checks.Load() != 0 {
		t.Fatal("goal was verified before the accepted child finished")
	}
	close(releaseChild)
	state = waitSubagentState(t, core, runsFinished)
	for _, session := range state.Sessions {
		if session.ID == parentID && (session.Goal.Status != contracts.GoalComplete || session.Goal.TokensUsed != 48) {
			t.Fatalf("child/verifier requests were not included in the goal: %+v, parent calls=%d checks=%d", session.Goal, parentCalls.Load(), checks.Load())
		}
	}
}

func goalTestCore(t *testing.T, protocol model.Protocol, handler http.Handler) (*Engine, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	if err = core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
			ID: "goal", Name: "Goal fixture", Protocol: protocol, BaseURL: server.URL,
			Models: []model.Model{{ID: "model"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err = core.Execute(context.Background(), contracts.Command{
		Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace},
	}); err != nil {
		t.Fatal(err)
	}
	return core, workspace
}

func sendGoal(t *testing.T, core *Engine, text string) {
	t.Helper()
	state, _ := core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			Text: text, ModelID: "goal/model", ProjectID: state.Projects[0].ID,
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func isGoalVerification(body map[string]any) bool {
	system := ""
	switch value := body["system"].(type) {
	case string:
		system = value
	case []any:
		// Anthropic systems are split into cacheable blocks; the verifier text
		// is the first block's text.
		if len(value) > 0 {
			if first, ok := value[0].(map[string]any); ok {
				system, _ = first["text"].(string)
			}
		}
	}
	if system == "" {
		if value, ok := body["instructions"].(string); ok {
			system = value
		} else if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			first, _ := messages[0].(map[string]any)
			system, _ = first["content"].(string)
		}
	}
	return strings.HasPrefix(system, "Review whether the objective below is fully satisfied")
}

func goalResponse(
	w http.ResponseWriter,
	protocol model.Protocol,
	text string,
	call *model.Call,
	known bool,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	switch protocol {
	case model.Chat:
		delta, stop := map[string]any{"content": text}, "stop"
		if call != nil {
			delta, stop = map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": call.ID, "function": map[string]any{"name": call.Name, "arguments": call.Arguments},
			}}}, "tool_calls"
		}
		event := map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": stop}}}
		if known {
			event["usage"] = map[string]any{"prompt_tokens": 5, "completion_tokens": 3}
		}
		sendSSE(w, event)
		fmt.Fprint(w, "data: [DONE]\n\n")
	case model.Responses:
		output := []any{}
		if call != nil {
			output = append(output, map[string]any{
				"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments,
			})
		} else {
			sendSSE(w, map[string]any{"type": "response.output_text.delta", "delta": text})
		}
		response := map[string]any{"status": "completed", "output": output}
		if known {
			response["usage"] = map[string]any{"input_tokens": 5, "output_tokens": 3}
		}
		sendSSE(w, map[string]any{"type": "response.completed", "response": response})
	case model.Messages:
		message := map[string]any{}
		if known {
			message["usage"] = map[string]any{"input_tokens": 5}
		}
		sendSSE(w, map[string]any{"type": "message_start", "message": message})
		stop := "end_turn"
		if call != nil {
			var input any
			_ = json.Unmarshal([]byte(call.Arguments), &input)
			sendSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{
				"type": "tool_use", "id": call.ID, "name": call.Name, "input": input,
			}})
			stop = "tool_use"
		} else {
			sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}})
		}
		event := map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop}}
		if known {
			event["usage"] = map[string]any{"output_tokens": 3}
		}
		sendSSE(w, event)
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}
