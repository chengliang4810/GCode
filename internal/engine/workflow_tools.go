package engine

// 工作流模型工具。脚本是程序，工具只负责三件事：把脚本编译成程序、
// 让用户确认、把运行交给宿主。运行状态始终留在引擎里，工具只读它。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/workflow"
)

// workflowToolNames 是工作流工具的名字。模式允许时它们一起出现，
// 不允许时一个都不出现：禁用的能力不应该出现在工具目录里。
var workflowToolNames = []string{
	"CreateWorkflow", "AmendWorkflow", "SaveWorkflow", "ListSavedWorkflows", "EvalWorkflowSnippet",
	"ListWorkflowRuns", "GetWorkflowRun", "ResumeWorkflowRun", "ResolveWorkflowQuestion", "FillWorkflowHole",
}

// workflowTool 是工作流工具的外壳。每个实例只负责一个名字，
// 引擎继承工具调用所在的会话。
type workflowTool struct {
	engine    *Engine
	sessionID string
	name      string
}

// workflowSessionTools 按会话模式装配工作流工具。禁用模式不提供任何工作流工具，
// 按需模式要等用户先激活；激活之后压缩上下文也不会撤销。
func (e *Engine) workflowSessionTools(session contracts.Session, base []agent.Tool) []agent.Tool {
	// 只有主会话能创建和修订工作流；子智能体与实例会话都拿不到这些工具。
	if session.ParentSessionID != "" || session.ParentRunID != "" || session.SubagentType != "" {
		return base
	}
	mode, activated := e.workflowSessionMode(session.ID)
	if !workflow.Available(mode, activated) {
		return base
	}
	tools := append([]agent.Tool{}, base...)
	for _, name := range workflowToolNames {
		tools = append(tools, workflowTool{engine: e, sessionID: session.ID, name: name})
	}
	return tools
}

func (t workflowTool) Definition() model.Tool {
	text := map[string]any{"type": "string"}
	switch t.name {
	case "CreateWorkflow":
		return model.Tool{
			Name:        t.name,
			Description: "Compile a workflow script and start it in the background. Compilation and argument validation are built in; an unchanged script does not need a separate preflight.",
			Parameters: workflowSchema(map[string]any{
				"script":      map[string]any{"type": "string", "description": `JSON text, e.g. {"arguments":[{"name":"target","type":"string"}],"phases":[{"id":"plan","name":"Plan","instances":[{"id":"review","prompt":"Inspect the change"}]}]}. Put optional edges inside their source phase, not at the root. IDs and argument names use ASCII letters, digits, underscores or dashes.`},
				"name":        text,
				"arguments":   map[string]any{"type": "object", "additionalProperties": text, "description": `Runtime values keyed by names declared in the script, e.g. {"target":"the change"}.`},
				"model":       map[string]any{"type": "string", "description": "Model for the instances; empty keeps the session model."},
				"concurrency": map[string]any{"type": "integer", "minimum": 1},
			}, "script"),
		}
	case "AmendWorkflow":
		return model.Tool{
			Name:        t.name,
			Description: "Revise a workflow run with a new script. The old run is stopped as superseded and a successor run starts.",
			Parameters: workflowSchema(map[string]any{
				"run_id":      map[string]any{"type": "string", "description": "The run to revise."},
				"script":      map[string]any{"type": "string", "description": "The revised workflow script."},
				"arguments":   map[string]any{"type": "object", "additionalProperties": text},
				"model":       text,
				"concurrency": map[string]any{"type": "integer", "minimum": 1},
			}, "run_id", "script"),
		}
	case "SaveWorkflow":
		return model.Tool{
			Name:        t.name,
			Description: "Save a workflow definition so it can be launched again from the workflow hub. Names use 1 to 64 letters, digits, dots, dashes or underscores.",
			Parameters: workflowSchema(map[string]any{
				"name":        text,
				"scope":       map[string]any{"type": "string", "enum": []string{"project", "global"}},
				"description": text,
				"script":      map[string]any{"type": "string", "description": "The script to save; omitted when saving a run's script."},
				"run_id":      map[string]any{"type": "string", "description": "Save the script of this run instead of a literal script."},
				"project_id":  text,
			}, "name", "scope"),
		}
	case "ListSavedWorkflows":
		return model.Tool{
			Name:        t.name,
			Description: "List the saved workflow definitions, newest first, with their scope, phase count and last run state.",
			Parameters: workflowSchema(map[string]any{
				"scope":      map[string]any{"type": "string", "enum": []string{"project", "global"}},
				"project_id": text,
			}),
		}
	case "EvalWorkflowSnippet":
		return model.Tool{
			Name:        t.name,
			Description: "Optionally lint a workflow script without running it. Returns diagnostics and phase and instance counts. CreateWorkflow and AmendWorkflow already compile; do not repeat linting an unchanged script that compiled successfully.",
			Parameters:  workflowSchema(map[string]any{"script": text}, "script"),
		}
	case "ListWorkflowRuns":
		return model.Tool{
			Name:        t.name,
			Description: "List workflow runs with their state, phase count and start time.",
			Parameters:  workflowSchema(map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50}}),
		}
	case "GetWorkflowRun":
		return model.Tool{
			Name:        t.name,
			Description: "Read one workflow run: state, phases, instances, escalated questions, waiting holes and delivered artifacts.",
			Parameters:  workflowSchema(map[string]any{"run_id": text}, "run_id"),
		}
	case "ResumeWorkflowRun":
		return model.Tool{
			Name:        t.name,
			Description: "Resume a stopped workflow run. Settled phases are kept; the phase that was interrupted runs again.",
			Parameters:  workflowSchema(map[string]any{"run_id": text}, "run_id"),
		}
	case "ResolveWorkflowQuestion":
		return model.Tool{
			Name:        t.name,
			Description: "Answer a question an instance escalated to you. The answer is delivered to the waiting instance.",
			Parameters: workflowSchema(map[string]any{
				"run_id":      text,
				"question_id": text,
				"answer":      text,
			}, "run_id", "question_id", "answer"),
		}
	default:
		return model.Tool{
			Name:        t.name,
			Description: "Fill a workflow hole with the missing input the run is waiting for.",
			Parameters: workflowSchema(map[string]any{
				"run_id":  text,
				"hole_id": text,
				"text":    text,
			}, "run_id", "hole_id", "text"),
		}
	}
}

