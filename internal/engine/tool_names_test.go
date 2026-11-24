package engine

import (
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

// stubTasks 是会话后台任务的只读视图，和运行中的 runTasks 暴露同样的名单。
type stubTasks []contracts.BackgroundTask

func (s stubTasks) List() []contracts.BackgroundTask { return s }

func (s stubTasks) Stop(id string) error { return nil }

// toolRow 走一次 message.start + tool.start，返回工具行的展示投影。
func toolRow(t *testing.T, core *Engine, name, arguments string) *contracts.ToolCall {
	t.Helper()
	current := ""
	call := model.Call{ID: "call", Name: name, Arguments: arguments}
	for _, event := range []agent.Event{
		{Kind: "message.start"},
		{Kind: "tool.start", Call: call},
	} {
		if err := core.applyAgentEventLocked("session", "run", &current, event); err != nil {
			t.Fatal(err)
		}
	}
	messages := core.state.Messages["session"]
	last := messages[len(messages)-1]
	return last.Blocks[len(last.Blocks)-1].Tool
}

func TestToolRowNamesTheSubagentTaskInsteadOfItsID(t *testing.T) {
	core := &Engine{state: contracts.Snapshot{
		Messages: map[string][]contracts.Message{},
		Sessions: []contracts.Session{{ID: "task-check", ParentSessionID: "session", Title: "Verify controls"}},
	}}
	tool := toolRow(t, core, "TaskOutput", `{"task_id":"task-check","block":true}`)
	if tool.TaskID != "task-check" || tool.Summary != "Verify controls" {
		t.Fatalf("the row must name the task, not its id: %+v", tool)
	}
}

func TestToolRowNamesTheBackgroundCommandInsteadOfItsID(t *testing.T) {
	core := &Engine{state: contracts.Snapshot{Messages: map[string][]contracts.Message{}}}
	core.sessionTools = map[string]*sessionToolSet{
		"session": {tasks: stubTasks{{ID: "task-bg", SessionID: "session", Title: "Background check"}}},
	}
	tool := toolRow(t, core, "TaskOutput", `{"task_id":"task-bg","block":true}`)
	if tool.Summary != "Background check" {
		t.Fatalf("the row must name the background command: %+v", tool)
	}
}

func TestToolRowKeepsTheActionWhenTheTaskHasNoName(t *testing.T) {
	core := &Engine{state: contracts.Snapshot{Messages: map[string][]contracts.Message{}}}
	tool := toolRow(t, core, "TaskStop", `{"task_id":"task-missing"}`)
	if tool.TaskID != "task-missing" || tool.Summary != "" {
		t.Fatalf("an unknown task must not lend its id to the row: %+v", tool)
	}
}

func TestToolRowNamesTheWorkflowRunInsteadOfItsID(t *testing.T) {
	core := &Engine{state: contracts.Snapshot{
		Messages:     map[string][]contracts.Message{},
		WorkflowRuns: []contracts.WorkflowRun{{ID: "workflow-run-2", Name: "Review"}},
	}}
	tool := toolRow(t, core, "GetWorkflowRun", `{"run_id":"workflow-run-2"}`)
	if tool.Summary != "Review" {
		t.Fatalf("the row must name the run, not its id: %+v", tool)
	}
	unnamed := toolRow(t, core, "GetWorkflowRun", `{"run_id":"workflow-run-9"}`)
	if unnamed.Summary != "" {
		t.Fatalf("an unknown run must not lend its id to the row: %+v", unnamed)
	}
}
