package desktop

import (
	"crypto/rand"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type interruptedEditorState struct {
	open, hasAttachment                    bool
	session, message, text, requestID, err string
}

func (a *App) interruptedMessageAction(c *ui.Context, m contracts.Message) {
	if m.SessionID != a.sessionID || m.Role != "user" || a.hasLocalSend(m.SessionID) {
		return
	}
	messages := a.state.Messages[m.SessionID]
	at := contracts.InterruptedTurnMessage(a.currentSession(), messages, a.state.Runs)
	if at < 0 || messages[at].ID != m.ID {
		return
	}
	iconButton(c.Key("edit-interrupted:"+m.ID), "pen", tr(c, "Edit stopped message")).OnClick(func() {
		state := interruptedEditorState{open: true, session: m.SessionID, message: m.ID}
		for _, b := range m.Blocks {
			if b.Kind == "text" {
				state.text = b.Text
			}
			state.hasAttachment = state.hasAttachment || b.Attachment != nil || b.Annotation != nil
		}
		a.interruptedEditor = state
	})
}

func (a *App) interruptedMessageDialog(c *ui.Context) {
	s := &a.interruptedEditor
	if !s.open || s.session != a.sessionID {
		return
	}
	a.settingsEditorDialog(c, "edit-interrupted-message", "Edit stopped message", &s.open, s.requestID != "", 560, s.err, func() {
		controlTextArea(c.Key("interrupted-message-text"), &s.text).FillWidth().Height(160).Label(tr(c, "Message"))
	}, func() {
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c.Key("cancel-interrupted-edit"), tr(c, "Cancel")).OnClick(func() { s.open = false })
			primaryButton(c.Key("resend-interrupted-message"), tr(c, "Replace and resend")).
				Disabled(!s.hasAttachment && strings.TrimSpace(s.text) == "").OnClick(func() {
				s.requestID = rand.Text()
				s.err = ""
				a.dispatch(contracts.Command{Kind: "message.send", ProjectID: a.currentSession().ProjectID, Send: &contracts.SendRequest{
					SessionID: s.session, ReplaceMessageID: s.message, Text: strings.TrimSpace(s.text), RequestID: s.requestID,
				}})
			})
		})
	})
}
