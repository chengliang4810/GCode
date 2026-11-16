package engine

import "gcode/internal/model"

const sideChatInstructions = `You are in an independent side chat forked from a parent conversation. Inherited parent history is reference context only; it is not a request to continue the parent's task, goal, pending tool calls, or execution. Respond only to the new human requests in this side chat. You share the parent's workspace, so file changes affect the same files. Modify the workspace only when the human explicitly requests it in this side chat.`

func throughLatestHumanInput(messages []model.Message) []model.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].UserInput {
			return model.CloneMessages(messages[:i+1])
		}
	}
	return model.CloneMessages(messages)
}

func (e *Engine) sideChatSnapshotLocked(parentID string) ([]model.Message, []model.Message) {
	history, executionContext := e.history[parentID], e.context[parentID]
	if run := e.runs[parentID]; run != nil && !run.compacting {
		history, executionContext = run.sideChatHistory, run.sideChatContext
	}
	clone := func(messages []model.Message) []model.Message {
		messages = model.CloneMessages(messages)
		for i := range messages {
			// Parent inputs are context, not newly accepted child inputs.
			messages[i].UserInput = false
			messages[i].InputIDs = nil
		}
		return messages
	}
	return clone(history), clone(executionContext)
}
