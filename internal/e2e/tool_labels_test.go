//go:build e2e

package e2e

import (
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
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Execute real local tools, persist their projections, then reopen that history
// in a private native window. Only model, page, search and MCP servers are local fixtures.
func TestDesktopLocalSemanticToolLabels(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			skillPath := filepath.Join(workspace, ".claude", "skills", "fixture-review", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(skillPath), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(skillPath, []byte("---\nname: fixture-review\ndescription: Local review fixture\n---\nReview the local controls."), 0600); err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/page":
					fmt.Fprint(w, "<h1>Local documentation</h1><p>Page fixture output.</p>")
					return
				case "/search":
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"results":[{"title":"Local result","url":"https://example.test/docs","content":"Search fixture output."}]}`)
					return
				case "/mcp":
					if r.Method == http.MethodDelete {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					var request struct {
						ID     any    `json:"id"`
						Method string `json:"method"`
					}
					_ = json.NewDecoder(r.Body).Decode(&request)
					var result any
					switch request.Method {
					case "server/discover":
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{
							"jsonrpc": "2.0", "id": request.ID,
							"error": map[string]any{"code": -32601, "message": "Method not found"},
						})
						return
					case "initialize":
						result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}}
					case "notifications/initialized":
						w.WriteHeader(http.StatusAccepted)
						return
					case "tools/list":
						result = map[string]any{"tools": []any{map[string]any{
							"name": "read_text", "title": "读取文本", "inputSchema": map[string]any{"type": "object"},
						}}}
					case "tools/call":
						result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "MCP fixture output."}}}
					default:
						t.Errorf("unexpected MCP method %s", request.Method)
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
					return
				}
				var body struct {
					Messages []struct {
						Role    string `json:"role"`
						Content any    `json:"content"`
					} `json:"messages"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				taskID := ""
				for _, message := range body.Messages {
					if text, ok := message.Content.(string); ok && strings.HasPrefix(text, "Background task started: ") {
						taskID = strings.TrimSpace(strings.TrimPrefix(text, "Background task started: "))
					}
				}
				base := "http://" + r.Host
				calls := []model.Call{
					{ID: "background", Name: "Bash", Arguments: `{"command":"sleep 30","description":"后台校验","run_in_background":true}`},
					{ID: "output", Name: "TaskOutput", Arguments: fmt.Sprintf(`{"task_id":%q,"block":false}`, taskID)},
					{ID: "stop", Name: "TaskStop", Arguments: fmt.Sprintf(`{"task_id":%q}`, taskID)},
					{ID: "goal", Name: "goal_read", Arguments: `{}`},
					{ID: "directory", Name: "ListDirectory", Arguments: `{"path":"."}`},
					{ID: "page", Name: "WebFetch", Arguments: fmt.Sprintf(`{"url":%q}`, base+"/page")},
					{ID: "skill", Name: "Skill", Arguments: `{"skill":"fixture-review","args":"src"}`},
					{ID: "search", Name: "WebSearch", Arguments: `{"query":"local documentation"}`},
					{ID: "mcp", Name: "mcp__files__read_text", Arguments: `{}`},
					{ID: "agent", Name: "Agent", Arguments: `{"description":"调查界面","prompt":"Inspect controls","subagent_type":"missing-fixture-agent"}`},
				}
				index := int(requests.Add(1)) - 1
				delta := map[string]any{"content": "工具展示校验完成"}
				finish := "stop"
				if index < len(calls) {
					call := calls[index]
					delta = map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": call.ID, "function": map[string]any{"name": call.Name, "arguments": call.Arguments},
					}}}
					finish = "tool_calls"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				wire, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", wire)
			}))
			defer server.Close()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.General.CompactMode = false
			settings.Appearance.ReduceMotion = true
			settings.General.WebSearch.Enabled, settings.General.WebSearch.Endpoint = true, server.URL
			provider := model.Provider{ID: "labels", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Tool labels"}},
				{Kind: "resource.mcp.save", Resource: &contracts.ResourceInput{MCP: &contracts.MCPServerInput{
					ID: "files", Name: "files", Transport: "http", URL: server.URL + "/mcp", Enabled: true,
				}}},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: "tool-label-seed", ProjectID: snapshot.Projects[0].ID, ModelID: "labels/test", Text: "核对工具名称",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(60 * time.Second)
			for {
				snapshot, err = core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle && snapshot.Sessions[0].LastOutcome == "completed" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("real semantic tool fixture did not finish")
				}
				time.Sleep(20 * time.Millisecond)
			}
			sid := snapshot.Sessions[0].ID
			snapshot, _ = core.Snapshot(t.Context(), sid)
			groupID := ""
			tools := map[string]*contracts.ToolCall{}
			for _, message := range snapshot.Messages[sid] {
				for _, block := range message.Blocks {
					if block.Tool != nil {
						tools[block.Tool.Name] = block.Tool
						if groupID == "" {
							groupID = message.ID + ":activity:" + block.ID
						}
					}
				}
			}
			if requests.Load() != 11 || len(tools) != 10 || tools["mcp__files__read_text"].Summary != "files · 读取文本" {
				t.Fatalf("real tool projections were not retained: requests=%d tools=%v", requests.Load(), tools)
			}
			for name, tool := range tools {
				if name != "Agent" && (tool.IsError || tool.Status != "completed") {
					t.Fatalf("real tool %s did not complete: %+v", name, tool)
				}
			}
			if !tools["Agent"].IsError || tools["Agent"].Kind != contracts.ToolAgent {
				t.Fatal("invalid subagent did not retain the semantic failure record")
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "labels", data, "--local", "--appearance", appearance,
				"--language", "zh-CN", "--width", "900", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sid, "press", "")
			app.act("activity-toggle:"+groupID, "press", "")
			app.await("semantic tool names and summaries reach native history", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, text := range []string{"读取任务结果: 后台校验", "停止任务: 后台校验", "读取目标:", "列出目录: .",
					"读取网页:", "使用技能: fixture-review", "搜索网页: local documentation", "调用工具: files · 读取文本", "启动智能体: 调查界面"} {
					if !activityTreeContains(tree, text) {
						return fmt.Errorf("missing native semantic label %q", text)
					}
				}
				return nil
			})
			app.capture("semantic-tool-labels")
			for _, name := range []string{"TaskStop", "goal_read", "Skill", "mcp__files__read_text"} {
				tool := tools[name]
				toggle := "activity-item-toggle:" + groupID + ":" + tool.ID
				app.act(toggle, "press", "")
				app.waitKey("activity-detail-viewport:" + groupID + ":" + tool.ID)
				app.capture("detail-" + name)
				app.act(toggle, "press", "")
			}
		})
	}
}
