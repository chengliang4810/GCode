package engine

import (
	"errors"
	"slices"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func (e *Engine) drainPendingLocked(sessionID string, idle bool) []model.Message {
	messages := []model.Message{}
	remaining := make([]contracts.QueuedMessage, 0, len(e.state.Queue))
	for _, queued := range e.state.Queue {
		internal := queued.Source == "subagent" || queued.Source == "hook"
		// Human inputs each open a new task. They are handed off at a safe
		// boundary instead of being folded into this task's statistics.
		if !internal {
			remaining = append(remaining, queued)
			continue
		}
		if queued.SessionID != sessionID || (!idle && queued.Delivery != "steer") {
			remaining = append(remaining, queued)
			continue
		}
		if idle && len(messages) > 0 {
			remaining = append(remaining, queued)
			continue
		}
		input := queued.Text
		message, err := e.inputMessageLocked(input, queued.Attachments)
		if err != nil {
			message = model.Message{Role: "user", Text: input + "\nAttachment error: " + err.Error()}
		}
		message.UserInput = false
		message.InputIDs = nil
		messages = append(messages, message)
	}
	e.state.Queue = remaining
	if len(messages) > 0 {
		e.publishLocked(contracts.Event{Kind: "queue.changed", SessionID: sessionID})
	}
	return messages
}

// A steer takes the next model-step boundary, after the current tool batch.
// Ordinary inputs wait for idle. Neither path cancels an accepted request.
func (e *Engine) handoffUserInputLocked(sessionID string, run *activeRun, idle bool) bool {
	if run.nextInput != nil {
		return true
	}
	if !idle && run.messageID == "" {
		// Every accepted user task gets its first request before a later
		// steer can hand off again.
		return false
	}
	for _, queued := range e.state.Queue {
		if queued.SessionID != sessionID || queued.Source == "subagent" || queued.Source == "hook" ||
			(!idle && queued.Delivery != "steer") {
			continue
		}
		run.nextInput = &queued
		return true
	}
	return false
}

// sendQueuedNowLocked carries out a queue row's Send now: the row leaves the
// queue, the live run is stopped so the message replaces the turn — not the
// agent — and the row opens the next turn once that run has settled.
func (e *Engine) sendQueuedNowLocked(id string) error {
	index := slices.IndexFunc(e.state.Queue, func(queued contracts.QueuedMessage) bool {
		return queued.ID == id
	})
	if index < 0 {
		return errors.New("queued message not found")
	}
	queued := e.state.Queue[index]
	if run := e.runs[queued.SessionID]; run != nil && !e.closed {
		if run.nextInput != nil {
			// A handoff already owns the next boundary; retain this accepted
			// input in the queue without asking the user to wait or stop.
			return nil
		}
		e.state.Queue = slices.Delete(e.state.Queue, index, index+1)
		run.nextInput = &queued
		run.inputInterrupt = true
		if err := e.persistLocked(queued.SessionID); err != nil {
			run.nextInput = nil
			run.inputInterrupt = false
			e.state.Queue = slices.Insert(e.state.Queue, index, queued)
			return err
		}
		e.publishLocked(contracts.Event{Kind: "queue.changed", SessionID: queued.SessionID})
		run.cancel()
		return nil
	}
	e.state.Queue = slices.Delete(e.state.Queue, index, index+1)
	if err := e.handoffQueuedLocked(queued.SessionID, queued); err != nil {
		return err
	}
	e.publishLocked(contracts.Event{Kind: "queue.changed", SessionID: queued.SessionID})
	return nil
}

// handoffQueuedLocked opens a session's next turn with a queue row that has
// already left the queue. The row returns to the queue if the turn cannot
// start, mirroring the automatic handoff at the end of a run.
func (e *Engine) handoffQueuedLocked(sessionID string, queued contracts.QueuedMessage) error {
	before := contracts.CloneSnapshot(e.state)
	history, executionContext := model.CloneMessages(e.history[sessionID]), model.CloneMessages(e.context[sessionID])
	internal := queued.Source == "subagent" || queued.Source == "hook"
	var err error
	if internal {
		e.history[sessionID] = append(e.history[sessionID], model.Message{Role: "user", Text: queued.Text})
		e.context[sessionID] = append(e.context[sessionID], model.Message{Role: "user", Text: queued.Text})
		index := e.sessionIndex(sessionID)
		if index < 0 {
			err = errors.New("session not found")
		} else {
			err = e.startAutomaticLocked(e.state.Sessions[index], "")
		}
	} else {
		err = e.startQueuedUserLocked(sessionID, queued)
	}
	if err != nil {
		e.state = before
		e.history[sessionID], e.context[sessionID] = history, executionContext
		e.state.Queue = append(e.state.Queue, queued)
		if queued.Source == "subagent" {
			e.failSubagentContinuationLocked(sessionID, err)
		} else {
			_ = e.persistLocked(sessionID)
		}
	}
	return err
}

func (e *Engine) startQueuedUserLocked(sessionID string, queued contracts.QueuedMessage) error {
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return errors.New("session not found")
	}
	session := e.state.Sessions[index]
	var selected, reasoning *string
	if queued.ModelID != "" {
		selected = &queued.ModelID
	}
	if queued.ReasoningLevel != "" {
		reasoning = &queued.ReasoningLevel
	}
	if err := e.setSessionModelLocked(&session, selected, reasoning); err != nil {
		return err
	}
	prompt := queued.Prompt
	if prompt == "" {
		var err error
		prompt, err = expandPrompt(e.storage.ConfigDirectory(), session.CWD, queued.Text)
		if err != nil {
			return err
		}
	}
	message, err := e.inputMessageLocked(prompt, queued.Attachments)
	if err != nil {
		return err
	}
	message.SourceText, message.InputIDs = queued.Text, []string{queued.ID}
	e.history[sessionID] = append(e.history[sessionID], message)
	e.context[sessionID] = append(e.context[sessionID], message)
	e.appendUserLocked(sessionID, queued.Text)
	messages := e.state.Messages[sessionID]
	messages[len(messages)-1].ID = queued.ID
	messages[len(messages)-1].RequestID = queued.RequestID
	for _, item := range queued.Attachments {
		attachment := item
		messages[len(messages)-1].Blocks = append(messages[len(messages)-1].Blocks, contracts.Block{ID: id("block"), Kind: "attachment", Attachment: &attachment})
	}
	if err := e.startLocked(session, queued.ID); err != nil {
		return err
	}
	if run := e.runs[sessionID]; run != nil && run.goal.id != "" {
		run.pending[queued.ID] = queued
	}
	return nil
}

