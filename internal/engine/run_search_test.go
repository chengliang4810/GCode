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

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestSearchToolsRoundTripThroughThreeProtocols(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				text := string(body)
				if !strings.Contains(text, `"name":"Glob"`) ||
					!strings.Contains(text, `"name":"Grep"`) || strings.Contains(text, `"name":"find"`) {
					t.Errorf("model received the wrong search tool registry: %s", text)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch requests.Add(1) {
				case 1:
					emitSearchCall(w, protocol, "glob-1", "Glob", `{"pattern":"src/**/*.go"}`)
				case 2:
					if !strings.Contains(text, "src/main.go") || !strings.Contains(text, "src/deep/other.go") {
						t.Errorf("model did not receive actual glob results: %s", text)
					}
					emitSearchCall(w, protocol, "grep-1", "Grep",
						`{"pattern":"SearchNeedle","glob":"src/**/*.go","output_mode":"content","head_limit":1}`)
				case 3:
					if !strings.Contains(text, "src/main.go:2:func SearchNeedle() {}") {
						t.Errorf("model did not receive actual grep content: %s", text)
					}
					emitSearchCompletion(w, protocol)
				default:
					t.Error("unexpected extra model step")
				}
			}))
			defer server.Close()
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "src", "deep"), 0700); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{
				"src/main.go":       "package demo\nfunc SearchNeedle() {}\n",
				"src/deep/other.go": "package deep\n", "outside.go": "SearchNeedle outside scope",
			} {
				if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			store, err := storage.Open(storage.Options{Home: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			ctx := context.Background()
			provider := model.Provider{
				ID: "p", Name: "Custom", Protocol: protocol, BaseURL: server.URL,
				Models: []model.Model{{ID: "m"}},
			}
			if err := core.Execute(ctx, contracts.Command{
				Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
			}); err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{
				Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: root, Name: "Search fixture"},
			}); err != nil {
				t.Fatal(err)
			}
			before, _ := core.Snapshot(ctx)
			if err := core.Execute(ctx, contracts.Command{
				Kind: "message.send",
				Send: &contracts.SendRequest{ProjectID: before.Projects[0].ID, ModelID: "p/m", Text: "search files"},
			}); err != nil {
				t.Fatal(err)
			}
			summary, err := core.Snapshot(ctx)
			if err != nil || len(summary.Sessions) != 1 {
				t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
			}
			sid := summary.Sessions[0].ID
			after := waitIdle(t, core, sid)
			after, err = core.Snapshot(ctx, sid)
			if err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 3 || after.Sessions[0].LastOutcome != "completed" ||
				len(core.history[sid]) != 6 {
				t.Fatalf("search round trip failed: %+v", after.Runs)
			}
			kinds := map[string]contracts.ToolKind{}
			for _, message := range after.Messages[sid] {
				for _, block := range message.Blocks {
					if block.Tool != nil {
						kinds[block.Tool.Name] = block.Tool.Kind
						if block.Tool.Status != "completed" || block.Tool.IsError {
							t.Fatalf("search tool failed: %+v", block.Tool)
						}
					}
				}
			}
			if kinds["Glob"] != contracts.ToolGlob || kinds["Grep"] != contracts.ToolSearch {
				t.Fatalf("search tool projection lost its kind: %+v", kinds)
			}
		})
	}
}

func TestWebSearchSurvivesSessionRuntimeToolSelection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled-%t", enabled), func(t *testing.T) {
			var calls atomic.Int32
			var searches atomic.Int32
			search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				searches.Add(1)
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request["query"] != "tool catalog" {
					t.Error("search did not receive the model query")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results":[{"title":"Catalog evidence","url":"https://example.test/catalog","content":"tool catalog verified"}]}`))
			}))
			defer search.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				found := 0
				for _, raw := range body["tools"].([]any) {
					if raw.(map[string]any)["function"].(map[string]any)["name"] == "WebSearch" {
						found++
					}
				}
				if (found == 1) != enabled || found > 1 {
					t.Errorf("enabled=%t but actual request contains %d WebSearch tools", enabled, found)
				}
				if calls.Add(1) == 1 && enabled {
					w.Header().Set("Content-Type", "text/event-stream")
					emitSearchCall(w, model.Chat, "web-search", "WebSearch", `{"query":"tool catalog"}`)
					return
				}
				if enabled {
					wire, _ := json.Marshal(body)
					if !strings.Contains(string(wire), "Catalog evidence") {
						t.Error("search result did not return to the model")
					}
				}
				subagentText(w, model.Chat, "Tool catalog checked.")
			}))
			defer server.Close()
			core, _ := subagentCore(t, server.URL, server.URL, model.Chat)
			state, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := state.Settings
			settings.General.WebSearch.Enabled = enabled
			settings.General.WebSearch.Endpoint = search.URL
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			startSubagentParent(t, core)
			state = waitSubagentState(t, core, func(s contracts.Snapshot) bool { return len(s.Runs) == 1 && runsFinished(s) })
			wantCalls, wantSearches := int32(1), int32(0)
			if enabled {
				wantCalls, wantSearches = 2, 1
			}
			if calls.Load() != wantCalls || searches.Load() != wantSearches || state.Sessions[0].LastOutcome != "completed" {
				t.Fatal("runtime tool catalog request did not complete")
			}
		})
	}
}

func emitSearchCall(w http.ResponseWriter, protocol model.Protocol, id, name, arguments string) {
	switch protocol {
	case model.Chat:
		sendSSE(w, map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": id, "function": map[string]any{"name": name, "arguments": arguments},
			}}}, "finish_reason": "tool_calls",
		}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	case model.Responses:
		sendSSE(w, map[string]any{
			"type": "response.completed", "response": map[string]any{
				"status": "completed", "output": []any{map[string]any{
					"type": "function_call", "call_id": id, "name": name, "arguments": arguments,
				}},
			},
		})
	case model.Messages:
		var input map[string]any
		_ = json.Unmarshal([]byte(arguments), &input)
		sendSSE(w, map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": input},
		})
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}

func emitSearchCompletion(w http.ResponseWriter, protocol model.Protocol) {
	switch protocol {
	case model.Chat:
		sendSSE(w, map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"content": "search verified"}, "finish_reason": "stop",
		}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	case model.Responses:
		sendSSE(w, map[string]any{
			"type": "response.completed", "response": map[string]any{
				"status": "completed", "output": []any{map[string]any{
					"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "search verified"}},
				}},
			},
		})
	case model.Messages:
		sendSSE(w, map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": "search verified"},
		})
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}
