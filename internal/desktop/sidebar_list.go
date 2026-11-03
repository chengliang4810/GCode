package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"strings"
)

func (a *App) prepareSidebarList(rows []sidebarRow) {
	cursor := a.sidebarUI.selection
	stale := cursor >= len(rows) || (cursor >= 0 && rows[cursor].Session.ID != a.sessionID)
	if !a.sidebarUI.initialized || a.sidebarUI.selectedSession != a.sessionID || stale {
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
	a.sidebarList.Header = func(i int) bool { return a.sidebarHeaderText(rows[i]) != "" || rows[i].More || rows[i].Empty }
	a.sidebarList.Key = func(i int) any { return "sidebar-list-row:" + rows[i].Key }
	a.sidebarList.Label = func(i int) string {
		if rows[i].Empty {
			return a.sidebarEmptyText(rows[i])
		}
		if rows[i].Header != "" {
			return a.sidebarHeaderText(rows[i])
		}
		return rows[i].Session.Title
	}
	a.sidebarList.Reorder = func(from []int, to int) { a.reorderSidebarRows(rows, from, to) }
	if a.sidebarUI.revealID != "" {
		for i, row := range rows {
			if row.Session.ID == a.sidebarUI.revealID && row.Group == a.sidebarUI.revealGroup {
				a.sidebarList.ScrollIntoView(i)
				a.sidebarUI.revealID = ""
				a.sidebarUI.revealGroup = ""
				break
			}
		}
	}
	if request := a.sidebarUI.revealProject; request != nil {
		for _, project := range a.state.Projects {
			if project.Path == request.Path && project.DeviceID == request.DeviceID {
				for i, row := range rows {
					if row.Project && row.ProjectID == project.ID {
						a.sidebarList.ScrollIntoView(i)
						a.sidebarUI.revealProject = nil
						break
					}
				}
				break
			}
		}
	}
}

func (a *App) selectedSidebarSession(rows []sidebarRow) string {
	i := a.sidebarUI.selection
	if i >= 0 && i < len(rows) && rows[i].Header == "" && !rows[i].More && !rows[i].Empty {
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
	timeline := strings.HasPrefix(row.Key, "timeline:")
	above, below := float32(0), float32(0)
	if index > 0 {
		above = 10
	}
	if !closed && !timeline {
		below = 2
	}
	heading := controlSidebarHeading(c.Key(row.Key+":toggle"), a.sidebarHeaderText(row)).FillWidth().Margin(above, 0, below, 0).OnClick(func() {
		a.expanded[row.Key] = !a.expanded[row.Key]
	})
	if row.SessionGroup {
		heading.ContextMenu(func(menu *ui.Menu) {
			if menu.Item(tr(c, "Rename")).Chosen() {
				a.startSidebarGroupRename(row.GroupID)
			}
			if menu.Item(tr(c, "Delete")).Chosen() {
				a.dispatch(contracts.Command{Kind: "group.delete", ID: row.GroupID})
			}
		})
	}
	heading.Children(func() {
		if row.ProjectID != "" && a.state.Settings.ShowIcons {
			icon(c, "folder", 13)
		}
		if renaming := row.SessionGroup && a.sidebarUI.renameID == "group:"+row.GroupID; renaming {
			input := ui.TextInputBase(c, &a.sidebarUI.renameTitle).Label(tr(c, "Group name")).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase)).OnSubmit(a.commitSidebarRename).OnShortcut(0, ui.KeyEscape, func() { a.sidebarUI.renameID = "" })
			if a.sidebarUI.focusRename {
				input.Focus().SetTextSelection(0, len([]rune(a.sidebarUI.renameTitle)))
				a.sidebarUI.focusRename = false
			}
		} else {
			ui.Text(c, a.sidebarHeaderText(row)).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
		}
		if closed && !timeline {
			ui.Text(c, fmt.Sprint(row.Count)).FontSize(design.TextSize(c, design.TextXS)).Font("Geist Mono").Padding(0, 4).Radius(4).Background(a.palette.Hover)
		}
		ui.Spacer(c)
		if timeline {
			return
		}
		glyph := "alt-arrow-down"
		if closed {
			glyph = "alt-arrow-right"
		}
		icon(c, glyph, 12)
	})
}
