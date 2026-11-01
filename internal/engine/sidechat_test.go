package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestSideChatSnapshotBoundariesDuringRunAndCompaction(t *testing.T) {
	stable := []model.Message{
		{Role: "user", Text: "earlier input", UserInput: true},
		{Role: "assistant", Text: "earlier completed answer"},
		{Role: "user", Text: "current input", UserInput: true},
	}
	live := append(model.CloneMessages(stable),
		model.Message{Role: "assistant", Calls: []model.Call{{ID: "active", Name: "Read"}}},
		model.Message{Role: "tool", CallID: "active", Text: "current tool output"},
		model.Message{Role: "user", Text: "host-generated goal continuation"},
	)
	for _, test := range []struct {
		name string
		run  *activeRun
		want []model.Message
	}{
		{name: "idle retains completed exchanges", want: live},
		{name: "active uses human boundary despite live compaction", run: &activeRun{
			sideChatHistory: throughLatestHumanInput(live),
			sideChatContext: throughLatestHumanInput(live),
		}, want: stable},
		{name: "manual compaction retains settled context", run: &activeRun{compacting: true}, want: live},
	} {
		t.Run(test.name, func(t *testing.T) {
			executionContext := live
			if test.run != nil && !test.run.compacting {
				executionContext = []model.Message{{Role: "user", Text: "new live compacted summary"}}
			}
			core := &Engine{
				history: map[string][]model.Message{"parent": live},
				context: map[string][]model.Message{"parent": executionContext},
				runs:    map[string]*activeRun{"parent": test.run},
			}
			history, executionContext := core.sideChatSnapshotLocked("parent")
			want := model.CloneMessages(test.want)
			for i := range want {
				want[i].UserInput = false
			}
			if !reflect.DeepEqual(history, want) || !reflect.DeepEqual(executionContext, want) {
				t.Fatalf("wrong fork boundary: history=%+v context=%+v", history, executionContext)
			}
		})
	}
}

func TestSideChatSnapshotIsHiddenIndependentAndPersistent(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = core.Close() }()
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	parent := core.state.Sessions[0]
	parent.Pinned, parent.Archived, parent.Unread, parent.PinOrder = true, true, true, 4
	parent.Goal = &contracts.SessionGoal{ID: "parent-goal", Status: contracts.GoalPaused}
	parent.SubagentType, parent.RunningSubagents, parent.LastOutcome = "Explore", 2, "completed"
	core.state.Sessions[0] = parent
	core.history[parent.ID] = []model.Message{
		{Role: "user", Text: "parent request", UserInput: true, InputIDs: []string{"parent-input"}},
		{Role: "assistant", Calls: []model.Call{{ID: "Stable_Call", Name: "Read", Arguments: `{"file_path":"proof.txt"}`}}},
		{Role: "tool", CallID: "Stable_Call", Text: "parent proof", Images: []model.Image{{MIME: "image/png", Data: "original"}}},
		{Role: "assistant", Text: "parent answer"},
	}
	core.context[parent.ID] = model.CloneMessages(core.history[parent.ID])
	core.state.Messages[parent.ID] = []contracts.Message{{ID: "parent-display", SessionID: parent.ID, Role: "assistant"}}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: parent.ID, RequestID: "side-once"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	side := snapshot.Sessions[0]
	if side.ID == parent.ID || side.ParentSessionID != parent.ID || !side.SideChat || side.CWD != parent.CWD ||
		side.ProjectID != parent.ProjectID || side.ModelID != parent.ModelID {
		t.Fatalf("side chat did not inherit its workspace and model: %+v", side)
	}
	if side.Goal != nil || side.SubagentType != "" || side.RunningSubagents != 0 || side.Status != contracts.Idle ||
		side.Pinned || side.Archived || side.Unread || side.PinOrder != 0 || side.LastOutcome != "" {
		t.Fatalf("side chat inherited parent execution or presentation state: %+v", side)
	}
	if len(snapshot.Messages[side.ID]) != 0 || len(core.history[side.ID]) != 4 || len(core.context[side.ID]) != 4 {
		t.Fatal("side chat lost hidden model history or exposed it as display messages")
	}
	for _, message := range core.history[side.ID] {
		if message.UserInput || len(message.InputIDs) != 0 {
			t.Fatal("inherited history retained parent input ownership")
		}
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "message.retry", SessionID: side.ID}); err == nil {
		t.Fatal("empty side chat may not retry the inherited parent task")
	}
	beforeHistory := model.CloneMessages(core.history[side.ID])
	beforeContext := model.CloneMessages(core.context[side.ID])
	core.history[parent.ID][1].Calls[0].Name = "changed-parent"
	core.context[parent.ID][2].Images[0].Data = "changed-parent"
	core.history[parent.ID] = append(core.history[parent.ID], model.Message{Role: "user", Text: "later parent input"})
	if !reflect.DeepEqual(core.history[side.ID], beforeHistory) || !reflect.DeepEqual(core.context[side.ID], beforeContext) {
		t.Fatal("parent changes mutated the side chat snapshot")
	}
	core.history[side.ID][2].Images[0].Data = "changed-child"
	if core.history[parent.ID][2].Images[0].Data != "original" || core.context[side.ID][2].Images[0].Data != "original" {
		t.Fatal("side chat mutations leaked into parent or execution context")
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: parent.ID, RequestID: "side-once"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.Sessions) != 2 {
		t.Fatal("retrying a creation request created another side chat")
	}
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
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.Messages[side.ID]) != 0 || !reflect.DeepEqual(core.context[side.ID], beforeContext) ||
		len(core.history[side.ID]) != 4 {
		t.Fatal("restart lost hidden inherited history or exposed it in the transcript")
	}
}

