package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/workflow"
)

// workflowTask 是一次运行在主机内的执行句柄。它只负责调度与唤醒；
// 运行事实仍然写在 Engine.state 和 journal 里。
type workflowTask struct {
	runID   string
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	answers map[string]chan string
	holes   map[string]chan string
}

// workflowActorRoundLimit 限制一个实例为同一问题/留白反复升级的轮数。
const workflowActorRoundLimit = 3

// workflowPollInterval 是等待实例完成时的观察间隔。
const workflowPollInterval = 150 * time.Millisecond

// startWorkflowExecutionLocked 把已接受的运行交给后台调度。宿主关闭会取消它。
func (e *Engine) startWorkflowExecutionLocked(runID string) {
	if e.workflowTasks == nil {
		e.workflowTasks = map[string]*workflowTask{}
	}
	if e.workflowTasks[runID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(e.background)
	task := &workflowTask{
		runID: runID, cancel: cancel, done: make(chan struct{}),
		answers: map[string]chan string{}, holes: map[string]chan string{},
	}
	e.workflowTasks[runID] = task
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		defer close(task.done)
		e.executeWorkflowRun(ctx, task)
	}()
}

// cancelWorkflowExecution 停止调度，并等待其观察循环退出。
func (e *Engine) cancelWorkflowExecution(runID string) {
	task := e.workflowTasks[runID]
	if task == nil {
		return
	}
	delete(e.workflowTasks, runID)
	task.cancel()
}

// executeWorkflowRun 按阶段声明顺序推进，阶段内按并发度执行实例。
// 边的条件可以跳过某一站，也可以让运行回到前面的阶段。
func (e *Engine) executeWorkflowRun(ctx context.Context, task *workflowTask) {
	defer func() {
		e.mu.Lock()
		if e.workflowTasks[task.runID] == task {
			delete(e.workflowTasks, task.runID)
		}
		e.mu.Unlock()
	}()
	for {
		if ctx.Err() != nil {
			e.workflowStopOnCancel(task.runID)
			return
		}
		decision := e.workflowAdvance(task.runID)
		switch decision.Kind {
		case workflow.AdvanceFinish:
			e.settleWorkflowRun(task.runID, "", "")
			return
		case workflow.AdvanceSkip:
			continue
		}
		e.workflowEnterPhase(task.runID, decision.PhaseID)
		if err := e.executeWorkflowPhase(ctx, task, decision.PhaseID); err != nil {
			e.workflowStopOnCancel(task.runID)
			return
		}
	}
}

// workflowAdvance 计算并落实下一步。跳过和回边都是事实，先写下来再继续。
func (e *Engine) workflowAdvance(runID string) workflow.AdvanceDecision {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return workflow.AdvanceDecision{Kind: workflow.AdvanceFinish}
	}
	run := &e.state.WorkflowRuns[index]
	decision := workflow.Advance(*run)
	now := time.Now()
	switch decision.Kind {
	case workflow.AdvanceSkip:
		for i := range run.Phases {
			if run.Phases[i].ID != decision.PhaseID {
				continue
			}
			run.Phases[i].Status = contracts.WorkflowPhaseSkipped
			run.Phases[i].SettledAt = &now
			run.UpdatedAt = now
			if err := e.persistLocked(); err != nil {
				log.Printf("workflow skip %s/%s: %v", run.ID, decision.PhaseID, err)
			}
			e.appendWorkflowJournal(*run, workflow.JournalRecord{
				Kind: workflow.JournalPhaseSettled, At: now, RunID: run.ID,
				Data: mustEncodeWorkflowPayload(workflow.PhasePayload{PhaseID: decision.PhaseID, Name: run.Phases[i].Name, Index: run.Phases[i].Index, Status: contracts.WorkflowPhaseSkipped, SettledAt: &now, Rounds: run.Phases[i].Rounds}),
			})
		}
		e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	case workflow.AdvanceEnter:
		if decision.ResetFrom == "" {
			break
		}
		// 回边：落点及其之后的阶段重新变成待执行，轮次保留。
		from := -1
		for i := range run.Phases {
			if run.Phases[i].ID == decision.ResetFrom {
				from = i
				break
			}
		}
		if from < 0 {
			break
		}
		for i := from; i < len(run.Phases); i++ {
			run.Phases[i].Status = contracts.WorkflowPhasePending
			run.Phases[i].EnteredAt, run.Phases[i].SettledAt = nil, nil
			run.Phases[i].Observed, run.Phases[i].Settled = 0, 0
		}
		run.UpdatedAt = now
		if err := e.persistLocked(); err != nil {
			log.Printf("workflow loop %s/%s: %v", run.ID, decision.ResetFrom, err)
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalPhaseReset, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.PhasePayload{PhaseID: decision.ResetFrom}),
		})
		e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	}
	return decision
}

