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

// Execution fixtures count model/tool steps exactly; naming is covered separately.
func openExecutionFixture(t *testing.T, options Options) (*Engine, error) {
	t.Helper()
	if options.Client == nil {
		options.Client = model.NewClient()
		retry := model.DefaultRetryPolicy()
		retry.Wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
		options.Client.RetryPolicy = &retry
	}
	core, err := Open(options)
	if err == nil {
		snapshot, snapshotErr := core.Snapshot(context.Background())
		if snapshotErr != nil {
			_ = core.Close()
			return nil, snapshotErr
		}
		settings := snapshot.Settings
		settings.General.ThreadNamingModel = "first-message"
		err = core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings})
		if err != nil {
			_ = core.Close()
		}
	}
	return core, err
}

func TestRealProtocolToolLoopAndPersistentHistory(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var calls atomic.Int32
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if calls.Add(1) == 1 {
					arguments := `{"file_path":"answer.txt","content":"真实 Go 工具写入"}`
					switch protocol {
					case model.Chat:
						sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"reasoning_content": "tool-round-reasoning", "tool_calls": []any{map[string]any{"index": 0, "id": "write-1", "function": map[string]any{"name": "Write", "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
						fmt.Fprint(w, "data: [DONE]\n\n")
					case model.Responses:
						sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "write-1", "name": "Write", "arguments": arguments}}}})
					case model.Messages:
						sendSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "write-1", "name": "Write", "input": map[string]any{}}})
						sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": arguments}})
						sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}})
						sendSSE(w, map[string]any{"type": "message_stop"})
					}
					return
				}
				data, _ := json.Marshal(request)
				if !strings.Contains(string(data), "write-1") || !strings.Contains(string(data), "Wrote") {
					t.Error("second request lost tool result")
				}
				if protocol == model.Chat && !strings.Contains(string(data), `"reasoning_content":"tool-round-reasoning"`) {
					t.Error("second request lost the reasoning from the tool-call round")
				}
				switch protocol {
				case model.Chat:
					sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "完成"}, "finish_reason": "stop"}}})
					fmt.Fprint(w, "data: [DONE]\n\n")
				case model.Responses:
					sendSSE(w, map[string]any{"type": "response.output_text.delta", "delta": "完成"})
					sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{}}})
				case model.Messages:
					sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "完成"}})
					sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
					sendSSE(w, map[string]any{"type": "message_stop"})
				}
			}))
			defer fixture.Close()
			home := t.TempDir()
			workspace := t.TempDir()
			store, err := storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			config := model.Provider{ID: "custom", Name: "My provider", Protocol: protocol, BaseURL: fixture.URL, APIKey: "fixture-only-secret", Models: []model.Model{{ID: "model"}}}
			if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Test"}}); err != nil {
				t.Fatal(err)
			}
			before, _ := core.Snapshot(ctx)
			if before.CustomProviders[0].Provider.APIKey != "" || !before.CustomProviders[0].HasAPIKey {
				t.Fatal("secret leaked or missing key metadata")
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{RequestID: "one", ProjectID: before.Projects[0].ID, ModelID: "custom/model", Text: "写文件并确认"}}); err != nil {
				t.Fatal(err)
			}
			summary, err := core.Snapshot(ctx)
			if err != nil || len(summary.Sessions) != 1 {
				t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
			}
			sid := summary.Sessions[0].ID
			state := waitIdle(t, core, sid)
			state = waitForProjectFiles(t, core, sid)
			if calls.Load() != 2 || len(state.Sessions) != 1 || state.Sessions[0].LastOutcome != "completed" {
				t.Fatalf("unfinished run: %+v", state.Runs)
			}
			state, err = core.Snapshot(ctx, sid)
			if err != nil {
				t.Fatal(err)
			}
			trajectory, trajectoryErr := core.ReadModelTrajectory(ctx, sid)
			if trajectoryErr != nil || len(trajectory.Records) != 2 {
				t.Fatalf("real tool-loop trajectory missing: records=%d error=%v", len(trajectory.Records), trajectoryErr)
			}
			if len(trajectory.Records[0].Response.Message.Calls) != 1 || trajectory.Records[0].Response.Message.Calls[0].ID != "write-1" || trajectory.Records[0].CompletedAt == nil ||
				len(trajectory.Records[1].Messages) < 3 || trajectory.Records[1].Response.Message.Text != "完成" {
				t.Fatal("trajectory lost the actual request context, tool call or response")
			}
			for _, message := range state.Messages[sid] {
				if message.Role == "assistant" && message.ModelID != "custom/model" {
					t.Fatalf("request model was not recorded for usage statistics: %+v", message)
				}
			}
			data, err := os.ReadFile(filepath.Join(workspace, "answer.txt"))
			if err != nil || string(data) != "真实 Go 工具写入" {
				t.Fatalf("file write failed %s %v", data, err)
			}
			if len(state.Changes) != 1 || state.Changes[0].NewContent != string(data) || len(state.Files) != 1 || state.Files[0].Path != "answer.txt" {
				t.Fatal("tool changes did not reach the real diff and file tree")
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err = openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			restored, _ := core.Snapshot(ctx, sid)
			restoredTrajectory, trajectoryErr := core.ReadModelTrajectory(ctx, sid)
			if trajectoryErr != nil || len(restoredTrajectory.Records) != 2 {
				t.Fatal("restart lost the separately persisted model trajectory")
			}
			for _, message := range restored.Messages[sid] {
				if message.Role == "assistant" && message.ModelID != "custom/model" {
					t.Fatal("restart lost the recorded request model")
				}
			}
			if len(restored.Sessions) != 1 || len(restored.Messages[sid]) != len(state.Messages[sid]) || len(restored.Providers) != 1 {
				t.Fatal("restart lost runtime data")
			}
		})
	}
}

