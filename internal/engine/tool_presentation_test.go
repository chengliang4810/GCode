package engine

import (
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestToolPresentationDecodesReadableFieldsAndPreservesArguments(t *testing.T) {
	for _, test := range []struct {
		name, tool, args, command, summary string
	}{
		{"command", "Bash", `{"command":"go test ./...","timeout":60}`, "go test ./...", "Bash"},
		{"search", "Grep", `{"pattern":"func main","path":"src"}`, "", "func main · src"},
		{"Glob", "Glob", `{"pattern":"**/*.go"}`, "", "**/*.go"},
		{"directory", "ListDirectory", `{"path":"src"}`, "", "src"},
		{"invalid", "Bash", `{"command":`, "", "Bash"},
		{"todo", "TodoWrite", `{"todos":[{"text":"one","status":"completed"},{"text":"two","status":"in_progress"}]}`, "", "1/2"},
		{"task output", "TaskOutput", `{"task_id":"task-check","block":true}`, "", "task-check"},
		{"task stop", "TaskStop", `{"task_id":"task-check"}`, "", "task-check"},
		{"skill", "Skill", `{"skill":"review-code","args":"src"}`, "src", "review-code"},
		{"goal", "goal_read", `{}`, "", ""},
		{"agent", "Agent", `{"description":"Inspect layout","prompt":"Check the controls"}`, "Check the controls", "Inspect layout"},
		{"fetch", "WebFetch", `{"url":"https://example.test/docs"}`, "", "https://example.test/docs"},
		{"web", "WebSearch", `{"query":"Go documentation"}`, "", "Go documentation"},
		{"mcp", "mcp__files__read_text", `{"path":"README.md"}`, "", "files · read_text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := time.Now()
			call := model.Call{ID: "call", Name: test.tool, Arguments: test.args}
			got := toolPresentation(call, "/workspace/src", started)
			if got.Command != test.command || got.Summary != test.summary || got.Args != test.args {
				t.Fatalf("incorrect readable projection: %+v", got)
			}
			if got.ID != call.ID || got.Path != "/workspace/src" || got.StartedAt != started {
				t.Fatalf("lost call identity: %+v", got)
			}
		})
	}
}

func TestBuiltinToolInventoryHasReadableCategories(t *testing.T) {
	tools, closeTools := agent.RunToolSet(t.Context(), t.TempDir())
	defer closeTools()
	tools = append(tools, agent.NewWebSearchTool(nil))
	for _, tool := range tools {
		name := tool.Definition().Name
		if toolKind(name) == contracts.ToolUnknown {
			t.Errorf("built-in tool %s has no presentation category", name)
		}
	}
	for _, name := range []string{"Skill", "Agent", "goal_read"} {
		if toolKind(name) == contracts.ToolUnknown {
			t.Errorf("host tool %s has no presentation category", name)
		}
	}
}

func TestToolPresentationKeepsMCPDisplayNamesAndUpdatedInputs(t *testing.T) {
	core := &Engine{state: contracts.Snapshot{Messages: map[string][]contracts.Message{}}}
	call := model.Call{ID: "mcp-call", Name: "mcp__plugin_files_service__read_text", Arguments: `{"path":"first.md"}`}
	current := ""
	for _, event := range []agent.Event{
		{Kind: "message.start"},
		{Kind: "tool.start", Call: call, DisplayName: "文件服务 · 读取文本"},
		{Kind: "tool.input.changed", Call: model.Call{ID: call.ID, Name: call.Name, Arguments: `{"path":"second.md"}`}},
	} {
		if err := core.applyAgentEventLocked("session", "run", &current, event); err != nil {
			t.Fatal(err)
		}
	}
	got := core.state.Messages["session"][0].Blocks[0].Tool
	if got.Kind != contracts.ToolMCP || got.Summary != "文件服务 · 读取文本" ||
		got.Args != `{"path":"second.md"}` {
		t.Fatalf("hook input change lost display metadata: %+v", got)
	}
}

func TestGoalReadPresentationOwnsItsSnapshotAndEmptyGoal(t *testing.T) {
	budget := int64(1000)
	goal := &contracts.SessionGoal{Objective: "Verify tools", Status: contracts.GoalActive, TokenBudget: &budget}
	tool := toolPresentation(model.Call{Name: "goal_read", Arguments: `{}`}, "", time.Now())
	tool.Output = "raw goal protocol"
	goalReadPresentation(&tool, goal)
	goal.Objective, *goal.TokenBudget = "later goal", 2000
	clone := contracts.CloneBlocks([]contracts.Block{{Tool: &tool}})[0].Tool
	if tool.Summary != "Verify tools" || tool.Output != "" || tool.GoalSnapshot.Objective != "Verify tools" ||
		*clone.GoalSnapshot.TokenBudget != 1000 {
		t.Fatalf("goal result changed with live goal: %+v", tool)
	}
	clone.GoalSnapshot.Objective = "subscriber edit"
	if tool.GoalSnapshot.Objective != "Verify tools" {
		t.Fatal("goal result clone aliases the authoritative snapshot")
	}
	empty := toolPresentation(model.Call{Name: "goal_read", Arguments: `{}`}, "", time.Now())
	empty.Output = `{"goal":null}`
	goalReadPresentation(&empty, (*contracts.SessionGoal)(nil))
	if empty.GoalSnapshot != nil || empty.Output != "" {
		t.Fatal("empty goal still displays a protocol envelope")
	}
}

