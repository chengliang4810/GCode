package engine

import "gcode/internal/workflow"

func (e *Engine) workflowSessionMode(sessionID string) (mode string, activated bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	mode = workflow.NormalizeMode(e.state.Settings.WorkflowMode)
	if index := e.sessionIndex(sessionID); index >= 0 {
		activated = e.state.Sessions[index].WorkflowActivated
	}
	return mode, activated
}

func (e *Engine) activateWorkflowSessionLocked(sessionID string) bool {
	index := e.sessionIndex(sessionID)
	if index < 0 || e.state.Sessions[index].WorkflowActivated {
		return false
	}
	e.state.Sessions[index].WorkflowActivated = true
	return true
}

func (e *Engine) workflowCurrentMessage(sessionID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	messages := e.state.Messages[sessionID]
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].ID
		}
	}
	return ""
}
