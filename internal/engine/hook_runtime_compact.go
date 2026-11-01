package engine

import (
	"context"
	"strings"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/mcp"
)

// Resource discovery and connections run on the compact worker, using the
// configuration/profile snapshots taken before launching it.
func (e *Engine) compactHookResources(ctx context.Context, session contracts.Session, root string, config map[string]map[string]string, profiles []contracts.SubagentProfile) (runtimeResources, []agent.Tool, func(), error) {
	resources := runtimeResources{}
	processEnv, err := e.processEnvironment(session.StorageRootID, session.ID)
	if err != nil {
		return resources, nil, func() {}, err
	}
	bundles, err := e.plugins.BundlesFor(session.CWD)
	if err != nil {
		return resources, nil, func() {}, err
	}
	resources, err = e.prepareRuntimeResources(root, session, bundles, config, profiles)
	if err != nil {
		return resources, nil, func() {}, err
	}
	userTools, closeUserTools, warnings, err := loadRunTools(ctx, root, session.CWD, processEnv)
	resources.warnings = append(resources.warnings, warnings...)
	if err != nil {
		closeUserTools()
		return resources, nil, func() {}, err
	}
	tools, err := e.sessionRuntimeTools(session, resources.envFile)
	if err != nil {
		closeUserTools()
		return resources, nil, func() {}, err
	}
	for _, tool := range userTools {
		if strings.HasPrefix(tool.Definition().Name, "mcp__") {
			tools = append(tools, tool)
		}
	}
	var clients []*mcp.Client
	cleanup := func() {
		for _, client := range clients {
			_ = client.Close()
		}
		closeUserTools()
	}
	for _, config := range resources.mcp {
		client, connectErr := connectMCP(ctx, mcpProcessEnvironment(config, processEnv), session.CWD)
		if connectErr != nil {
			continue
		}
		remote, connectErr := client.Tools(ctx)
		resources.warnings = append(resources.warnings, mcpDiscoveryWarnings(client)...)
		if connectErr != nil {
			_ = client.Close()
			continue
		}
		clients = append(clients, client)
		for _, tool := range remote {
			tools = append(tools, tool)
		}
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return resources, nil, func() {}, err
	}
	return resources, tools, cleanup, nil
}