// workflowSchema 把属性与必填字段包成一个 JSON schema。
func workflowSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Concurrent 让只读工具与其它工具并行；会等用户或改运行的禁止并行。
func (t workflowTool) Concurrent() bool {
	switch t.name {
	case "EvalWorkflowSnippet", "ListSavedWorkflows", "ListWorkflowRuns", "GetWorkflowRun":
		return true
	}
	return false
}

func (t workflowTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	if ctx.Err() != nil {
		return model.ToolOutput{}, ctx.Err()
	}
	switch t.name {
	case "CreateWorkflow":
		return t.create(ctx, raw)
	case "AmendWorkflow":
		return t.amend(ctx, raw)
	case "SaveWorkflow":
		return t.save(ctx, raw)
	case "ListSavedWorkflows":
		return t.listSaved(raw)
	case "EvalWorkflowSnippet":
		return t.evaluate(raw)
	case "ListWorkflowRuns":
		return t.listRuns(raw)
	case "GetWorkflowRun":
		return t.getRun(raw)
	case "ResumeWorkflowRun":
		return t.resume(ctx, raw)
	case "ResolveWorkflowQuestion":
		return t.resolve(ctx, raw)
	default:
		return t.fill(ctx, raw)
	}
}

// compiledOutput 把编译结果作为工具结果返回。编译失败不是工具故障：
// 模型需要看到诊断，然后把脚本改对。
func compiledOutput(compiled workflow.Compiled) model.ToolOutput {
	data := map[string]any{
		"ok":          !compiled.Failed(),
		"diagnostics": compiled.Diagnostics,
		"phases":      compiled.PhaseCount(),
		"instances":   compiled.ActorCount(),
		"arguments":   compiled.Arguments,
	}
	if compiled.Failed() {
		text := "The workflow script did not compile.\n" + compiled.Report()
		return model.ToolOutput{Text: text, Data: data}
	}
	text := fmt.Sprintf("The workflow script compiled: %d phases, %d instances.", compiled.PhaseCount(), compiled.ActorCount())
	if report := compiled.Report(); report != "" {
		text += "\nWarnings:\n" + report
	}
	return model.ToolOutput{Text: text, Data: data}
}

