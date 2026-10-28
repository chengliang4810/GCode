package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"strings"
	"unicode/utf8"
)

func (a *App) chatView(c *ui.Context) {
	if a.state.Settings.General.StopWithEscape {
		c.OnShortcut(0, ui.KeyEscape, a.stopRun)
	}
	messages := transcriptEntries(a.state.Messages[a.sessionID])
	reserve, _ := a.infoVisibility()
	root := ui.Column(c).Grow(1).FillHeight().AlignItems(ui.Stretch).Padding(0, reserve, 0, 0).Clip()
	if len(messages) == 0 {
		a.paintConversationBackground(c, root)
	}
	root.Children(func() {
		if a.expanded["conversation-find"] {
			ui.Row(c).FillWidth().Padding(4, 12).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.conversationQuery).Placeholder("Find in conversation").Label("Find in conversation").Grow(1)
				matches := conversationMatches(messages, a.conversationQuery)
				ui.Text(c, fmt.Sprintf("%d matches", len(matches))).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
				move := func(delta int) {
					if len(matches) > 0 {
						a.findIndex = (a.findIndex + delta + len(matches)) % len(matches)
						a.messages.ScrollIntoView(matches[a.findIndex])
					}
				}
				iconButton(c, "alt-arrow-up", "Previous match").OnClick(func() { move(-1) })
				iconButton(c, "alt-arrow-down", "Next match").OnClick(func() { move(1) })
				iconButton(c, "close", "Close find").OnClick(func() { a.expanded["conversation-find"] = false })
			})
		}
		if len(messages) == 0 {
			ui.Spacer(c).Grow(1)
			a.composerView(c, true)
			ui.Spacer(c).Grow(1)
		} else {
			a.messages.Key = func(i int) any { return messages[i].ID }
			a.messages.FollowEnd = true
			if !a.reduceMotion(c) && a.state.Settings.Appearance.SmoothScrolling {
				a.messages.FollowAnimation = &ui.Smooth
			} else {
				a.messages.FollowAnimation = nil
			}
			a.prepareTranscriptMarkdown(c)
			ui.List(c, &a.messages, len(messages), func(i int) {
				m := messages[i]
				c.Key(m.ID)
				ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
					width := float32(a.state.Settings.Appearance.TranscriptWidth)
					if width <= 0 {
						width = 736
					}
					ui.Column(c).FillWidth().MaxWidth(width).Padding(0, 24).Children(func() {
						a.message(c, m)
						if i == len(messages)-1 {
							a.workingTrailer(c, a.sessionID)
						}
					})
				})
			}).Grow(1).Gap(16).Padding(28, 0, 24, 0)
			if !a.messages.AtEnd() {
				ui.Row(c).FillWidth().Justify(ui.Center).Children(func() { iconButton(c, "arrow-down", "Back to latest").OnClick(func() { a.messages.ScrollToEnd() }) })
			}
			for _, queued := range a.state.Queue {
				if queued.SessionID == a.sessionID {
					q := queued
					ui.Row(c).Padding(6, 24).Gap(10).AlignItems(ui.Center).Children(func() {
						icon(c, "clock-circle", 14)
						ui.Text(c, q.Text).Grow(1).SingleLine().TextColor(a.palette.Muted)
						iconButton(c, "pen", "Edit queued message").OnClick(func() {
							a.draft = q.Text
							a.attachments = append([]contracts.Attachment(nil), q.Attachments...)
							a.annotations = append([]contracts.Annotation(nil), q.Annotations...)
							a.accessMode, a.plan = q.AccessMode, q.Plan
							if q.ModelID != "" {
								a.modelID = q.ModelID
							}
							if q.ReasoningLevel != "" {
								a.reasoning = q.ReasoningLevel
							}
							if q.DeviceID != "" {
								a.deviceID = q.DeviceID
							}
							if q.Branch != "" {
								a.branch = q.Branch
							}
							if q.WorktreeID != "" {
								a.checkout = "New worktree"
							} else {
								a.checkout = "Current checkout"
							}
							a.normalizeReasoning()
							a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID})
						})
						iconButton(c, "close", "Remove queued message").OnClick(func() { a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID}) })
					})
				}
			}
			a.composerView(c, false)
		}
		ui.Row(c).Height(24).Padding(0, 24).Gap(6).AlignItems(ui.Center).Children(func() {
			session := a.currentSession()
			if session.Status == contracts.Errored {
				ui.Text(c, "Run failed").FontSize(design.UISize(c, 11)).TextColor(a.palette.Danger)
			}
		})
	})
}

