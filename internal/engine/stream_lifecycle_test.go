package engine

import (
	"context"
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

// A turn runs one model round per tool batch: message.start, the streamed
// reply, message.end, the round's tools, then the next round. message.end
// therefore closes a round, not the turn. It must not clear the message's
// Streaming flag, or a transcript that folds live work on that flag closes
// its activity group the moment a command starts and reopens it with the
// next round — with no user input. Only the run settling clears it.
func TestTurnStaysStreamingAcrossToolRounds(t *testing.T) {
	var calls atomic.Int32
	toolStarted := make(chan struct{})
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "probe-1", "function": map[string]any{"name": "Bash", "arguments": `{"command":"printf probe"}`}}}}, "finish_reason": "tool_calls"}}})
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		<-toolStarted
		sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "done"}, "finish_reason": "stop"}}})
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
	workspace := t.TempDir()
	ctx := context.Background()
	config := model.Provider{ID: "custom", Name: "P", Protocol: model.Chat, BaseURL: fixture.URL, APIKey: "k", Models: []model.Model{{ID: "model"}}}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "T"}}); err != nil {
		t.Fatal(err)
	}
	before, _ := core.Snapshot(ctx)
	events, err := core.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{ProjectID: before.Projects[0].ID, ModelID: "custom/model", Text: "run it"}}); err != nil {
		t.Fatal(err)
	}
	// The window between message.end and the round's tool.start is exactly
	// where the reported fold collapsed. Assert on the tool.start event.
	deadline := time.After(20 * time.Second)
	for {
		select {
		case event := <-events:
			state, err := core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !hasRunningTool(state.Messages[event.SessionID]) {
				continue
			}
			for _, message := range state.Messages[event.SessionID] {
				if message.Role == "assistant" && !message.Streaming {
					t.Fatalf("message %q stopped streaming while its tool round was still running", message.ID)
				}
			}
			close(toolStarted)
			waitIdle(t, core)
			state, _ = core.Snapshot(ctx)
			for _, message := range state.Messages[event.SessionID] {
				if message.Role == "assistant" && message.Streaming {
					t.Fatalf("settled message %q still streams", message.ID)
				}
			}
			return
		case <-deadline:
			t.Fatal("run never reached a tool round")
		}
	}
}

func hasRunningTool(messages []contracts.Message) bool {
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.Tool != nil && block.Tool.Status == "running" {
				return true
			}
		}
	}
	return false
}
