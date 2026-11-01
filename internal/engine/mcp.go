package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gcode/internal/agent"
	"gcode/internal/mcp"
	"gcode/internal/model"
)

func loadRunTools(ctx context.Context, root, cwd string, environment map[string]string) ([]agent.Tool, func(), []string, error) {
	tools := agent.CoreTools()
	var warnings []string
	var clients []*mcp.Client
	closeAll := func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}
	catalog, err := readResourceCatalog(root)
	if err != nil {
		return nil, closeAll, warnings, err
	}
	for _, config := range catalog.MCP {
		if config.Disabled {
			continue
		}
		client, err := connectMCP(ctx, mcpProcessEnvironment(config, environment), cwd)
		if err != nil {
			closeAll()
			return nil, func() {}, warnings, err
		}
		clients = append(clients, client)
		listCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		serverTools, err := client.Tools(listCtx)
		cancel()
		warnings = append(warnings, mcpDiscoveryWarnings(client)...)
		if err != nil {
			closeAll()
			return nil, func() {}, warnings, err
		}
		for _, tool := range serverTools {
			tools = append(tools, tool)
		}
	}
	return tools, closeAll, warnings, nil
}

func mcpDiscoveryWarnings(client *mcp.Client) []string {
	var warnings []string
	for _, diagnostic := range client.ToolDiagnostics() {
		warnings = append(warnings, "MCP tool skipped: "+diagnostic.Message)
	}
	return warnings
}

// The namespace is stable across providers; protocol filtering never renames it.
func filterMCPToolsForProtocol(tools []agent.Tool, protocol model.Protocol) ([]agent.Tool, []string) {
	limit := 128
	if protocol == model.Chat {
		limit = 64
	}
	filtered := make([]agent.Tool, 0, len(tools))
	var warnings []string
	for _, tool := range tools {
		name := tool.Definition().Name
		if strings.HasPrefix(name, "mcp__") && len(name) > limit {
			warnings = append(warnings, fmt.Sprintf("MCP tool %q skipped for %s: qualified name has %d characters, limit %d; other server tools remain available.", name, protocol, len(name), limit))
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered, warnings
}

// connectMCP selects the server's actual protocol era before exposing any tools.
// Discovery is read-only; business calls are never retried as negotiation.
func connectMCP(ctx context.Context, config mcp.Config, cwd string) (*mcp.Client, error) {
	return mcp.ConnectWithOptions(ctx, config, cwd, mcp.Options{ProtocolVersion: "auto"})
}
