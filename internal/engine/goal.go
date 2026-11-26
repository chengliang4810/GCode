package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

const goalVerdictTool = "submit_goal_verdict"

var errGoalHalted = errors.New("goal execution stopped")

type goalScope struct {
	sessionID string
	id        string
}

func (e *Engine) goalLocked(scope goalScope) *contracts.SessionGoal {
	index := e.sessionIndex(scope.sessionID)
	if index < 0 {
		return nil
	}
	goal := e.state.Sessions[index].Goal
	if goal == nil || goal.ID != scope.id {
		return nil
	}
	return goal
}

func settleGoalTime(goal *contracts.SessionGoal, now time.Time) {
	if goal.ActiveSince != nil {
		goal.TimeUsedMS += max(int64(0), now.Sub(*goal.ActiveSince).Milliseconds())
		goal.ActiveSince = nil
	}
	goal.LastSeenAt, goal.UpdatedAt = now, now
}

func goalBudgetReason(goal *contracts.SessionGoal) string {
	if goal.TokenBudget == nil {
		return ""
	}
	if goal.UnknownUsageCount > 0 {
		return "Provider usage is incomplete; the remaining token budget cannot be verified."
	}
	if goal.TokensUsed >= *goal.TokenBudget {
		return "The goal token budget has been reached."
	}
	return ""
}

func (e *Engine) goalCommandLocked(command contracts.Command) error {
	if err := e.ensureSessionDataLocked(command.SessionID); err != nil {
		return err
	}
	index := e.sessionIndex(command.SessionID)
	if index < 0 {
		return errors.New("select a conversation before managing its goal")
	}
	session := &e.state.Sessions[index]
	if session.SubagentType != "" {
		return errors.New("subagent sessions cannot own goals")
	}
	if command.Kind == "goal.set" || command.Kind == "goal.replace" {
		if command.Kind == "goal.set" && session.Goal != nil && session.Goal.Status != contracts.GoalComplete {
			return errors.New("a goal already exists; use /goal replace <objective> to replace it")
		}
		if e.runs[session.ID] != nil || session.RunningSubagents > 0 {
			return errors.New("pause the current goal and wait for its tasks to stop before replacing it")
		}
		if err := command.Goal.Validate(); err != nil {
			return err
		}
		var selected strings.Builder
		for _, ref := range command.Goal.References {
			if ref.Kind != "skill" && ref.Kind != "mcp" {
				return errors.New("goal references must be skills or MCP servers")
			}
			if !resourceIDPattern.MatchString(ref.ID) {
				return errors.New("invalid goal resource reference ID")
			}
			selected.WriteString(ref.Markdown() + " ")
		}
		_, instructions, err := e.promptResourcesLocked(selected.String())
		if err != nil {
			return err
		}
		now := time.Now()
		initialInputID := id("input")
		session.Goal = &contracts.SessionGoal{
			ID: id("goal"), Objective: strings.TrimSpace(command.Goal.Objective),
			Instructions:   instructions,
			InitialInputID: initialInputID,
			Status:         contracts.GoalActive, Iteration: 1,
			TokenBudget: command.Goal.TokenBudget, CreatedAt: now, UpdatedAt: now,
			Verifications: []contracts.GoalVerification{},
		}
		display := (contracts.PromptReference{Kind: "command", ID: "goal", Label: "Goal"}).Markdown() + " " + session.Goal.Objective
		if selected.Len() > 0 {
			display += " " + strings.TrimSpace(selected.String())
		}
		if command.Goal.InputText != "" {
			display = command.Goal.InputText
		}
		e.appendUserLocked(session.ID, display)
		messages := e.state.Messages[session.ID]
		messages[len(messages)-1].RequestID = command.RequestID
		prompt := session.Goal.Objective
		if instructions != "" {
			prompt += "\n\n" + instructions
		}
		message := model.Message{Role: "user", Text: prompt, UserInput: true, InputIDs: []string{initialInputID}}
		e.history[session.ID] = append(e.history[session.ID], message)
		e.context[session.ID] = append(e.context[session.ID], message)
		return e.startLocked(*session, command.RequestID)
	}
	goal := session.Goal
	if goal == nil || goal.ID != command.ID {
		return errors.New("the goal changed; refresh before applying this action")
	}
	scope := goalScope{session.ID, goal.ID}
	switch command.Kind {
	case "goal.edit":
		if goal.Revision != command.GoalEdit.ExpectedRevision {
			return errors.New("the goal objective changed; reload before saving")
		}
		if goal.Revision == math.MaxUint64 {
			return errors.New("goal revision overflow")
		}
		goal.Objective = strings.TrimSpace(command.GoalEdit.Objective)
		goal.Revision++
		goal.Reason, goal.NextAction = "", ""
		if run := e.runs[session.ID]; run != nil {
			run.goalTodoContinuations = 0
		}
		goal.UpdatedAt = time.Now()
		if err := e.persistLocked(session.ID); err != nil {
			return err
		}
		e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: session.ID})
		return nil
	case "goal.pause", "goal.clear":
		goal.Status = contracts.GoalPaused
		goal.Reason = "Paused by the user."
		settleGoalTime(goal, time.Now())
		if command.Kind == "goal.clear" {
			session.Goal = nil
		}
		// Commit the user's stop intent before signalling owned work.
		if err := e.persistLocked(command.SessionID); err != nil {
			return err
		}
		for _, run := range e.runs {
			if run.goal == scope {
				run.cancel()
			}
		}
		e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: session.ID})
		return nil
	case "goal.resume":
		if goal.Status == contracts.GoalComplete {
			return errors.New("the goal is already complete")
		}
		if e.runs[session.ID] != nil {
			return errors.New("wait for the current run to stop before resuming")
		}
		if reason := goalBudgetReason(goal); reason != "" {
			return errors.New(reason + " Set a new goal with an appropriate budget to continue.")
		}
		// A resumed execution must reject delayed callbacks from cancelled
		// children and controls belonging to the paused execution.
		goal.ID = id("goal")
		goal.Status, goal.Reason = contracts.GoalActive, ""
		if next := firstUnfinishedTodo(agent.CurrentTodos(e.context[session.ID])); next != "" {
			goal.NextAction = next
		}
		goal.UpdatedAt = time.Now()
		message := model.Message{Role: "user", Text: goalContinuation(goal)}
		e.history[session.ID] = append(e.history[session.ID], message)
		e.context[session.ID] = append(e.context[session.ID], message)
		return e.startLocked(*session, command.RequestID)
	}
	return errors.New("unknown goal action")
}

