package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/model"
)

func TestPluginMCPToolNameLimitsFollowModelProtocolWithoutRenaming(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Messages, model.Responses, model.Chat} {
		t.Run(string(protocol), func(t *testing.T) {
			longTool := strings.Repeat("l", 70)
			overTool := strings.Repeat("z", 140)
			prefix := "mcp__plugin_sample_remote__"
			selected := longTool
			if protocol == model.Chat {
				selected = "short"
			}
			var called, modelCalls atomic.Int32
			mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				var request struct {
					ID     uint64         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				var result any
				switch request.Method {
				case "server/discover":
					result = map[string]any{"resultType": "complete", "supportedVersions": []string{"2026-07-28"}, "capabilities": map[string]any{"tools": map[string]any{}}}
				case "tools/list":
					var tools []any
					for _, name := range []string{overTool, longTool, "short"} {
						tools = append(tools, map[string]any{"name": name, "inputSchema": map[string]any{"type": "object"}})
					}
					result = map[string]any{"resultType": "complete", "tools": tools}
				case "tools/call":
					called.Add(1)
					if request.Params["name"] != selected {
						t.Errorf("tool name was renamed or wrongly selected: %v", request.Params["name"])
					}
					result = map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": "LONG NAME MCP EXECUTED"}}}
				default:
					t.Errorf("unexpected MCP method: %s", request.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			}))
			defer mcpServer.Close()
			core, _ := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if modelCalls.Add(1) == 1 {
					var names []string
					for _, raw := range body["tools"].([]any) {
						tool := raw.(map[string]any)
						if protocol == model.Chat {
							tool = tool["function"].(map[string]any)
						}
						name, _ := tool["name"].(string)
						names = append(names, name)
					}
					has := func(want string) bool {
						for _, name := range names {
							if name == want {
								return true
							}
						}
						return false
					}
					if !has(prefix+"short") || has(prefix+overTool) || has(prefix+longTool) != (protocol != model.Chat) {
						t.Errorf("incorrect protocol tool catalog: %v", names)
					}
					raw, _ := json.Marshal(body)
					if !strings.Contains(string(raw), "namespace limit of 128") {
						t.Error("worker was not told why the overlong tool was skipped")
					}
					if protocol == model.Chat && !strings.Contains(string(raw), "limit 64") {
						t.Error("worker was not told about Chat's tool name limit")
					}
					subagentWireCall(w, protocol, prefix+selected, `{}`)
					return
				}
				raw, _ := json.Marshal(body)
				if !strings.Contains(string(raw), "LONG NAME MCP EXECUTED") {
					t.Error("actual MCP result did not return to the model")
				}
				subagentText(w, protocol, "Completed")
			}))
			market := t.TempDir()
			writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
			writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"sample"}`)
			config, _ := json.Marshal(map[string]any{"remote": map[string]any{"type": "http", "url": mcpServer.URL}})
			writePluginFile(t, market, "sample/.mcp.json", string(config))
			installRuntimeFixture(t, core, market)
			sendGoal(t, core, "Use a compatible MCP tool")
			state := waitSubagentState(t, core, runsFinished)
			if called.Load() != 1 || modelCalls.Load() != 2 {
				t.Fatalf("tool filtering interrupted session: MCP=%d model=%d runs=%+v", called.Load(), modelCalls.Load(), state.Runs)
			}
		})
	}
}