// moveQueuedLocked reorders a queued row ahead of or behind another row of
// the same session.
func (e *Engine) moveQueuedLocked(in contracts.ReorderInput) error {
	if in.SourceID == "" || in.TargetID == "" {
		return errors.New("queued message id is required")
	}
	if in.SourceID == in.TargetID {
		return nil
	}
	from := slices.IndexFunc(e.state.Queue, func(queued contracts.QueuedMessage) bool {
		return queued.ID == in.SourceID
	})
	to := slices.IndexFunc(e.state.Queue, func(queued contracts.QueuedMessage) bool {
		return queued.ID == in.TargetID
	})
	if from < 0 || to < 0 {
		return errors.New("queued message not found")
	}
	queued := e.state.Queue[from]
	if e.state.Queue[to].SessionID != queued.SessionID {
		return errors.New("queued messages belong to different sessions")
	}
	e.state.Queue = slices.Delete(e.state.Queue, from, from+1)
	if from < to {
		to--
	}
	if in.After {
		to++
	}
	to = min(max(to, 0), len(e.state.Queue))
	e.state.Queue = slices.Insert(e.state.Queue, to, queued)
	if err := e.persistLocked(queued.SessionID); err != nil {
		return err
	}
	e.publishLocked(contracts.Event{Kind: "queue.changed", SessionID: queued.SessionID})
	return nil
}

func (e *Engine) wakeRunLocked(sessionID string) {
	if run := e.runs[sessionID]; run != nil && run.wake != nil {
		select {
		case run.wake <- struct{}{}:
		default:
		}
	}
}

func (e *Engine) restoreGoalInputsLocked(sessionID string, run *activeRun) {
	if len(run.pending) == 0 {
		return
	}
	for _, item := range run.pending {
		e.state.Queue = append(e.state.Queue, item)
	}
	slices.SortStableFunc(e.state.Queue, func(a, b contracts.QueuedMessage) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	e.history[sessionID] = slices.DeleteFunc(e.history[sessionID], func(message model.Message) bool {
		return slices.ContainsFunc(message.InputIDs, func(id string) bool { _, found := run.pending[id]; return found })
	})
	// A compacted summary can contain both earlier work and an input that
	// never reached the executor. Rebuild from the complete saved history
	// instead of deleting that summary and losing the earlier work.
	e.context[sessionID] = model.CloneMessages(e.history[sessionID])
	delete(e.contextMemories, sessionID)
	e.state.Messages[sessionID] = slices.DeleteFunc(e.state.Messages[sessionID], func(message contracts.Message) bool {
		_, found := run.pending[message.ID]
		return found
	})
	clear(run.pending)
}
