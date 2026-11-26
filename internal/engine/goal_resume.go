package engine

import (
	"errors"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func withdrawGoalResume(goal *contracts.SessionGoal) {
	goal.Status, goal.ResumeRequestID, goal.UpdatedAt = contracts.GoalPaused, "", time.Now()
}

func (e *Engine) goalResumeBusyLocked(sessionID string) bool {
	if e.runs[sessionID] != nil {
		return true
	}
	// An ordinary worker can settle before its background child returns.
	// Let that worker consume the result under its original task ownership.
	for _, run := range e.runs {
		if run.subagent != nil && run.subagent.parentSessionID == sessionID && run.goal.id == "" {
			return true
		}
	}
	return false
}

// startPendingGoalLocked runs at an idle boundary under the engine lock.
// Previously accepted queue entries get their ordinary run before the goal.
func (e *Engine) startPendingGoalLocked(session contracts.Session) error {
	if started, err := e.handoffNextQueuedLocked(session.ID, false); started {
		return err
	}
	previous := session.Goal
	if reason := goalBudgetReason(previous); reason != "" {
		return errors.New(reason)
	}
	goal := previous.Clone()
	goal.ID = id("goal")
	goal.Status, goal.Reason, goal.ResumeRequestID = contracts.GoalActive, "", ""
	goal.UpdatedAt = time.Now()
	history, executionContext := e.history[session.ID], e.context[session.ID]
	todos, checkpoint := agent.ResumeGoalTodos(history, goal.InitialInputID)
	if next := firstUnfinishedTodo(todos); next != "" {
		goal.NextAction = next
	}
	message := model.Message{Role: "user", Text: goalContinuation(goal)}
	e.history[session.ID] = append(history, checkpoint, message)
	e.context[session.ID] = append(executionContext, checkpoint, message)
	index := e.sessionIndex(session.ID)
	session.Goal, e.state.Sessions[index].Goal = goal, goal
	if err := e.startLocked(session, previous.ResumeRequestID); err != nil {
		e.state.Sessions[index].Goal = previous
		e.history[session.ID], e.context[session.ID] = history, executionContext
		return err
	}
	return nil
}

// After a task settles, consume a pending resume once. A failed admission
// leaves the original goal and its checkpoint available for an explicit retry.
func (e *Engine) settlePendingGoalResumeLocked(sessionID string, handoffErr error) {
	index := e.sessionIndex(sessionID)
	if e.closed || index < 0 {
		return
	}
	session := e.state.Sessions[index]
	if session.Goal == nil || session.Goal.Status != contracts.GoalResumePending {
		return
	}
	if handoffErr == nil {
		if e.goalResumeBusyLocked(sessionID) {
			return
		}
		handoffErr = e.startPendingGoalLocked(session)
		if handoffErr == nil {
			return
		}
	}
	// A failed queue handoff can restore the entire snapshot.
	goal := e.state.Sessions[index].Goal
	withdrawGoalResume(goal)
	goal.Reason = "Could not resume the goal: " + handoffErr.Error()
	if e.state.Settings.General.Language == "zh-CN" {
		goal.Reason = "目标恢复失败：" + handoffErr.Error()
	}
	if goalBudgetReason(goal) != "" {
		goal.Status = contracts.GoalBudgetLimited
	}
	saveErr := e.persistLocked(sessionID)
	e.publishLocked(contracts.Event{Kind: "goal.changed", SessionID: sessionID, Error: errors.Join(handoffErr, saveErr).Error()})
}
