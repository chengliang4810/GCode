package engine

import "gcode/internal/contracts"

// reconcileSessionLocked 恢复刚载入的会话，而不重新执行可能已发生的副作用。
// 启动载入与离线盘重新挂载必须使用同一规则，避免留下无实际执行者的运行状态。
func (e *Engine) reconcileSessionLocked(sessionID string) {
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return
	}
	session := &e.state.Sessions[index]
	session.RunningSubagents = 0
	if goal := session.Goal; goal != nil {
		if goal.ActiveSince != nil {
			lastSeen := goal.LastSeenAt
			if lastSeen.Before(*goal.ActiveSince) {
				lastSeen = *goal.ActiveSince
			}
			settleGoalTime(goal, lastSeen)
		}
		if goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying {
			goal.Status = contracts.GoalPaused
			goal.Reason = "The host stopped; inspect the saved work before resuming."
		}
	}
	if session.Status == contracts.Working || session.Status == contracts.AwaitingInput {
		session.Status = contracts.Idle
		session.LastOutcome = "interrupted"
	}
	for i := range e.state.Runs {
		run := &e.state.Runs[i]
		if run.SessionID == sessionID && run.Status == "running" {
			run.Status = "interrupted"
			run.FinishReason = "engineRestarted"
		}
	}
	for i := range e.state.Terminals {
		terminal := &e.state.Terminals[i]
		if terminal.SessionID == sessionID && terminal.Status == "running" {
			terminal.Status = "interrupted"
		}
	}
	messages := e.state.Messages[sessionID]
	for i := range messages {
		messages[i].Streaming = false
		for j := range messages[i].Blocks {
			block := &messages[i].Blocks[j]
			if block.Question != nil && !block.Question.Resolved {
				block.Question.Resolved = true
			}
			if tool := block.Tool; tool != nil && tool.Status == "running" {
				tool.Status = "interrupted"
			}
			if child := block.Subagent; child != nil && child.Status == "running" {
				child.Status = "interrupted"
				child.Summary = "Execution was interrupted when the host stopped."
			}
		}
	}
	e.state.Messages[sessionID] = messages
	e.history[sessionID] = closeInterruptedCalls(e.history[sessionID])
	e.context[sessionID] = closeInterruptedCalls(e.context[sessionID])
}