// workflowEnterPhase 记录阶段进入并推进运行状态。
func (e *Engine) workflowEnterPhase(runID, phaseID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return
	}
	run := &e.state.WorkflowRuns[index]
	now := time.Now()
	if run.Status == contracts.WorkflowStatusPending {
		run.Status = contracts.WorkflowStatusRunning
	}
	run.UpdatedAt = now
	for i := range run.Phases {
		if run.Phases[i].ID != phaseID {
			continue
		}
		run.Phases[i].Status = contracts.WorkflowPhaseRunning
		run.Phases[i].EnteredAt = &now
		run.Phases[i].Rounds++
		if err := e.persistLocked(); err != nil {
			log.Printf("workflow phase %s/%s: %v", run.ID, phaseID, err)
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalPhaseEntered, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.PhasePayload{PhaseID: phaseID, Name: run.Phases[i].Name, Index: run.Phases[i].Index, EnteredAt: &now}),
		})
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// executeWorkflowPhase 在并发上限内执行一个阶段的全部实例。
func (e *Engine) executeWorkflowPhase(ctx context.Context, task *workflowTask, phaseID string) error {
	actors := e.workflowPhaseActors(task.runID, phaseID)
	limit := e.workflowConcurrency(task.runID)
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, actor := range actors {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(a contracts.WorkflowActor) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			e.executeWorkflowActor(ctx, task, phaseID, a)
		}(actor)
	}
	wg.Wait()
	e.workflowSettlePhase(task.runID, phaseID)
	return ctx.Err()
}

// workflowPhaseActors 取阶段内实例。表外实例同样执行，只是卡片不展开它们。
func (e *Engine) workflowPhaseActors(runID, phaseID string) []contracts.WorkflowActor {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return nil
	}
	run := e.state.WorkflowRuns[index]
	actors := []contracts.WorkflowActor{}
	for _, actor := range run.Actors {
		if actor.PhaseID == phaseID {
			actors = append(actors, actor)
		}
	}
	return actors
}

// workflowConcurrency 读取当前并发设置，使实时调整立即生效。
func (e *Engine) workflowConcurrency(runID string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return workflow.DefaultConcurrency
	}
	return workflow.NormalizeConcurrency(e.state.WorkflowRuns[index].Concurrency)
}

// executeWorkflowActor 执行一个实例：提问、留白和产物都在同一个实例会话里完成。
func (e *Engine) executeWorkflowActor(ctx context.Context, task *workflowTask, phaseID string, actor contracts.WorkflowActor) {
	session, err := e.workflowActorSession(task.runID, actor)
	if err != nil {
		e.finishWorkflowActor(task.runID, actor, "", contracts.WorkflowActorFailed, "", err.Error())
		return
	}
	e.startWorkflowActor(task.runID, actor, session.ID)
	prompt := e.workflowActorPrompt(task.runID, phaseID, actor)
	text := prompt
	result := ""
	for round := 0; round < workflowActorRoundLimit; round++ {
		reply, err := e.workflowAskActor(ctx, session.ID, task.runID, actor, text)
		if err != nil {
			e.finishWorkflowActor(task.runID, actor, session.ID, contracts.WorkflowActorFailed, "", err.Error())
			return
		}
		parsed := workflow.ParseActorReply(reply)
		result = parsed.Text
		e.publishWorkflowArtifacts(task.runID, actor, parsed.Artifacts)
		if parsed.Question != "" {
			answer, err := e.workflowAwaitQuestion(ctx, task, task.runID, actor, parsed.Question)
			if err != nil {
				e.finishWorkflowActor(task.runID, actor, session.ID, contracts.WorkflowActorFailed, "", err.Error())
				return
			}
			text = "主 Agent 的回答：" + answer
			continue
		}
		if parsed.Hole != "" {
			fill, err := e.workflowAwaitHole(ctx, task, task.runID, phaseID, actor, parsed.Hole)
			if err != nil {
				e.finishWorkflowActor(task.runID, actor, session.ID, contracts.WorkflowActorFailed, "", err.Error())
				return
			}
			text = "留白补全：" + fill
			continue
		}
		break
	}
	e.finishWorkflowActor(task.runID, actor, session.ID, contracts.WorkflowActorDone, result, "")
}

