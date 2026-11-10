package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestInternalFeedbackKeepsAcceptedTaskModelUntilUserInput(t *testing.T) {
	var mu sync.Mutex
	var selected []string
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		mu.Lock()
		selected = append(selected, body["model"].(string))
		mu.Unlock()
		goalResponse(w, model.Chat, "Done.", nil, true)
	}))
	state, _ := core.Snapshot(t.Context())
	provider := state.CustomProviders[0].Provider
	provider.Models = append(provider.Models, model.Model{ID: "next"})
	if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "Initial task")
	state = waitIdle(t, core)
	sid, target := state.Sessions[0].ID, "goal/next"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: sid, ModelID: &target}}); err != nil {
		t.Fatal(err)
	}
	if err := core.queueHookResult(sid, agent.HookExecution{
		Rule: contracts.HookRule{ID: "internal-feedback"}, Result: agent.HookResult{Context: "Internal result"},
	}, true); err != nil {
		t.Fatal(err)
	}
	state = waitIdle(t, core, sid)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sid, Text: "New user task"}}); err != nil {
		t.Fatal(err)
	}
	state = waitIdle(t, core, sid)
	mu.Lock()
	captured := strings.Join(selected, ",")
	mu.Unlock()
	if captured != "model,model,next" || state.Sessions[0].ModelID != target {
		t.Fatalf("internal feedback applied the pending selection: models=%s session=%+v", captured, state.Sessions[0])
	}
}

func TestEachQueuedUserInputRetainsItsSubmittedSelection(t *testing.T) {
	for _, delivery := range []string{"steer", "follow-up"} {
		t.Run(delivery, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			var mu sync.Mutex
			var selected []string
			core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				mu.Lock()
				selected = append(selected, body["model"].(string))
				first := len(selected) == 1
				mu.Unlock()
				if first {
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				goalResponse(w, model.Chat, "Done.", nil, true)
			}))
			t.Cleanup(unblock)
			state, _ := core.Snapshot(t.Context())
			provider := state.CustomProviders[0].Provider
			provider.Models = []model.Model{{ID: "model"}, {ID: "two"}, {ID: "three"}}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
				t.Fatal(err)
			}
			sendGoal(t, core, "First task")
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("initial task did not start")
			}
			state, _ = core.Snapshot(t.Context())
			sid := state.Sessions[0].ID
			for _, selected := range []string{"goal/two", "goal/three"} {
				if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					SessionID: sid, Text: "Task for " + selected, Delivery: delivery, ModelID: selected,
				}}); err != nil {
					t.Fatal(err)
				}
			}
			latest := "goal/model"
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: sid, ModelID: &latest}}); err != nil {
				t.Fatal(err)
			}
			unblock()
			state = waitIdle(t, core, sid)
			mu.Lock()
			captured := strings.Join(selected, ",")
			mu.Unlock()
			if captured != "model,two,three" || len(state.Runs) != 3 || len(state.Queue) != 0 || state.Sessions[0].ModelID != latest {
				t.Fatalf("queued user tasks were merged or overwritten: models=%s runs=%+v queue=%+v session=%+v", captured, state.Runs, state.Queue, state.Sessions[0])
			}
			turns := map[string]string{}
			for _, request := range state.ModelRequests {
				turns[request.TurnID] = request.ModelID
			}
			if len(turns) != 3 {
				t.Fatalf("three user inputs did not own three task statistics: %+v", state.ModelRequests)
			}
		})
	}
}

