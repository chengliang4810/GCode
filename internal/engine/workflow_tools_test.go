package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

// workflowToolSet 取一个会话当前的工具集。
func workflowToolSet(t *testing.T, core *Engine) []agent.Tool {
	t.Helper()
	core.mu.Lock()
	session := core.state.Sessions[0]
	core.mu.Unlock()
	tools, err := core.sessionRuntimeTools(session, "")
	if err != nil {
		t.Fatal(err)
	}
	return tools
}

// toolData 取出工具结果的类型化载荷。
func toolData(t *testing.T, out model.ToolOutput) map[string]any {
	t.Helper()
	data, ok := out.Data.(map[string]any)
	if !ok {
		t.Fatalf("the tool returned %#v", out.Data)
	}
	return data
}

// hasWorkflowTool 报告工具集里有没有某个工具。
func hasWorkflowTool(tools []agent.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Definition().Name == name {
			return true
		}
	}
	return false
}

// activateWorkflowSession 打开会话的工作流能力。
func activateWorkflowSession(t *testing.T, core *Engine) {
	t.Helper()
	core.mu.Lock()
	defer core.mu.Unlock()
	core.state.Sessions[0].WorkflowActivated = true
}

// workflowConfirmation 等待一次运行确认出现。
func workflowConfirmation(t *testing.T, core *Engine) contracts.WorkflowConfirmation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		core.mu.Lock()
		pending := core.state.WorkflowConfirmations
		core.mu.Unlock()
		for _, confirmation := range pending {
			if confirmation.Status == contracts.WorkflowConfirmationPending {
				return confirmation
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the workflow confirmation never arrived")
	return contracts.WorkflowConfirmation{}
}

func TestWorkflowToolsFollowTheSessionMode(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	tools := workflowToolSet(t, core)
	if hasWorkflowTool(tools, "CreateWorkflow") {
		t.Fatal("the on-demand mode waits for the user before offering workflow tools")
	}
	activateWorkflowSession(t, core)
	tools = workflowToolSet(t, core)
	for _, name := range workflowToolNames {
		if !hasWorkflowTool(tools, name) {
			t.Fatalf("an activated session is missing the %s tool", name)
		}
	}
}

func TestWorkflowCreateReportsDiagnosticsWithoutStartingAnything(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	activateWorkflowSession(t, core)
	tools := workflowToolSet(t, core)
	out := callSessionTestTool(t, tools, "EvalWorkflowSnippet", map[string]any{"script": `{"phases":[{"id":"a","edges":[{"to":"nowhere"}]}]}`})
	if toolData(t, out)["ok"] != false {
		t.Fatalf("a broken script must not report success: %#v", out.Data)
	}
	diagnostics, _ := toolData(t, out)["diagnostics"].([]contracts.WorkflowDiagnostic)
	if len(diagnostics) == 0 {
		t.Fatal("the broken script reports diagnostics")
	}
	out = callSessionTestTool(t, tools, "CreateWorkflow", map[string]any{"script": `{"phases":[]}`})
	if toolData(t, out)["ok"] != false {
		t.Fatalf("a broken script must not report success: %#v", out.Data)
	}
	core.mu.Lock()
	runs := len(core.state.WorkflowRuns)
	core.mu.Unlock()
	if runs != 0 {
		t.Fatal("a broken script starts nothing")
	}
}

func TestWorkflowCreateWaitsForTheUserAndThenRuns(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	activateWorkflowSession(t, core)
	tools := workflowToolSet(t, core)
	create := sessionTestTool(t, tools, "CreateWorkflow")
	raw, err := json.Marshal(map[string]any{
		"script":   `{"name":"Two stations","phases":[{"id":"a","name":"A"},{"id":"b","name":"B"}]}`,
		"question": "Run this workflow?",
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan model.ToolOutput, 1)
	go func() {
		out, err := create.Execute(ctx, "", raw)
		if err != nil {
			t.Errorf("create workflow: %v", err)
		}
		results <- out
	}()
	confirmation := workflowConfirmation(t, core)
	if confirmation.Question != "Run this workflow?" {
		t.Fatalf("the confirmation keeps the question, got %q", confirmation.Question)
	}
	if confirmation.Name != "Two stations" {
		t.Fatalf("the confirmation keeps the name, got %q", confirmation.Name)
	}
	if len(confirmation.Program.Phases) != 2 {
		t.Fatalf("the confirmation carries the program, got %d phases", len(confirmation.Program.Phases))
	}
	core.mu.Lock()
	runs := len(core.state.WorkflowRuns)
	core.mu.Unlock()
	if runs != 0 {
		t.Fatal("nothing runs before the user decides")
	}
	if err := core.Execute(ctx, contracts.Command{
		Kind: "workflow.confirm",
		Workflow: &contracts.WorkflowCommand{Confirmation: &contracts.WorkflowConfirmationDecision{
			ID: confirmation.ID, Approve: true, Decision: contracts.WorkflowDecisionOnce,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-results:
		if toolData(t, out)["approved"] != true {
			t.Fatalf("the tool reports the approval: %#v", out.Data)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the tool did not return after the approval")
	}
	state := waitWorkflowState(t, core, func(state contracts.Snapshot) bool {
		return len(state.WorkflowRuns) == 1 && state.WorkflowRuns[0].Status == contracts.WorkflowStatusCompleted
	})
	if len(state.WorkflowRuns[0].Phases) != 2 {
		t.Fatalf("the run keeps both stations: %+v", state.WorkflowRuns[0].Phases)
	}
}

func TestWorkflowRejectionLeavesNoRun(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	activateWorkflowSession(t, core)
	tools := workflowToolSet(t, core)
	create := sessionTestTool(t, tools, "CreateWorkflow")
	raw, err := json.Marshal(map[string]any{"script": `{"name":"One","phases":[{"id":"a","name":"A"}]}`})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan model.ToolOutput, 1)
	go func() {
		out, err := create.Execute(ctx, "", raw)
		if err != nil {
			t.Errorf("create workflow: %v", err)
		}
		results <- out
	}()
	confirmation := workflowConfirmation(t, core)
	if err := core.Execute(ctx, contracts.Command{
		Kind: "workflow.confirm",
		Workflow: &contracts.WorkflowCommand{Confirmation: &contracts.WorkflowConfirmationDecision{
			ID: confirmation.ID, Approve: false, Decision: contracts.WorkflowDecisionReject, Feedback: "not this time",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-results:
		if toolData(t, out)["approved"] != false {
			t.Fatalf("the tool reports the rejection: %#v", out.Data)
		}
		if toolData(t, out)["feedback"] != "not this time" {
			t.Fatalf("the feedback goes back to the agent: %#v", out.Data)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the tool did not return after the rejection")
	}
	core.mu.Lock()
	runs := len(core.state.WorkflowRuns)
	core.mu.Unlock()
	if runs != 0 {
		t.Fatal("a rejected workflow leaves no run behind")
	}
}

func TestWorkflowSaveAndListDefinitions(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	activateWorkflowSession(t, core)
	tools := workflowToolSet(t, core)
	out := callSessionTestTool(t, tools, "SaveWorkflow", map[string]any{
		"name": "review-flow", "scope": "global", "description": "Review a change",
		"script": `{"name":"Review","phases":[{"id":"a","name":"A","instances":[{"id":"reader","name":"Reader"}]}]}`,
	})
	if toolData(t, out)["name"] != "review-flow" {
		t.Fatalf("the definition was saved: %#v", out.Data)
	}
	out = callSessionTestTool(t, tools, "ListSavedWorkflows", map[string]any{})
	definitions, _ := toolData(t, out)["workflows"].([]map[string]any)
	if len(definitions) != 1 || definitions[0]["name"] != "review-flow" {
		t.Fatalf("the saved definition is listed: %#v", out.Data)
	}
	if definitions[0]["phases"] != 1 {
		t.Fatalf("the definition reports its phase count: %#v", definitions[0])
	}
}

func TestWorkflowArgumentsAreValidatedAgainstTheDeclaration(t *testing.T) {
	if _, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "count", Type: "number"}}, map[string]string{"count": "many"}); err == nil {
		t.Fatal("a number argument must be a number")
	}
	if _, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "count", Type: "number"}}, map[string]string{"count": "Inf"}); err == nil {
		t.Fatal("a number argument must be finite")
	}
	if _, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "payload", Type: "json"}}, map[string]string{"payload": "{"}); err == nil {
		t.Fatal("a json argument must parse")
	}
	if _, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "flag", Type: "boolean"}}, map[string]string{"flag": "yes"}); err == nil {
		t.Fatal("a boolean argument must be true or false")
	}
	if _, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "target", Type: "string"}}, map[string]string{"other": "x"}); err == nil {
		t.Fatal("an unknown argument is rejected")
	}
	if _, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "target", Type: "string", Required: true}}, nil); err == nil {
		t.Fatal("a required argument must be provided")
	}
	values, err := workflowArgValues([]contracts.WorkflowArgument{{Name: "target", Type: "string"}, {Name: "count", Type: "number"}}, map[string]string{"target": "docs", "count": ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Name != "target" {
		t.Fatalf("an empty number is not passed: %#v", values)
	}
}