// workflowActorSession 为实例创建子会话。子会话继承运行的目录与模型，
// 但它是独立会话：关闭父会话不会复制执行逻辑。
func (e *Engine) workflowActorSession(runID string, actor contracts.WorkflowActor) (contracts.Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return contracts.Session{}, errors.New("workflow run not found")
	}
	run := e.state.WorkflowRuns[index]
	base := contracts.Session{}
	if baseIndex := e.sessionIndex(run.SessionID); baseIndex >= 0 {
		base = e.state.Sessions[baseIndex]
	}
	if actor.SessionID != "" {
		if existing := e.sessionIndex(actor.SessionID); existing >= 0 {
			if err := e.ensureSessionDataLocked(actor.SessionID); err != nil {
				return contracts.Session{}, err
			}
			e.retainSessionDataLocked([]string{actor.SessionID})
			return e.state.Sessions[existing], nil
		}
	}
	name := actor.Name
	if name == "" {
		name = "Workflow actor"
	}
	session, err := e.newSessionLocked(base.ProjectID, name, base.StorageRootID)
	if err != nil {
		return contracts.Session{}, err
	}
	session.ParentSessionID, session.ParentRunID = run.SessionID, run.ID
	session.Title, session.DeviceID = name, base.DeviceID
	if base.CWD != "" {
		session.CWD = base.CWD
	}
	session.Branch, session.WorktreeID = base.Branch, base.WorktreeID
	switch {
	case actor.ModelID != "":
		session.ModelID = actor.ModelID
	case run.ModelID != "":
		session.ModelID = run.ModelID
	case base.ModelID != "":
		session.ModelID = base.ModelID
	}
	session.ReasoningLevel = base.ReasoningLevel
	if i := e.sessionIndex(session.ID); i >= 0 {
		e.state.Sessions[i] = session
	}
	// 保留会话数据直到实例结算，否则空闲回收会清掉回复文本。
	e.retainSessionDataLocked([]string{session.ID})
	return session, nil
}

// workflowActorPrompt 组装实例任务说明。上游结果和参数都是事实，不是产物。
func (e *Engine) workflowActorPrompt(runID, phaseID string, actor contracts.WorkflowActor) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return ""
	}
	run := e.state.WorkflowRuns[index]
	var builder strings.Builder
	builder.WriteString("You are the workflow instance \"" + actor.Name + "\" in the phase \"" + workflowPhaseName(run, phaseID) + "\".")
	builder.WriteString("\nWorkflow: " + run.Name)
	if len(run.Args) > 0 {
		builder.WriteString("\nArguments:")
		for _, argument := range run.Args {
			builder.WriteString("\n- " + argument.Name + " (" + argument.Kind + "): " + argument.Text)
		}
	}
	results := []string{}
	for _, previous := range run.Actors {
		if previous.PhaseID == phaseID || previous.Result == "" {
			continue
		}
		results = append(results, "- "+previous.Name+": "+previous.Result)
	}
	if len(results) > 0 {
		builder.WriteString("\nEarlier results:\n" + strings.Join(results, "\n"))
	}
	if len(run.Holes) > 0 {
		for _, hole := range run.Holes {
			if hole.Status == contracts.WorkflowHoleFilled && hole.Text != "" {
				builder.WriteString("\nFilled gap \"" + hole.Title + "\": " + hole.Text)
			}
		}
	}
	builder.WriteString("\nTask: finish your part and reply with the result.")
	builder.WriteString("\nIf you need the main agent to clarify something, reply only with " + workflow.QuestionMarker + "your question" + workflow.QuestionMarker + ".")
	builder.WriteString("\nIf required input is missing and must be supplied by the user, reply only with " + workflow.HoleMarker + "missing input title" + workflow.HoleMarker + ".")
	builder.WriteString("\nTo publish a deliverable, add " + workflow.ArtifactMarker + "name|content" + workflow.ArtifactMarker + ". A deliverable is not the same as your reply text.")
	return builder.String()
}

