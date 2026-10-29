package desktop

import "github.com/egoist/mygo/ui"

const (
	titlebarHeight      float32 = 38
	titlebarEdgeInset   float32 = 6
	titlebarToggleSlots float32 = 28*2 + 4
)

// titlebarWidths follows shell/tabs.rs panel_titlebar_widths. The caption
// clearance is paid once across the surface and Files columns; the two
// global toggles retain their own fixed slots outside the tabs' clip.
func titlebarWidths(right, files, available, caption float32) (surface, controls float32) {
	controls = max(files-caption, titlebarToggleSlots)
	surface = max(0, min(right+files-caption-controls, available-controls))
	return surface, controls
}

func (a *App) titlebar(c *ui.Context) {
	bar := c.TitleBar()
	session := a.currentSession()
	onCanvas := a.route == "chat" && session.ID == ""
	viewport, _ := c.Size()
	columns := allocatePanels(viewport, a.sidebarWidth, a.filesWidth, a.rightWidth, a.fit, a.rightExpanded)
	caption := bar.Right + titlebarEdgeInset
	navigation := float32(0)
	if !a.fit.Sidebar {
		navigation = 10 + 28*2 + 5
		if bar.Left > 0 {
			navigation += bar.Left + 5
		}
	}
	takeover := !onCanvas && a.fit.Right && a.rightExpanded
	gapBudget := float32(24)
	if takeover {
		// In the source, row_left is sidebar-8 and the 8px row gap
		// cancels it. A collapsed sidebar instead clears the navigation.
		gapBudget = 0
		if !a.fit.Sidebar {
			gapBudget = 8
		}
	}
	surface, controls := titlebarWidths(columns.Right, columns.Files, viewport-columns.Sidebar-navigation-caption-gapBudget, caption)
	ui.Row(c).Key("main-titlebar").Height(titlebarHeight).Shrink(0).FillWidth().Padding(0, caption, 0, 0).AlignItems(ui.Center).DragWindow().Children(func() {
		if !a.fit.Sidebar {
			ui.Row(c).Width(navigation).Height(titlebarHeight).Shrink(0).Padding(4, 0, 0, 10).Gap(5).AlignItems(ui.Center).Children(func() {
				if bar.Left > 0 {
					ui.Spacer(c).Width(bar.Left).Shrink(0)
				}
				c.Key("titlebar-sidebar")
				iconButton(c, "sidebar-minimalistic", tr(c, "Toggle sidebar")).Shrink(0).OnClick(func() { a.setPanel(0, true) })
				c.Key("titlebar-back")
				iconButton(c, "alt-arrow-left", tr(c, "Back")).Shrink(0).OnClick(func() { a.back(-1) })
			})
		}
		identity := ui.Row(c).Grow(1).MinWidth(0).Height(titlebarHeight).AlignItems(ui.Center)
		if !takeover {
			identity.Padding(4, 8, 0, 10).Gap(8)
		}
		identity.Children(func() {
			if a.route == "settings" {
				ui.Text(c, tr(c, a.section)).FontWeight(600).SingleLine()
				return
			}
			if takeover {
				return
			}
			ui.Row(c).Grow(1).MinWidth(0).Clip().Gap(5).AlignItems(ui.Center).Children(func() {
				if session.ID != "" {
					ui.Text(c, session.Title).FontWeight(600).SingleLine().MinWidth(0)
					ui.Text(c, "~ @ "+a.deviceLabel()).TextColor(a.palette.Muted).SingleLine().Shrink(0)
				}
			})
			if session.ID != "" {
				c.Key("titlebar-session-info")
				iconButton(c, "info-circle", tr(c, "Session information")).Shrink(0).OnClick(a.toggleInfo)
			}
		})
		if a.route == "settings" || onCanvas {
			return
		}
		if a.fit.Right && surface >= 44 {
			ui.Box(c).Width(surface).Height(titlebarHeight).Shrink(0).Clip().Children(func() { a.workspace.TabStrip(c, a.rightExpanded) })
		}
		ui.Row(c).Width(controls).Height(titlebarHeight).Shrink(0).Padding(4, 0, 0, 0).Gap(4).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			c.Key("titlebar-files")
			iconButton(c, "folder-tree", tr(c, "Files")).Shrink(0).OnClick(func() { a.togglePanel(2) })
			c.Key("titlebar-right-pane")
			iconButton(c, "split-columns", tr(c, "Toggle right pane")).Shrink(0).OnClick(func() { a.togglePanel(1) }).ContextMenu(func(m *ui.Menu) {
				if m.Item(tr(c, "Expand pane")).Checked(a.rightExpanded).Chosen() {
					a.rightExpanded = !a.rightExpanded
					a.setPanel(1, true)
				}
				if m.Item(tr(c, "Terminal panel")).Checked(a.terminal).Chosen() {
					a.toggleTerminal()
				}
			})
		})
	})
}