// create 编译脚本并请求运行。
func (t workflowTool) create(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Script      string            `json:"script"`
		Name        string            `json:"name"`
		Arguments   map[string]string `json:"arguments"`
		Model       string            `json:"model"`
		Concurrency int               `json:"concurrency"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	compiled := workflow.CompileScript(input.Script)
	if compiled.Failed() {
		return compiledOutput(compiled), nil
	}
	args, err := workflowArgValues(compiled.Arguments, input.Arguments)
	if err != nil {
		return model.ToolOutput{}, err
	}
	request := &contracts.WorkflowRunRequest{
		Program:     compiled.Program,
		Name:        firstName(input.Name, compiled.Name),
		SessionID:   t.sessionID,
		MessageID:   t.engine.workflowCurrentMessage(t.sessionID),
		ProjectID:   t.engine.sessionProject(t.sessionID),
		ModelID:     input.Model,
		Concurrency: input.Concurrency,
		Args:        args,
		Script:      input.Script,
		Origin:      "tool",
	}
	return t.engine.launchWorkflowRun(ctx, request, "", "")
}

// amend 用新脚本修订一次运行。修订产生后继运行，旧运行被替代。
func (t workflowTool) amend(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		RunID       string            `json:"run_id"`
		Script      string            `json:"script"`
		Arguments   map[string]string `json:"arguments"`
		Model       string            `json:"model"`
		Concurrency int               `json:"concurrency"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	previous, ok := t.engine.workflowRunView(input.RunID)
	if !ok {
		return model.ToolOutput{}, errors.New("workflow run not found")
	}
	compiled := workflow.CompileScript(input.Script)
	if compiled.Failed() {
		return compiledOutput(compiled), nil
	}
	args, err := workflowArgValues(compiled.Arguments, input.Arguments)
	if err != nil {
		return model.ToolOutput{}, err
	}
	request := &contracts.WorkflowRunRequest{
		DefinitionID: previous.DefinitionID,
		Program:      compiled.Program,
		Name:         firstName(compiled.Name, previous.Name),
		SessionID:    previous.SessionID,
		MessageID:    t.engine.workflowCurrentMessage(previous.SessionID),
		ProjectID:    previous.ProjectID,
		HostID:       previous.HostID,
		ModelID:      previous.ModelID,
		Concurrency:  previous.Concurrency,
		Args:         args,
		Script:       input.Script,
		Origin:       "amend",
	}
	if input.Model != "" {
		request.ModelID = input.Model
	}
	if input.Concurrency > 0 {
		request.Concurrency = input.Concurrency
	}
	if len(args) == 0 {
		request.Args = previous.Args
	}
	return t.engine.launchWorkflowRun(ctx, request, previous.DefinitionID, previous.ID)
}

// save 保存工作流定义。脚本可以直接给出，也可以用某次运行的有效脚本。
func (t workflowTool) save(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Name        string `json:"name"`
		Scope       string `json:"scope"`
		Description string `json:"description"`
		Script      string `json:"script"`
		RunID       string `json:"run_id"`
		ProjectID   string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	script := input.Script
	program := contracts.WorkflowProgram{}
	var arguments []contracts.WorkflowArgument
	if input.RunID != "" {
		run, ok := t.engine.workflowRunView(input.RunID)
		if !ok {
			return model.ToolOutput{}, errors.New("workflow run not found")
		}
		if script == "" {
			script = run.Script
		}
	}
	if script != "" {
		compiled := workflow.CompileScript(script)
		if compiled.Failed() {
			return compiledOutput(compiled), nil
		}
		program, arguments = compiled.Program, compiled.Arguments
	} else if input.RunID != "" {
		run, _ := t.engine.workflowRunView(input.RunID)
		program = workflowProgramOf(run)
	}
	if len(program.Phases) == 0 {
		return model.ToolOutput{}, errors.New("a workflow script is required")
	}
	projectID := input.ProjectID
	if projectID == "" {
		projectID = t.engine.sessionProject(t.sessionID)
	}
	save := &contracts.WorkflowDefinitionSave{
		Name: input.Name, Description: input.Description, Scope: input.Scope,
		ProjectID: projectID, Script: script, Program: program, Arguments: arguments,
	}
	if err := t.engine.runWorkflowCommand(ctx, contracts.Command{
		Kind: "workflow.save", Workflow: &contracts.WorkflowCommand{Definition: save},
	}); err != nil {
		return model.ToolOutput{}, err
	}
	definition := t.engine.workflowDefinitionByName(input.Name, input.Scope, projectID)
	text := "The workflow was saved as " + input.Name + "."
	if input.RunID != "" {
		text = "Saved the script of run " + input.RunID + " as " + input.Name + "."
	}
	return model.ToolOutput{Text: text, Data: map[string]any{
		"name": definition.Name, "id": definition.ID, "scope": definition.Scope, "project_id": definition.ProjectID,
	}}, nil
}

