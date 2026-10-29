package desktop

import (
	"fmt"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) prepareSidebarList(rows []sidebarRow) {
	if !a.sidebarUI.initialized || a.sidebarUI.selectedSession != a.sessionID {
		a.sidebarUI.selection = -1
		for i, row := range rows {
			if row.Session.ID == a.sessionID && a.sessionID != "" {
				a.sidebarUI.selection = i
				break
			}
		}
		a.sidebarUI.selectedSession = a.sessionID
		a.sidebarUI.initialized = true
	}
	a.sidebarList.Selected = &a.sidebarUI.selection
	a.sidebarList.Header = func(i int) bool { return a.sidebarHeaderText(rows[i]) != "" || rows[i].More }
	a.sidebarList.Key = func(i int) any { return rows[i].Key }
	a.sidebarList.Label = func(i int) string {
		if rows[i].Header != "" {
			return a.sidebarHeaderText(rows[i])
		}
		return rows[i].Session.Title
	}
	a.sidebarList.Reorder = func(from []int, to int) { a.reorderSidebarRows(rows, from, to) }
}

func (a *App) selectedSidebarSession(rows []sidebarRow) string {
	i := a.sidebarUI.selection
	if i >= 0 && i < len(rows) && rows[i].Header == "" && !rows[i].More {
		return rows[i].Session.ID
	}
	return ""
}

func (a *App) openSidebarSelection(rows []sidebarRow) {
	if id := a.selectedSidebarSession(rows); id != "" && id != a.sessionID {
		a.navigate("chat/" + id)
		a.sidebarUI.focusList = true
	}
}

func (a *App) sidebarHeader(c *ui.Context, row sidebarRow, index int) {
	closed := a.expanded[row.Key]
	if row.Key == "group:archived" {
		closed = !a.sidebarUI.archivedOpen
	}
	above, below := float32(0), float32(0)
	if index > 0 {
		above = 10
	}
	if !closed {
		below = 2
	}
	ui.ButtonBase(c).Label(a.sidebarHeaderText(row)).FillWidth().Height(28).Margin(above, 0, below, 0).Padding(0, 8).Gap(8).AlignItems(ui.Center).TextColor(a.palette.Muted.Alpha(.5)).OnClick(func() {
		if row.Key == "group:archived" {
			a.sidebarUI.archivedOpen = !a.sidebarUI.archivedOpen
		} else {
			a.expanded[row.Key] = !a.expanded[row.Key]
		}
	}).Children(func() {
		if row.ProjectID != "" && a.state.Settings.ShowIcons {
			icon(c, "folder", 13)
		}
		ui.Text(c, a.sidebarHeaderText(row)).FontSize(design.UISize(c, 12)).FontWeight(500)
		if closed {
			ui.Text(c, fmt.Sprint(row.Count)).FontSize(design.UISize(c, 10)).Font("Geist Mono").Padding(0, 4).Radius(4).Background(a.palette.Hover)
		}
		ui.Spacer(c)
		glyph := "alt-arrow-down"
		if closed {
			glyph = "alt-arrow-right"
		}
		icon(c, glyph, 12)
	})
}
