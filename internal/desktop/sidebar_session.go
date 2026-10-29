package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func sidebarRowHeight(compact, location, branch, pr bool) float32 {
	if compact {
		return 29
	}
	height := float32(45)
	if branch {
		height = 61
	}
	if pr {
		height = 63
	}
	if !location {
		height -= 16
	}
	return height
}

func (a *App) sessionHarnessIcon(session contracts.Session) string {
	for _, model := range a.state.Models {
		if model.ID == session.ModelID {
			for _, provider := range a.state.Providers {
				if provider.ID == model.ProviderID && provider.Icon != "" {
					return provider.Icon
				}
			}
		}
	}
	return "bot"
}

func (a *App) sessionRow(c *ui.Context, session contracts.Session) {
	c.Key("session:" + session.ID)
	settings := a.state.Settings
	compact := settings.SidebarCompact
	branch := settings.SidebarShowBranch && session.Branch != ""
	pr := settings.SidebarShowPullRequest && session.PullRequestNumber > 0
	row := ui.ButtonBase(c).Label(session.Title).FillWidth().MinWidth(0).Height(sidebarRowHeight(compact, settings.ShowProjectLabels, branch, pr)).Padding(6, 8).Radius(8).AlignItems(ui.Stretch).Justify(ui.Start)
	if a.sessionID == session.ID {
		row.Background(a.palette.Active)
	} else if row.Hovered() {
		row.Background(a.palette.Hover)
	}
	hovered := row.Hovered()
	if session.Archived && a.sessionID != session.ID && !hovered {
		row.Opacity(.4)
	}
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
		if menu.Item(tr(c, "Rename")).Chosen() {
			a.startSidebarRename(session.ID)
		}
		pinLabel := "Pin"
		if session.Pinned {
			pinLabel = "Unpin"
		}
		if menu.Item(tr(c, pinLabel)).Chosen() {
			pin := !session.Pinned
			a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session.ID, Pinned: &pin}})
		}
		archiveLabel := "Archive"
		if session.Archived {
			archiveLabel = "Unarchive"
		}
		if menu.Item(tr(c, archiveLabel)).Chosen() {
			a.archiveSidebarSession(session.ID, !session.Archived)
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
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			if !compact && settings.ShowProjectLabels {
				ui.Row(c).Height(14).MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Text(c, a.projectNameFor(session.ProjectID)+" @ "+a.deviceNameFor(session.DeviceID)).Grow(1).MinWidth(0).SingleLine().FontSize(design.UISize(c, 10)).TextColor(a.palette.Muted)
					a.sidebarSessionCorner(c, session, hovered, false)
				})
			}
			ui.Row(c).Height(17).Gap(4).MinWidth(0).AlignItems(ui.Center).Children(func() {
				if compact {
					a.sidebarStatusGlyph(c, session)
				}
				if settings.SidebarShowHarness {
					icon(c, a.sessionHarnessIcon(session), 13)
				}
				if settings.ShowIcons && (sidebarGrouping(settings.SidebarGrouping) != "project" || session.Pinned) {
					icon(c, "folder", 13)
				}
				if renaming {
					input := ui.TextInputBase(c, &a.sidebarUI.renameTitle).Label(tr(c, "Session name")).Grow(1).MinWidth(0).FontSize(design.UISize(c, 13)).OnSubmit(a.commitSidebarRename).OnShortcut(0, ui.KeyEscape, func() { a.sidebarUI.renameID = "" })
					if a.sidebarUI.focusRename {
						input.Focus().SetTextSelection(0, len([]rune(a.sidebarUI.renameTitle)))
						a.sidebarUI.focusRename = false
						row.ScrollIntoView()
					}
				} else {
					ui.Text(c, session.Title).Grow(1).MinWidth(0).SingleLine().FontSize(design.UISize(c, 13))
				}
				if compact {
					if pr {
						a.sidebarPRBadge(c, session)
					}
					a.sidebarSessionCorner(c, session, hovered, true)
				}
			})
			if !compact && (branch || pr) {
				height := float32(14)
				if pr {
					height = 16
				}
				ui.Row(c).Height(height).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
					if branch {
						ui.Text(c, session.Branch).Grow(1).MinWidth(0).SingleLine().FontSize(design.UISize(c, 10)).TextColor(a.palette.Faint)
					} else {
						ui.Spacer(c)
					}
					if pr {
						a.sidebarPRBadge(c, session)
					}
				})
			}
		})
	})
}

func (a *App) sidebarPRBadge(c *ui.Context, session contracts.Session) {
	ui.ButtonBase(c).Label(fmt.Sprintf(tr(c, "Open pull request #%d"), session.PullRequestNumber)).Height(16).Padding(0, 4).Gap(3).Radius(4).AlignItems(ui.Center).Background(a.palette.Success.Alpha(.08)).TextColor(a.palette.Success).OnClick(func() {
		for _, pr := range a.state.PullRequests {
			if pr.Number == session.PullRequestNumber {
				a.workspace.OpenPR(pr.ID)
				return
			}
		}
		a.navigate("pull-requests")
	}).Children(func() {
		icon(c, "pull-request", 10)
		ui.Text(c, fmt.Sprintf(tr(c, "#%d"), session.PullRequestNumber)).Font("Geist Mono").FontSize(design.UISize(c, 10))
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

func (a *App) sidebarSessionCorner(c *ui.Context, session contracts.Session, hovered, compact bool) {
	ui.Row(c).Height(14).MinWidth(30).Shrink(0).Justify(ui.End).AlignItems(ui.Center).Children(func() {
		if hovered {
			label, glyph := "Archive session", "archive-minimalistic"
			if session.Archived {
				label, glyph = "Unarchive session", "archive-up-minimalistic"
			}
			iconButton(c, glyph, tr(c, label)+" "+session.ID).Tooltip(tr(c, label)).Size(18, 18).OnClick(func() { a.archiveSidebarSession(session.ID, !session.Archived) })
		} else if !compact && session.Status != contracts.Idle {
			label, color := "Working", a.palette.Accent
			if session.Status == contracts.AwaitingInput {
				label, color = "Input", a.palette.Warning
			}
			if session.Status == contracts.Errored {
				label, color = "Failed", a.palette.Danger
			}
			ui.Text(c, tr(c, label)).FontSize(design.UISize(c, 10)).TextColor(color)
		} else {
			ui.Text(c, relativeAge(session.UpdatedAt, a.state.Sessions, a.state.Settings.General.Language)).FontSize(design.UISize(c, 10)).TextColor(a.palette.Faint)
		}
	})
}
