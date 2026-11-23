package engine

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/workflow"
)

// workflowNamePattern 约束可保存的工作流名称，保证跨平台文件名安全。
var workflowNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// workflowCommandLocked 是工作流命令的唯一入口。核心拥有运行状态，UI 只发命令。
func (e *Engine) workflowCommandLocked(command contracts.Command) error {
	payload := command.Workflow
	if payload == nil {
		return errors.New("workflow payload is required")
	}
	switch command.Kind {
	case "workflow.run":
		return e.workflowRunLocked(payload)
	case "workflow.launch":
		return e.workflowLaunchLocked(payload)
	case "workflow.stop":
		return e.workflowStopLocked(payload)
	case "workflow.retune":
		return e.workflowRetuneLocked(payload)
	case "workflow.resolve":
		return e.workflowResolveLocked(payload)
	case "workflow.fill":
		return e.workflowFillLocked(payload)
	case "workflow.save":
		return e.workflowSaveLocked(payload)
	case "workflow.delete":
		return e.workflowDeleteLocked(payload)
	case "workflow.resume":
		return e.workflowResumeLocked(payload)
	default:
		return errors.New("unknown workflow command")
	}
}

// workflowRunLocked 接受一次全新的运行请求。
func (e *Engine) workflowRunLocked(payload *contracts.WorkflowCommand) error {
	if payload.Run == nil {
		return errors.New("a workflow run request is required")
	}
	_, err := e.startWorkflowLocked(payload.Run, "")
	return err
}

// workflowLaunchLocked 用已保存的定义再次运行。参数以定义声明为准。
func (e *Engine) workflowLaunchLocked(payload *contracts.WorkflowCommand) error {
	if payload.Run == nil {
		return errors.New("a workflow run request is required")
	}
	definition := e.workflowDefinitionLocked(payload.Run.DefinitionID)
	if definition == nil {
		return errors.New("workflow definition not found")
	}
	request := *payload.Run
	request.Program = definition.Program
	if request.Name == "" {
		request.Name = definition.Name
	}
	if request.Script == "" {
		request.Script = definition.Script
	}
	if request.ProjectID == "" {
		request.ProjectID = definition.ProjectID
	}
	request.Args = mergeWorkflowArgs(definition.Arguments, request.Args)
	for _, argument := range definition.Arguments {
		if argument.Required && !hasWorkflowArg(request.Args, argument.Name) {
			return fmt.Errorf("workflow argument %q is required", argument.Name)
		}
	}
	_, err := e.startWorkflowLocked(&request, definition.ID)
	return err
}

