package engine

import (
	"context"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func todoLists(messages []contracts.Message) []contracts.Block {
	var lists []contracts.Block
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.Kind == "todos" {
				lists = append(lists, block)
			}
		}
	}
	return lists
}

func TestTodoSnapshotProjectsOnlySuccessfulValidatedCalls(t *testing.T) {
	for _, test := range []struct {
		name, tool, status, args string
		isError, wantList        bool
	}{
		{"success", "TodoWrite", "completed", `{"todos":[{"text":"Verify native UI","status":"in_progress"}]}`, false, true},
		{"running", "TodoWrite", "running", `{"todos":[{"text":"Not committed","status":"pending"}]}`, false, false},
		{"failed", "TodoWrite", "failed", `{"todos":[{"text":"Rejected","status":"pending"}]}`, true, false},
		{"error result", "TodoWrite", "completed", `{"todos":[{"text":"Rejected","status":"pending"}]}`, true, false},
		{"invalid input", "TodoWrite", "completed", `{"todos":[{"text":"Invalid","status":"done"}]}`, false, false},
		{"malformed", "TodoWrite", "completed", `{`, false, false},
		{"unrelated", "mcp__todo__TodoWrite", "completed", `{"todos":[{"text":"External tool","status":"pending"}]}`, false, false},
		{"clear", "TodoWrite", "completed", `{"todos":[]}`, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := contracts.ToolCall{ID: "call", Name: test.tool, Status: test.status, Args: test.args, IsError: test.isError}
			core := &Engine{state: contracts.Snapshot{Messages: map[string][]contracts.Message{
				"session": {{ID: "message", Role: "assistant", Blocks: []contracts.Block{{ID: "call", Kind: "tool", Tool: &tool}}}},
			}}}
			snapshot, err := core.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			lists := todoLists(snapshot.Messages["session"])
			if (len(lists) == 1) != test.wantList {
				t.Fatalf("got %d todo lists, want list=%t", len(lists), test.wantList)
			}
			if test.wantList && test.name != "clear" {
				if lists[0].Todos[0].Text != "Verify native UI" || lists[0].Todos[0].Status != "in_progress" ||
					lists[0].ID == "" || lists[0].Todos[0].ID == "" {
					t.Fatalf("lost typed todo content or stable identity: %+v", lists[0])
				}
				lists[0].Todos[0].Text = "subscriber mutation"
				again, _ := core.Snapshot(context.Background())
				if todoLists(again.Messages["session"])[0].Todos[0].Text != "Verify native UI" {
					t.Fatal("snapshot mutation leaked into core")
				}
			}
			if len(core.state.Messages["session"][0].Blocks) != 1 {
				t.Fatal("display projection duplicated authoritative state")
			}
		})
	}
}

func TestTodoEventsAndResynchronizationCarryLists(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	core := &Engine{
		state: contracts.Snapshot{Messages: map[string][]contracts.Message{}},
		subs:  map[uint64]chan contracts.Event{}, done: make(chan struct{}),
	}
	events, err := core.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	<-events
	var current string
	for _, event := range []agent.Event{
		{Kind: "message.start"},
		{Kind: "tool.start", Call: model.Call{ID: "one", Name: "TodoWrite", Arguments: `{"todos":[{"text":"First","status":"in_progress"},{"text":"Second","status":"pending"}]}`}},
		{Kind: "tool.end", Call: model.Call{ID: "one"}, Output: "success"},
		{Kind: "tool.start", Call: model.Call{ID: "two", Name: "TodoWrite", Arguments: `{"todos":[{"text":"First","status":"completed"},{"text":"Second","status":"completed"}]}`}},
		{Kind: "tool.end", Call: model.Call{ID: "two"}, Output: "success"},
	} {
		if err := core.applyAgentEventLocked("session", "run", &current, event); err != nil {
			t.Fatal(err)
		}
	}
	// Consumers see the typed checklist immediately on tool completion.
	var changed []contracts.Message
	for range 5 {
		event := <-events
		if event.Message != nil {
			changed = append(changed, *event.Message)
		}
	}
	if lists := todoLists(changed[2:3]); len(lists) != 1 || lists[0].Todos[0].Status != "in_progress" {
		t.Fatalf("first successful tool event lacks todos: %+v", lists)
	}
	assertLatest := func(messages []contracts.Message) {
		t.Helper()
		lists := todoLists(messages)
		if len(lists) != 2 || lists[1].Todos[0].Status != "completed" || lists[1].Todos[1].Status != "completed" {
			t.Fatalf("updates lost call order or completion: %+v", lists)
		}
	}
	assertLatest(changed[4:5])
	reconnected, err := core.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertLatest((<-reconnected).Snapshot.Messages["session"])
	// Full state publications and slow-consumer recovery use the same view.
	core.publishLocked(contracts.Event{Kind: "run.finished"})
	assertLatest((<-events).Snapshot.Messages["session"])
	for range 300 {
		core.publishLocked(contracts.Event{Kind: "message.changed", Message: &core.state.Messages["session"][0]})
	}
	recovered := <-events
	if recovered.Kind != "snapshot" {
		t.Fatalf("expected slow-consumer snapshot, got %q", recovered.Kind)
	}
	assertLatest(recovered.Snapshot.Messages["session"])
}