// listSaved 列出保存的定义。
func (t workflowTool) listSaved(raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Scope     string `json:"scope"`
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	definitions := t.engine.workflowDefinitions()
	entries := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		if input.Scope != "" && definition.Scope != input.Scope {
			continue
		}
		if input.ProjectID != "" && definition.ProjectID != input.ProjectID {
			continue
		}
		last := t.engine.workflowLastRunOf(definition.ID)
		entry := map[string]any{
			"id": definition.ID, "name": definition.Name, "description": definition.Description,
			"scope": definition.Scope, "project_id": definition.ProjectID,
			"phases": len(definition.Program.Phases), "arguments": definition.Arguments,
		}
		if last.ID != "" {
			entry["last_run"] = map[string]any{"id": last.ID, "status": last.Status, "stop_reason": last.StopReason}
		}
		entries = append(entries, entry)
	}
	return model.ToolOutput{Text: describeList("Saved workflows", len(entries)), Data: map[string]any{"workflows": entries}}, nil
}

// evaluate 只编译不运行，用来看诊断。
func (t workflowTool) evaluate(raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Script string `json:"script"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	compiled := workflow.CompileScript(input.Script)
	output := compiledOutput(compiled)
	output.Data.(map[string]any)["phase_names"] = workflow.PhaseNames(compiled.Program)
	return output, nil
}

// listRuns 列出运行，最新的在前。
func (t workflowTool) listRuns(raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}
	runs := t.engine.workflowRunList()
	if len(runs) > input.Limit {
		runs = runs[:input.Limit]
	}
	entries := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		entries = append(entries, map[string]any{
			"id": run.ID, "name": run.Name, "status": run.Status, "stop_reason": run.StopReason,
			"session_id": run.SessionID, "phases": len(run.Phases), "started_at": run.StartedAt,
			"resumable": workflow.CanResume(run),
		})
	}
	return model.ToolOutput{Text: describeList("Workflow runs", len(entries)), Data: map[string]any{"runs": entries}}, nil
}

// getRun 读取一次运行的投影。
func (t workflowTool) getRun(raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	run, ok := t.engine.workflowRunView(input.RunID)
	if !ok {
		return model.ToolOutput{}, errors.New("workflow run not found")
	}
	view := workflow.ProjectRun(run)
	data := map[string]any{
		"run":       runSummary(run),
		"phases":    phaseEntries(view),
		"questions": run.Questions,
		"holes":     run.Holes,
		"artifacts": run.Artifacts,
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return model.ToolOutput{}, err
	}
	return model.ToolOutput{Text: string(encoded), Data: data}, nil
}

// resume 恢复一次被停掉的运行。
func (t workflowTool) resume(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	if err := t.engine.runWorkflowCommand(ctx, contracts.Command{
		Kind: "workflow.resume", Workflow: &contracts.WorkflowCommand{RunID: input.RunID},
	}); err != nil {
		return model.ToolOutput{}, err
	}
	return model.ToolOutput{Text: "Run " + input.RunID + " was resumed.", Data: map[string]any{"run_id": input.RunID}}, nil
}

// resolve 把回答交给等待中的实例。
func (t workflowTool) resolve(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		RunID      string `json:"run_id"`
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	answer := strings.TrimSpace(input.Answer)
	if answer == "" {
		return model.ToolOutput{}, errors.New("an answer is required")
	}
	if err := t.engine.runWorkflowCommand(ctx, contracts.Command{
		Kind: "workflow.resolve",
		Workflow: &contracts.WorkflowCommand{Question: &contracts.WorkflowQuestionAnswer{
			RunID: input.RunID, ID: input.QuestionID, Text: answer,
		}},
	}); err != nil {
		return model.ToolOutput{}, err
	}
	return model.ToolOutput{Text: "The answer was delivered to the waiting instance.", Data: map[string]any{"run_id": input.RunID, "question_id": input.QuestionID}}, nil
}

// fill 补全留白。
func (t workflowTool) fill(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		RunID  string `json:"run_id"`
		HoleID string `json:"hole_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return model.ToolOutput{}, errors.New("text is required")
	}
	if err := t.engine.runWorkflowCommand(ctx, contracts.Command{
		Kind: "workflow.fill",
		Workflow: &contracts.WorkflowCommand{Hole: &contracts.WorkflowHoleFill{
			RunID: input.RunID, ID: input.HoleID, Text: text,
		}},
	}); err != nil {
		return model.ToolOutput{}, err
	}
	return model.ToolOutput{Text: "The hole was filled.", Data: map[string]any{"run_id": input.RunID, "hole_id": input.HoleID}}, nil
}
