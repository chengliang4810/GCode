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
		if queued.SessionID != sessionID || (!idle && queued.Delivery != "steer") {
			remaining = append(remaining, queued)
			continue
		}
		// The queue hands over one message per turn: the next row waits
		// until the one it follows has run. A steer joins the live turn
		// instead, at the loop's next step.
		if idle && len(messages) > 0 {
			remaining = append(remaining, queued)
			continue
		}
		input := queued.Prompt
		internal := queued.Source == "subagent" || queued.Source == "hook"
		if internal {
			input = queued.Text
		}
		message, err := e.inputMessageLocked(input, queued.Attachments)
		if err != nil {
			message = model.Message{Role: "user", Text: input + "\nAttachment error: " + err.Error()}
		}
		message.UserInput = !internal
		if message.UserInput {
			message.SourceText = queued.Text
		}
		message.InputIDs = nil
		if message.UserInput {
			message.InputIDs = []string{queued.ID}
		}
		messages = append(messages, message)
		if run := e.runs[sessionID]; run != nil && run.goal.id != "" && !internal {
			run.pending[queued.ID] = queued
		}
		if !internal {
			e.appendUserLocked(sessionID, queued.Text)
			if run := e.runs[sessionID]; run != nil && run.goal.id != "" {
				items := e.state.Messages[sessionID]
				items[len(items)-1].ID = queued.ID
			}
		}
	}
	e.state.Queue = remaining
	if len(messages) > 0 {
		e.publishLocked(contracts.Event{Kind: "queue.changed", SessionID: sessionID})
	}
	return messages
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
		if run.afterStop != nil {
			// The response is already stopping for another row; this one stays
			// queued so the next turn keeps it.
			return errors.New("the running response is already stopping")
		}
		e.state.Queue = slices.Delete(e.state.Queue, index, index+1)
		run.afterStop = &queued
		if err := e.persistLocked(queued.SessionID); err != nil {
			run.afterStop = nil
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
			err = e.startLocked(e.state.Sessions[index], "")
		}
	} else {
		err = e.sendLocked(contracts.SendRequest{
			SessionID: sessionID, Text: queued.Text, Attachments: queued.Attachments,
			ModelID: queued.ModelID, ReasoningLevel: queued.ReasoningLevel,
		})
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
	e.state.Messages[sessionID] = slices.DeleteFunc(e.state.Messages[sessionID], func(message contracts.Message) bool {
		_, found := run.pending[message.ID]
		return found
	})
	clear(run.pending)
}