// workflowPhaseName 取阶段名称，缺失时回落到身份。
func workflowPhaseName(run contracts.WorkflowRun, phaseID string) string {
	for _, phase := range run.Phases {
		if phase.ID == phaseID {
			if phase.Name != "" {
				return phase.Name
			}
			return phase.ID
		}
	}
	return phaseID
}

// workflowAskActor 发送一轮任务并等待实例会话运行结束。
func (e *Engine) workflowAskActor(ctx context.Context, sessionID, runID string, actor contracts.WorkflowActor, text string) (string, error) {
	requestID := fmt.Sprintf("%s-%s", runID, actor.ID)
	command := contracts.Command{
		Kind: "message.send",
		Send: &contracts.SendRequest{SessionID: sessionID, Text: text, RequestID: requestID + "-" + fmt.Sprint(time.Now().UnixNano())},
	}
	if err := e.executeCommand(ctx, command, nil); err != nil {
		return "", err
	}
	return e.awaitWorkflowSession(ctx, sessionID)
}

// awaitWorkflowSession 等待实例会话的当前运行结算。观察间隔远小于一次模型往返。
func (e *Engine) awaitWorkflowSession(ctx context.Context, sessionID string) (string, error) {
	ticker := time.NewTicker(workflowPollInterval)
	defer ticker.Stop()
	for {
		e.mu.Lock()
		active := e.runs[sessionID] != nil
		done, output, runErr := e.workflowSessionOutcomeLocked(sessionID)
		e.mu.Unlock()
		if !active && done {
			return output, runErr
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// workflowSessionOutcomeLocked 读取会话最近一次运行的结局。
func (e *Engine) workflowSessionOutcomeLocked(sessionID string) (bool, string, error) {
	var record *contracts.Run
	for i := range e.state.Runs {
		if e.state.Runs[i].SessionID != sessionID {
			continue
		}
		if record == nil || e.state.Runs[i].StartedAt.After(record.StartedAt) {
			record = &e.state.Runs[i]
		}
	}
	if record == nil || record.Status == contracts.RunStatusRunning {
		return false, "", nil
	}
	output := finalReply(e.history[sessionID])
	if record.Status != contracts.RunStatusCompleted {
		message := record.Error
		if message == "" {
			message = record.Status
		}
		return true, output, errors.New(message)
	}
	return true, output, nil
}

// workflowAwaitQuestion 记录升级问题并等待主 Agent 回答。问题属于运行，不属于实例行。
func (e *Engine) workflowAwaitQuestion(ctx context.Context, task *workflowTask, runID string, actor contracts.WorkflowActor, text string) (string, error) {
	questionID := id("workflow-question")
	channel := make(chan string, 1)
	task.mu.Lock()
	task.answers[questionID] = channel
	task.mu.Unlock()
	defer func() {
		task.mu.Lock()
		delete(task.answers, questionID)
		task.mu.Unlock()
	}()
	now := time.Now()
	e.mu.Lock()
	if index := e.workflowRunIndex(runID); index >= 0 {
		run := &e.state.WorkflowRuns[index]
		run.Questions = append(run.Questions, contracts.WorkflowQuestion{
			ID: questionID, RunID: run.ID, ActorID: actor.ID, Text: text, Status: "asked", AskedAt: now,
		})
		run.UpdatedAt = now
		for i := range run.Actors {
			if run.Actors[i].ID == actor.ID {
				run.Actors[i].Status = contracts.WorkflowActorAsking
			}
		}
		if err := e.persistLocked(); err != nil {
			e.mu.Unlock()
			return "", err
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalQuestionAsked, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.QuestionPayload{QuestionID: questionID, ActorID: actor.ID, Text: text, AskedAt: &now}),
		})
		e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	}
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case answer := <-channel:
		return answer, nil
	}
}

