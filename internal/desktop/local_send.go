package desktop

import (
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// localSend is a display projection, never an accepted core message. Keep the
// original command so a failure remains recoverable even after further typing.
type localSend struct {
	command   contracts.Command
	message   contracts.Message
	epoch     uint64
	failure   string
	completed bool
}

func (a *App) stageLocalSend(command contracts.Command) {
	command = command.Clone()
	req := command.Send
	message := contracts.Message{
		ID: "pending:" + req.RequestID, RequestID: req.RequestID, SessionID: req.SessionID,
		Role: "user", CreatedAt: a.now().UTC(),
		Blocks: []contracts.Block{{ID: req.RequestID + ":text", Kind: "text", Text: req.Text}},
	}
	for _, item := range req.Attachments {
		attachment := item
		message.Blocks = append(message.Blocks, contracts.Block{
			ID: req.RequestID + ":attachment:" + item.ID, Kind: "attachment", Attachment: &attachment,
		})
	}
	for i := range req.Annotations {
		annotation := req.Annotations[i]
		message.Blocks = append(message.Blocks, contracts.Block{
			ID: req.RequestID + ":" + annotation.Link(), Kind: "annotation", Annotation: &annotation,
		})
	}
	a.localSends = append(a.localSends, &localSend{command: command, message: message, epoch: a.draftEpoch})
}

func (a *App) localSendByRequest(id string) *localSend {
	for _, send := range a.localSends {
		if send.message.RequestID == id {
			return send
		}
	}
	return nil
}

func (a *App) hasLocalSend(sessionID string) bool {
	for _, send := range a.localSends {
		if send.message.SessionID == sessionID {
			return true
		}
	}
	return false
}

func (a *App) projectedMessages(sessionID string) []contracts.Message {
	messages := a.state.Messages[sessionID]
	for _, send := range a.localSends {
		if send.message.SessionID != sessionID {
			continue
		}
		// A core event can precede the command acknowledgment.
		if slices.ContainsFunc(messages, func(m contracts.Message) bool { return m.RequestID == send.message.RequestID }) ||
			slices.ContainsFunc(a.state.Queue, func(q contracts.QueuedMessage) bool { return q.RequestID == send.message.RequestID }) {
			continue
		}
		messages = append(slices.Clip(messages), send.message)
	}
	return messages
}

func (a *App) localSendFinished(command contracts.Command, err error) {
	if command.Kind != "message.send" || command.Send == nil {
		return
	}
	if send := a.localSendByRequest(command.Send.RequestID); send != nil {
		if err != nil {
			send.failure = err.Error()
		}
	}
}

func (a *App) localSendSnapshotApplied(command contracts.Command, err error) {
	if err != nil || command.Kind != "message.send" || command.Send == nil {
		return
	}
	if send := a.localSendByRequest(command.Send.RequestID); send != nil {
		send.completed = true
		a.reconcileLocalSends()
	}
}

func (a *App) reconcileLocalSends() {
	a.localSends = slices.DeleteFunc(a.localSends, func(send *localSend) bool {
		req := send.command.Send
		if req.SessionID == "" && send.epoch != a.draftEpoch {
			return true
		}
		for _, run := range a.state.Runs {
			if run.RequestID == req.RequestID {
				send.message.SessionID = run.SessionID
				break
			}
		}
		sessionID := send.message.SessionID
		if sessionID != "" && !a.hasSession(sessionID) {
			return true
		}
		for _, message := range a.state.Messages[sessionID] {
			if message.RequestID == req.RequestID {
				return true
			}
		}
		for _, queued := range a.state.Queue {
			if queued.RequestID == req.RequestID {
				return true
			}
		}
		// Once the command's snapshot is applied, an existing session's core
		// state is authoritative even for commands that add no message. A new
		// session keeps its projection until its scoped history arrives.
		_, loaded := a.state.Messages[sessionID]
		return send.completed && (req.SessionID != "" || sessionID != "" && loaded)
	})
}

func (a *App) retryLocalSend(id string) {
	send := a.localSendByRequest(id)
	if send == nil || send.failure == "" {
		return
	}
	req := send.command.Send
	busy := false
	for _, session := range a.state.Sessions {
		if session.ID == req.SessionID {
			busy = session.Status == contracts.Working || session.Status == contracts.AwaitingInput
			break
		}
	}
	side := strings.HasPrefix(req.RequestID, "side:")
	if side {
		state := a.sideComposer(req.SessionID)
		if state.Pending != nil || state.Importing > 0 {
			return
		}
		state.Pending = &sidePendingSend{RequestID: id, Draft: req.Text, Attachments: req.Attachments, Queued: busy, Steer: busy && req.Delivery == "steer"}
		state.Failure = ""
		if state.Draft == req.Text && slices.Equal(state.Attachments, req.Attachments) {
			state.Draft, state.Attachments = "", nil
		}
	} else {
		if _, pending := a.pendingSends[req.SessionID]; pending || a.pendingNew ||
			a.composerFooterGitBusy() || a.attachmentImports[a.attachmentScope()] > 0 {
			return
		}
		if a.pendingSends == nil {
			a.pendingSends = make(map[string]primaryPendingSend)
		}
		a.pendingSends[req.SessionID] = primaryPendingSend{RequestID: id, DraftEpoch: send.epoch, StartedAt: a.now().UTC(), Queued: busy, Steer: busy && req.Delivery == "steer"}
		if req.SessionID == "" {
			a.pendingNew, a.launchingNewSession = true, true
			a.pendingNewRequestID, a.pendingNewEpoch = id, send.epoch
		}
		text := req.Text
		for i := len(req.Annotations) - 1; i >= 0; i-- {
			text = strings.TrimSuffix(text, "\n\n"+req.Annotations[i].Link())
		}
		if a.sessionID == req.SessionID && a.draft == text && slices.Equal(a.attachments, req.Attachments) &&
			slices.EqualFunc(a.annotations, req.Annotations, func(x, y contracts.Annotation) bool { return x.Link() == y.Link() }) {
			a.draft, a.attachments = "", nil
			a.annotations = nil
			a.workspace.ResetAnnotations()
			a.drafts[a.sessionID] = ""
		}
	}
	send.failure = ""
	send.completed = false
	a.dispatch(send.command)
}

func (a *App) localSendStatus(c *ui.Context, message contracts.Message) {
	send := a.localSendByRequest(message.RequestID)
	if send == nil || message.ID != send.message.ID && !strings.HasSuffix(message.ID, ":"+send.message.ID) {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if send.failure == "" {
			ui.Text(c, tr(c, "Sending…")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			return
		}
		ui.Text(c, tr(c, "Send failed")).Tooltip(send.failure).
			FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Danger)
		textButton(c.Key("retry-send:"+message.RequestID), tr(c, "Retry")).OnClick(func() { a.retryLocalSend(message.RequestID) })
		iconButton(c.Key("dismiss-send:"+message.RequestID), "close", tr(c, "Dismiss")).OnClick(func() {
			a.localSends = slices.DeleteFunc(a.localSends, func(candidate *localSend) bool { return candidate == send })
		})
	})
}
