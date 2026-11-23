package engine

// 工作流工具用到的引擎侧动作：启动运行、保存定义、读取投影。
// 这些动作和工具共用同一份状态，工具不保存任何副本。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/workflow"
)

// launchWorkflowRun 真正启动一次运行。修订会先停掉被替代的旧运行。
func (e *Engine) launchWorkflowRun(ctx context.Context, request *contracts.WorkflowRunRequest, definitionID, previousRun string) (model.ToolOutput, error) {
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	if previousRun != "" {
		e.markWorkflowSuperseded(previousRun)
	}
	e.mu.Lock()
	runID, err := e.startWorkflowLocked(request, definitionID)
	e.mu.Unlock()
	if err != nil {
		return model.ToolOutput{}, err
	}
	return model.ToolOutput{
		Text: "The workflow run started and executes in the background. Its card appears beside the launch activity.",
		Data: map[string]any{"run_id": runID, "name": request.Name},
	}, nil
}

// markWorkflowSuperseded 把被修订替代的旧运行标成被替代。它不再提供恢复。
func (e *Engine) markWorkflowSuperseded(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return
	}
	run := &e.state.WorkflowRuns[index]
	if workflow.IsTerminal(run.Status) {
		return
	}
	e.cancelWorkflowExecution(runID)
	now := time.Now()
	run.Status, run.StopReason, run.Resumable = contracts.WorkflowStatusStopped, contracts.WorkflowStopSuperseded, false
	run.FinishedAt, run.UpdatedAt = &now, now
	for i := range run.Actors {
		if run.Actors[i].Status == contracts.WorkflowActorRunning || run.Actors[i].Status == contracts.WorkflowActorAsking {
			run.Actors[i].Status = contracts.WorkflowActorPending
		}
	}
	if err := e.persistLocked(); err != nil {
		log.Printf("workflow supersede %s: %v", runID, err)
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind: workflow.JournalRunStopped, At: now, RunID: run.ID,
		Data: mustEncodeWorkflowPayload(workflow.SettlePayload{Status: run.Status, StopReason: run.StopReason, FinishedAt: &now}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// runWorkflowCommand 在引擎内部执行一条工作流命令。
func (e *Engine) runWorkflowCommand(ctx context.Context, command contracts.Command) error {
	return e.executeCommand(ctx, command, nil)
}

// sessionProject 取会话所属的项目。
func (e *Engine) sessionProject(sessionID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return ""
	}
	return e.state.Sessions[index].ProjectID
}

// workflowDefinitions 读出全部保存的定义，最新的在前。
func (e *Engine) workflowDefinitions() []contracts.WorkflowDefinition {
	e.mu.Lock()
	defer e.mu.Unlock()
	definitions := make([]contracts.WorkflowDefinition, 0, len(e.state.WorkflowDefinitions))
	for _, definition := range e.state.WorkflowDefinitions {
		definitions = append(definitions, definition)
	}
	sort.SliceStable(definitions, func(i, j int) bool {
		return definitions[i].UpdatedAt.After(definitions[j].UpdatedAt)
	})
	return definitions
}

// workflowDefinitionByName 按名字与作用域找定义。保存同名定义会覆盖它。
func (e *Engine) workflowDefinitionByName(name, scope, projectID string) contracts.WorkflowDefinition {
	for _, definition := range e.workflowDefinitions() {
		if definition.Name != name || definition.Scope != scope {
			continue
		}
		if scope == "project" && definition.ProjectID != projectID {
			continue
		}
		return definition
	}
	return contracts.WorkflowDefinition{}
}

// workflowLastRunOf 找一个定义最近的一次运行。
func (e *Engine) workflowLastRunOf(definitionID string) contracts.WorkflowRun {
	if definitionID == "" {
		return contracts.WorkflowRun{}
	}
	var latest contracts.WorkflowRun
	found := false
	for _, run := range e.workflowRunList() {
		if run.DefinitionID != definitionID {
			continue
		}
		if !found || run.StartedAt.After(latest.StartedAt) {
			latest, found = run, true
		}
	}
	return latest
}

// workflowRunList 读出全部运行，最新的在前。
func (e *Engine) workflowRunList() []contracts.WorkflowRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	runs := make([]contracts.WorkflowRun, 0, len(e.state.WorkflowRuns))
	for _, run := range e.state.WorkflowRuns {
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	return runs
}

// workflowProgramOf 从一次运行里取回程序。运行本身不保存程序，
// 所以这里按阶段的声明重建一份。
func workflowProgramOf(run contracts.WorkflowRun) contracts.WorkflowProgram {
	program := contracts.WorkflowProgram{}
	for _, phase := range run.Phases {
		spec := contracts.WorkflowPhaseSpec{ID: phase.ID, Name: phase.Name}
		for _, actor := range run.Actors {
			if actor.PhaseID != phase.ID {
				continue
			}
			spec.Instances = append(spec.Instances, contracts.WorkflowActorSpec{ID: actor.ID, Name: actor.Name, ModelID: actor.ModelID})
		}
		program.Phases = append(program.Phases, spec)
	}
	return program
}

// workflowArgValues 把模型给的实参按声明校验。未知参数、越界的数字和
// 解析不了的 JSON 都被拒绝；空值不当成传参。
func workflowArgValues(declared []contracts.WorkflowArgument, provided map[string]string) ([]contracts.WorkflowArgValue, error) {
	values := map[string]contracts.WorkflowArgValue{}
	for name := range provided {
		if _, ok := values[name]; ok {
			continue
		}
		found := false
		for _, argument := range declared {
			if argument.Name == name {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown workflow argument %q", name)
		}
	}
	for _, argument := range declared {
		text, ok := provided[argument.Name]
		if !ok {
			if argument.HasDefault {
				values[argument.Name] = contracts.WorkflowArgValue{Name: argument.Name, Kind: argument.Type, Text: argument.Default}
			}
			continue
		}
		trimmed := strings.TrimSpace(text)
		switch argument.Type {
		case "number":
			if trimmed == "" {
				continue
			}
			number, err := strconv.ParseFloat(trimmed, 64)
			if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
				return nil, fmt.Errorf("workflow argument %q must be a finite number", argument.Name)
			}
		case "json":
			if trimmed == "" {
				continue
			}
			if !json.Valid([]byte(trimmed)) {
				return nil, fmt.Errorf("workflow argument %q must be valid JSON", argument.Name)
			}
		case "boolean":
			if trimmed != "true" && trimmed != "false" {
				return nil, fmt.Errorf("workflow argument %q must be true or false", argument.Name)
			}
		default:
			if trimmed == "" {
				continue
			}
		}
		values[argument.Name] = contracts.WorkflowArgValue{Name: argument.Name, Kind: argument.Type, Text: text}
	}
	for _, argument := range declared {
		if argument.Required {
			if _, ok := values[argument.Name]; !ok {
				return nil, fmt.Errorf("workflow argument %q is required", argument.Name)
			}
		}
	}
	ordered := make([]contracts.WorkflowArgValue, 0, len(values))
	for _, argument := range declared {
		if value, ok := values[argument.Name]; ok {
			ordered = append(ordered, value)
		}
	}
	return ordered, nil
}

// firstName 取第一个非空的名字。
func firstName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// describeList 说明一个列表里有多少条。
func describeList(label string, count int) string {
	if count == 0 {
		return label + ": none."
	}
	return fmt.Sprintf("%s: %d.", label, count)
}

// runSummary 是给模型看的运行摘要。
func runSummary(run contracts.WorkflowRun) map[string]any {
	return map[string]any{
		"id": run.ID, "name": run.Name, "status": run.Status, "stop_reason": run.StopReason,
		"error": run.Error, "model_id": run.ModelID, "concurrency": run.Concurrency,
		"session_id": run.SessionID, "message_id": run.MessageID, "result": run.Result,
		"started_at": run.StartedAt, "finished_at": run.FinishedAt, "resumable": workflow.CanResume(run),
		"args": run.Args,
	}
}

// phaseEntries 把阶段投影成给模型看的条目。
func phaseEntries(view workflow.RunView) []map[string]any {
	entries := make([]map[string]any, 0, len(view.Phases))
	for _, phase := range view.Phases {
		actors := make([]map[string]any, 0, len(phase.Actors))
		for _, actor := range phase.Actors {
			actors = append(actors, map[string]any{
				"id": actor.ID, "name": actor.Name, "status": actor.Status,
				"session_id": actor.SessionID, "error": actor.Error,
			})
		}
		entries = append(entries, map[string]any{
			"id": phase.ID, "name": phase.Name, "index": phase.Index, "status": phase.Status,
			"observed": phase.Observed, "settled": phase.Settled, "rounds": phase.Rounds, "instances": actors,
		})
	}
	return entries
}