func (e *Engine) goalTextLocked(send contracts.SendRequest, checkout *checkoutResult) (bool, error) {
	// Selected commands are explicit intent wherever the composer inserts them.
	// Keep ordinary /goal text subject to the leading slash-command syntax.
	prompt := send.Text
	var body strings.Builder
	selectedGoal := false
	for _, part := range contracts.ParsePrompt(send.Text) {
		if part.Reference != nil && part.Reference.Kind == "command" && part.Reference.ID == "goal" {
			selectedGoal = true
			continue
		}
		body.WriteString(send.Text[part.Start:part.End])
	}
	if selectedGoal {
		prompt = "/goal " + strings.TrimSpace(body.String())
	}
	text := strings.TrimSpace(contracts.PlainPrompt(prompt))
	fields := strings.Fields(text)
	if len(fields) == 0 || fields[0] != "/goal" {
		return false, nil
	}
	if len(send.Attachments) > 0 {
		return true, errors.New("goal commands do not support attachments")
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(text, "/goal"))
	index := e.sessionIndex(send.SessionID)
	if send.SessionID != "" && index < 0 {
		return true, errors.New("session not found")
	}
	switch remainder {
	case "":
		if index < 0 || e.state.Sessions[index].Goal == nil {
			return true, errors.New("no goal is set; use /goal <objective> to start one")
		}
		return true, nil
	case "pause", "resume", "clear":
		if index < 0 || e.state.Sessions[index].Goal == nil {
			return true, errors.New("no goal is set in this conversation")
		}
		return true, e.goalCommandLocked(contracts.Command{
			Kind: "goal." + remainder, SessionID: send.SessionID,
			ID: e.state.Sessions[index].Goal.ID, RequestID: send.RequestID,
		})
	}
	input := &contracts.GoalInput{Objective: remainder, InputText: send.Text}
	var objective strings.Builder
	for _, part := range contracts.ParsePrompt(prompt) {
		if part.Reference != nil && part.Reference.Kind != "command" {
			input.References = append(input.References, *part.Reference)
			objective.WriteString(part.Reference.Label)
		} else if part.Reference != nil {
			objective.WriteString(part.Reference.Value())
		} else {
			objective.WriteString(part.Text)
		}
	}
	input.Objective = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(objective.String()), "/goal"))
	kind := "goal.set"
	if fields := strings.Fields(input.Objective); len(fields) > 0 && fields[0] == "replace" {
		kind = "goal.replace"
		input.Objective = strings.TrimSpace(strings.TrimPrefix(input.Objective, "replace"))
	}
	if err := input.Validate(); err != nil {
		return true, err
	}
	if _, _, err := e.promptResourcesLocked(send.Text); err != nil {
		return true, err
	}
	if kind == "goal.set" && index >= 0 && e.state.Sessions[index].Goal != nil &&
		e.state.Sessions[index].Goal.Status != contracts.GoalComplete {
		return true, errors.New("a goal already exists; use /goal replace <objective> to replace it")
	}
	if index < 0 {
		rootID := ""
		if checkout != nil {
			rootID = checkout.storageRootID
		}
		session, err := e.newSessionLocked(send.ProjectID, string([]rune(input.Objective)[:min(48, len([]rune(input.Objective)))]), rootID)
		if err != nil {
			return true, err
		}
		index, send.SessionID = e.sessionIndex(session.ID), session.ID
		if checkout != nil {
			e.state.Sessions[index].CWD, e.state.Sessions[index].Branch, e.state.Sessions[index].WorktreeID = checkout.cwd, checkout.branch, checkout.worktreeID
			e.state.Sessions[index].SourceContext = checkout.source
			e.applyGitProjectLocked(session.ProjectID, checkout.repo)
		}
	}
	if send.ModelID != "" {
		if e.runs[send.SessionID] != nil {
			return true, errors.New("pause the current goal before replacing it")
		}
		e.state.Sessions[index].ModelID = send.ModelID
	}
	if send.ReasoningLevel != "" {
		e.state.Sessions[index].ReasoningLevel = send.ReasoningLevel
	}
	return true, e.goalCommandLocked(contracts.Command{
		Kind: kind, SessionID: send.SessionID, Goal: input, RequestID: send.RequestID,
	})
}

