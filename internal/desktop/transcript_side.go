package desktop

import (
	"crypto/rand"
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
	FocusPending   bool
	FooterUsage    composerUsageState
}

type sidePendingSend struct {
	RequestID, Draft string
	Attachments      []contracts.Attachment
	Queued           bool
	Steer            bool
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
		state.effortMenu = false
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
	if session.SubagentType != "" {
		state.Failure = a.t("Subagent conversations are read-only.")
		return false
	}
	if state.Pending != nil || state.Importing > 0 || (strings.TrimSpace(text) == "" && len(state.Attachments) == 0) {
		return false
	}
	id := "side:" + rand.Text()
	busy := session.Status == contracts.Working || session.Status == contracts.AwaitingInput
	delivery := ""
	if busy && a.activeTurnSend() == "steer" {
		delivery = "steer"
	}
	state.Pending = &sidePendingSend{RequestID: id, Draft: text, Attachments: append([]contracts.Attachment(nil), state.Attachments...), Queued: busy, Steer: delivery == "steer"}
	state.Failure = ""
	a.dispatch(contracts.Command{Kind: "message.send", ProjectID: session.ProjectID, Send: &contracts.SendRequest{
		RequestID: id, SessionID: session.ID, ProjectID: session.ProjectID, DeviceID: session.DeviceID,
		ModelID: state.modelID, ReasoningLevel: state.reasoning, Delivery: delivery,
		Branch: session.Branch, WorktreeID: session.WorktreeID, Text: strings.TrimSpace(text), Attachments: append([]contracts.Attachment(nil), state.Attachments...),
	}})
	return state.Failure == ""
}

// sideTranscriptCache returns the projection cache of a side pane, kept with
// the pane so its rows are not re-projected on every frame.
func (a *App) sideTranscriptCache(sessionID string) *transcriptCache {
	if a.sideTranscripts == nil {
		a.sideTranscripts = map[string]*transcriptCache{}
	}
	cache := a.sideTranscripts[sessionID]
	if cache == nil {
		cache = &transcriptCache{}
		a.sideTranscripts[sessionID] = cache
	}
	return cache
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
		state.modelID, state.reasoning = session.ModelID, session.ReasoningLevel
		state.configured = true
	}
	// The pane's rows are its own: joined and projected like the central
	// transcript's, with identities prefixed so a fold opened here does not
	// open in the conversation behind it.
	cache := a.sideTranscriptCache(sessionID)
	entries := cache.entriesFor(a.state.Revision, sessionID, a.state.Messages[sessionID], a.state.Settings.General.CompactMode)
	messages := make([]contracts.Message, len(entries))
	copy(messages, entries)
	for i := range messages {
		messages[i].ID = "side:" + sessionID + ":" + messages[i].ID
	}
	conversation := cache.conversationFor(messages, a.state.Settings.General.CompactMode)
	if _, _, working := a.workingState(sessionID); len(conversation) == 0 && working {
		conversation = []conversationRow{{Key: "side:" + sessionID + ":working", Message: -1}}
	}
	a.prepareTranscriptMarkdown(c)
	root := ui.Column(c).Fill().Key("side-transcript:" + sessionID)
	list := ui.Local(root, "messages", func() ui.ListState { return ui.ListState{FollowEnd: true} })
	list.Key = func(i int) any { return conversation[i].Key }
	a.transcriptListScope(c, root, list, sessionID, func() {
		a.goalBanner(c, session)
		a.sessionStorageNotice(c, session)
		pane := transcriptPane{cache: cache, scope: "side:" + sessionID, session: sessionID}
		a.transcriptScope(c, pane.scope, func() {
			ui.List(c, list, len(conversation), func(i int) {
				a.transcriptRowView(c, pane, messages, conversation, i, transcriptRowLayout{padding: 16, nested: true})
			}).Grow(1).FillWidth().Gap(0).Padding(20, 0)
		})
		if session.SubagentType != "" {
			ui.Row(c).FillWidth().Padding(12, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				label := session.ModelID
				for _, candidate := range a.state.Models {
					if candidate.ID == session.ModelID {
						label = a.runtimeModelLabel(candidate)
					}
				}
				if session.ReasoningLevel != "" {
					label += " · " + tr(c, session.ReasoningLevel)
				}
				ui.Text(c, label).Grow(1).SingleLine().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				if session.Status == contracts.Working || session.Status == contracts.AwaitingInput {
					textButton(c, tr(c, "Stop response")).Label(tr(c, "Stop response in ") + session.Title).OnClick(func() { a.stopSideMessage(session.ID) })
				}
			})
			return
		}
		a.sideQueuedMessages(c, sessionID, state)
		a.sideComposerView(c, session, state)
	})
}

