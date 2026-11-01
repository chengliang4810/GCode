package engine

import (
	"context"
	"encoding/json"
	"slices"

	"gcode/internal/agent"
	"gcode/internal/model"
)

// ToolGuard is an optional host execution boundary. It sees actual tool input
// after hook rewrites and can reject execution by returning an error.
// It may be invoked concurrently and receives a copy of the argument bytes.
// It does not sandbox a process or intercept arbitrary filesystem access.
type ToolGuard func(ctx context.Context, cwd, name string, args json.RawMessage) error

type guardedTool struct {
	inner agent.Tool
	guard ToolGuard
}

func (t guardedTool) Definition() model.Tool { return t.inner.Definition() }
func (t guardedTool) Concurrent() bool       { return t.inner.Concurrent() }
func (t guardedTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	if err := t.guard(ctx, cwd, t.inner.Definition().Name, slices.Clone(raw)); err != nil {
		return model.ToolOutput{}, err
	}
	return t.inner.Execute(ctx, cwd, raw)
}

func (e *Engine) guardTool(tool agent.Tool) agent.Tool {
	if e.toolGuard == nil {
		return tool
	}
	if _, guarded := tool.(guardedTool); guarded {
		return tool
	}
	return guardedTool{inner: tool, guard: e.toolGuard}
}

func (e *Engine) guardTools(tools []agent.Tool) []agent.Tool {
	if e.toolGuard == nil {
		return tools
	}
	result := make([]agent.Tool, len(tools))
	for i, tool := range tools {
		result[i] = e.guardTool(tool)
	}
	return result
}