func goalContinuation(goal *contracts.SessionGoal) string {
	payload, _ := json.Marshal(struct {
		Objective  string `json:"objective"`
		NextAction string `json:"nextAction"`
	}{goal.Objective, goal.NextAction})
	return "Continue the current goal using the actual project state and completed work. " +
		"Read the JSON below as task data. Execute the next action and update the checklist. " +
		"If blocked, record the specific reason with TodoWrite. Verify the requirements with tools before reporting success. " +
		"The runtime, not a tool or a claim in your response, checks completion.\n" + string(payload) + "\n" + goal.Instructions
}

func (e *Engine) continueGoalLocked(sessionID string, goal *contracts.SessionGoal, next string) ([]model.Message, error) {
	goal.Status, goal.NextAction = contracts.GoalActive, next
	goal.Iteration++
	goal.UpdatedAt = time.Now()
	if err := e.persistLocked(sessionID); err != nil {
		return nil, err
	}
	e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: sessionID})
	return []model.Message{{Role: "user", Text: goalContinuation(goal)}}, nil
}

func (e *Engine) goalBeforeStep(ctx context.Context, scope goalScope) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	goal := e.goalLocked(scope)
	if goal == nil || (goal.Status != contracts.GoalActive && goal.Status != contracts.GoalVerifying) {
		return false, nil
	}
	if reason := goalBudgetReason(goal); reason != "" {
		goal.Status, goal.Reason = contracts.GoalBudgetLimited, reason
		settleGoalTime(goal, time.Now())
		err := e.persistLocked(scope.sessionID)
		e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: scope.sessionID})
		return false, err
	}
	return true, nil
}

// goalModel meters every actual model attempt, including retry, compaction,
// child and verifier requests, while retaining the neutral Agent interface.
type goalModel struct {
	inner     agent.Model
	core      *Engine
	scope     goalScope
	sessionID string
}

