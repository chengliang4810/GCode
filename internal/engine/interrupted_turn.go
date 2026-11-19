package engine

import (
	"errors"
	"slices"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

// The enclosing send transaction restores all three views if the replacement
// cannot be prepared or persisted.
func (e *Engine) replaceInterruptedTurnLocked(send *contracts.SendRequest) error {
	id := send.SessionID
	ix := e.sessionIndex(id)
	if ix < 0 || e.runs[id] != nil || send.Delivery != "" || send.Checkout != nil {
		return errors.New("only the last message from a stopped or failed turn can be edited")
	}
	messages := e.state.Messages[id]
	at := contracts.EditableTurnMessage(e.state.Sessions[ix], messages, e.state.Runs)
	if at < 0 || messages[at].ID != send.ReplaceMessageID {
		return errors.New("only the last message from a stopped or failed turn can be edited")
	}

	var source string
	send.Attachments = nil
	for _, b := range messages[at].Blocks {
		if b.Kind == "text" {
			source = b.Text
		}
		if b.Attachment != nil {
			send.Attachments = append(send.Attachments, *b.Attachment)
		}
	}
	history, view := e.history[id], e.context[id]
	h := lastHumanInput(history)
	if h < 0 || history[h].SourceText != source || len(history[h].InputIDs) != 1 {
		return errors.New("the original message is no longer available in execution history")
	}
	inputID := history[h].InputIDs[0]
	v := lastHumanInput(view)
	if v < 0 || !slices.Equal(view[v].InputIDs, []string{inputID}) ||
		slices.ContainsFunc(view[v+1:], func(m model.Message) bool { return m.Summary }) {
		return errors.New("the original message cannot be edited after context compaction")
	}
	e.history[id] = history[:h:h]
	e.context[id] = view[:v:v]
	e.state.Messages[id] = messages[:at:at]
	return nil
}

func lastHumanInput(messages []model.Message) int {
	for i, m := range slices.Backward(messages) {
		if m.UserInput {
			return i
		}
	}
	return -1
}
