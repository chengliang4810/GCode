package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) sidebarView(c *ui.Context) {
	wash := ui.Hex("#000000").Alpha(.05)
	if a.palette.Dark {
		wash = ui.Hex("#ffffff").Alpha(.05)
	}
	ui.Column(c).Label(tr(c, "Session sidebar")).Fill().Background(wash).Children(func() {
		bar := c.TitleBar()
		ui.Row(c).Height(38).Padding(4, 8, 0, max(10, bar.Left)).Gap(4).AlignItems(ui.Center).DragWindow().Children(func() {
			iconButton(c, "sidebar-minimalistic", tr(c, "Toggle sidebar")).Size(24, 24).OnClick(func() { a.setPanel(0, false) })
			iconButton(c, "alt-arrow-left", tr(c, "Back")).Size(24, 24).Disabled(a.historyIndex <= 0).OnClick(func() { a.back(-1) })
			iconButton(c, "alt-arrow-right", tr(c, "Forward")).Size(24, 24).Disabled(a.historyIndex+1 >= len(a.history)).OnClick(func() { a.back(1) })
			ui.Spacer(c)
			iconButton(c, "plus", tr(c, "New session")).Size(24, 24).OnClick(a.newConversation)
			iconButton(c, "menu-dots", tr(c, "Sidebar view options")).Size(24, 24).Menu(a.sidebarViewMenu)
		})
		if a.route == "settings" {
			a.settingsNavigation(c)
		} else {
			if a.state.Settings.SidebarShowProjectFilter {
				ui.Row(c).Padding(8, 8, 4, 8).Children(func() {
					label := tr(c, "All projects")
					if a.sidebarProjectID != "" {
						label = a.projectNameFor(a.sidebarProjectID)
					}
					b := ui.ButtonBase(c).Label(label).Grow(1).MinWidth(0).Height(29).Padding(0, 8).Gap(8).Radius(8).AlignItems(ui.Center).OnClick(func() { a.sidebarProjectMenu = !a.sidebarProjectMenu })
					b.Children(func() { icon(c, "folder", 16); ui.Text(c, label).SingleLine().MinWidth(0).Grow(1).FontWeight(500) })
					a.sidebarProjectPopover(c, b)
				})
			} else {
				ui.Spacer(c).Height(4).Shrink(0)
			}
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
				if row.Header != "" {
					a.sidebarHeader(c, row, i)
				} else if row.More {
					textButton(c, tr(c, "Show more")).FillWidth().OnClick(func() { a.sidebarUI.archiveLimit = max(10, a.sidebarUI.archiveLimit) + 25 })
				} else {
					a.sessionRow(c, row.Session)
				}
				c.SetTheme(&listTheme)
				restoreRoot()
			}).Label(tr(c, "Sessions list")).Grow(1).FillWidth().Padding(4, 8).Gap(2)
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
			if len(rows) == 0 {
				ui.Text(c, tr(c, "No sessions")).TextColor(a.palette.Muted).Padding(16)
			}
		}
		ui.Row(c).Height(54).Padding(10, 12).Gap(4).AlignItems(ui.Center).Children(func() {
			account := ui.ButtonBase(c.Key("sidebar-account")).Label(tr(c, "Account menu: ")+a.state.User.Name).
				Expanded(a.sidebarUI.accountOpen).
				Height(28).MinWidth(0).Padding(0, 8).Gap(8).Radius(8).AlignItems(ui.Center).
				OnClick(func() {
					a.sidebarUI.accountOpen = !a.sidebarUI.accountOpen
					a.sidebarUI.accountSelection = 0
				})
			account.Children(func() {
				ui.Box(c).Size(16, 16).Shrink(0).Radius(8).Background(ui.RGB(255, 255, 255)).TextColor(ui.RGB(20, 20, 20)).Center().Children(func() { ui.Text(c, a.state.User.Initials).FontSize(design.UISize(c, 10)).FontWeight(600) })
				ui.Text(c, a.state.User.Name).SingleLine().MinWidth(0)
			})
			a.accountPopover(c, account)
			ui.Spacer(c)
			iconButton(c, "pull-request", tr(c, "Pull requests")).Size(28, 28).OnClick(func() { a.navigate("pull-requests") })
			iconButton(c, "settings", tr(c, "Settings")).Size(28, 28).OnClick(func() { a.navigate("settings") })
		})
	})
	a.sidebarDeleteDialog(c)
}

func (a *App) accountPopover(c *ui.Context, anchor ui.Element) {
	state := &a.sidebarUI
	labels := []string{tr(c, "Settings"), tr(c, "Sign out")}
	choose := func(index int) {
		state.accountOpen = false
		switch index {
		case 0:
			a.navigate("settings")
		case 1:
			a.dispatch(contracts.Command{Kind: "account.signout"})
		}
	}
	ui.PopoverBase(c, anchor, &state.accountOpen, func(panel ui.Element) {
		// Native system menus fit the display, not the application window.
		panel.Label(tr(c, "Account menu")).AttachTo(anchor, ui.AnchorTopLeft, ui.AnchorBottomLeft).
			Margin(0, 0, 6, 0).SnapToWindow(8).Width(180).Padding(4).Radius(8).
			Background(a.palette.Overlay).Border(1, a.palette.Border)
		state.accountList.Selected = &state.accountSelection
		state.accountList.Label = func(index int) string { return labels[index] }
		panel.Children(func() {
			ui.List(c.Key("account-actions"), &state.accountList, len(labels), func(index int) {
				button := textButton(c, labels[index]).FillWidth().Height(30).
					OnClick(func() { choose(index) })
				if state.accountSelection == index {
					button.Background(a.palette.Hover)
				}
			}).Height(60).AutoFocus().OnSubmit(func() { choose(state.accountSelection) })
		})
	})
}