// workflowAwaitHole 记录留白并等待补全。补全扩展同一个运行。
func (e *Engine) workflowAwaitHole(ctx context.Context, task *workflowTask, runID, phaseID string, actor contracts.WorkflowActor, title string) (string, error) {
	holeID := id("workflow-hole")
	channel := make(chan string, 1)
	task.mu.Lock()
	task.holes[holeID] = channel
	task.mu.Unlock()
	defer func() {
		task.mu.Lock()
		delete(task.holes, holeID)
		task.mu.Unlock()
	}()
	now := time.Now()
	e.mu.Lock()
	if index := e.workflowRunIndex(runID); index >= 0 {
		run := &e.state.WorkflowRuns[index]
		run.Holes = append(run.Holes, contracts.WorkflowHole{
			ID: holeID, RunID: run.ID, PhaseID: phaseID, ParentID: actor.ID, Title: title, Status: contracts.WorkflowHoleWaiting, CreatedAt: now,
		})
		run.UpdatedAt = now
		for i := range run.Actors {
			if run.Actors[i].ID == actor.ID {
				run.Actors[i].Status = contracts.WorkflowActorAsking
			}
		}
		if err := e.persistLocked(); err != nil {
			e.mu.Unlock()
			return "", err
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalHoleWaiting, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.HolePayload{HoleID: holeID, PhaseID: phaseID, ParentID: actor.ID, Title: title, CreatedAt: &now}),
		})
		e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
	}
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case fill := <-channel:
		return fill, nil
	}
}

// deliverWorkflowAnswerLocked 把回答交给等待中的实例。调用者持有状态锁。
func (e *Engine) deliverWorkflowAnswerLocked(runID, questionID, text string) {
	task := e.workflowTasks[runID]
	if task == nil {
		return
	}
	task.mu.Lock()
	channel := task.answers[questionID]
	delete(task.answers, questionID)
	task.mu.Unlock()
	if channel != nil {
		channel <- text
	}
}

// deliverWorkflowHoleLocked 把留白补全交给等待中的实例。调用者持有状态锁。
func (e *Engine) deliverWorkflowHoleLocked(runID, holeID, text string) {
	task := e.workflowTasks[runID]
	if task == nil {
		return
	}
	task.mu.Lock()
	channel := task.holes[holeID]
	delete(task.holes, holeID)
	task.mu.Unlock()
	if channel != nil {
		channel <- text
	}
}

// startWorkflowActor 记录实例开始。
func (e *Engine) startWorkflowActor(runID string, actor contracts.WorkflowActor, sessionID string) {
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return
	}
	run := &e.state.WorkflowRuns[index]
	for i := range run.Actors {
		if run.Actors[i].ID != actor.ID {
			continue
		}
		run.Actors[i].Status = contracts.WorkflowActorRunning
		run.Actors[i].SessionID = sessionID
		run.Actors[i].StartedAt = &now
		run.UpdatedAt = now
		refreshWorkflowPhaseCounts(run, actor.PhaseID)
		started := run.Actors[i]
		if err := e.persistLocked(); err != nil {
			log.Printf("workflow actor %s: %v", actor.ID, err)
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalActorStarted, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.ActorPayload{ActorID: started.ID, PhaseID: started.PhaseID, Name: started.Name, ModelID: started.ModelID, SessionID: sessionID, Status: started.Status, StartedAt: &now}),
		})
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// finishWorkflowActor 记录实例结算。
func (e *Engine) finishWorkflowActor(runID string, actor contracts.WorkflowActor, sessionID, status, result, failure string) {
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.releaseSessionDataLocked([]string{sessionID})
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return
	}
	run := &e.state.WorkflowRuns[index]
	for i := range run.Actors {
		if run.Actors[i].ID != actor.ID {
			continue
		}
		run.Actors[i].Status = status
		run.Actors[i].Result = result
		run.Actors[i].Error = failure
		run.Actors[i].FinishedAt = &now
		run.UpdatedAt = now
		refreshWorkflowPhaseCounts(run, actor.PhaseID)
		settled := run.Actors[i]
		if err := e.persistLocked(); err != nil {
			log.Printf("workflow actor %s: %v", actor.ID, err)
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalActorSettled, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.ActorPayload{ActorID: settled.ID, PhaseID: settled.PhaseID, Name: settled.Name, ModelID: settled.ModelID, SessionID: settled.SessionID, Status: settled.Status, Error: failure, Result: result, FinishedAt: &now}),
		})
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// refreshWorkflowPhaseCounts 把实例状态汇总到阶段上。
func refreshWorkflowPhaseCounts(run *contracts.WorkflowRun, phaseID string) {
	observed, settled := workflow.PhaseCounts(workflowPhaseActorsOf(run, phaseID))
	state := workflow.PhaseState(workflowPhaseActorsOf(run, phaseID))
	for i := range run.Phases {
		if run.Phases[i].ID != phaseID {
			continue
		}
		run.Phases[i].Observed, run.Phases[i].Settled = observed, settled
		if run.Phases[i].Status == contracts.WorkflowPhaseFailed {
			return
		}
		// 已经进入的阶段仍然是运行中：还有实例没开始，不代表阶段没开始。
		if state == contracts.WorkflowPhasePending && run.Phases[i].Status == contracts.WorkflowPhaseRunning {
			return
		}
		run.Phases[i].Status = state
	}
}

