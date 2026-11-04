package contracts

import (
	"fmt"
	"strings"
)

type ToolKind string

const (
	ToolExec       ToolKind = "exec"
	ToolReadFile   ToolKind = "readFile"
	ToolWriteFile  ToolKind = "writeFile"
	ToolEditFile   ToolKind = "editFile"
	ToolApplyPatch ToolKind = "applyPatch"
	ToolSearch     ToolKind = "search"
	ToolGlob       ToolKind = "glob"
	ToolWebFetch   ToolKind = "webFetch"
	ToolWebSearch  ToolKind = "webSearch"
	ToolTodo       ToolKind = "todo"
	ToolMCP        ToolKind = "mcp"
	ToolUnknown    ToolKind = "unknown"
)

// Category keeps earlier saved tools, whose Kind was absent, readable.
func (t ToolCall) Category() ToolKind {
	if t.Kind != "" {
		return t.Kind
	}
	switch strings.ToLower(t.Name) {
	case "bash", "exec", "command", "terminal", "shell", "run":
		return ToolExec
	case "read", "readfile", "read_file":
		return ToolReadFile
	case "write", "writefile", "write_file":
		return ToolWriteFile
	case "edit", "editfile", "edit_file":
		return ToolEditFile
	case "applypatch", "apply_patch", "patch":
		return ToolApplyPatch
	case "grep", "search", "rg":
		return ToolSearch
	case "glob", "findfiles":
		return ToolGlob
	case "webfetch", "web_fetch":
		return ToolWebFetch
	case "websearch", "web_search":
		return ToolWebSearch
	case "todowrite", "todo":
		return ToolTodo
	}
	return ToolUnknown
}

func (t ToolCall) Failed() bool {
	return t.IsError || t.Status == "failed" || t.Status == "error" || (t.ExitCode != nil && *t.ExitCode != 0)
}

func (t ToolCall) IsQuestion() bool {
	return t.Name == "AskUserQuestion"
}

// ActivitySummary is derived from actual ordered activity, never fixture labels.
// EditedFiles deduplicates paths across write/edit/apply-patch operations.
type ActivitySummary struct {
	ToolCalls   int `json:"toolCalls"`
	Commands    int `json:"commands"`
	EditedFiles int `json:"editedFiles"`
	Reads       int `json:"reads"`
	Searches    int `json:"searches"`
	Fetches     int `json:"fetches"`
	Todos       int `json:"todos"`
	Other       int `json:"other"`
	Failed      int `json:"failed"`
	Thoughts    int `json:"thoughts"`
	Notes       int `json:"notes"`
	Subagents   int `json:"subagents"`
}

func SummarizeActivity(blocks []Block) ActivitySummary {
	var summary ActivitySummary
	tools := make(map[string]ToolCall)
	order := []string{}
	seen := make(map[string]bool)
	for i, block := range blocks {
		if block.Tool != nil {
			if block.Tool.IsQuestion() {
				continue
			}
			id := block.Tool.ID
			if id == "" {
				id = fmt.Sprintf("block:%s:%d", block.ID, i)
			}
			previous, exists := tools[id]
			if !exists {
				order = append(order, id)
			}
			current := *block.Tool
			if exists {
				if current.Kind == "" {
					current.Kind = previous.Kind
				}
				if current.Name == "" {
					current.Name = previous.Name
				}
				if current.Path == "" {
					current.Path = previous.Path
				}
				if current.Diff == nil {
					current.Diff = previous.Diff
				}
			}
			tools[id] = current
			continue
		}
		if block.ID != "" && seen[block.ID] {
			continue
		}
		seen[block.ID] = true
		switch block.Kind {
		case "reasoning":
			if strings.TrimSpace(block.Text) != "" {
				summary.Thoughts++
			}
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				summary.Notes++
			}
		case "subagent":
			if block.Subagent != nil {
				summary.Subagents++
				summary.ToolCalls++
				summary.Other++
			}
		}
	}
	edited := make(map[string]bool)
	for _, id := range order {
		tool := tools[id]
		summary.ToolCalls++
		if tool.Failed() {
			summary.Failed++
		}
		switch tool.Category() {
		case ToolExec:
			summary.Commands++
		case ToolWriteFile, ToolEditFile, ToolApplyPatch:
			path := tool.Path
			if path == "" && tool.Diff != nil {
				path = tool.Diff.Path
			}
			if path == "" {
				path = "patch"
			}
			edited[path] = true
		case ToolReadFile:
			summary.Reads++
		case ToolSearch, ToolGlob, ToolWebSearch:
			summary.Searches++
		case ToolWebFetch:
			summary.Fetches++
		case ToolTodo:
			summary.Todos++
		default:
			summary.Other++
		}
	}
	summary.EditedFiles = len(edited)
	return summary
}

// TurnActivity returns the working prefix of an assistant turn; trailing reply
// text remains outside the compact activity group, as in the source transcript.
func TurnActivity(messages []Message, turnID string) []Block {
	blocks := []Block{}
	last := -1
	for _, message := range messages {
		if message.Role != "assistant" || message.TurnID != turnID {
			continue
		}
		for _, block := range message.Blocks {
			blocks = append(blocks, block)
			switch block.Kind {
			case "reasoning", "tool", "toolCall", "toolResult", "todo", "todos", "subagent", "permission", "question":
				last = len(blocks) - 1
			}
		}
	}
	return blocks[:last+1]
}

func (s ActivitySummary) Label() string {
	segments := []string{}
	plural := func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, one)
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	if s.Thoughts == 1 {
		segments = append(segments, "thought process")
	} else if s.Thoughts > 1 {
		segments = append(segments, fmt.Sprintf("thought %d times", s.Thoughts))
	}
	if s.Notes == 1 {
		segments = append(segments, "1 progress update")
	} else if s.Notes > 1 {
		segments = append(segments, fmt.Sprintf("%d progress updates", s.Notes))
	}
	baseStart := len(segments)
	if s.Commands > 0 {
		segments = append(segments, "ran "+plural(s.Commands, "command", "commands"))
	}
	if s.EditedFiles > 0 {
		segments = append(segments, "edited "+plural(s.EditedFiles, "file", "files"))
	}
	if s.Reads > 0 {
		segments = append(segments, "read "+plural(s.Reads, "file", "files"))
	}
	if s.Searches > 0 {
		segments = append(segments, "searched "+plural(s.Searches, "time", "times"))
	}
	if s.Fetches > 0 {
		segments = append(segments, "fetched "+plural(s.Fetches, "page", "pages"))
	}
	if s.Todos > 0 {
		segments = append(segments, "updated todos")
	}
	if s.Other > 0 {
		segments = append(segments, "called "+plural(s.Other, "tool", "tools"))
	}
	if s.Failed > 0 {
		segments = append(segments, fmt.Sprintf("%d failed", s.Failed))
	}
	if baseStart < len(segments) && segments[baseStart] != "" {
		segments[baseStart] = strings.ToUpper(segments[baseStart][:1]) + segments[baseStart][1:]
	}
	label := strings.Join(segments, " · ")
	if label != "" {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	return label
}