func (a *App) message(c *ui.Context, m contracts.Message) {
	if m.Role == "user" {
		bubble := ui.Hex("#000000").Alpha(.04)
		if a.palette.Dark {
			bubble = ui.Hex("#ffffff").Alpha(.08)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			maxWidth := min(float32(a.state.Settings.Appearance.TranscriptWidth), a.chatWidth) * .8
			if maxWidth <= 0 {
				maxWidth = 590
			}
			ui.Column(c).MaxWidth(maxWidth).Padding(10, 16).Radius(16).Background(bubble).Gap(8).Children(func() {
				for _, b := range m.Blocks {
					c.Key(b.ID)
					if b.Text != "" {
						foldable := strings.Count(b.Text, "\n") >= 5 || utf8.RuneCountInString(b.Text) > 400
						expanded := a.expanded["user:"+m.ID]
						body := ui.Column(c).Clip()
						if foldable && !expanded {
							body.MaxHeight(110)
						}
						body.Children(func() { markdown.View(c, b.Text, &a.userMD) })
						if foldable {
							name, label := "alt-arrow-down", "Expand message"
							if expanded {
								name, label = "alt-arrow-up", "Collapse message"
							}
							iconButton(c, name, label).Size(20, 16).OnClick(func() { a.expanded["user:"+m.ID] = !a.expanded["user:"+m.ID] })
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
						textButton(c, item.Label()).OnClick(func() { a.workspace.FocusAnnotation(item.Index) })
					}
				}
			})
		})
		return
	}
	a.assistantTranscript(c, m)
}

func (a *App) composerView(c *ui.Context, home bool) {
	ui.Row(c).FillWidth().Justify(ui.Center).Padding(0, 24).Children(func() {
		width := float32(a.state.Settings.Appearance.TranscriptWidth)
		if width <= 0 {
			width = 736
		}
		ui.Column(c).FillWidth().MaxWidth(width).Gap(10).Children(func() {
			if home {
				ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() { a.projectPicker(c); a.devicePicker(c) })
			}
			a.nativeComposerSurface(c, composerSurfaceProps{
				draft: &a.draft, input: &a.composer, surface: &a.composerSurface, controls: a.centralControls(),
				inputLabel: "Message", attachLabel: "Attach files", sendLabel: "Send message", stopLabel: "Stop response",
				running:       a.currentSession().Status == contracts.Working,
				canSend:       !a.pendingNew && a.modelID != "" && (a.sessionID != "" || !a.modelNoAgents()) && a.attachmentImports[a.attachmentScope()] == 0 && composerHasContent(a.draft, len(a.attachments)+len(a.annotations)),
				controlsWidth: max(float32(0), min(width, a.chatWidth-48)*.45),
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
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				icon(c, "folder", 12)
				ui.Select(c, &a.checkout, []string{"Current checkout", "New worktree"}).Label("Checkout").FontSize(design.UISize(c, 11))
				icon(c, "branch", 12)
				branches := []string{"main"}
				for _, p := range a.state.Projects {
					if p.ID == a.projectID {
						branches = p.Branches
					}
				}
				ui.Select(c, &a.branch, branches).Label("Branch").FontSize(design.UISize(c, 11))
				ui.Spacer(c)
			})
		})
	})
}
func (a *App) projectPicker(c *ui.Context) {
	b := ui.ButtonBase(c).Padding(3, 4).Radius(5).Gap(5).Label("Select project").Children(func() {
		icon(c, "folder", 12)
		ui.Text(c, a.projectLabel()).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
		icon(c, "alt-arrow-down", 10)
	})
	b.OnClick(func() { a.projectMenu = !a.projectMenu })
	a.projectPopover(c, b)
}
func (a *App) devicePicker(c *ui.Context) {
	b := ui.ButtonBase(c).Padding(3, 4).Radius(5).Gap(5).Label("Select device").Children(func() {
		icon(c, "monitor", 12)
		ui.Text(c, a.deviceLabel()).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
		icon(c, "alt-arrow-down", 10)
	})
	b.OnClick(func() { a.deviceMenu = !a.deviceMenu })
	ui.Popover(c, b, &a.deviceMenu, func() {
		for _, device := range a.state.Devices {
			d := device
			menuChoice(c, d.Name, a.deviceID == d.ID, func() { a.deviceID = d.ID; a.deviceMenu = false })
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
		ui.Text(c, item.Name).FontSize(design.UISize(c, 11)).SingleLine()
		if item.Status == "uploading" || item.Status == "transferring" {
			ui.Spinner(c).Size(12, 12)
		}
		if removable {
			iconButton(c, "close", "Remove "+item.Name).Size(16, 16).OnClick(func() {
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