func (a *App) sideComposerView(c *ui.Context, session contracts.Session, state *sideComposerState) {
	if session.SubagentType != "" {
		return
	}
	controls := a.sideControls(session.ID, state)
	viewport, _ := c.Size()
	columns := allocatePanels(viewport, a.shownSidebarWidth(), a.filesWidth, a.rightWidth, a.fit, a.rightExpanded)
	transcriptWidth := float32(a.state.Settings.Appearance.TranscriptWidth)
	if transcriptWidth <= 0 {
		transcriptWidth = 736
	}
	outerWidth := composerOuterWidth(columns.Right, transcriptWidth, false)
	ui.Column(c).FillWidth().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(outerWidth).AlignSelf(ui.Center).Padding(0, 16, 16, 16).Gap(8).Children(func() {
			running := session.Status == contracts.Working || session.Status == contracts.AwaitingInput
			canSend := state.Pending == nil && state.Importing == 0 && state.modelID != "" && composerHasContent(state.Draft, len(state.Attachments))
			mode := composerSend
			switch {
			case running && !canSend:
				mode = composerStop
			case running && a.activeTurnSend() == "queue":
				mode = composerQueue
			case running:
				mode = composerSteer
			}
			a.nativeComposerSurface(c, composerSurfaceProps{
				keyPrefix: "side:" + session.ID + ":",
				draft:     &state.Draft, input: &state.Input, surface: &state.Surface, controls: controls,
				inputLabel: tr(c, "Message to ") + session.Title, attachLabel: tr(c, "Attach files to ") + session.Title,
				sendLabel: tr(c, "Send message to ") + session.Title, stopLabel: tr(c, "Stop response in ") + session.Title,
				steerLabel: "Steer the running response", queueLabel: "Queue for the next turn", sendMode: mode,
				running:   running,
				canSend:   canSend,
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
				a.conversationMetricsView(c, session.ID)
				a.composerUsage(c, session, &state.FooterUsage, true)
			})
			if state.Failure != "" {
				ui.Text(c, state.Failure).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger).Margin(6, 12, 0, 12)
			}
		})
		// The central conversation keeps a status strip below its composer;
		// the pane reserves the same strip so both pills share one bottom
		// line even though only the conversation ever writes to it.
		ui.Row(c).Height(composerStatusHeight)
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
		row := ui.Row(c).FillWidth().Padding(6, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, "queue-drag-handle", 14)
			ui.Text(c, q.Text).SingleLine().Grow(1).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			if a.state.Runtime != nil {
				iconButton(c, "queue-send", tr(c, "Send now (interrupt)")).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "queue.send", ID: q.ID})
				})
			}
			iconButton(c, "pen", tr(c, "Edit queued side message")).OnClick(func() {
				state.Draft = q.Text
				state.Attachments = append([]contracts.Attachment(nil), q.Attachments...)
				state.FocusPending = true
				a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID})
			})
			iconButton(c, "close", tr(c, "Remove queued side message")).OnClick(func() { a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID}) })
		})
		a.queueDragTarget(c, row, q.SessionID, q.ID)
	}
}
