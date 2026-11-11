package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestContextMemoryPersistsAcrossDisableResumeAndFork(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	if core.state.Settings.General.HierarchicalContext {
		t.Fatal("feature must default off")
	}
	settings := core.state.Settings
	settings.General.HierarchicalContext = true
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	id := core.state.Sessions[0].ID
	memory := agent.ContextMemory{Blocks: []agent.ContextBlock{{ID: "b_original", Tier: 1, Summary: "earlier work", Originals: []model.Message{
		{ContextRef: "m_original", Role: "tool", CallID: "original", Text: "精确原文 TOKEN_EXPIRED"},
	}}}}
	view := []model.Message{{Role: "user", Text: "task", UserInput: true}, {Role: "user", Summary: true, ContextRef: "b_original", Text: "earlier work"}}
	core.mu.Lock()
	err := core.replaceContextMemoryLocked(id, view, memory)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Turning the feature off stops management, not retention of the archive.
	settings.General.HierarchicalContext = false
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	err = core.ensureSessionDataLocked(id)
	got := agent.CloneContextMemory(core.contextMemories[id])
	restored := model.CloneMessages(core.context[id])
	core.mu.Unlock()
	if err != nil || core.state.Settings.General.HierarchicalContext || !reflect.DeepEqual(got, memory) || !reflect.DeepEqual(restored, view) {
		t.Fatal("resume did not retain disabled archive", err, got, restored)
	}
	settings = core.state.Settings
	settings.General.HierarchicalContext = true
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.fork", ID: id}); err != nil {
		t.Fatal(err)
	}
	child := core.state.Sessions[0].ID
	core.mu.Lock()
	defer core.mu.Unlock()
	if err := core.ensureSessionDataLocked(child); err != nil {
		t.Fatal(err)
	}
	if err := core.ensureSessionDataLocked(id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(core.contextMemories[child], memory) {
		t.Fatal("fork lost archive")
	}
	core.contextMemories[child].Blocks[0].Originals[0].Text = "child-only"
	if core.contextMemories[id].Blocks[0].Originals[0].Text == "child-only" {
		t.Fatal("fork aliases parent archive")
	}
}

func TestContextMemoryCommitFailureKeepsPreviousView(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	sessionID := core.state.Sessions[0].ID
	core.mu.Lock()
	defer core.mu.Unlock()
	if err := core.ensureSessionDataLocked(sessionID); err != nil {
		t.Fatal(err)
	}
	before := model.CloneMessages(core.context[sessionID])
	record := core.sessionRecords[sessionID]
	folder := filepath.Join(core.storage.Directory(), "sessions", sessionID)
	backup := folder + ".blocked"
	if err := os.Rename(folder, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(folder, []byte("blocked"), 0600); err != nil {
		_ = os.Rename(backup, folder)
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(folder); _ = os.Rename(backup, folder) }()
	err := core.replaceContextMemoryLocked(sessionID, []model.Message{{Role: "user", Text: "uncommitted"}}, agent.ContextMemory{NextNudge: 123})
	if err == nil || !reflect.DeepEqual(before, core.context[sessionID]) || core.contextMemories[sessionID].NextNudge != 0 || core.sessionRecords[sessionID] != record {
		t.Fatal("failed archive transaction changed committed state", err)
	}
}

// This exercises the real core and all three wire adapters against deterministic
// loopback providers. The fixture chooses references from actual tool output.
func TestContextMemoryProtocolToolLoop(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var steps atomic.Int32
			firstRef, lastRef := "", ""
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				raw, _ := json.Marshal(body)
				if !strings.Contains(string(raw), "ContextStatus") {
					t.Error("core did not expose memory tools")
				}
				step := steps.Add(1)
				name, args := "ContextStatus", map[string]any{}
				switch step {
				case 2:
					var find func(any)
					find = func(value any) {
						switch value := value.(type) {
						case string:
							if strings.Contains(value, "visible messages;") {
								for _, line := range strings.Split(value, "\n") {
									fields := strings.Fields(line)
									if len(fields) > 2 && fields[0] == "1" {
										firstRef = fields[1]
									}
									if len(fields) > 2 && fields[0] == "2" {
										lastRef = fields[1]
									}
								}
							}
						case []any:
							for _, item := range value {
								find(item)
							}
						case map[string]any:
							for _, item := range value {
								find(item)
							}
						}
					}
					find(body)
					if firstRef == "" || lastRef == "" {
						t.Error("status did not report selectable references")
					}
					name, args = "CompressContext", map[string]any{"start_ref": firstRef, "end_ref": lastRef, "summary": "Earlier logs reported TOKEN_EXPIRED. No fix was verified."}
				case 3:
					if !strings.Contains(string(raw), "Folded as b_") {
						t.Error("fold result missing")
					}
					name, args = "SearchContext", map[string]any{"query": "TOKEN_EXPIRED"}
				case 4:
					if !strings.Contains(string(raw), "Matches:") {
						t.Error("search result missing")
					}
					name, args = "DecompressContext", map[string]any{"ref": firstRef, "limit": 200}
				case 5:
					if !strings.Contains(string(raw), "Historical data") || !strings.Contains(string(raw), "精确错误 TOKEN_EXPIRED") {
						t.Error("archived original did not reach the model")
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if step < 5 {
					arguments, _ := json.Marshal(args)
					switch protocol {
					case model.Chat:
						sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprint("memory-", step), "function": map[string]any{"name": name, "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
						fmt.Fprint(w, "data: [DONE]\n\n")
					case model.Responses:
						sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": fmt.Sprint("memory-", step), "name": name, "arguments": string(arguments)}}}})
					case model.Messages:
						sendSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": fmt.Sprint("memory-", step), "name": name, "input": map[string]any{}}})
						sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(arguments)}})
						sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}})
						sendSSE(w, map[string]any{"type": "message_stop"})
					}
					return
				}
				switch protocol {
				case model.Chat:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				case model.Responses:
					sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "done"}}}}}})
				case model.Messages:
					sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "done"}})
					sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
					sendSSE(w, map[string]any{"type": "message_stop"})
				}
			}))
			defer fixture.Close()
			home := t.TempDir()
			core := openStorageTestCore(t, home)
			defer core.Close()
			settings := core.state.Settings
			settings.General.HierarchicalContext = true
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			provider := model.Provider{ID: "memory", Name: "Fixture", Protocol: protocol, BaseURL: fixture.URL, Models: []model.Model{{ID: "model", ContextWindow: 200000, MaxOutput: 2000}}}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
				t.Fatal(err)
			}
			sessionID := core.state.Sessions[0].ID
			for range 20 {
				storeTestMessage(t, core, sessionID, "精确错误 TOKEN_EXPIRED\n"+strings.Repeat("original log ", 100))
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, ModelID: "memory/model", Text: "Continue inspecting the old error."}}); err != nil {
				t.Fatal(err)
			}
			snapshot := waitIdle(t, core, sessionID)
			if steps.Load() != 5 || snapshot.Sessions[0].LastOutcome != "completed" {
				t.Fatalf("memory tool run failed: steps=%d runs=%+v", steps.Load(), snapshot.Runs)
			}
			core.mu.Lock()
			err := core.ensureSessionDataLocked(sessionID)
			blocks := len(core.contextMemories[sessionID].Blocks)
			core.mu.Unlock()
			if err != nil || blocks != 1 {
				t.Fatal("real core did not persist its archive", blocks, err)
			}
		})
	}
}
