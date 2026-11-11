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
		TaskID      string `json:"task_id"`
		Skill       string `json:"skill"`
		SkillArgs   string `json:"args"`
		Prompt      string `json:"prompt"`
		AgentType   string `json:"subagent_type"`
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
	case contracts.ToolTaskOutput, contracts.ToolTaskStop:
		tool.TaskID, tool.Summary = args.TaskID, args.TaskID
	case contracts.ToolSkill:
		tool.Summary, tool.Command = args.Skill, args.SkillArgs
	case contracts.ToolAgent:
		tool.Summary, tool.Command = args.Description, args.Prompt
		if tool.Summary == "" {
			tool.Summary = args.AgentType
		}
	case contracts.ToolGoalRead:
		tool.Summary = ""
	case contracts.ToolDirectory:
		tool.Summary = args.Path
		if tool.Path == "" {
			tool.Path = args.Path
		}
	case contracts.ToolMCP:
		service, operation, _ := strings.Cut(strings.TrimPrefix(call.Name, "mcp__"), "__")
		tool.Summary = service + " · " + operation
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

func taskStopPresentation(tool *contracts.ToolCall, data any) {
	result, ok := data.(map[string]any)
	if !ok {
		return
	}
	status, ok := result["status"].(string)
	if !ok {
		return
	}
	tool.TaskStatus = status
	tool.Command, _ = result["command"].(string)
	description, _ := result["description"].(string)
	if description != "" {
		tool.Summary = description
	}
	tool.Output = ""
}

func goalReadPresentation(tool *contracts.ToolCall, data any) {
	goal, ok := data.(*contracts.SessionGoal)
	if !ok {
		return
	}
	tool.GoalSnapshot = goal.Clone()
	if goal != nil {
		tool.Summary = goal.Objective
	}
	tool.Output = ""
}

// TaskOutput has two current result shapes: an owned command wraps its task,
// while a subagent returns the task fields directly. Project either at the
// event boundary so clients display the result instead of a protocol envelope.
func taskOutputPresentation(tool *contracts.ToolCall, data any) {
	result, ok := data.(map[string]any)
	if !ok {
		return
	}
	if task, ok := result["task"].(map[string]any); ok {
		result = task
	}
	output, ok := result["output"].(string)
	if !ok {
		return
	}
	tool.Output = output
	if id, ok := result["task_id"].(string); ok && id != "" {
		tool.TaskID = id
	}
	tool.TaskStatus, _ = result["status"].(string)
	tool.Command, _ = result["command"].(string)
	description, _ := result["description"].(string)
	tool.Summary = tool.TaskID
	if strings.TrimSpace(tool.Command) != "" {
		tool.Summary = tool.Command
	}
	if strings.TrimSpace(description) != "" {
		tool.Summary = description
	}
	switch code := result["exit_code"].(type) {
	case int:
		tool.ExitCode = &code
	case float64:
		value := int(code)
		tool.ExitCode = &value
	}
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
