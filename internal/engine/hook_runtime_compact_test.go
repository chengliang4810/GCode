package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
)

func TestHookRuntimeCompactionConnectsModernPluginMCP(t *testing.T) {
	var discovered, called atomic.Int32
	server := modernMCPFixture(t, false, &discovered, &called)
	defer server.Close()
	core, _ := resourceCore(t)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := state.Sessions[0]
	session.CWD = t.TempDir()
	market := t.TempDir()
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"sample"}`)
	config, err := json.Marshal(map[string]any{"remote": map[string]any{"type": "http", "url": server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	writePluginFile(t, market, "sample/.mcp.json", string(config))
	installRuntimeFixture(t, core, market)
	_, tools, cleanup, err := core.compactHookResources(context.Background(), session, core.storage.ConfigDirectory(), nil, nil)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools {
		if tool.Definition().Name != "mcp__plugin_sample_remote__echo" {
			continue
		}
		found = true
		output, err := tool.Execute(context.Background(), session.CWD, json.RawMessage(`{}`))
		if err != nil || !strings.Contains(output.Text, "MODERN MCP BUSINESS RESULT") {
			t.Fatalf("compaction MCP tool failed: %+v %v", output, err)
		}
	}
	if !found || discovered.Load() != 1 || called.Load() != 1 {
		t.Fatalf("compaction skipped modern connection: found=%v discovery=%d calls=%d", found, discovered.Load(), called.Load())
	}
}
