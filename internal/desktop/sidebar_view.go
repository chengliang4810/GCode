package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"time"
)

func (a *App) sidebarView(c *ui.Context) {
	// The sidebar's tone: the reference's sidebar_tone, a wash over the
	// shell glass with a hairline on its right edge, painted under the
	// sidebar's own content (crates/ui/src/shell.rs).
	tone := design.SidebarTone(a.palette)
	ui.Column(c).Label(tr(c, "Session sidebar")).Fill().Background(tone).Draw(func(p *ui.Painter, bounds ui.Rect) {
		p.Fill(ui.Rect{X: bounds.X + bounds.W - 1, Y: bounds.Y, W: 1, H: bounds.H}, a.palette.Border, 0)
	}).Children(func() {
		bar := c.TitleBar()
		ui.Row(c).Height(38).Padding(4, 8, 0, max(10, bar.Left)).Gap(4).AlignItems(ui.Center).DragWindow().Children(func() {
			iconButton(c, "sidebar-minimalistic", tr(c, "Toggle sidebar")).OnClick(func() { a.setPanel(0, false) })
			iconButton(c.Key("sidebar-back"), "alt-arrow-left", tr(c, "Back")).Disabled(a.historyIndex <= 0).OnClick(func() { a.back(-1) })
			iconButton(c.Key("sidebar-forward"), "alt-arrow-right", tr(c, "Forward")).Disabled(a.historyIndex+1 >= len(a.history)).OnClick(func() { a.back(1) })
			ui.Spacer(c)
			if a.route == "settings" {
				iconButton(c, "plus", tr(c, "New conversation")).OnClick(a.newConversation)
			}
		})
		if a.route == "settings" {
			a.settingsNavigation(c)
		} else {
			if len(a.state.Sessions) > 0 {
				c.After(time.Minute)
			}
			a.sidebarNavigation(c)
			a.sidebarListToolbar(c)
			rows := a.sidebarRows()
			a.prepareSidebarList(rows)
			// List owns native selection/focus/reorder. Its default solid Accent
			// fill must yield to the reference's row wash, including rounded edges.
			baseTheme := c.Theme()
			listTheme := *baseTheme
			listTheme.Accent = ui.Transparent
			listTheme.AccentText = a.palette.Text
			restoreRoot := func() {
				c.Root().Background(design.Glass(a.palette, a.state.Settings.Appearance.Surface, c.Vibrancy()))
			}
			c.SetTheme(&listTheme)
			list := ui.List(c, &a.sidebarList, len(rows), func(i int) {
				c.SetTheme(baseTheme)
				row := rows[i]
				if row.Inset {
					// Indent the nested row by shrinking its box, not by
					// shifting it: a margin moves a full-width row past the
					// list's right padding, so its wash ran to the sidebar's
					// edge while top-level rows stopped short.
					ui.Column(c).FillWidth().Padding(0, 0, 0, design.Sidebar().ChildInset).Children(func() {
						a.sidebarConversationContent(c, row)
					})
				} else if row.Project {
					a.sidebarProjectRow(c, row)
				} else if row.Key == "section:projects" || row.Key == "section:conversations" || row.Key == "section:grouped" {
					a.sidebarSection(c, row)
				} else if row.Header != "" {
					a.sidebarHeader(c, row, i)
				} else {
					a.sidebarConversationContent(c, row)
				}
				c.SetTheme(&listTheme)
				restoreRoot()
			}).Label(tr(c, "Sessions list")).Grow(1).FillWidth().
				Padding(4, design.Sidebar().PaddingX).Gap(design.Sidebar().RowGap)
			c.SetTheme(baseTheme)
			restoreRoot()
			list.OnChange(func() { a.openSidebarSelection(rows) }).OnSubmit(func() { a.openSidebarSelection(rows) })
			if a.sidebarUI.focusList {
				list.Focus()
				a.sidebarUI.focusList = false
			}
			list.OnShortcut(0, ui.KeyF2, func() {
				if id := a.selectedSidebarSession(rows); id != "" {
					a.startSidebarRename(id)
				}
			})
		}
		ui.Row(c.Key("sidebar-footer")).Height(54).Padding(10, 12).Gap(4).AlignItems(ui.Center).Children(func() {
			iconButton(c.Key("sidebar-settings"), "settings", tr(c, "Settings")).OnClick(func() { a.navigate("settings") })
			project, _ := a.footerGitProject()
			pullRequests := iconButton(c.Key("sidebar-pull-requests"), "pull-request", tr(c, "Pull requests")).
				Disabled(!project.GitDetected || a.sidebarUI.pullRequests != nil).
				OnClick(a.openSidebarPullRequests)
			if !project.GitDetected {
				pullRequests.Tooltip(tr(c, "This project has no supported pull request remote."))
			}
			usageIconButton(c.Key("sidebar-usage"), tr(c, "Usage statistics")).OnClick(func() { a.navigate("settings/Usage statistics") })
			ui.Spacer(c)
			if a.checkForUpdates != nil && a.updateAvailable {
				updateIconButton(c.Key("sidebar-updates"), tr(c, "Check for updates…")).OnClick(a.checkForUpdates)
			}
		})
	})
}

func (a *App) sidebarNavigation(c *ui.Context) {
	s := design.Sidebar()
	ui.Column(c.Key("sidebar-primary-navigation")).FillWidth().
		Padding(s.NavigationPaddingY, s.PaddingX).Children(func() {
		controlSidebarRow(c.Key("sidebar-new-conversation"), tr(c, "New conversation"), false).
			OnClick(a.newConversation).Children(func() {
			icon(c, "add-circle", s.IconSize)
			ui.Text(c, tr(c, "New conversation")).Grow(1).MinWidth(0).SingleLine()
			for _, shortcut := range a.state.Settings.Shortcuts {
				if shortcut.ID == "newSession" {
					ui.Text(c.Key("sidebar-new-shortcut"), paletteShortcut(shortcut.Keys)).
						FontSize(design.TextSize(c, design.TextXS)).TextColor(a.palette.Faint)
					break
				}
			}
		})
	})
	ui.Box(c).FillWidth().Height(1).Background(a.palette.Border).Shrink(0)
}

