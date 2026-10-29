package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
)

type sideComposerState struct {
	composerControls
	configured     bool
	configSequence uint64
	configPending  map[uint64]bool
	Draft          string
	Attachments    []contracts.Attachment
	Input          ui.Handle
	Surface        ui.Handle
	Pending        *sidePendingSend
	Failure        string
	Importing      int
	Sequence       uint64
	FocusPending   bool
	FooterUsage    composerUsageState
}

type sidePendingSend struct {
	RequestID, Draft string
	Attachments      []contracts.Attachment
	Queued           bool
}

func (a *App) sideComposer(sessionID string) *sideComposerState {
	if a.sideComposers == nil {
		a.sideComposers = make(map[string]*sideComposerState)
	}
	if a.sideComposers[sessionID] == nil {
		a.sideComposers[sessionID] = &sideComposerState{}
	}
	return a.sideComposers[sessionID]
}

func (a *App) dismissSideComposer(id string) {
	if state := a.sideComposers[id]; state != nil {
		state.modelMenu = false
		state.modelListPage = false
		state.modelQuery = ""
		state.FocusPending = false
	}
}

func (a *App) sideCommandFinished(command contracts.Command, err error) bool {
	if command.Kind == "session.update" && command.Session != nil {
		state := a.sideComposers[command.Session.ID]
		if state != nil && state.configPending[command.Version] {
			delete(state.configPending, command.Version)
			if err != nil {
				state.Failure = err.Error()
				state.configured = false
			}
			return true
		}
	}
	if command.Kind != "message.send" || command.Send == nil {
		return false
	}
	state := a.sideComposers[command.Send.SessionID]
	if state == nil || state.Pending == nil || state.Pending.RequestID != command.Send.RequestID {
		return false
	}
	pending := state.Pending
	state.Pending = nil
	if err != nil {
		state.Failure = err.Error()
		return true
	}
	state.Failure = ""
	if state.Draft == pending.Draft {
		state.Draft = ""
	}
	kept := make([]contracts.Attachment, 0, len(state.Attachments))
	for _, item := range state.Attachments {
		sent := false
		for _, submitted := range pending.Attachments {
			if submitted.ID == item.ID {
				sent = true
				break
			}
		}
		if !sent {
			kept = append(kept, item)
		}
	}
	state.Attachments = kept
	return true
}

func (a *App) sendSideMessage(session contracts.Session, state *sideComposerState, text string) bool {
	if state.Pending != nil || state.Importing > 0 || (strings.TrimSpace(text) == "" && len(state.Attachments) == 0) {
		return false
	}
	state.Sequence++
	id := fmt.Sprintf("side:%s:%d", session.ID, state.Sequence)
	state.Pending = &sidePendingSend{RequestID: id, Draft: text, Attachments: append([]contracts.Attachment(nil), state.Attachments...), Queued: session.Status == contracts.Working || session.Status == contracts.AwaitingInput}
	state.Failure = ""
	access := state.accessMode
	if access == "" {
		access = contracts.AccessFullAccess
	}
	a.dispatch(contracts.Command{Kind: "message.send", ProjectID: session.ProjectID, Send: &contracts.SendRequest{
		RequestID: id, SessionID: session.ID, ProjectID: session.ProjectID, DeviceID: session.DeviceID,
		ModelID: state.modelID, ReasoningLevel: state.reasoning, AccessMode: access, Plan: state.plan,
		Branch: session.Branch, WorktreeID: session.WorktreeID, Text: strings.TrimSpace(text), Attachments: append([]contracts.Attachment(nil), state.Attachments...),
	}})
	return state.Failure == ""
}

// sideTranscript mounts the same native message renderer in a workspace tab.
// Scroll and fold identities belong to this pane, so opening an agent never
// navigates the central conversation or replaces its draft and selection.
func (a *App) sideTranscript(c *ui.Context, sessionID string) {
	var session contracts.Session
	for _, candidate := range a.state.Sessions {
		if candidate.ID == sessionID {
			session = candidate
			break
		}
	}
	if session.ID == "" {
		return
	}
	state := a.sideComposer(sessionID)
	if len(state.configPending) == 0 {
		state.modelID, state.reasoning, state.accessMode, state.plan = session.ModelID, session.ReasoningLevel, session.AccessMode, session.Plan
		state.configured = true
	}
	messages := transcriptEntries(a.state.Messages[sessionID])
	for i := range messages {
		messages[i].ID = "side:" + sessionID + ":" + messages[i].ID
	}
	rows := len(messages)
	if _, _, working := a.workingState(sessionID); rows == 0 && working {
		rows = 1
	}
	a.prepareTranscriptMarkdown(c)
	root := ui.Column(c).Fill().Key("side-transcript:" + sessionID)
	list := ui.Local(root, "messages", func() ui.ListState { return ui.ListState{FollowEnd: true} })
	list.Key = func(i int) any {
		if i == len(messages) {
			return "side:" + sessionID + ":working-trailer"
		}
		return messages[i].ID
	}
	if !a.reduceMotion(c) && a.state.Settings.Appearance.SmoothScrolling {
		list.FollowAnimation = &ui.Smooth
	} else {
		list.FollowAnimation = nil
	}
	root.Children(func() {
		ui.List(c, list, rows, func(i int) {
			c.Key(list.Key(i))
			ui.Column(c).FillWidth().Padding(0, 16).Children(func() {
				if i < len(messages) {
					a.message(c, messages[i])
				}
				if i == rows-1 {
					a.workingTrailer(c, sessionID)
				}
			})
		}).Grow(1).FillWidth().Gap(16).Padding(20, 0)
		a.sideQueuedMessages(c, sessionID, state)
		a.sideComposerView(c, session, state)
	})
}

