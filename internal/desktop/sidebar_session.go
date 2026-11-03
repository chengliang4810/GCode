package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) sessionRow(c *ui.Context, session contracts.Session) {
	c.Key("sidebar-session:" + session.ID)
	settings := a.state.Settings
	row := controlSidebarConversationRow(c, session.Title, sidebarConversationStyle{
		Selected: a.sessionID == session.ID,
	})
	hovered := row.Hovered()
	if row.DoubleClicked() {
		a.startSidebarRename(session.ID)
	}
	renaming := a.sidebarUI.renameID == session.ID
	row.OnClick(func() {
		if !renaming {
			a.navigate("chat/" + session.ID)
		}
	})
	row.ContextMenu(func(menu *ui.Menu) {
		if menu.Item(tr(c, "Call trajectory")).Chosen() {
			a.openModelTrajectory(session.ID)
		}
		menu.Separator()
		if menu.Item(tr(c, "Rename")).Chosen() {
			a.startSidebarRename(session.ID)
		}
		pinLabel := "Pin"
		if session.Pinned {
			pinLabel = "Unpin"
		}
		if menu.Item(tr(c, pinLabel)).Chosen() {
			pin := !session.Pinned
			target := session
			target.Pinned = pin
			a.revealSidebarSession(target)
			a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session.ID, Pinned: &pin}})
		}
		if menu.Item(tr(c, "Archive")).Chosen() {
			a.archiveSidebarSession(session.ID)
		}
		menu.Submenu("Copy", func(menu *ui.Menu) {
			if menu.Item(tr(c, "Title")).Chosen() {
				c.Services().WriteClipboard(session.Title)
			}
			if menu.Item(tr(c, "Session ID")).Chosen() {
				c.Services().WriteClipboard(session.ID)
			}
			if menu.Item(tr(c, "Branch")).Disabled(session.Branch == "").Chosen() {
				c.Services().WriteClipboard(session.Branch)
			}
		})
		if len(a.state.SessionGroups) > 0 || session.GroupID != "" {
			menu.Submenu(tr(c, "Move to group"), func(menu *ui.Menu) {
				for _, group := range a.orderedSessionGroups() {
					group := group
					if menu.Item(group.Title).Checked(session.GroupID == group.ID).Chosen() {
						a.assignSidebarSessionGroup(session.ID, group.ID)
					}
				}
				if menu.Item(tr(c, "Remove from group")).Disabled(session.GroupID == "").Chosen() {
					a.assignSidebarSessionGroup(session.ID, "")
				}
			})
		}
		unreadLabel := "Mark unread"
		if session.Unread {
			unreadLabel = "Mark read"
		}
		if menu.Item(tr(c, unreadLabel)).Chosen() {
			value := !session.Unread
			a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session.ID, Unread: &value}})
		}
		menu.Separator()
		if menu.Item(tr(c, "Delete")).Chosen() {
			a.sidebarUI.deleteID = session.ID
			a.sidebarUI.deleteOpen = true
		}
	})
	row.Children(func() {
		ui.Row(c).FillWidth().Height(design.Sidebar().RowHeight).Gap(4).MinWidth(0).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Width(design.Sidebar().ConversationLeadingWidth).Shrink(0).Center().Children(func() {
				if session.Status != contracts.Idle || session.Unread {
					a.sidebarStatusGlyph(c, session)
				}
			})
			if renaming {
				input := ui.TextInputBase(c, &a.sidebarUI.renameTitle).Label(tr(c, "Session name")).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase)).OnSubmit(a.commitSidebarRename).OnShortcut(0, ui.KeyEscape, func() { a.sidebarUI.renameID = "" })
				if a.sidebarUI.focusRename {
					input.Focus().SetTextSelection(0, len([]rune(a.sidebarUI.renameTitle)))
					a.sidebarUI.focusRename = false
					row.ScrollIntoView()
				}
			} else {
				ui.Text(c, session.Title).Grow(1).MinWidth(0).SingleLine().FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
			}
			a.sidebarSessionCorner(c, session, hovered)
		})
	})
	// The compact row shows the title only; location, branch and activity
	// surface in the shared hover card.
	infos := []sidebarHoverInfo{{Icon: "folder", Text: a.projectNameFor(session.ProjectID) + " @ " + a.deviceNameFor(session.DeviceID)}}
	if session.Branch != "" {
		infos = append(infos, sidebarHoverInfo{Icon: "git-branch", Text: session.Branch, Mono: true})
	}
	infos = append(infos, sidebarHoverInfo{Icon: "clock-circle", Text: relativeAge(session.UpdatedAt, a.now(), settings.General.Language)})
	ui.TooltipBase(c, row, func(tip ui.Element) {
		// The card reads beside the row it describes, at that row's own
		// height, not above it nor anywhere else in the window.
		tip.AttachTo(row, ui.AnchorEnd, ui.AnchorStart).MarginStart(6)
		tip.Children(func() {
			controlSidebarHoverCard(c, session.Title, infos)
		})
	})
}

func (a *App) sidebarStatusGlyph(c *ui.Context, session contracts.Session) {
	switch session.Status {
	case contracts.Working:
		ui.Spinner(c).Size(12, 12)
	case contracts.AwaitingInput:
		ui.Box(c).Size(6, 6).Radius(3).Background(a.palette.Warning)
	case contracts.Errored:
		icon(c, "close-circle", 12)
	default:
		if session.Unread {
			ui.Box(c).Size(6, 6).Radius(3).Background(a.palette.Accent)
		}
	}
}

func (a *App) sidebarSessionCorner(c *ui.Context, session contracts.Session, hovered bool) {
	ui.Row(c).MinWidth(design.Sidebar().ConversationTrailingWidth).Shrink(0).Justify(ui.End).AlignItems(ui.Center).Children(func() {
		if hovered {
			inlineIconButton(c, "archive-minimalistic", tr(c, "Archive session")+" "+session.ID).Tooltip(tr(c, "Archive session")).OnClick(func() { a.archiveSidebarSession(session.ID) })
		} else {
			ui.Text(c, relativeAge(session.UpdatedAt, a.now(), a.state.Settings.General.Language)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Faint)
		}
	})
}