func TestSideChatRunningParentBoundaryAndProtocolRequests(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			requests := make(chan string, 4)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				body := string(data)
				w.Header().Set("Content-Type", "text/event-stream")
				if strings.Contains(body, "SIDE_QUESTION") {
					requests <- body
					sideChatFixtureResponse(w, protocol, false)
				} else {
					sideChatFixtureResponse(w, protocol, true)
				}
			}))
			defer provider.Close()
			home := t.TempDir()
			store, err := storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err := Open(Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = core.Close() }()
			ctx := context.Background()
			config := model.Provider{ID: "p", Name: "P", Protocol: protocol, BaseURL: provider.URL, Models: []model.Model{{ID: "m"}}}
			if err = core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
				t.Fatal(err)
			}
			if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Parent"}); err != nil {
				t.Fatal(err)
			}
			parent := core.state.Sessions[0]
			core.history[parent.ID] = []model.Message{
				{Role: "user", Text: "STABLE_USER", UserInput: true},
				{Role: "assistant", Calls: []model.Call{{ID: "Stable_Call", Name: "Read", Arguments: `{"file_path":"proof.txt"}`}}},
				{Role: "tool", CallID: "Stable_Call", Text: "STABLE_TOOL_RESULT"},
				{Role: "assistant", Text: "STABLE_ANSWER"},
			}
			core.context[parent.ID] = model.CloneMessages(core.history[parent.ID])
			if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				SessionID: parent.ID, ModelID: "p/m", Text: "PARENT_CURRENT_INPUT",
			}}); err != nil {
				t.Fatal(err)
			}
			waitSideChatSession(t, core, parent.ID, contracts.AwaitingInput)
			if err = core.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: parent.ID}); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := core.Snapshot(ctx)
			side := snapshot.Sessions[0]
			if len(snapshot.Messages[side.ID]) != 0 || len(core.context[side.ID]) != 5 {
				t.Fatalf("running parent boundary was not copied as hidden context: %+v", core.context[side.ID])
			}
			for _, message := range core.context[side.ID] {
				for _, call := range message.Calls {
					if call.ID == "Active_Call" {
						t.Fatal("side chat copied the active parent tool request")
					}
				}
			}
			if err = core.Execute(ctx, contracts.Command{Kind: "message.stop", SessionID: parent.ID}); err != nil {
				t.Fatal(err)
			}
			waitSideChatSession(t, core, parent.ID, contracts.Idle)
			// Resume the side chat from disk before making its first model request.
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
			if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				SessionID: side.ID, Text: "SIDE_QUESTION",
			}}); err != nil {
				t.Fatal(err)
			}
			waitSideChatSession(t, core, side.ID, contracts.Idle)
			select {
			case body := <-requests:
				for _, want := range []string{"STABLE_USER", "Stable_Call", "STABLE_TOOL_RESULT", "STABLE_ANSWER",
					"PARENT_CURRENT_INPUT", "SIDE_QUESTION", "reference context only", "explicitly requests it in this side chat"} {
					if !strings.Contains(body, want) {
						t.Errorf("%s request omitted %q: %s", protocol, want, body)
					}
				}
				if strings.Contains(body, "Active_Call") || strings.Contains(body, "ACTIVE_PARENT_QUESTION") {
					t.Fatal("side request included active parent execution")
				}
			default:
				t.Fatal("side chat did not issue a model request")
			}
			snapshot, _ = core.Snapshot(ctx)
			if len(snapshot.Messages[side.ID]) != 2 || snapshot.Messages[side.ID][0].Blocks[0].Text != "SIDE_QUESTION" ||
				snapshot.Sessions[0].LastOutcome != "completed" {
				t.Fatalf("side chat transcript is not independent: %+v", snapshot.Messages[side.ID])
			}
		})
	}
}

