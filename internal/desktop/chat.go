package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/linktarget"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"strings"
	"unicode/utf8"
)

const (
	composerBottomInset      = float32(24)
	transcriptListTopPadding = float32(28)
)

func (a *App) chatView(c *ui.Context) {
	composerFocused := a.composer.Focused(c)
	for _, side := range a.sideComposers {
		composerFocused = composerFocused || side.Input.Focused(c)
	}
	if a.state.Settings.General.StopWithEscape && !composerFocused {
		c.OnShortcut(0, ui.KeyEscape, a.stopRun)
	}
	messages := a.transcriptCache.entriesFor(a.state.Revision, a.sessionID, a.state.Messages[a.sessionID], a.state.Settings.General.CompactMode)
	rows := a.transcriptCache.conversationFor(messages, a.state.Settings.General.CompactMode)
	reserve, _ := a.infoVisibility()

	// The blank canvas is the route with no selected session. A send starts
	// the handoff immediately (launchingNewSession) instead of waiting for
	// the host to mint the session, so the composer answers the key at once.
	docked := a.sessionID != "" || a.launchingNewSession
	reduced := a.reduceMotion(c)
	frame := a.composerDock.tick(c.Now(), docked, reduced)
	if reduced {
		// Without motion the pill switches modes in one frame: the handoff
		// must not morph its height over the hidden clock.
		a.composerHandoff = composerHandoff{}
	} else if frame.Flipped && frame.Active && a.composerPillHeight > 0 {
		a.composerHandoff = composerHandoff{
			Amount: 1, Height: a.composerPillHeight,
			Expanded: a.composerExpanded, Hero: !frame.Previous,
		}
	} else if a.composerHandoff.Amount > 0 {
		if a.composerHandoff.Hero {
			a.composerHandoff.Amount = 1 - frame.Amount
		} else {
			a.composerHandoff.Amount = frame.Amount
		}
		if !frame.Active {
			a.composerHandoff.Amount = 0
		}
	}
	if frame.Active {
		c.AnimationFrame()
	}
	a.composerFrame = frame

	root := ui.Column(c).Bind(&a.conversationViewport).Grow(1).FillHeight().AlignItems(ui.Stretch).Clip()
	a.paintConversationBackground(c, root, 1-frame.Dissolve)
	a.transcriptListScope(c, root, &a.messages, a.sessionID, func() {
		ui.Column(c).FillWidth().Padding(0, reserve, 0, 0).Children(func() {
			a.sessionStorageNotice(c, a.currentSession())
			if a.expanded["conversation-find"] {
				ui.Row(c).FillWidth().Padding(4, 12).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c, &a.conversationQuery).Placeholder(tr(c, "Find in conversation")).Label(tr(c, "Find in conversation")).Grow(1)
					matches := a.conversationMatches(&a.transcriptCache, messages, rows, a.conversationQuery)
					ui.Text(c, fmt.Sprintf(tr(c, "%d matches"), len(matches))).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
					move := func(delta int) {
						if len(matches) > 0 {
							a.findIndex = (a.findIndex + delta + len(matches)) % len(matches)
							a.messages.ScrollIntoView(matches[a.findIndex])
						}
					}
					iconButton(c, "alt-arrow-up", tr(c, "Previous match")).OnClick(func() { move(-1) })
					iconButton(c, "alt-arrow-down", tr(c, "Next match")).OnClick(func() { move(1) })
					iconButton(c, "close", tr(c, "Close find")).OnClick(func() { a.expanded["conversation-find"] = false })
				})
			}
		})
		if docked {
			transcript := ui.Column(c).Key("chat-transcript").Grow(1).FillWidth().MinHeight(0)
			if frame.Transcript < .999 {
				transcript.Opacity(frame.Transcript)
			}
			transcript.Children(func() {
				if len(messages) == 0 {
					ui.Spacer(c).Grow(1)
					return
				}
				a.messages.Key = func(i int) any { return rows[i].Key }
				a.messages.FollowEnd = true
				a.prepareTranscriptMarkdown(c)
				width := float32(a.state.Settings.Appearance.TranscriptWidth)
				if width <= 0 {
					width = 736
				}
				pane := transcriptPane{cache: &a.transcriptCache, scope: a.sessionID, session: a.sessionID}
				a.transcriptScope(c, a.sessionID, func() {
					// Reserve the card inside the content; the scroll viewport
					// and its scrollbar still reach the conversation pane edge.
					ui.List(c.Key("chat-transcript-list"), &a.messages, len(rows), func(i int) {
						a.transcriptRowView(c, pane, messages, rows, i, transcriptRowLayout{width: width, padding: 24})
					}).Grow(1).FillWidth().Gap(0).Padding(transcriptListTopPadding, reserve, 24, 0)
				})
				a.messageRail(c, a.transcriptCache.rail)
				if !a.messages.AtEnd() {
					ui.Row(c).FillWidth().Padding(0, reserve, 0, 0).Justify(ui.Center).Children(func() {
						iconButton(c.Key("transcript-latest"), "arrow-down", tr(c, "Back to latest")).OnClick(func() { a.messages.ScrollToEnd() })
					})
				}
				ui.Column(c).FillWidth().Padding(0, reserve, 0, 0).Children(func() {
					a.queueRows(c, a.sessionID)
				})
			})
		} else {
			ui.Spacer(c).Grow(1)
		}
		a.composerView(c, !docked)
		if frame.Amount < .999 {
			// The blank canvas centers the composer between the transcript
			// area and an equal flexible band below it. The band collapses
			// with the dock clock, so the block travels to the bottom in
			// layout: hit testing, carets and overlays follow it exactly, and
			// a content-height change in the same frame cannot make it lag.
			ui.Spacer(c).Grow(1 - frame.Amount).MinHeight(0)
		}
		ui.Box(c).Height(composerBottomInset)
	})
}

