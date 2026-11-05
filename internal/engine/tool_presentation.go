package engine

import (
	"encoding/json"
	"strings"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

// Keep original arguments for inspection and project readable fields once at
// the core event boundary, before publishing them to either local or remote UI.
func toolPresentation(call model.Call, path string, started time.Time) contracts.ToolCall {
	tool := contracts.ToolCall{
		ID: call.ID, Kind: toolKind(call.Name), Name: call.Name,
		Args: call.Arguments, Path: path, Status: "running", StartedAt: started,
	}
	var args struct {
		Command     string `json:"command"`
		Pattern     string `json:"pattern"`
		Path        string `json:"path"`
		FilePath    string `json:"file_path"`
		Description string `json:"description"`
		Query       string `json:"query"`
		URL         string `json:"url"`
	}
	if json.Unmarshal([]byte(call.Arguments), &args) != nil {
		tool.Summary = call.Name
		return tool
	}
	tool.Command = args.Command
	if tool.Path == "" {
		tool.Path = args.FilePath
	}
	switch tool.Kind {
	case contracts.ToolTodo:
		if update, err := agent.ParseTodoUpdate(json.RawMessage(call.Arguments)); err == nil {
			tool.Summary = todoProgress(update)
		} else {
			tool.Summary = call.Name
		}
	case contracts.ToolSearch:
		tool.Summary = args.Pattern
		if args.Path != "" {
			tool.Summary += " · " + args.Path
		}
	case contracts.ToolGlob:
		tool.Summary = args.Pattern
		if tool.Summary == "" {
			tool.Summary = args.Path
		}
	case contracts.ToolWebFetch:
		tool.Summary = args.URL
	case contracts.ToolWebSearch:
		tool.Summary = args.Query
	default:
		tool.Summary = strings.TrimSpace(call.Name)
		if tool.Kind == contracts.ToolExec && args.Description != "" {
			tool.Summary = args.Description
		}
	}
	return tool
}

// readLineCount reads the text window a Read actually returned. Image reads
// and rewrites that replaced the typed result report none.
func readLineCount(data any) int {
	result, ok := data.(map[string]any)
	if !ok || result["type"] != "text" {
		return 0
	}
	file, ok := result["file"].(map[string]any)
	if !ok {
		return 0
	}
	// A hook rewrite arrives as decoded JSON, where every number is float64.
	switch lines := file["numLines"].(type) {
	case int:
		return max(lines, 0)
	case float64:
		return max(int(lines), 0)
	}
	return 0
}