// workflowPhaseActorsOf 取阶段内的实例视图。
func workflowPhaseActorsOf(run *contracts.WorkflowRun, phaseID string) []contracts.WorkflowActor {
	actors := []contracts.WorkflowActor{}
	for _, actor := range run.Actors {
		if actor.PhaseID == phaseID {
			actors = append(actors, actor)
		}
	}
	return actors
}

// workflowSettlePhase 结算一个阶段。阶段失败不隐藏已完成的实例。
func (e *Engine) workflowSettlePhase(runID, phaseID string) {
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return
	}
	run := &e.state.WorkflowRuns[index]
	refreshWorkflowPhaseCounts(run, phaseID)
	for i := range run.Phases {
		if run.Phases[i].ID != phaseID {
			continue
		}
		status := run.Phases[i].Status
		switch status {
		case contracts.WorkflowPhaseRunning, contracts.WorkflowPhasePending:
			// 阶段已经走完：没有实例的阶段同样算走完。
			status = contracts.WorkflowPhaseDone
		}
		run.Phases[i].Status = status
		run.Phases[i].SettledAt = &now
		run.UpdatedAt = now
		if err := e.persistLocked(); err != nil {
			log.Printf("workflow phase %s: %v", phaseID, err)
		}
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalPhaseSettled, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.PhasePayload{PhaseID: phaseID, Name: run.Phases[i].Name, Index: run.Phases[i].Index, Status: status, SettledAt: &now, Rounds: run.Phases[i].Rounds}),
		})
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// publishWorkflowArtifacts 收集实例显式发布的产物。
func (e *Engine) publishWorkflowArtifacts(runID string, actor contracts.WorkflowActor, drafts []workflow.ArtifactDraft) {
	if len(drafts) == 0 {
		return
	}
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return
	}
	run := &e.state.WorkflowRuns[index]
	for _, draft := range drafts {
		artifact := contracts.WorkflowArtifact{
			ID: id("workflow-artifact"), RunID: run.ID, Name: draft.Name, Content: draft.Content,
			Version: 1, Order: len(run.Artifacts), Size: int64(len(draft.Content)), PublishedAt: now,
		}
		if run.Artifacts == nil {
			artifact.Primary = true
		}
		run.Artifacts = append(run.Artifacts, artifact)
	}
	run.UpdatedAt = now
	if err := e.persistLocked(); err != nil {
		log.Printf("workflow artifacts %s: %v", run.ID, err)
	}
	for _, artifact := range run.Artifacts[len(run.Artifacts)-len(drafts):] {
		e.appendWorkflowJournal(*run, workflow.JournalRecord{
			Kind: workflow.JournalArtifact, At: now, RunID: run.ID,
			Data: mustEncodeWorkflowPayload(workflow.ArtifactPayload{Artifact: artifact}),
		})
	}
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// settleWorkflowRun 结算整次运行。
func (e *Engine) settleWorkflowRun(runID, status, message string) {
	now := time.Now()
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
	failed := false
	for _, actor := range run.Actors {
		if actor.Status == contracts.WorkflowActorFailed {
			if message == "" {
				message = actor.Error
			}
			failed = true
			break
		}
	}
	if status == "" {
		if failed {
			status = contracts.WorkflowStatusErrored
		} else {
			status = contracts.WorkflowStatusCompleted
		}
	}
	for i := range run.Phases {
		if run.Phases[i].Status == contracts.WorkflowPhaseRunning || run.Phases[i].Status == contracts.WorkflowPhasePending {
			if status == contracts.WorkflowStatusCompleted {
				run.Phases[i].Status = contracts.WorkflowPhaseDone
			} else {
				run.Phases[i].Status = contracts.WorkflowPhaseFailed
			}
		}
	}
	run.Status, run.Error, run.FinishedAt, run.UpdatedAt = status, message, &now, now
	run.Result = workflowRunResult(*run)
	if err := e.persistLocked(); err != nil {
		log.Printf("workflow settle %s: %v", run.ID, err)
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind: workflow.JournalRunSettled, At: now, RunID: run.ID,
		Data: mustEncodeWorkflowPayload(workflow.SettlePayload{Status: status, Error: message, Result: run.Result, FinishedAt: &now}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// workflowStopOnCancel 把取消转成可解释的停止状态，而不是丢失的运行。
func (e *Engine) workflowStopOnCancel(runID string) {
	now := time.Now()
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
	run.Status, run.StopReason, run.Resumable = contracts.WorkflowStatusStopped, contracts.WorkflowStopInterrupted, true
	run.FinishedAt, run.UpdatedAt = &now, now
	resetWorkflowRunningPhases(run)
	for i := range run.Actors {
		if run.Actors[i].Status == contracts.WorkflowActorRunning || run.Actors[i].Status == contracts.WorkflowActorAsking {
			run.Actors[i].Status = contracts.WorkflowActorPending
		}
	}
	if err := e.persistLocked(); err != nil {
		log.Printf("workflow stop %s: %v", run.ID, err)
	}
	e.appendWorkflowJournal(*run, workflow.JournalRecord{
		Kind: workflow.JournalRunStopped, At: now, RunID: run.ID,
		Data: mustEncodeWorkflowPayload(workflow.SettlePayload{Status: run.Status, StopReason: run.StopReason, FinishedAt: &now}),
	})
	e.publishLocked(contracts.Event{Kind: "workflow.changed", SessionID: run.SessionID, RunID: run.ID})
}

// workflowRunResult 是运行的最终摘要：产物优先，其次最后一个完成的实例结果。
func workflowRunResult(run contracts.WorkflowRun) string {
	if primary, ok := workflow.PrimaryArtifact(run.Artifacts); ok {
		return primary.Name
	}
	for i := len(run.Actors) - 1; i >= 0; i-- {
		if run.Actors[i].Status == contracts.WorkflowActorDone && run.Actors[i].Result != "" {
			return run.Actors[i].Result
		}
	}
	return ""
}

// appendWorkflowJournal 追加一条事实。序号在进程内单调递增，重启后从文件恢复。
func (e *Engine) appendWorkflowJournal(run contracts.WorkflowRun, record workflow.JournalRecord) {
	if e.workflowSequences == nil {
		e.workflowSequences = map[string]uint64{}
	}
	record.RunID, record.Sequence = run.ID, e.workflowSequences[run.ID]+1
	e.workflowSequences[run.ID] = record.Sequence
	path := workflow.JournalPath(e.storage.ConfigDirectory(), run.ID)
	if err := workflow.AppendRecords(path, []workflow.JournalRecord{record}); err != nil {
		log.Printf("workflow journal %s: %v", run.ID, err)
	}
}

// mustEncodeWorkflowPayload 编码 journal 载荷。编码失败在写入前即被捕获，
// 载荷都是本地结构，因此这里把失败当作编程错误记日志而不是静默丢弃。
func mustEncodeWorkflowPayload(value any) json.RawMessage {
	data, err := workflow.EncodePayload(value)
	if err != nil {
		log.Printf("workflow journal payload: %v", err)
		return nil
	}
	return data
}

// recoverWorkflowRunsLocked 把宿主停机时仍在运行的运行标记为中断，而不是假装成功。
func (e *Engine) recoverWorkflowRunsLocked(now time.Time) error {
	for i := range e.state.WorkflowRuns {
		run := &e.state.WorkflowRuns[i]
		if !workflow.IsActive(run.Status) {
			continue
		}
		run.Status, run.StopReason, run.Resumable = contracts.WorkflowStatusStopped, contracts.WorkflowStopInterrupted, true
		run.FinishedAt, run.UpdatedAt = &now, now
		for j := range run.Actors {
			if run.Actors[j].Status == contracts.WorkflowActorRunning || run.Actors[j].Status == contracts.WorkflowActorAsking {
				run.Actors[j].Status = contracts.WorkflowActorPending
			}
		}
	}
	return nil
}

// workflowRunView 返回一次运行的只读快照，供测试与投影阅读。
func (e *Engine) workflowRunView(runID string) (contracts.WorkflowRun, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	index := e.workflowRunIndex(runID)
	if index < 0 {
		return contracts.WorkflowRun{}, false
	}
	return e.state.WorkflowRuns[index], true
}