// startWorkflowLocked 落库一次运行并把执行交给后台任务。它返回这次运行的身份。
func (e *Engine) startWorkflowLocked(request *contracts.WorkflowRunRequest, definitionID string) (string, error) {
	if len(request.Program.Phases) == 0 {
		return "", errors.New("a workflow program with at least one phase is required")
	}
	now := time.Now()
	sessionID, projectID, sessionModel := "", request.ProjectID, ""
	if request.SessionID != "" {
		index := e.sessionIndex(request.SessionID)
		if index < 0 {
			return "", errors.New("workflow session not found")
		}
		session := e.state.Sessions[index]
		sessionID, projectID, sessionModel = session.ID, session.ProjectID, session.ModelID
		if request.ProjectID == "" {
			request.ProjectID = projectID
		}
	}
	modelID := request.ModelID
	if modelID == "" {
		modelID = sessionModel
	}
	if modelID == "" && len(e.state.Models) > 0 {
		modelID = e.state.Models[0].ID
	}
	run := contracts.WorkflowRun{
		ID:           id("workflow-run"),
		DefinitionID: definitionID,
		Name:         strings.TrimSpace(request.Name),
		SessionID:    sessionID,
		MessageID:    request.MessageID,
		ProjectID:    projectID,
		HostID:       request.HostID,
		Status:       contracts.WorkflowStatusPending,
		ModelID:      modelID,
		Concurrency:  workflow.NormalizeConcurrency(request.Concurrency),
		Args:         slices.Clone(request.Args),
		Script:       request.Script,
		Origin:       request.Origin,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	if run.Name == "" {
		run.Name = "Workflow"
	}
	expandWorkflowProgram(&run, request.Program)
	e.state.WorkflowRuns = append(e.state.WorkflowRuns, run)
	e.activateWorkflowSessionForRunLocked(run.SessionID)
	if err := e.persistLocked(); err != nil {
		e.state.WorkflowRuns = e.state.WorkflowRuns[:len(e.state.WorkflowRuns)-1]
		return "", err
	}
	e.appendWorkflowJournal(run, workflow.JournalRecord{
		Sequence: 1,
		Kind:     workflow.JournalRunStarted,
		At:       now,
		RunID:    run.ID,
		Data: mustEncodeWorkflowPayload(workflow.RunStartedPayload{
			Name:        run.Name,
			ModelID:     run.ModelID,
			Concurrency: run.Concurrency,
			SessionID:   run.SessionID,
			MessageID:   run.MessageID,
			Program:     request.Program,
		}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	e.startWorkflowExecutionLocked(run.ID)
	return run.ID, nil
}

// expandWorkflowProgram 把程序展开成运行事实。阶段与实例都获得稳定身份；
// 声明的阶段边一并带进运行，跳段与回边是显示事实，不让界面自己猜。
func expandWorkflowProgram(run *contracts.WorkflowRun, program contracts.WorkflowProgram) {
	for index, phase := range program.Phases {
		run.Phases = append(run.Phases, contracts.WorkflowPhase{ID: phase.ID, Name: phase.Name, Index: index, Status: contracts.WorkflowPhasePending})
		for _, instance := range phase.Instances {
			run.Actors = append(run.Actors, contracts.WorkflowActor{
				ID:      instance.ID,
				PhaseID: phase.ID,
				Name:    instance.Name,
				Status:  contracts.WorkflowActorPending,
				ModelID: instance.ModelID,
			})
		}
	}
	run.Edges = workflowProgramEdges(program)
}

// workflowProgramEdges 收集程序声明的阶段边。边写在两端时只留一份。
func workflowProgramEdges(program contracts.WorkflowProgram) []contracts.WorkflowEdge {
	var edges []contracts.WorkflowEdge
	seen := map[string]bool{}
	for _, phase := range program.Phases {
		for _, edge := range phase.Edges {
			key := edge.From + "\x00" + edge.To + "\x00" + edge.Kind + "\x00" + edge.When
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, edge)
		}
	}
	return edges
}

// workflowResumeLocked 恢复一次被停掉的运行。恢复的是同一个运行：
// 已经结算的阶段保留，停在一半的阶段重新执行。
func (e *Engine) workflowResumeLocked(payload *contracts.WorkflowCommand) error {
	runID := payload.RunID
	if runID == "" {
		runID = payload.ID
	}
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return errors.New("workflow run not found")
	}
	run := &e.state.WorkflowRuns[index]
	if !workflow.CanResume(*run) {
		return errors.New("this workflow run cannot be resumed")
	}
	now := time.Now()
	run.Status, run.StopReason, run.Resumable = contracts.WorkflowStatusPending, "", false
	run.FinishedAt, run.UpdatedAt = nil, now
	resetWorkflowRunningPhases(run)
	e.activateWorkflowSessionForRunLocked(run.SessionID)
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind: workflow.JournalRunResumed, At: now, RunID: run.ID,
		Data: mustEncodeWorkflowPayload(workflow.SettlePayload{Status: run.Status}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	e.startWorkflowExecutionLocked(run.ID)
	return nil
}

// resetWorkflowRunningPhases 把停在一半的阶段重新变成待执行。
// 调用者持有状态锁。
func resetWorkflowRunningPhases(run *contracts.WorkflowRun) {
	for i := range run.Phases {
		if run.Phases[i].Status != contracts.WorkflowPhaseRunning {
			continue
		}
		run.Phases[i].Status = contracts.WorkflowPhasePending
		run.Phases[i].EnteredAt, run.Phases[i].SettledAt = nil, nil
	}
}

// workflowStopLocked 停止一次运行。停止后的运行可以被恢复。
func (e *Engine) workflowStopLocked(payload *contracts.WorkflowCommand) error {
	runID := payload.RunID
	if runID == "" {
		runID = payload.ID
	}
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return errors.New("workflow run not found")
	}
	run := &e.state.WorkflowRuns[index]
	if workflow.IsTerminal(run.Status) {
		return nil
	}
	e.cancelWorkflowExecution(runID)
	now := time.Now()
	run.Status, run.StopReason, run.Resumable = contracts.WorkflowStatusStopped, contracts.WorkflowStopUser, true
	run.FinishedAt, run.UpdatedAt = &now, now
	resetWorkflowRunningPhases(run)
	for i := range run.Actors {
		if run.Actors[i].Status == contracts.WorkflowActorRunning || run.Actors[i].Status == contracts.WorkflowActorAsking {
			run.Actors[i].Status = contracts.WorkflowActorPending
		}
	}
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind:  workflow.JournalRunSettled,
		At:    now,
		RunID: run.ID,
		Data:  mustEncodeWorkflowPayload(workflow.SettlePayload{Status: run.Status, StopReason: run.StopReason}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	return nil
}

// workflowRetuneLocked 调整运行设置。只改并发是同一次运行的实时调整；
// 改模型会产生后继运行，旧运行被替代且不再提供恢复。
func (e *Engine) workflowRetuneLocked(payload *contracts.WorkflowCommand) error {
	if payload.Settings == nil {
		return errors.New("workflow settings are required")
	}
	change := *payload.Settings
	index := e.workflowRunIndex(change.RunID)
	if index < 0 {
		return errors.New("workflow run not found")
	}
	run := &e.state.WorkflowRuns[index]
	if !workflow.CanConfigure(*run) {
		return errors.New("this workflow run no longer accepts changes")
	}
	switch workflow.ChangeOutcome(change) {
	case workflow.OutcomeNone:
		return errors.New("no workflow setting changed")
	case workflow.OutcomeRetune:
		run.Concurrency = workflow.NormalizeConcurrency(change.Concurrency)
		run.UpdatedAt = time.Now()
		if err := e.persistLocked(); err != nil {
			return err
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind:  workflow.JournalRunRetuned,
			At:    run.UpdatedAt,
			RunID: run.ID,
			Data:  mustEncodeWorkflowPayload(workflow.RetunePayload{Concurrency: run.Concurrency}),
		})
		e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
		return nil
	default:
		modelID, changed := workflow.ResolveModel(change, e.workflowSessionModelLocked(*run))
		if !changed {
			return errors.New("no workflow setting changed")
		}
		return e.workflowSuccessorLocked(*run, modelID, change.Concurrency)
	}
}

// workflowSuccessorLocked 用新的模型产生后继运行，并停掉被替代的运行。
func (e *Engine) workflowSuccessorLocked(run contracts.WorkflowRun, modelID string, concurrency int) error {
	e.cancelWorkflowExecution(run.ID)
	now := time.Now()
	index := e.workflowRunIndex(run.ID)
	if index >= 0 {
		previous := &e.state.WorkflowRuns[index]
		previous.Status, previous.StopReason, previous.Resumable = contracts.WorkflowStatusStopped, contracts.WorkflowStopModel, false
		previous.FinishedAt, previous.UpdatedAt = &now, now
	}
	successor := run
	successor.ID = id("workflow-run")
	successor.PreviousRunID = run.ID
	successor.Status = contracts.WorkflowStatusPending
	successor.StopReason, successor.Error, successor.Result = "", "", ""
	successor.ModelID = modelID
	successor.FinishedAt, successor.Resumable = nil, false
	successor.StartedAt, successor.UpdatedAt = now, now
	if concurrency > 0 {
		successor.Concurrency = workflow.NormalizeConcurrency(concurrency)
	}
	successor.Phases = slices.Clone(run.Phases)
	successor.Actors = slices.Clone(run.Actors)
	successor.Questions = nil
	successor.Holes = nil
	successor.Artifacts = nil
	for i := range successor.Phases {
		successor.Phases[i].Status = contracts.WorkflowPhasePending
		successor.Phases[i].EnteredAt, successor.Phases[i].SettledAt = nil, nil
	}
	for i := range successor.Actors {
		successor.Actors[i].Status = contracts.WorkflowActorPending
		successor.Actors[i].SessionID, successor.Actors[i].Result = "", ""
		successor.Actors[i].Error = ""
		successor.Actors[i].StartedAt, successor.Actors[i].FinishedAt = nil, nil
	}
	e.state.WorkflowRuns = append(e.state.WorkflowRuns, successor)
	if err := e.persistLocked(); err != nil {
		e.state.WorkflowRuns = e.state.WorkflowRuns[:len(e.state.WorkflowRuns)-1]
		return err
	}
	e.appendWorkflowJournal(successor, workflow.JournalRecord{
		Kind:  workflow.JournalRunStarted,
		At:    now,
		RunID: successor.ID,
		Data: mustEncodeWorkflowPayload(workflow.RunStartedPayload{
			Name: successor.Name, ModelID: successor.ModelID, Concurrency: successor.Concurrency, SessionID: successor.SessionID,
		}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: successor.SessionID, RunID: successor.ID})
	e.startWorkflowExecutionLocked(successor.ID)
	return nil
}

// workflowResolveLocked 把主 Agent 的回答交给等待中的实例。
func (e *Engine) workflowResolveLocked(payload *contracts.WorkflowCommand) error {
	if payload.Question == nil {
		return errors.New("a workflow question is required")
	}
	answer := *payload.Question
	index := e.workflowRunIndex(answer.RunID)
	if index < 0 {
		return errors.New("workflow run not found")
	}
	run := &e.state.WorkflowRuns[index]
	question := -1
	for i := range run.Questions {
		if run.Questions[i].ID == answer.ID {
			question = i
			break
		}
	}
	if question < 0 {
		return errors.New("workflow question not found")
	}
	now := time.Now()
	run.Questions[question].Status = "resolved"
	run.Questions[question].Answer = answer.Text
	run.Questions[question].ResolvedAt = &now
	run.UpdatedAt = now
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind:  workflow.JournalQuestionAnswer,
		At:    now,
		RunID: run.ID,
		Data:  mustEncodeWorkflowPayload(workflow.QuestionPayload{QuestionID: answer.ID, Answer: answer.Text}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	e.deliverWorkflowAnswerLocked(run.ID, answer.ID, answer.Text)
	return nil
}

// workflowFillLocked 补全留白。补全扩展同一个运行，不新建运行。
func (e *Engine) workflowFillLocked(payload *contracts.WorkflowCommand) error {
	if payload.Hole == nil {
		return errors.New("a workflow hole is required")
	}
	fill := *payload.Hole
	index := e.workflowRunIndex(fill.RunID)
	if index < 0 {
		return errors.New("workflow run not found")
	}
	run := &e.state.WorkflowRuns[index]
	hole := -1
	for i := range run.Holes {
		if run.Holes[i].ID == fill.ID {
			hole = i
			break
		}
	}
	if hole < 0 {
		return errors.New("workflow hole not found")
	}
	now := time.Now()
	run.Holes[hole].Status = contracts.WorkflowHoleFilled
	run.Holes[hole].Text = fill.Text
	run.Holes[hole].FilledAt = &now
	run.UpdatedAt = now
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind:  workflow.JournalHoleFilled,
		At:    now,
		RunID: run.ID,
		Data:  mustEncodeWorkflowPayload(workflow.HolePayload{HoleID: fill.ID, Text: fill.Text}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	e.deliverWorkflowHoleLocked(run.ID, fill.ID, fill.Text)
	return nil
}

// workflowSaveLocked 保存或更新一个工作流定义。
func (e *Engine) workflowSaveLocked(payload *contracts.WorkflowCommand) error {
	if payload.Definition == nil {
		return errors.New("a workflow definition is required")
	}
	save := *payload.Definition
	save.Name = strings.TrimSpace(save.Name)
	if !workflowNamePattern.MatchString(save.Name) {
		return errors.New("workflow names use 1 to 64 letters, digits, dots, dashes or underscores")
	}
	if len(save.Program.Phases) == 0 {
		return errors.New("a workflow program with at least one phase is required")
	}
	if save.Scope != "project" && save.Scope != "global" {
		return errors.New("workflow scope must be project or global")
	}
	if save.Scope == "project" && save.ProjectID == "" {
		return errors.New("a project workflow needs a project")
	}
	now := time.Now()
	definition := contracts.WorkflowDefinition{
		ID: save.ID, Name: save.Name, Description: strings.TrimSpace(save.Description),
		Scope: save.Scope, ProjectID: save.ProjectID, HostID: save.HostID,
		Script: save.Script, Program: save.Program, Arguments: slices.Clone(save.Arguments),
		CreatedAt: now, UpdatedAt: now,
	}
	index := -1
	if definition.ID != "" {
		index = e.workflowDefinitionIndex(definition.ID)
	}
	if index >= 0 {
		definition.CreatedAt = e.state.WorkflowDefinitions[index].CreatedAt
		e.state.WorkflowDefinitions[index] = definition
	} else {
		definition.ID = id("workflow")
		e.state.WorkflowDefinitions = append(e.state.WorkflowDefinitions, definition)
	}
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", ResourceID: definition.ID})
	return nil
}

// workflowDeleteLocked 删除一个工作流定义。
func (e *Engine) workflowDeleteLocked(payload *contracts.WorkflowCommand) error {
	definitionID := payload.ID
	if definitionID == "" {
		definitionID = payload.RunID
	}
	index := e.workflowDefinitionIndex(definitionID)
	if index < 0 {
		return errors.New("workflow definition not found")
	}
	e.state.WorkflowDefinitions = slices.Delete(e.state.WorkflowDefinitions, index, index+1)
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", ResourceID: definitionID})
	return nil
}

// workflowRunIndex 找到一次运行的位置。
func (e *Engine) workflowRunIndex(runID string) int {
	if runID == "" {
		return -1
	}
	return slices.IndexFunc(e.state.WorkflowRuns, func(run contracts.WorkflowRun) bool { return run.ID == runID })
}

// workflowDefinitionIndex 找到一个定义的位置。
func (e *Engine) workflowDefinitionIndex(definitionID string) int {
	if definitionID == "" {
		return -1
	}
	return slices.IndexFunc(e.state.WorkflowDefinitions, func(definition contracts.WorkflowDefinition) bool { return definition.ID == definitionID })
}

// workflowDefinitionLocked 按 ID 取定义。
func (e *Engine) workflowDefinitionLocked(definitionID string) *contracts.WorkflowDefinition {
	index := e.workflowDefinitionIndex(definitionID)
	if index < 0 {
		return nil
	}
	return &e.state.WorkflowDefinitions[index]
}

// workflowSessionModelLocked 取运行所属会话的模型。模型为空时回落到会话模型。
func (e *Engine) workflowSessionModelLocked(run contracts.WorkflowRun) string {
	if run.SessionID == "" {
		return ""
	}
	index := e.sessionIndex(run.SessionID)
	if index < 0 {
		return ""
	}
	return e.state.Sessions[index].ModelID
}

// mergeWorkflowArgs 把定义声明的参数与运行传入的值合并。空值不覆盖声明。
func mergeWorkflowArgs(declared []contracts.WorkflowArgument, provided []contracts.WorkflowArgValue) []contracts.WorkflowArgValue {
	values := map[string]contracts.WorkflowArgValue{}
	order := []string{}
	for _, argument := range declared {
		values[argument.Name] = contracts.WorkflowArgValue{Name: argument.Name, Kind: argument.Type, Text: argument.Default}
		order = append(order, argument.Name)
	}
	for _, value := range provided {
		if _, ok := values[value.Name]; !ok {
			order = append(order, value.Name)
		}
		values[value.Name] = value
	}
	merged := make([]contracts.WorkflowArgValue, 0, len(order))
	for _, name := range order {
		merged = append(merged, values[name])
	}
	return merged
}

// hasWorkflowArg 判断参数是否已经提供。
func hasWorkflowArg(values []contracts.WorkflowArgValue, name string) bool {
	return slices.ContainsFunc(values, func(value contracts.WorkflowArgValue) bool { return value.Name == name && value.Text != "" })
}