func TestSideChatRetryRequiresOwnHumanInputAfterCompaction(t *testing.T) {
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		sideChatFixtureResponse(w, model.Chat, false)
	}))
	defer provider.Close()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	config := model.Provider{ID: "p", Name: "P", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "m"}}}
	if err = core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	parent := core.state.Sessions[0]
	core.state.Sessions[0].ModelID = "p/m"
	for i := range 6 {
		core.history[parent.ID] = append(core.history[parent.ID],
			model.Message{Role: "user", Text: fmt.Sprintf("parent input %d", i), UserInput: true},
			model.Message{Role: "assistant", Text: fmt.Sprintf("parent answer %d", i)},
		)
	}
	core.context[parent.ID] = model.CloneMessages(core.history[parent.ID])
	if err = core.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: parent.ID}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	sideID := snapshot.Sessions[0].ID
	if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: sideID, Text: "/compact",
	}}); err != nil {
		t.Fatal(err)
	}
	waitSideChatSession(t, core, sideID, contracts.Idle)
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.Messages[sideID]) != 1 || snapshot.Messages[sideID][0].Role != "assistant" || requests.Load() != 1 {
		t.Fatal("compaction did not produce the expected assistant-only status message")
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "message.retry", SessionID: sideID}); err == nil {
		t.Fatal("compaction status message allowed retrying inherited parent history")
	}
	if requests.Load() != 1 {
		t.Fatal("rejected retry still invoked the model")
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: sideID, Text: "my own side chat question",
	}}); err != nil {
		t.Fatal(err)
	}
	waitSideChatSession(t, core, sideID, contracts.Idle)
	// Model-only context can grow after the accepted input. Ensure compaction
	// summarizes that input away, while the full execution log retains ownership.
	core.mu.Lock()
	for i := range 10 {
		message := model.Message{Role: "assistant", Text: fmt.Sprintf("later model step %d", i)}
		core.history[sideID] = append(core.history[sideID], message)
		core.context[sideID] = append(core.context[sideID], message)
	}
	core.mu.Unlock()
	if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: sideID, Text: "/compact",
	}}); err != nil {
		t.Fatal(err)
	}
	waitSideChatSession(t, core, sideID, contracts.Idle)
	core.mu.Lock()
	contextHasInput := slices.ContainsFunc(core.context[sideID], func(message model.Message) bool { return message.UserInput })
	historyHasInput := slices.ContainsFunc(core.history[sideID], func(message model.Message) bool { return message.UserInput })
	core.mu.Unlock()
	if contextHasInput || !historyHasInput {
		t.Fatal("fixture failed to summarize the accepted human input while retaining full history")
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "message.retry", SessionID: sideID}); err != nil {
		t.Fatalf("compaction incorrectly removed the side chat's retry eligibility: %v", err)
	}
	waitSideChatSession(t, core, sideID, contracts.Idle)
	if requests.Load() != 4 {
		t.Fatalf("unexpected model requests: %d", requests.Load())
	}
}

func sideChatFixtureResponse(w http.ResponseWriter, protocol model.Protocol, question bool) {
	arguments := `{"questions":[{"question":"ACTIVE_PARENT_QUESTION","header":"Continue","options":[{"label":"Continue","description":"Continue the task"},{"label":"Stop","description":"Stop the task"}],"multiSelect":false}]}`
	switch protocol {
	case model.Chat:
		delta := map[string]any{"content": "SIDE_ANSWER"}
		finish := "stop"
		if question {
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "Active_Call",
				"function": map[string]any{"name": "AskUserQuestion", "arguments": arguments}}}}
			finish = "tool_calls"
		}
		sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	case model.Responses:
		output := []any{}
		if question {
			output = []any{map[string]any{"type": "function_call", "call_id": "Active_Call", "name": "AskUserQuestion", "arguments": arguments}}
		} else {
			sendSSE(w, map[string]any{"type": "response.output_text.delta", "delta": "SIDE_ANSWER"})
		}
		sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": output}})
	case model.Messages:
		stop := "end_turn"
		if question {
			var input map[string]any
			_ = json.Unmarshal([]byte(arguments), &input)
			sendSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{
				"type": "tool_use", "id": "Active_Call", "name": "AskUserQuestion",
				"input": input,
			}})
			stop = "tool_use"
		} else {
			sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "SIDE_ANSWER"}})
		}
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}

func waitSideChatSession(t *testing.T, core *Engine, sessionID string, status contracts.SessionStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range snapshot.Sessions {
			if session.ID == sessionID {
				if session.Status == status {
					return
				}
				if session.Status == contracts.Errored {
					t.Fatalf("session failed: %+v", snapshot.Runs)
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach %s", sessionID, status)
}