func (a *App) sideComposerView(c *ui.Context, session contracts.Session, state *sideComposerState) {
	controls := a.sideControls(session.ID, state)
	viewport, _ := c.Size()
	columns := allocatePanels(viewport, a.sidebarWidth, a.filesWidth, a.rightWidth, a.fit, a.rightExpanded)
	transcriptWidth := float32(a.state.Settings.Appearance.TranscriptWidth)
	if transcriptWidth <= 0 {
		transcriptWidth = 736
	}
	outerWidth := composerOuterWidth(columns.Right, transcriptWidth, false)
	ui.Column(c).FillWidth().MaxWidth(outerWidth).AlignSelf(ui.Center).Padding(0, 16, 16, 16).Gap(8).Children(func() {
		a.nativeComposerSurface(c, composerSurfaceProps{
			draft: &state.Draft, input: &state.Input, surface: &state.Surface, controls: controls,
			inputLabel: tr(c, "Message to ") + session.Title, attachLabel: tr(c, "Attach files to ") + session.Title,
			sendLabel: tr(c, "Send message to ") + session.Title, stopLabel: tr(c, "Stop response in ") + session.Title,
			running:   session.Status == contracts.Working || session.Status == contracts.AwaitingInput,
			canSend:   state.Pending == nil && state.Importing == 0 && state.modelID != "" && composerHasContent(state.Draft, len(state.Attachments)),
			widthHint: max(0, outerWidth-32),
			autofocus: true, focusPending: &state.FocusPending, stopWithEscape: a.state.Settings.General.StopWithEscape,
			renderStaged: func() {
				if len(state.Attachments) > 0 {
					ui.Row(c).Wrap().Padding(12, 16, 0, 16).Gap(6).Children(func() {
						for _, item := range state.Attachments {
							c.Key(item.ID)
							ui.Row(c).AlignItems(ui.Center).Children(func() {
								a.attachmentChip(c, item, false)
								iconButton(c, "close", tr(c, "Remove side attachment: ")+item.Name).Size(20, 20).OnClick(func() {
									for i, candidate := range state.Attachments {
										if candidate.ID == item.ID {
											state.Attachments = append(state.Attachments[:i], state.Attachments[i+1:]...)
											break
										}
									}
								})
							})
						}
					})
				}
			},
			onSend: func(text string) bool { return a.sendSideMessage(session, state, text) }, onStop: func() { a.stopSideMessage(session.ID) },
			onChooseAttach: func() { a.sideAttachmentWork(session.ID, state, contracts.AttachmentImportRequest{}, true) },
			onPaste:        func(services ui.Services) bool { return a.pasteSideAttachments(session.ID, state, services) },
			onFilesDrop: func(paths []string) {
				a.sideAttachmentWork(session.ID, state, contracts.AttachmentImportRequest{Paths: paths}, false)
			},
			onMention: func(file workbench.FileDrag) { state.Draft += " @" + file.Path },
		})
		ui.Row(c).FillWidth().Height(composerFooterHeight).Margin(0, 0, -8, 0).Justify(ui.End).Padding(0, 10, 0, 4).Children(func() {
			a.composerUsage(c, session, &state.FooterUsage, true)
		})
		if state.Failure != "" {
			ui.Text(c, state.Failure).FontSize(design.UISize(c, 12)).TextColor(a.palette.Danger).Margin(6, 12, 0, 12)
		}
	})
}

func (a *App) stopSideMessage(id string) {
	for _, session := range a.state.Sessions {
		if session.ID == id && (session.Status == contracts.Working || session.Status == contracts.AwaitingInput) {
			a.dispatch(contracts.Command{Kind: "message.stop", SessionID: id})
			return
		}
	}
}

func (a *App) sideQueuedMessages(c *ui.Context, id string, state *sideComposerState) {
	for _, queued := range a.state.Queue {
		if queued.SessionID != id {
			continue
		}
		q := queued
		ui.Row(c).FillWidth().Padding(6, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, "clock-circle", 14)
			ui.Text(c, q.Text).SingleLine().Grow(1).FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted)
			iconButton(c, "pen", tr(c, "Edit queued side message")).OnClick(func() {
				state.Draft = q.Text
				state.Attachments = append([]contracts.Attachment(nil), q.Attachments...)
				state.FocusPending = true
				a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID})
			})
			iconButton(c, "close", tr(c, "Remove queued side message")).OnClick(func() { a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID}) })
		})
	}
}