func TestLiveModelChangeSuppliesNewProviderToInheritedSubagent(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var calls atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		switch calls.Add(1) {
		case 1:
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			goalResponse(w, model.Chat, "", &model.Call{ID: "write", Name: "Write", Arguments: `{"file_path":"first.txt","content":"done"}`}, true)
		case 2:
			subagentCall(w, `{"description":"Inherited task","prompt":"Inspect","subagent_type":"worker"}`)
		case 3:
			if body["model"] != "new-model" || body["reasoning_effort"] != "high" {
				t.Errorf("child did not inherit new request selection: %+v", body)
			}
			subagentText(w, model.Chat, "CHILD DONE")
		default:
			subagentText(w, model.Chat, "PARENT DONE")
		}
	}))
	t.Cleanup(unblock)
	state, _ := core.Snapshot(t.Context())
	settings := state.Settings
	settings.Subagents = []contracts.SubagentProfile{{ID: "worker", Name: "Worker", Description: "Independent task"}}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		Text: "Delegate after writing", ModelID: "goal/model", ProjectID: state.Projects[0].ID,
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("parent did not start")
	}
	state, _ = core.Snapshot(t.Context())
	sid, target, effort := state.Sessions[0].ID, "new-provider/new-model", "High"
	provider := state.CustomProviders[0].Provider
	provider.ID, provider.Models = "new-provider", []model.Model{{ID: "new-model", ReasoningLevels: []string{"high"}}}
	for _, command := range []contracts.Command{
		{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
		{Kind: "session.update", Session: &contracts.SessionUpdate{ID: sid, ModelID: &target, ReasoningLevel: &effort}},
		{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sid, Text: "Delegate with the new model", Delivery: "steer"}},
	} {
		if err := core.Execute(t.Context(), command); err != nil {
			t.Fatal(err)
		}
	}
	unblock()
	state = waitSubagentState(t, core, func(state contracts.Snapshot) bool { return runsFinished(state) && len(state.Runs) == 3 })
	if calls.Load() != 4 {
		t.Fatalf("inherited task did not execute: calls=%d runs=%+v", calls.Load(), state.Runs)
	}
	for _, session := range state.Sessions {
		if session.LastOutcome != "completed" || session.ModelID != target {
			t.Fatalf("inherited task used stale configuration: %+v", session)
		}
	}
}

func TestLiveModelChangeKeepsRetriesOnAdmittedRequest(t *testing.T) {
	waiting, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var mu sync.Mutex
	var requests []map[string]any
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		attempt := len(requests)
		mu.Unlock()
		switch attempt {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			goalResponse(w, model.Chat, "", &model.Call{ID: "retry-write", Name: "Write", Arguments: `{"file_path":"retry.txt","content":"once"}`}, true)
		default:
			goalResponse(w, model.Chat, "Done.", nil, true)
		}
	}))
	t.Cleanup(unblock)
	core.client.RetryPolicy.Wait = func(ctx context.Context, _ time.Duration) error {
		close(waiting)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	state, _ := core.Snapshot(t.Context())
	provider := state.CustomProviders[0].Provider
	provider.Models = []model.Model{
		{ID: "model", ReasoningLevels: []string{"low", "high"}},
		{ID: "other", ReasoningLevels: []string{"low", "high"}},
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		Text: "Write retry.txt", ModelID: "goal/model", ReasoningLevel: "Low",
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("retry did not wait")
	}
	state, _ = core.Snapshot(t.Context())
	sid, target, effort := state.Sessions[0].ID, "goal/other", "High"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{
		ID: sid, ModelID: &target, ReasoningLevel: &effort,
	}}); err != nil {
		t.Fatal(err)
	}
	unblock()
	state = waitIdle(t, core, sid)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sid, Text: "Next task"}}); err != nil {
		t.Fatal(err)
	}
	state = waitIdle(t, core, sid)
	mu.Lock()
	captured := append([]map[string]any(nil), requests...)
	mu.Unlock()
	if len(captured) != 4 || captured[0]["model"] != "model" || captured[1]["model"] != "model" ||
		requestEffort(captured[1], model.Chat) != "low" || captured[2]["model"] != "model" ||
		requestEffort(captured[2], model.Chat) != "low" || captured[3]["model"] != "other" ||
		requestEffort(captured[3], model.Chat) != "high" || state.Runs[0].Status != "completed" {
		t.Fatalf("retry changed the admitted request: requests=%+v runs=%+v", captured, state.Runs)
	}
}

