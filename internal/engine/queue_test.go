package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func queueCore(t *testing.T) (*Engine, string) {
	t.Helper()
	core, _ := resourceCore(t)
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return core, state.Sessions[0].ID
}

func queuedRow(sessionID, id, text string, at time.Time) contracts.QueuedMessage {
	return contracts.QueuedMessage{ID: id, SessionID: sessionID, Text: text, Prompt: text, CreatedAt: at}
}

// Only internal inputs join a task. Each user input, including steering,
// starts a separate task at its delivery boundary.
func TestIdleDrainHandsOverOneQueuedMessage(t *testing.T) {
	core, sessionID := queueCore(t)
	base := time.Now()
	core.mu.Lock()
	defer core.mu.Unlock()
	core.state.Queue = []contracts.QueuedMessage{
		queuedRow(sessionID, "one", "ONE", base),
		queuedRow(sessionID, "two", "TWO", base.Add(time.Second)),
		queuedRow(sessionID, "three", "THREE", base.Add(2*time.Second)),
	}
	if messages := core.drainPendingLocked(sessionID, true); len(messages) != 0 {
		t.Fatalf("idle drain folded user input into the current task: %+v", messages)
	}
	run := &activeRun{messageID: "started"}
	if core.handoffUserInputLocked(sessionID, run, false) {
		t.Fatal("ordinary queued input preempted an active step")
	}
	if !core.handoffUserInputLocked(sessionID, run, true) || run.nextInput.ID != "one" {
		t.Fatal("idle did not hand off the first input")
	}
	if len(core.state.Queue) != 3 {
		t.Fatalf("drain left %+v, want the rest in order", core.state.Queue)
	}
	core.state.Queue[2].Delivery = "steer"
	run = &activeRun{messageID: "started"}
	if !core.handoffUserInputLocked(sessionID, run, false) || run.nextInput.ID != "three" {
		t.Fatal("step boundary did not select the steering input")
	}
}

// Moving a row before or after another row of the same session changes the
// order the queue will hand over, and a cross-session move is refused.
func TestQueueMoveReordersWithinTheSession(t *testing.T) {
	core, sessionID := queueCore(t)
	base := time.Now()
	core.mu.Lock()
	defer core.mu.Unlock()
	core.state.Queue = []contracts.QueuedMessage{
		queuedRow(sessionID, "a", "A", base),
		queuedRow(sessionID, "b", "B", base.Add(time.Second)),
		queuedRow(sessionID, "c", "C", base.Add(2*time.Second)),
	}
	if err := core.moveQueuedLocked(contracts.ReorderInput{SourceID: "c", TargetID: "a"}); err != nil {
		t.Fatal(err)
	}
	if got := queueIDs(core.state.Queue); got != "c,a,b" {
		t.Fatalf("move before = %s, want c,a,b", got)
	}
	if err := core.moveQueuedLocked(contracts.ReorderInput{SourceID: "c", TargetID: "b", After: true}); err != nil {
		t.Fatal(err)
	}
	if got := queueIDs(core.state.Queue); got != "a,b,c" {
		t.Fatalf("move after = %s, want a,b,c", got)
	}
	core.state.Queue = append(core.state.Queue, queuedRow("other", "x", "X", base))
	if err := core.moveQueuedLocked(contracts.ReorderInput{SourceID: "x", TargetID: "a"}); err == nil {
		t.Fatal("a cross-session move was accepted")
	}
	if got := queueIDs(core.state.Queue); got != "a,b,c,x" {
		t.Fatalf("refused move changed the order: %s", got)
	}
}

func queueIDs(queue []contracts.QueuedMessage) string {
	ids := make([]string, len(queue))
	for i, queued := range queue {
		ids[i] = queued.ID
	}
	return strings.Join(ids, ",")
}

// A queue row's Send now stops the live response instead of waiting for it,
// and the row opens the next turn ahead of the rows that were queued before
// it. The remaining rows still follow one turn at a time.
func TestSendQueuedNowStopsTheRunAndOpensTheNextTurn(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var asked []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		last := ""
		if messages, ok := body["messages"].([]any); ok {
			for _, raw := range messages {
				message, _ := raw.(map[string]any)
				if text, ok := message["content"].(string); ok && message["role"] == "user" {
					last = text
				}
			}
		}
		mu.Lock()
		asked = append(asked, last)
		first := len(asked) == 1
		mu.Unlock()
		if first {
			once.Do(func() { close(started) })
			select {
			case <-r.Context().Done():
				return
			case <-release:
			}
		}
		goalResponse(w, model.Chat, "Reply.", nil, true)
	})
	core, _ := goalTestCore(t, model.Chat, handler)
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "first", ModelID: "goal/model"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first request did not start")
	}
	state, _ := core.Snapshot(ctx)
	sessionID := state.Sessions[0].ID
	for _, text := range []string{"queued one", "queued two"} {
		if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: text, SessionID: sessionID}}); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = core.Snapshot(ctx, sessionID)
	if len(state.Queue) != 2 || state.Queue[0].Text != "queued one" || state.Queue[1].Text != "queued two" {
		t.Fatalf("queue = %+v", state.Queue)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "queue.send", ID: state.Queue[1].ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx, sessionID)
	if len(state.Queue) != 1 || state.Queue[0].Text != "queued one" {
		t.Fatalf("Send now left %+v, want the earlier row only", state.Queue)
	}
	close(release)
	state = waitQueueDone(t, core, sessionID)
	if len(asked) != 3 {
		t.Fatalf("model requests = %q, want three turns", asked)
	}
	for i, want := range []string{"first", "queued two", "queued one"} {
		if !strings.Contains(asked[i], want) {
			t.Fatalf("request %d = %q, want %q", i+1, asked[i], want)
		}
	}
	if len(state.Queue) != 0 {
		t.Fatalf("queue did not drain: %+v", state.Queue)
	}
	var texts []string
	for _, message := range state.Messages[sessionID] {
		if message.Role == "user" {
			for _, block := range message.Blocks {
				texts = append(texts, block.Text)
			}
		}
	}
	if strings.Join(texts, "|") != "first|queued two|queued one" {
		t.Fatalf("transcript order = %v", texts)
	}
	if state.Runs[0].Status != "interrupted" {
		t.Fatalf("the interrupted run = %+v", state.Runs[0])
	}
}

func waitQueueDone(t *testing.T, core *Engine, sessionID string) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, err := core.Snapshot(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Queue) == 0 && len(state.Sessions) > 0 && state.Sessions[0].Status == contracts.Idle &&
			len(state.Runs) > 0 && state.Runs[len(state.Runs)-1].Status != "running" {
			// The interrupted run must settle before the handoff starts the
			// next one; both have finished once no run is live.
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue did not settle: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