// userMessage renders a user message's bubble: its blocks, attachments and
// annotations. The bubble shares the transcript's selection scope rather than
// opening its own, so a selection crosses its blocks the way the list's rows
// do.
func (a *App) userMessage(c *ui.Context, m contracts.Message) {
	bubble := ui.Hex("#000000").Alpha(.04)
	if a.palette.Dark {
		bubble = ui.Hex("#ffffff").Alpha(.08)
	}
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		maxWidth := min(float32(a.state.Settings.Appearance.TranscriptWidth), a.chatWidth) * .8
		if maxWidth <= 0 {
			maxWidth = 590
		}
		root := ui.Column(c).MaxWidth(maxWidth).Padding(10, 16).Radius(16).Background(bubble).Gap(8)
		root.Children(func() {
			for _, b := range m.Blocks {
				c.Key(b.ID)
				if b.Text != "" {
					foldable := strings.Count(b.Text, "\n") >= 5 || utf8.RuneCountInString(b.Text) > 400
					expanded := a.expanded["user:"+m.ID]
					body := ui.Column(c).Clip()
					if foldable && !expanded {
						body.MaxHeight(110)
					}
					body.Children(func() { markdown.View(c, linktarget.NormalizeMarkdown(b.Text), a.transcriptOptions(m.SessionID, true)) })
					if foldable {
						name, label := "alt-arrow-down", "Expand message"
						if expanded {
							name, label = "alt-arrow-up", "Collapse message"
						}
						disclosureIconButton(c, name, tr(c, label)).OnClick(func() { a.expanded["user:"+m.ID] = !a.expanded["user:"+m.ID] })
					}
				}
				if b.Attachment != nil {
					if strings.HasPrefix(b.Attachment.MIME, "image/") {
						a.imageAttachment(c, *b.Attachment)
					} else {
						a.attachmentChip(c, *b.Attachment, false)
					}
				}
				if b.Annotation != nil {
					item := *b.Annotation
					textButton(c, a.annotationLabel(item.Index)).OnClick(func() { a.workspace.FocusAnnotation(item.Index) })
				}
			}
		})
	})
}

