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
						sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write-1", "function": map[string]any{"name": "Write", "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
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
			core, err := Open(Options{Storage: store})
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
			state := waitIdle(t, core)
			state = waitForProjectFiles(t, core)
			if calls.Load() != 2 || len(state.Sessions) != 1 || state.Sessions[0].LastOutcome != "completed" {
				t.Fatalf("unfinished run: %+v", state.Runs)
			}
			sid := state.Sessions[0].ID
			for _, message := range state.Messages[sid] {
				if message.Role == "assistant" && message.ModelID != "custom/model" {
					t.Fatalf("request model was not recorded for usage statistics: %+v", message)
				}
			}
			data, err := os.ReadFile(filepath.Join(workspace, "answer.txt"))
			if err != nil || string(data) != "真实 Go 工具写入" {
				t.Fatalf("file write failed %s %v", data, err)
			}
			if len(core.history[sid]) != 4 || core.history[sid][2].CallID != "write-1" {
				t.Fatal("incomplete execution history")
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
			core, err = Open(Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			restored, _ := core.Snapshot(ctx)
			for _, message := range restored.Messages[sid] {
				if message.Role == "assistant" && message.ModelID != "custom/model" {
					t.Fatal("restart lost the recorded request model")
				}
			}
			if len(restored.Sessions) != 1 || len(core.history[sid]) != 4 || len(restored.Providers) != 1 {
				t.Fatal("restart lost runtime data")
			}
		})
	}
}

func TestRestartAndModelSwitchPreserveScopedContinuation(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Responses, model.Messages} {
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
				if protocol == model.Responses {
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
				core, err := Open(Options{Storage: store})
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
				state := waitIdle(t, core)
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
func waitIdle(t *testing.T, core *Engine) contracts.Snapshot {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		state, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Runs) > 0 && state.Runs[len(state.Runs)-1].Status != "running" {
			return state
		}
		select {
		case <-deadline:
			t.Fatal("run did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