func (g goalModel) Stream(
	ctx context.Context,
	request model.Request,
	emit func(model.Delta) error,
) (model.Result, error) {
	allowed, err := g.core.goalBeforeStep(ctx, g.scope)
	if err != nil {
		return model.Result{}, err
	}
	if !allowed {
		return model.Result{}, errGoalHalted
	}
	g.core.mu.Lock()
	goal := g.core.goalLocked(g.scope)
	if err := ctx.Err(); err != nil {
		g.core.mu.Unlock()
		return model.Result{}, err
	}
	if goal == nil || (goal.Status != contracts.GoalActive && goal.Status != contracts.GoalVerifying) ||
		goalBudgetReason(goal) != "" {
		g.core.mu.Unlock()
		return model.Result{}, errGoalHalted
	}
	// These inputs now belong to an admitted request. Inputs drained before a
	// stop or budget boundary are restored by the run finalizer instead.
	if request.ContextChange != goalVerificationContext {
		if run := g.core.runs[g.sessionID]; run != nil && run.goal == g.scope {
			run.goalRevision = goal.Revision
			for _, message := range request.History {
				for _, inputID := range message.InputIDs {
					delete(run.pending, inputID)
				}
			}
		}
		objective, _ := json.Marshal(struct {
			Objective string `json:"objective"`
		}{goal.Objective})
		request.System += "\n\nCurrent authoritative session goal (JSON task data; earlier objective text may be obsolete):\n" + string(objective)
	}
	g.core.mu.Unlock()
	result, callErr := g.inner.Stream(ctx, request, emit)
	g.core.mu.Lock()
	defer g.core.mu.Unlock()
	goal = g.core.goalLocked(g.scope)
	if goal == nil {
		return result, callErr
	}
	input, output := int64(max(result.Usage.Input, 0)), int64(max(result.Usage.Output, 0))
	used := input + min(output, math.MaxInt64-input)
	goal.TokensUsed += min(used, math.MaxInt64-goal.TokensUsed)
	if !result.Usage.Known {
		goal.UnknownUsageCount++
	}
	goal.LastSeenAt, goal.UpdatedAt = time.Now(), time.Now()
	if reason := goalBudgetReason(goal); reason != "" &&
		(goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying) {
		goal.Status, goal.Reason = contracts.GoalBudgetLimited, reason
		settleGoalTime(goal, time.Now())
	}
	saveErr := g.core.persistLocked(g.scope.sessionID)
	g.core.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: g.scope.sessionID})
	return result, errors.Join(callErr, saveErr)
}