func (a *App) sidebarListToolbar(c *ui.Context) {
	ui.Row(c.Key("sidebar-list-toolbar")).FillWidth().Padding(8, 8, 4, 18).
		Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tr(c, "Conversation list")).Grow(1).SingleLine().
			FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).TextColor(a.palette.Muted)
		iconButton(c.Key("sidebar-search"), "palette-search", tr(c, "Search conversations")).
			OnClick(func() {
				a.paletteQuery = ""
				a.paletteOpen = true
				a.commands.tab, a.commands.selected, a.commands.activeKey = 1, 0, ""
				a.refreshPaletteSearch()
			})
		iconButton(c.Key("sidebar-view-options"), "filter", tr(c, "Sidebar view options")).
			Menu(a.sidebarViewMenu)
	})
}

func (a *App) sidebarSection(c *ui.Context, row sidebarRow) {
	projects := row.Key == "section:projects"
	grouped := row.Key == "section:grouped"
	closed := &a.sidebarUI.conversationsClosed
	if projects {
		closed = &a.sidebarUI.projectsClosed
	}
	ui.Row(c.Key(row.Key)).FillWidth().MinHeight(design.Sidebar().HeaderHeight).
		Margin(6, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		controlSidebarHeading(c, a.sidebarHeaderText(row)).Expanded(!*closed).OnClick(func() { *closed = !*closed }).
			Children(func() {
				ui.Text(c, a.sidebarHeaderText(row)).SingleLine()
				if *closed {
					icon(c, "alt-arrow-right", 12)
				}
			})
		if projects {
			label, glyph := "Expand all projects", "alt-arrow-down"
			expanded := a.sidebarProjectsExpanded()
			if expanded {
				label, glyph = "Collapse all projects", "fold-vertical"
			}
			iconButton(c.Key("sidebar-toggle-projects"), glyph, tr(c, label)).
				Disabled(len(a.state.Projects) == 0).OnClick(func() {
				for _, project := range a.state.Projects {
					a.expanded["project:"+project.ID] = expanded
				}
				a.sidebarUI.projectsClosed = false
			})
			iconButton(c.Key("sidebar-add-project"), "plus", tr(c, "New project")).OnClick(a.openNewProject)
		}
		if grouped {
			iconButton(c.Key("sidebar-add-group"), "plus", tr(c, "New group")).OnClick(a.createNewSessionGroup)
		}
	})
}

// createNewSessionGroup adds an empty group titled with ZCode's default name;
// the header's context menu renames it.
func (a *App) createNewSessionGroup() {
	a.dispatch(contracts.Command{Kind: "group.create", Text: a.t("New group")})
}

func (a *App) sidebarProjectRow(c *ui.Context, row sidebarRow) {
	offline := false
	var project contracts.Project
	found := false
	for _, p := range a.state.Projects {
		if p.ID != row.ProjectID {
			continue
		}
		project, found = p, true
		for _, device := range a.state.Devices {
			if device.ID == project.DeviceID && !deviceOnline(device) {
				offline = true
			}
		}
	}
	closed := a.expanded[row.Group]
	button := controlSidebarProjectRow(c.Key(row.Key), row.Header, false, offline).Expanded(!closed)
	// The folder glyph itself carries the folded/open state; project rows show
	// no separate chevron.
	button.OnClick(func() {
		a.expanded[row.Group] = !a.expanded[row.Group]
	}).Children(func() {
		if a.state.Settings.ShowIcons {
			folder := "folder"
			if !closed {
				folder = "folder-with-files"
			}
			icon(c, folder, design.Sidebar().IconSize)
		}
		ui.Text(c, row.Header).SingleLine().MinWidth(0).Grow(1)
		if offline {
			ui.Box(c).Label(tr(c, "Device offline")).Children(func() { icon(c, "wifi-off", 12) })
		}
	})
	if found {
		infos := []sidebarHoverInfo{
			{Icon: "chat-round-line", Text: fmt.Sprintf(tr(c, "%d chats"), row.Count)},
			{Icon: "folder", Text: project.Path, Mono: true},
		}
		ui.TooltipBase(c, button, func(tip ui.Element) {
			// Beside the project row, as the session hover card is.
			tip.AttachTo(button, ui.AnchorEnd, ui.AnchorStart).MarginStart(6)
			tip.Children(func() {
				controlSidebarHoverCard(c, row.Header, infos)
			})
		})
	}
}

func (a *App) sidebarProjectsExpanded() bool {
	for _, project := range a.state.Projects {
		if !a.expanded["project:"+project.ID] {
			return true
		}
	}
	return false
}

func (a *App) sidebarConversationContent(c *ui.Context, row sidebarRow) {
	switch {
	case row.Empty:
		controlSidebarEmptyRow(c.Key("sidebar-empty:"+row.Group), tr(c, "No conversations"))
	case row.More:
		controlSidebarMoreRow(c.Key(row.Key), tr(c, "Show more")).OnClick(func() {
			if a.sidebarUI.limits == nil {
				a.sidebarUI.limits = make(map[string]int)
			}
			a.sidebarUI.limits[row.Group] = max(design.Sidebar().PageSize, a.sidebarUI.limits[row.Group]) + design.Sidebar().PageSize
		}).Children(func() { ui.Text(c, tr(c, "Show more")) })
	default:
		a.sessionRow(c, row.Session)
	}
}
