package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// A stop request must be visible in the run record before the run settles, and
// the reason must survive settlement so a user stop stays distinguishable from
// every other cancellation. These tests hold a tool batch open so the transient
// stopping status is observable instead of racing settlement.

func runFor(state contracts.Snapshot, sessionID string) *contracts.Run {
	for i := range state.Runs {
		if state.Runs[i].SessionID == sessionID {
			return &state.Runs[i]
		}
	}
	return nil
}

// waitRunSettled reads snapshots until the session's run reports a finish time.
// Subscriber events are unusable here: a stopping run is not running, so an
// event-driven idle wait would return before settlement.
func waitRunSettled(t *testing.T, core *Engine, sessionID string) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		state, err := core.Snapshot(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if run := runFor(state, sessionID); run != nil && run.FinishedAt != nil {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not settle: %+v", state.Runs)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// openBlockedRunFixture starts a run whose first tool batch waits in a guard
// until release is closed. The guard deliberately ignores cancellation: a tool
// batch is drained before the run settles, which keeps stopping observable.
func openBlockedRunFixture(t *testing.T, entered chan struct{}, release <-chan struct{}) (*Engine, string) {
	t.Helper()
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "stop-probe-1", "function": map[string]any{"name": "Bash", "arguments": `{"command":"true"}`}}}}, "finish_reason": "tool_calls"}}})
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "done"}, "finish_reason": "stop"}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(fixture.Close)
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{
		Storage: store,
		ToolGuard: func(ctx context.Context, cwd, name string, args json.RawMessage) error {
			if name != "Bash" {
				return nil
			}
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	ctx := context.Background()
	config := model.Provider{ID: "p", Name: "P", Protocol: model.Chat, BaseURL: fixture.URL, Models: []model.Model{{ID: "m"}}}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "run a tool", ModelID: "p/m"}}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("sessions = %+v", state.Sessions)
	}
	select {
	case <-entered:
	case <-time.After(20 * time.Second):
		t.Fatal("the run never reached its tool batch")
	}
	return core, state.Sessions[0].ID
}

func TestStopSemanticsUserStopPublishesStoppingThenInterrupted(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	core, sid := openBlockedRunFixture(t, entered, release)
	ctx := context.Background()
	state, err := core.Snapshot(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if run := runFor(state, sid); run == nil || run.Status != contracts.RunStatusRunning {
		t.Fatalf("run before stop = %+v", state.Runs)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.stop", SessionID: sid}); err != nil {
		t.Fatal(err)
	}
	// The tool batch still holds the run open, so the request is observable.
	state, err = core.Snapshot(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	run := runFor(state, sid)
	if run == nil || run.Status != contracts.RunStatusStopping || run.FinishReason != contracts.FinishReasonUserStop {
		t.Fatalf("run during stop = %+v, want stopping/userStop", state.Runs)
	}
	close(release)
	state = waitRunSettled(t, core, sid)
	run = runFor(state, sid)
	if run == nil || run.Status != contracts.RunStatusInterrupted || run.FinishReason != contracts.FinishReasonUserStop {
		t.Fatalf("run after stop = %+v, want interrupted/userStop", state.Runs)
	}
	for _, session := range state.Sessions {
		if session.ID == sid && session.LastOutcome != "interrupted" {
			t.Fatalf("session outcome = %q, want interrupted", session.LastOutcome)
		}
	}
}

func TestStopSemanticsOtherCancellationRecordsCanceled(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	core, sid := openBlockedRunFixture(t, entered, release)
	ctx := context.Background()
	// Archiving cancels the live run without a user stop request.
	if err := core.Execute(ctx, contracts.Command{Kind: "session.archive", ID: sid, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	close(release)
	state := waitRunSettled(t, core, sid)
	run := runFor(state, sid)
	if run == nil || run.Status != contracts.RunStatusInterrupted || run.FinishReason != contracts.FinishReasonCanceled {
		t.Fatalf("run after archive = %+v, want interrupted/canceled", state.Runs)
	}
}

func TestStopSemanticsCompletionRecordsStop(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "完成"}, "finish_reason": "stop"}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer fixture.Close()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	config := model.Provider{ID: "p", Name: "P", Protocol: model.Chat, BaseURL: fixture.URL, Models: []model.Model{{ID: "m"}}}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "hello", ModelID: "p/m"}}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("sessions = %+v", state.Sessions)
	}
	state = waitRunSettled(t, core, state.Sessions[0].ID)
	run := runFor(state, state.Sessions[0].ID)
	if run == nil || run.Status != contracts.RunStatusCompleted || run.FinishReason != contracts.FinishReasonStop {
		t.Fatalf("completed run = %+v, want completed/stop", state.Runs)
	}
}