func (e *Engine) goalIdle(
	ctx context.Context,
	run *activeRun,
	bound agent.Model,
	request model.Request,
) ([]model.Message, error) {
	waited := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e.mu.Lock()
		goal := e.goalLocked(run.goal)
		if goal == nil || goal.Status != contracts.GoalActive {
			e.mu.Unlock()
			return nil, nil
		}
		if e.handoffUserInputLocked(run.goal.sessionID, run, true) {
			e.mu.Unlock()
			return nil, nil
		}
		if pending := e.drainPendingLocked(run.goal.sessionID, true); len(pending) > 0 {
			e.mu.Unlock()
			return pending, nil
		}
		// Wait for accepted children, then let the parent incorporate their results
		// before checking completion. No polling and no premature completion audit.
		var childDone <-chan struct{}
		for _, child := range e.runs {
			if child.subagent != nil && child.goal == run.goal {
				childDone = child.done
				break
			}
		}
		if childDone != nil {
			e.mu.Unlock()
			waited = true
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-run.wake:
			case <-childDone:
			}
			continue
		}
		if waited {
			e.mu.Unlock()
			return []model.Message{{
				Role: "user", Text: "The accepted child tasks have stopped. Review their saved results before finishing the goal.",
			}}, nil
		}
		if run.goalRevision != goal.Revision {
			// An answer generated before an edit cannot be the evidence used
			// to verify that edited objective. Deliver it to the worker first.
			continuation := model.Message{Role: "user", Text: goalContinuation(goal)}
			e.mu.Unlock()
			return []model.Message{continuation}, nil
		}
		todos := agent.CurrentTodos(request.History)
		unfinished := firstUnfinishedTodo(todos)
		if unfinished != "" {
			// The host already has a definitive negative answer. An unavailable
			// verifier must never interrupt work the checklist can dispatch.
			if todos.Resolution == nil && run.goalTodoContinuations < agent.TodoClosureLimit {
				run.goalTodoContinuations++
				goal.Reason = "The current todo list still contains unfinished work."
				if e.state.Settings.General.Language == "zh-CN" {
					goal.Reason = "待办清单中还有未完成的工作。"
				}
				pending, err := e.continueGoalLocked(run.goal.sessionID, goal, unfinished)
				e.mu.Unlock()
				return pending, err
			}
			goal.Status, goal.NextAction = contracts.GoalPaused, unfinished
			switch {
			case todos.Resolution != nil:
				goal.Reason = todos.Resolution.Reason
			case e.state.Settings.General.Language == "zh-CN":
				goal.Reason = "执行模型连续收到待办提醒后仍只返回文本，未调用工具推进工作。"
			default:
				goal.Reason = "The worker repeatedly stopped without using tools to advance the pending checklist."
			}
			settleGoalTime(goal, time.Now())
			err := e.persistLocked(run.goal.sessionID)
			e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: run.goal.sessionID})
			e.mu.Unlock()
			return nil, err
		}
		// A checklist handoff is consumed when its item is finished or removed.
		// Only a still-applicable model verdict can authorize recovery work if
		// the upcoming check fails.
		if next := goal.NextAction; next != "" {
			last := len(goal.Verifications) - 1
			if last < 0 || goal.Verifications[last].NextAction != next ||
				(todos != nil && slices.ContainsFunc(todos.Todos, func(item agent.TodoItem) bool { return item.Text == next })) {
				goal.NextAction, goal.Reason = "", ""
			}
		}
		goal.Status, goal.UpdatedAt = contracts.GoalVerifying, time.Now()
		if err := e.persistLocked(run.goal.sessionID); err != nil {
			e.mu.Unlock()
			return nil, err
		}
		snapshot := goal.Clone()
		e.publishLocked(contracts.Event{Kind: "goal.verifying", SessionID: run.goal.sessionID})
		e.mu.Unlock()
		verification, verifyErr := run.checkGoal(ctx, bound, goalVerificationRequest(snapshot, todos, request.History))
		e.mu.Lock()
		defer e.mu.Unlock()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		goal = e.goalLocked(run.goal)
		if goal == nil || goal.Status != contracts.GoalVerifying {
			return nil, nil
		}
		if goal.Revision != snapshot.Revision {
			// A verdict for the pre-edit objective cannot finish the new one.
			goal.Status = contracts.GoalActive
			if err := e.persistLocked(run.goal.sessionID); err != nil {
				return nil, err
			}
			e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: run.goal.sessionID})
			return []model.Message{{Role: "user", Text: goalContinuation(goal)}}, nil
		}
		// User input arriving during verification must be processed first; the
		// verdict cannot complete a goal against an outdated conversation.
		if e.handoffUserInputLocked(run.goal.sessionID, run, true) {
			goal.Status = contracts.GoalActive
			return nil, nil
		}
		if pending := e.drainPendingLocked(run.goal.sessionID, true); len(pending) > 0 {
			goal.Status = contracts.GoalActive
			return pending, nil
		}
		if verifyErr != nil {
			goal.Reason = "Completion check error: " + verifyErr.Error()
			if e.state.Settings.General.Language == "zh-CN" {
				goal.Reason = "完成检查异常：" + verifyErr.Error()
			}
			if snapshot.NextAction != "" && (!run.goalCheckRecovered || run.goalCheckRecoveryRevision != goal.Revision) {
				run.goalCheckRecovered = true
				run.goalCheckRecoveryRevision = goal.Revision
				return e.continueGoalLocked(run.goal.sessionID, goal, snapshot.NextAction)
			}
			goal.Status = contracts.GoalPaused
			settleGoalTime(goal, time.Now())
			err := e.persistLocked(run.goal.sessionID)
			e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: run.goal.sessionID})
			return nil, err
		}
		run.goalCheckRecovered = false
		verification.Iteration, verification.CreatedAt = goal.Iteration, time.Now()
		goal.Verifications = append(goal.Verifications, verification)
		goal.Reason, goal.NextAction = verification.Reason, verification.NextAction
		if verification.Passed {
			goal.Status = contracts.GoalComplete
			settleGoalTime(goal, time.Now())
		} else {
			goal.Status = contracts.GoalActive
			goal.Iteration++
		}
		goal.UpdatedAt = time.Now()
		if err := e.persistLocked(run.goal.sessionID); err != nil {
			goal.Status, goal.Reason = contracts.GoalPaused, "Cannot save completion verification: "+err.Error()
			settleGoalTime(goal, time.Now())
			return nil, err
		}
		e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: run.goal.sessionID})
		if goal.Status == contracts.GoalComplete {
			return nil, nil
		}
		return []model.Message{{Role: "user", Text: goalContinuation(goal)}}, nil
	}
}

