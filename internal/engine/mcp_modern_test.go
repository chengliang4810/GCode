package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/mcp"
	"gcode/internal/model"
)

func modernMCPFixture(t *testing.T, plugin bool, discovered, called *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("modern server received a session HTTP method")
			w.WriteHeader(http.StatusBadRequest)
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
		meta, _ := request.Params["_meta"].(map[string]any)
		if r.Header.Get("Mcp-Method") != request.Method || meta["io.modelcontextprotocol/protocolVersion"] != mcp.ModernProtocolVersion {
			t.Errorf("production connection omitted stateless metadata: %s %v", request.Method, meta)
		}
		if plugin && r.Header.Get("X-Plugin-Values") != `["private-member-one","private-member-two"]` {
			t.Errorf("plugin configuration not available to actual MCP request: %q", r.Header.Get("X-Plugin-Values"))
		}
		var result any
		switch request.Method {
		case "server/discover":
			discovered.Add(1)
			result = map[string]any{"resultType": "complete", "supportedVersions": []string{mcp.ModernProtocolVersion}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			description := "Modern fixture echo"
			if plugin {
				description += " private-member-one"
			}
			result = map[string]any{"resultType": "complete", "tools": []any{map[string]any{"name": "echo", "description": description, "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			called.Add(1)
			text := "MODERN MCP BUSINESS RESULT"
			structured := map[string]any{"ok": true}
			if plugin {
				text += " private-member-one"
				structured["credential"] = "private-member-two"
			}
			result = map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": text}}, "structuredContent": structured}
		default:
			t.Errorf("modern-only server received unsupported method %s", request.Method)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32601, "message": "modern-only fixture does not initialize"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
}

func TestModernMCPProductionUserTestAndRuntimeConnection(t *testing.T) {
	var discovered, called atomic.Int32
	server := modernMCPFixture(t, false, &discovered, &called)
	defer server.Close()
	core, root := resourceCore(t)
	input := contracts.MCPServerInput{ID: "modern", Name: "modern", Transport: "http", URL: server.URL, Enabled: true}
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.test", ID: "modern"}); err != nil {
		t.Fatal(err)
	}
	state := waitResourceState(t, core, func(s contracts.Snapshot) bool {
		return len(s.UserResources.MCPServers) > 0 && s.UserResources.MCPServers[0].Test.State != "testing"
	})
	if state.UserResources.MCPServers[0].Test.State != "ready" {
		t.Fatalf("modern diagnostic failed: %+v", state.UserResources.MCPServers[0].Test)
	}
	tools, closeTools, _, err := loadRunTools(context.Background(), root, t.TempDir(), nil)
	defer closeTools()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools {
		if tool.Definition().Name == "mcp__modern__echo" {
			found = true
			out, err := tool.Execute(context.Background(), "", json.RawMessage(`{}`))
			if err != nil || !strings.Contains(out.Text, "MODERN MCP BUSINESS RESULT") {
				t.Fatalf("modern tool not executed: %+v %v", out, err)
			}
		}
	}
	if !found || discovered.Load() != 2 || called.Load() != 1 {
		t.Fatalf("product path did not discover/call modern server: found=%v discovery=%d calls=%d", found, discovered.Load(), called.Load())
	}
}

func TestModernPluginMCPProductionRunRedactsSensitiveListMembers(t *testing.T) {
	var discovered, called, modelCalls atomic.Int32
	server := modernMCPFixture(t, true, &discovered, &called)
	defer server.Close()
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		raw, _ := json.Marshal(body)
		for _, secret := range []string{"private-member-one", "private-member-two", "default-member-one", "default-member-two"} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("plugin member reached model history/schema: %s", secret)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if modelCalls.Add(1) == 1 {
			subagentWireCall(w, model.Chat, "mcp__plugin_sample_remote__echo", `{}`)
			return
		}
		if !strings.Contains(string(raw), "MODERN MCP BUSINESS RESULT") {
			t.Error("actual modern MCP business result absent")
		}
		subagentText(w, model.Chat, "Modern plugin completed")
	}))
	market := t.TempDir()
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"sample","userConfig":{"values":{"type":"string","title":"Private list","multiple":true,"sensitive":true,"default":["default-member-one","default-member-two"]}}}`)
	config, _ := json.Marshal(map[string]any{"remote": map[string]any{"type": "http", "url": server.URL, "headers": map[string]string{"X-Plugin-Values": "${user_config.values}"}}})
	writePluginFile(t, market, "sample/.mcp.json", string(config))
	installRuntimeFixture(t, core, market)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "plugins.configure", ID: "sample@fixture", Plugin: &contracts.PluginInput{Configuration: map[string]string{"values": `["private-member-one","private-member-two"]`}}}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "Exercise installed modern MCP plugin")
	state := waitSubagentState(t, core, runsFinished)
	if discovered.Load() != 1 || called.Load() != 1 || modelCalls.Load() != 2 {
		t.Fatalf("modern runtime not connected: discover=%d tools=%d models=%d runs=%+v", discovered.Load(), called.Load(), modelCalls.Load(), state.Runs)
	}
	raw, _ := json.Marshal(state)
	for _, secret := range []string{"private-member-one", "private-member-two", "default-member-one", "default-member-two"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("private plugin member reached public state: %s", secret)
		}
	}
}
