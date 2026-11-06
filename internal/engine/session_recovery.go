package engine

import "gcode/internal/contracts"

// reconcileSessionLocked 恢复刚载入的会话，而不重新执行可能已发生的副作用。
// 启动载入与离线盘重新挂载必须使用同一规则，避免留下无实际执行者的运行状态。
func (e *Engine) reconcileSessionLocked(sessionID string) {
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return
	}
	dirty := false
	session := &e.state.Sessions[index]
	if session.ModelRetry != nil {
		session.ModelRetry = nil
		dirty = true
	}
	if session.RunningSubagents != 0 {
		session.RunningSubagents = 0
		dirty = true
	}
	if goal := session.Goal; goal != nil {
		if goal.ActiveSince != nil {
			lastSeen := goal.LastSeenAt
			if lastSeen.Before(*goal.ActiveSince) {
				lastSeen = *goal.ActiveSince
			}
			settleGoalTime(goal, lastSeen)
			dirty = true
		}
		if goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying {
			goal.Status = contracts.GoalPaused
			goal.Reason = "The host stopped; inspect the saved work before resuming."
			dirty = true
		}
	}
	if session.Status == contracts.Working || session.Status == contracts.AwaitingInput {
		session.Status = contracts.Idle
		session.LastOutcome = "interrupted"
		dirty = true
	}
	for i := range e.state.Runs {
		run := &e.state.Runs[i]
		if run.SessionID == sessionID && run.Status == "running" {
			run.Status = "interrupted"
			run.FinishReason = "engineRestarted"
			dirty = true
		}
	}
	for i := range e.state.Terminals {
		terminal := &e.state.Terminals[i]
		if terminal.SessionID == sessionID && terminal.Status == "running" {
			terminal.Status = "interrupted"
			dirty = true
		}
	}
	messages := e.state.Messages[sessionID]
	for i := range messages {
		if messages[i].Streaming {
			messages[i].Streaming = false
			dirty = true
		}
		for j := range messages[i].Blocks {
			block := &messages[i].Blocks[j]
			if block.Question != nil && !block.Question.Resolved {
				block.Question.Resolved = true
				dirty = true
			}
			if tool := block.Tool; tool != nil && tool.Status == "running" {
				tool.Status = "interrupted"
				dirty = true
			}
			if child := block.Subagent; child != nil && child.Status == "running" {
				child.Status = "interrupted"
				child.Summary = "Execution was interrupted when the host stopped."
				dirty = true
			}
		}
	}
	e.state.Messages[sessionID] = messages
	history := closeInterruptedCalls(e.history[sessionID])
	if len(history) != len(e.history[sessionID]) {
		dirty = true
	}
	e.history[sessionID] = history
	executionContext := closeInterruptedCalls(e.context[sessionID])
	if len(executionContext) != len(e.context[sessionID]) {
		dirty = true
	}
	e.context[sessionID] = executionContext
	if dirty {
		e.markSessionDirtyLocked(sessionID)
	}
}