func (a *App) composerView(c *ui.Context, hero bool) {
	width := float32(a.state.Settings.Appearance.TranscriptWidth)
	if width <= 0 {
		width = 736
	}
	reserve, _ := a.infoVisibility()
	frame := a.composerFrame
	heroWidth := composerOuterWidth(a.chatWidth-reserve, width, true)
	threadWidth := composerOuterWidth(a.chatWidth-reserve, width, false)
	outerWidth := threadWidth + (heroWidth-threadWidth)*frame.Amount
	ui.Row(c).Key("main-composer").FillWidth().Padding(0, reserve, 0, 0).Justify(ui.Center).Children(func() {
		block := ui.Column(c).Key("main-composer-block").FillWidth().MaxWidth(outerWidth).Padding(0, 16, 16, 16)
		block.Children(func() {
			if a.questionComposer(c, a.sessionID, "main:"+a.sessionID) {
				return
			}
			if selectors := frame.Selectors; selectors > .001 {
				// The blank canvas's target selectors leave before the pill
				// travels; keeping them in one clipped container lets their
				// height collapse with their opacity instead of jumping.
				ui.Column(c).Key("composer-selectors").FillWidth().Height(20*selectors).Margin(0, 0, 8*selectors, 0).Clip().Children(func() {
					ui.Row(c).FillWidth().Height(20).Padding(0, 10).Justify(ui.End).Gap(8).Opacity(selectors).Children(func() {
						a.projectPicker(c)
						a.devicePicker(c)
					})
				})
			}
			running := a.currentSession().Status == contracts.Working
			canSend := !a.pendingNew && !a.composerFooterGitBusy() && a.modelID != "" && (a.sessionID != "" || !a.modelNoAgents()) && a.attachmentImports[a.attachmentScope()] == 0 && composerHasContent(a.draft, len(a.attachments)+len(a.annotations))
			// The reference's SendButtonMode: Stop only with an empty
			// composer; typing during a run sends by the setting.
			mode := composerSend
			switch {
			case running && !canSend:
				mode = composerStop
			case running && a.activeTurnSend() == "queue":
				mode = composerQueue
			case running:
				mode = composerSteer
			}
			a.composerAttachedPanels(c, a.currentSession(), "main")
			a.nativeComposerSurface(c, composerSurfaceProps{
				draft: &a.draft, input: &a.composer, surface: &a.composerSurface, controls: a.centralControls(),
				inputLabel: "Message", attachLabel: "Attach files", sendLabel: "Send message", stopLabel: "Stop response",
				steerLabel: "Steer the running response", queueLabel: "Queue for the next turn", sendMode: mode,
				running:        running,
				stopWithEscape: a.state.Settings.General.StopWithEscape,
				canSend:        canSend,
				hero:           hero,
				handoff:        a.composerHandoff,
				widthHint:      max(0, outerWidth-32),
				renderStaged: func() {
					if len(a.attachments) > 0 || len(a.annotations) > 0 {
						ui.Row(c).Wrap().Padding(12, 16, 0, 16).Gap(6).Children(func() {
							a.annotationChips(c)
							for _, attachment := range a.attachments {
								a.attachmentChip(c, attachment, true)
							}
						})
					}
				},
				onSend: a.send, onStop: func() { a.dispatch(contracts.Command{Kind: "message.stop", SessionID: a.sessionID}) },
				onChooseAttach: a.chooseAttachments, onPaste: a.pasteAttachments,
				onFilesDrop: func(paths []string) { a.importAttachments(contracts.AttachmentImportRequest{Paths: paths}) },
				onMention:   func(file workbench.FileDrag) { a.draft += " @" + file.Path },
			})
			ui.Box(c).Height(8).Shrink(0)
			ui.Row(c).Key("composer-footer").FillWidth().Height(composerFooterHeight).Margin(0, 0, -8, 0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Children(func() { a.composerFooterGit(c) })
				a.conversationMetricsView(c, a.sessionID)
				if footer := frame.Footer; footer > .001 {
					ui.Row(c).Shrink(0).Padding(0, 10, 0, 4).Opacity(footer).Children(func() { a.composerUsage(c, a.currentSession(), &a.footerUsage, false) })
				}
			})
		})
	})
}
func (a *App) projectPicker(c *ui.Context) {
	group := controlComposerTargetGroup(c.Key("composer-project-picker-group"))
	clearable := !a.projectMenu && a.sessionID == "" && !a.pendingNew && a.projectID != "" && (group.Hovered() || group.FocusWithin())
	group.Children(func() {
		if clearable {
			controlComposerTargetClear(c.Key("composer-project-clear"), tr(c, "Don't work in a project")).OnClick(a.clearComposerProject)
		} else {
			ui.Box(c.Key("composer-project-folder")).Size(design.Controls().ComposerTargetIconSize, design.Controls().ComposerTargetIconSize).
				Children(func() { icon(c, "folder", design.Controls().ComposerTargetIconSize) })
		}
		b := controlComposerTargetPicker(c.Key("composer-project-picker"), tr(c, "Select project")+": "+a.projectLabel()).Children(func() {
			ui.Text(c, a.projectLabel()).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			icon(c, "alt-arrow-down", 10)
		})
		b.OnClick(a.toggleProjectMenu)
	})
	a.projectPopover(c, group)
}
func (a *App) devicePicker(c *ui.Context) {
	a.reconcileComposerCloudDevice()
	b := ui.ButtonBase(c).Padding(3, 4).Radius(5).Gap(5).Label(tr(c, "Select device")).Children(func() {
		icon(c, "monitor", 12)
		ui.Text(c, a.deviceLabel()).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		icon(c, "alt-arrow-down", 10)
	})
	b.OnClick(func() { a.deviceMenu = !a.deviceMenu })
	ui.Popover(c, b, &a.deviceMenu, func() {
		for _, device := range a.state.Devices {
			d := device
			if footerCloudDevice(d.ID) {
				continue
			}
			menuChoice(c, d.Name, a.deviceID == d.ID, func() { a.pickComposerDevice(d.ID) })
		}
		if project, found := a.footerGitProject(); found && footerCloudRepositoryAvailable(a.state, project) {
			menuChoice(c, tr(c, "Cloud"), footerCloudDevice(a.deviceID), func() { a.pickComposerDevice(a.composerCloudDeviceID()) })
		}
	})
}
func (a *App) attachmentChip(c *ui.Context, item contracts.Attachment, removable bool) {
	ui.Row(c).Padding(6, 8).Radius(8).Border(1, a.palette.Border).Gap(6).AlignItems(ui.Center).Children(func() {
		image := loadMedia(item.AssetRef)
		if strings.HasPrefix(item.MIME, "image/") && (image.bitmap != nil || image.svg != nil) {
			drawMedia(c, image, 24, 24).Radius(4)
		} else {
			icon(c, "document", 14)
		}
		ui.Text(c, item.Name).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
		if item.Status == "uploading" || item.Status == "transferring" {
			ui.Spinner(c).Size(12, 12)
		}
		if removable {
			chipDismissButton(c, tr(c, "Remove ")+item.Name).OnClick(func() {
				for i, x := range a.attachments {
					if x.ID == item.ID {
						a.attachments = append(a.attachments[:i], a.attachments[i+1:]...)
						break
					}
				}
			})
		}
	})
}