func TestTaskStopPresentationDistinguishesRequestedAndFinishedStops(t *testing.T) {
	for _, status := range []string{"stopping", "killed", "completed"} {
		t.Run(status, func(t *testing.T) {
			tool := toolPresentation(model.Call{Name: "TaskStop", Arguments: `{"task_id":"task-check"}`}, "", time.Now())
			tool.Output = "raw task protocol"
			taskStopPresentation(&tool, map[string]any{"status": status, "description": "Verify controls", "command": "go test ./..."})
			if tool.TaskStatus != status || tool.Summary != "Verify controls" || tool.Command != "go test ./..." || tool.Output != "" {
				t.Fatalf("lost stop result: %+v", tool)
			}
		})
	}
}

func TestTaskOutputPresentationFollowsCommandAndSubagentResults(t *testing.T) {
	for _, test := range []struct {
		name, status, output, description, command string
		nested                                     bool
		code                                       any
	}{
		{"completed command", "completed", "ok desktop\n", "Verify desktop controls", "go test ./internal/desktop", true, 0},
		{"failed command", "failed", "assertion failed\n", "", "go test ./...", true, float64(1)},
		{"running command", "running", "first line\n", "Compile desktop", "go build ./cmd/rcode", true, nil},
		{"stopped command", "killed", "", "Cancelled build", "go build ./...", true, 137},
		{"completed subagent", "completed", "Found the layout entry point.", "Investigate layout", "", false, nil},
		{"running subagent", "running", "", "", "", false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := &Engine{state: contracts.Snapshot{Messages: map[string][]contracts.Message{}}}
			call := model.Call{ID: "call", Name: "TaskOutput", Arguments: `{"task_id":"task-check","block":true}`}
			task := map[string]any{
				"task_id": "task-check", "status": test.status, "output": test.output,
				"description": test.description, "command": test.command, "exit_code": test.code,
			}
			data := task
			if test.nested {
				data = map[string]any{"retrieval_status": "success", "task": task}
			}
			current := ""
			for _, event := range []agent.Event{
				{Kind: "message.start"},
				{Kind: "tool.start", Call: call},
				{Kind: "tool.end", Call: call, Data: data, Output: "raw protocol envelope"},
			} {
				if err := core.applyAgentEventLocked("session", "run", &current, event); err != nil {
					t.Fatal(err)
				}
			}
			got := core.state.Messages["session"][0].Blocks[0].Tool
			wantSummary := "task-check"
			if test.command != "" {
				wantSummary = test.command
			}
			if test.description != "" {
				wantSummary = test.description
			}
			if got.Kind != contracts.ToolTaskOutput || got.TaskID != "task-check" ||
				got.TaskStatus != test.status || got.Output != test.output ||
				got.Summary != wantSummary || got.Command != test.command || got.Args != call.Arguments {
				t.Fatalf("lost task result projection: %+v", got)
			}
			if got.Status != "completed" || got.IsError {
				t.Fatal("task state was confused with retrieval state")
			}
			if (got.ExitCode != nil) != (test.code != nil) {
				t.Fatal("lost exit code presence")
			}
			if test.status == "failed" && !got.Failed() {
				t.Fatal("failed command result looks successful")
			}
		})
	}
}

func TestTaskOutputPresentationPreservesUnstructuredErrors(t *testing.T) {
	tool := toolPresentation(model.Call{Name: "TaskOutput", Arguments: `{"task_id":"expired"}`}, "", time.Now())
	tool.Output = "task not found"
	taskOutputPresentation(&tool, map[string]any{"error": "task not found"})
	if tool.Output != "task not found" || tool.TaskID != "expired" {
		t.Fatalf("lost task lookup error: %+v", tool)
	}
}

// A read reports the window it actually returned, so a row can state it
// without the file's text reaching the transcript.
func TestReadLineCountFollowsTheTypedResult(t *testing.T) {
	text := map[string]any{"type": "text", "file": map[string]any{"numLines": 80, "startLine": 1, "totalLines": 120}}
	rewritten := map[string]any{"type": "text", "file": map[string]any{"numLines": float64(3)}}
	for _, test := range []struct {
		name string
		data any
		want int
	}{
		{"typed text read", text, 80},
		{"hook rewrite", rewritten, 3},
		{"image read", map[string]any{"type": "image", "file": map[string]any{"numLines": 9}}, 0},
		{"replaced result", "plain text", 0},
		{"no result", nil, 0},
		{"negative window", map[string]any{"type": "text", "file": map[string]any{"numLines": -1}}, 0},
	} {
		if got := readLineCount(test.data); got != test.want {
			t.Fatalf("%s: readLineCount = %d, want %d", test.name, got, test.want)
		}
	}
}