func TestRestartAndModelSwitchPreserveScopedContinuation(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var calls atomic.Int32
			requests := make(chan string, 1)
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				requests <- string(body)
				first := calls.Add(1) == 1
				w.Header().Set("Content-Type", "text/event-stream")
				if protocol == model.Chat {
					delta := map[string]any{"content": "done"}
					if first {
						delta["reasoning_content"] = "private-replay"
					}
					sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": "stop"}}})
					fmt.Fprint(w, "data: [DONE]\n\n")
				} else if protocol == model.Responses {
					output := []any{}
					if first {
						output = append(output, map[string]any{
							"type": "reasoning", "id": "reasoning-one", "encrypted_content": "private-replay",
						})
					}
					output = append(output, map[string]any{
						"type": "message", "role": "assistant",
						"content": []any{map[string]any{"type": "output_text", "text": "done"}},
					})
					sendSSE(w, map[string]any{
						"type":     "response.completed",
						"response": map[string]any{"status": "completed", "output": output},
					})
				} else {
					if first {
						sendSSE(w, map[string]any{
							"type": "content_block_start", "index": 0,
							"content_block": map[string]any{
								"type": "thinking", "thinking": "reasoning", "signature": "private-replay",
							},
						})
					}
					sendSSE(w, map[string]any{
						"type": "content_block_start", "index": 1,
						"content_block": map[string]any{"type": "text", "text": "done"},
					})
					sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
					sendSSE(w, map[string]any{"type": "message_stop"})
				}
			}))
			defer fixture.Close()
			home := t.TempDir()
			openCore := func() *Engine {
				store, err := storage.Open(storage.Options{Home: home})
				if err != nil {
					t.Fatal(err)
				}
				core, err := openExecutionFixture(t, Options{Storage: store})
				if err != nil {
					_ = store.Close()
					t.Fatal(err)
				}
				return core
			}
			core := openCore()
			defer func() { _ = core.Close() }()
			provider := model.Provider{
				ID: "p", Name: "Configured channel", Protocol: protocol, BaseURL: fixture.URL,
				Models: []model.Model{{ID: "one"}, {ID: "two"}},
			}
			if err := core.Execute(context.Background(), contracts.Command{
				Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
			}); err != nil {
				t.Fatal(err)
			}
			sid := ""
			for step, modelID := range []string{"p/one", "p/one", "p/two", "p/one"} {
				if step == 1 {
					if err := core.Close(); err != nil {
						t.Fatal(err)
					}
					core = openCore()
				}
				if err := core.Execute(context.Background(), contracts.Command{
					Kind: "message.send",
					Send: &contracts.SendRequest{SessionID: sid, ModelID: modelID, Text: "continue"},
				}); err != nil {
					t.Fatal(err)
				}
				state := waitIdle(t, core, sid)
				if state.Sessions[0].LastOutcome != "completed" {
					t.Fatalf("step %d failed: %+v", step, state.Runs)
				}
				sid = state.Sessions[0].ID
				body := <-requests
				wantPrivate := step == 1 || step == 3
				if strings.Contains(body, "private-replay") != wantPrivate {
					t.Fatalf("step %d sent incorrect continuation: %s", step, body)
				}
			}
			if len(core.history[sid]) != 8 || core.history[sid][1].Continuation == nil {
				t.Fatal("restart or model switch discarded original execution history")
			}
		})
	}
}

func sendSSE(w http.ResponseWriter, value any) {
	data, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
func pinSessionView(t *testing.T, core *Engine, sessionID string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	events, err := core.Subscribe(ctx, sessionID)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	event, ok := <-events
	if !ok || event.Snapshot == nil {
		cancel()
		t.Fatal("session subscription closed before its initial snapshot")
	}
	return cancel
}

func waitIdle(t *testing.T, core *Engine, sessionIDs ...string) contracts.Snapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	events, err := core.Subscribe(ctx, sessionIDs...)
	if err != nil {
		t.Fatal(err)
	}
	var last contracts.Snapshot
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("engine subscription closed before the run settled: %+v", last.Runs)
			}
			if event.Snapshot == nil {
				continue
			}
			last = *event.Snapshot
			if len(last.Runs) == 0 || last.Runs[len(last.Runs)-1].Status == "running" {
				continue
			}
			// Re-read once after the completion event to synchronize with the producer
			// and preserve the latest state if it started a queued turn under the lock.
			last, err = core.Snapshot(ctx, sessionIDs...)
			if err != nil {
				t.Fatal(err)
			}
			if len(last.Runs) > 0 && last.Runs[len(last.Runs)-1].Status != "running" {
				return last
			}
		case <-ctx.Done():
			t.Fatalf("run did not finish: %v; runs=%+v sessions=%+v", ctx.Err(), last.Runs, last.Sessions)
		}
	}
}