func TestLiveModelChangesApplyAtUserInputBoundaries(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		for _, change := range []string{"reasoning", "model", "provider", "provider-config", "steer", "follow-up"} {
			t.Run(string(protocol)+"/"+change, func(t *testing.T) {
				started, release := make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				var mu sync.Mutex
				var requests []map[string]any
				var paths, keys []string
				nextProtocol := protocol
				if change == "provider" {
					switch protocol {
					case model.Chat:
						nextProtocol = model.Responses
					case model.Responses:
						nextProtocol = model.Messages
					default:
						nextProtocol = model.Chat
					}
				}
				core, workspace := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					requests = append(requests, body)
					paths = append(paths, r.URL.Path)
					keys = append(keys, r.Header.Get("Authorization")+r.Header.Get("x-api-key"))
					first := len(requests) == 1
					mu.Unlock()
					if first {
						close(started)
						select {
						case <-release:
						case <-r.Context().Done():
							t.Error("changing settings canceled the current request")
							return
						}
						goalResponse(w, protocol, "", &model.Call{ID: "live-write", Name: "Write", Arguments: `{"file_path":"live.txt","content":"preserved"}`}, true)
						return
					}
					responseProtocol := protocol
					if r.URL.Path == "/v1/responses" {
						responseProtocol = model.Responses
					} else if r.URL.Path == "/v1/messages" {
						responseProtocol = model.Messages
					} else {
						responseProtocol = model.Chat
					}
					goalResponse(w, responseProtocol, "Done.", nil, true)
				}))
				// Release before the core/server cleanup, including failed assertions.
				t.Cleanup(unblock)
				state, _ := core.Snapshot(t.Context())
				provider := state.CustomProviders[0].Provider
				provider.APIKey = "old-fixture-key"
				provider.Models = []model.Model{
					{ID: "model", ReasoningLevels: []string{"low", "high"}, ContextWindow: 100000, InputPrice: 1},
					{ID: "other", ReasoningLevels: []string{"low", "high"}, ContextWindow: 200000, InputPrice: 2},
				}
				save := func(p model.Provider) {
					t.Helper()
					if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: p}}); err != nil {
						t.Fatal(err)
					}
				}
				save(provider)
				if change == "provider" {
					target := model.CloneProvider(provider)
					target.ID, target.Name, target.Protocol, target.APIKey = "target", "Target", nextProtocol, "new-fixture-key"
					save(target)
				}
				if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					Text: "Write live.txt", ProjectID: state.Projects[0].ID, ModelID: "goal/model", ReasoningLevel: "Low",
				}}); err != nil {
					t.Fatal(err)
				}
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("first request did not start")
				}
				state, _ = core.Snapshot(t.Context())
				sid := state.Sessions[0].ID
				wantModel, wantReason, wantKey := "goal/model", "High", "old-fixture-key"
				switch change {
				case "model", "steer", "follow-up":
					wantModel = "goal/other"
				case "provider":
					wantModel, wantKey = "target/other", "new-fixture-key"
				case "provider-config":
					provider.APIKey = "new-fixture-key"
					provider.Models[0].ContextWindow = 300000
					provider.Models[0].InputPrice = 3
					save(provider)
					wantKey = "new-fixture-key"
				}
				if change == "steer" || change == "follow-up" {
					if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
						SessionID: sid, Text: "Keep going", ModelID: wantModel, ReasoningLevel: wantReason, Delivery: change,
					}}); err != nil {
						t.Fatalf("live send was rejected: %v", err)
					}
				} else if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{
					ID: sid, ModelID: &wantModel, ReasoningLevel: &wantReason,
				}}); err != nil {
					t.Fatalf("live update was rejected: %v", err)
				}
				state, _ = core.Snapshot(t.Context(), sid)
				if state.Sessions[0].ModelID != wantModel || state.Sessions[0].ReasoningLevel != wantReason || state.Runs[0].Status != "running" {
					t.Fatalf("selection was not saved without stopping: %+v", state)
				}
				unblock()
				state = waitIdle(t, core, sid)
				if change != "steer" && change != "follow-up" {
					if len(state.Runs) != 1 || state.Runs[0].Status != "completed" {
						t.Fatalf("picker edit interrupted the current task: %+v", state.Runs)
					}
					if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sid, Text: "Next task"}}); err != nil {
						t.Fatal(err)
					}
					state = waitIdle(t, core, sid)
				}
				if len(state.Runs) != 2 || state.Runs[0].Status != "completed" || state.Runs[1].Status != "completed" {
					t.Fatalf("change interrupted or restarted the run: %+v", state.Runs)
				}
				data, err := os.ReadFile(filepath.Join(workspace, "live.txt"))
				if err != nil || string(data) != "preserved" {
					t.Fatalf("in-flight tool call was lost: %q, %v", data, err)
				}
				mu.Lock()
				captured, capturedPaths, capturedKeys := append([]map[string]any(nil), requests...), append([]string(nil), paths...), append([]string(nil), keys...)
				mu.Unlock()
				wantCalls, boundary := 3, 2
				if change == "steer" {
					wantCalls, boundary = 2, 1
				}
				if len(captured) != wantCalls {
					t.Fatalf("requests=%d, want %d", len(captured), wantCalls)
				}
				if requestEffort(captured[0], protocol) != "low" || captured[0]["model"] != "model" || !strings.Contains(capturedKeys[0], "old-fixture-key") {
					t.Fatalf("first request was changed: %+v", captured[0])
				}
				for i, body := range captured {
					expectedProtocol, expectedModel, expectedEffort, expectedKey := nextProtocol, wantModel, "high", wantKey
					if i < boundary {
						expectedProtocol, expectedModel, expectedEffort, expectedKey = protocol, "goal/model", "low", "old-fixture-key"
					}
					wantID := strings.TrimPrefix(strings.TrimPrefix(expectedModel, "goal/"), "target/")
					if body["model"] != wantID || requestEffort(body, expectedProtocol) != expectedEffort || !strings.Contains(capturedKeys[i], expectedKey) {
						t.Fatalf("request %d used the wrong task settings: %+v", i, body)
					}
					encoded, _ := json.Marshal(body)
					if i > 0 && (!strings.Contains(string(encoded), "live-write") || !strings.Contains(string(encoded), "Wrote")) {
						t.Fatal("model/provider switch lost the tool exchange")
					}
					path := map[model.Protocol]string{model.Chat: "/v1/chat/completions", model.Responses: "/v1/responses", model.Messages: "/v1/messages"}[expectedProtocol]
					if capturedPaths[i] != path {
						t.Fatalf("request used %s, want %s", capturedPaths[i], path)
					}
				}
				var replies []contracts.Message
				for _, message := range state.Messages[sid] {
					if message.Role == "assistant" {
						replies = append(replies, message)
					}
				}
				if len(replies) != wantCalls || replies[0].ModelID != "goal/model" || replies[boundary].ModelID != wantModel ||
					replies[0].TurnID == replies[boundary].TurnID {
					t.Fatalf("reply model attribution is incorrect: %+v", replies)
				}
				if len(state.ModelRequests) != wantCalls || state.ModelRequests[boundary].ModelID != captured[boundary]["model"] ||
					state.ModelRequests[0].TurnID == state.ModelRequests[boundary].TurnID {
					t.Fatalf("accounting used the old model: %+v", state.ModelRequests)
				}
				wantWindow, wantCost := 100000, float64(5)/1_000_000
				if change == "model" || change == "provider" || change == "steer" || change == "follow-up" {
					wantWindow, wantCost = 200000, float64(10)/1_000_000
				} else if change == "provider-config" {
					wantWindow, wantCost = 300000, float64(15)/1_000_000
				}
				if replies[0].Usage.ContextLimit != 100000 || replies[boundary].Usage.ContextLimit != wantWindow || replies[boundary].Usage.Cost != wantCost {
					t.Fatalf("reply retained stale limits/prices: %+v", replies)
				}
			})
		}
	}
}

func requestEffort(body map[string]any, protocol model.Protocol) string {
	if protocol == model.Chat {
		effort, _ := body["reasoning_effort"].(string)
		return effort
	}
	key := "reasoning"
	if protocol == model.Messages {
		key = "output_config"
	}
	value, _ := body[key].(map[string]any)
	effort, _ := value["effort"].(string)
	return effort
}