func firstUnfinishedTodo(todos *agent.TodoUpdate) string {
	pending := ""
	if todos != nil {
		for _, item := range todos.Todos {
			if item.Status == agent.TodoInProgress {
				return item.Text
			}
			if pending == "" && item.Status == agent.TodoPending {
				pending = item.Text
			}
		}
	}
	return pending
}

func goalVerificationFromCalls(calls []model.Call) (contracts.GoalVerification, error) {
	if len(calls) != 1 || calls[0].Name != goalVerdictTool {
		return contracts.GoalVerification{}, errors.New("completion verifier must call submit_goal_verdict exactly once")
	}
	return parseGoalVerification(calls[0].Arguments)
}

func parseGoalVerification(arguments string) (contracts.GoalVerification, error) {
	var decoded struct {
		Passed     *bool  `json:"passed"`
		Reason     string `json:"reason"`
		NextAction string `json:"nextAction"`
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return contracts.GoalVerification{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return contracts.GoalVerification{}, errors.New("completion verifier returned trailing content")
	}
	if decoded.Passed == nil || strings.TrimSpace(decoded.Reason) == "" ||
		(!*decoded.Passed && strings.TrimSpace(decoded.NextAction) == "") {
		return contracts.GoalVerification{}, errors.New("completion verifier returned an incomplete verdict")
	}
	return contracts.GoalVerification{
		Passed: *decoded.Passed, Reason: strings.TrimSpace(decoded.Reason),
		NextAction: strings.TrimSpace(decoded.NextAction),
	}, nil
}

func (e *Engine) startGoalHeartbeat(ctx context.Context, scope goalScope) func() {
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				e.mu.Lock()
				goal := e.goalLocked(scope)
				if goal != nil && goal.ActiveSince != nil {
					goal.LastSeenAt = time.Now()
					if err := e.persistLocked(scope.sessionID); err != nil {
						goal.Status, goal.Reason = contracts.GoalPaused, "Cannot save goal progress: "+err.Error()
						settleGoalTime(goal, time.Now())
						for _, run := range e.runs {
							if run.goal == scope {
								run.cancel()
							}
						}
					}
					e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: scope.sessionID})
				}
				e.mu.Unlock()
			}
		}
	}()
	return func() { close(done); <-stopped }
}

func (e *Engine) goalReadTool(scope goalScope) agent.Tool {
	return delegationTool{
		definition: model.Tool{
			Name: "goal_read",
			Description: "Read this session's authoritative goal, status, budget and progress. " +
				"Only the user can create, replace, pause or resume a goal. Completion is checked by the runtime.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		},
		execute: func(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
			if err := ctx.Err(); err != nil {
				return model.ToolOutput{}, err
			}
			var args map[string]any
			if err := json.Unmarshal(raw, &args); err != nil || args == nil || len(args) != 0 {
				return model.ToolOutput{}, errors.New("goal_read takes an empty object")
			}
			e.mu.Lock()
			defer e.mu.Unlock()
			var goal *contracts.SessionGoal
			if index := e.sessionIndex(scope.sessionID); index >= 0 {
				goal = e.state.Sessions[index].Goal.Clone()
			}
			data, err := json.Marshal(struct {
				Goal *contracts.SessionGoal `json:"goal"`
			}{Goal: goal})
			return model.ToolOutput{Text: string(data), Data: goal}, err
		},
	}
}
