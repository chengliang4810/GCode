package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"sort"
	"strings"
)

func (a *App) startSidebarRename(id string) {
	for _, session := range a.state.Sessions {
		if session.ID == id {
			a.sidebarUI.renameID = id
			a.sidebarUI.renameTitle = session.Title
			a.sidebarUI.focusRename = true
			a.sidebarUI.focusList = false
			if session.Archived {
				a.sidebarUI.archivedOpen = true
			}
			for _, row := range a.sidebarRows() {
				if row.Header != "" {
					a.expanded[row.Key] = false
				}
			}
			return
		}
	}
}

func (a *App) commitSidebarRename() {
	id, title := a.sidebarUI.renameID, strings.TrimSpace(a.sidebarUI.renameTitle)
	a.sidebarUI.renameID = ""
	if id == "" || title == "" {
		return
	}
	for _, session := range a.state.Sessions {
		if session.ID == id && session.Title == title {
			return
		}
	}
	a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: id, Title: &title}})
}

func (a *App) archiveSidebarSession(id string, archived bool) {
	a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: id, Archived: &archived}})
	if archived && a.sessionID == id {
		a.nextSidebarSession(id)
	}
}

func (a *App) nextSidebarSession(skip string) {
	for _, session := range a.visibleSessions(false) {
		if session.ID != skip {
			a.navigate("chat/" + session.ID)
			return
		}
	}
	a.newConversation()
}

func (a *App) sidebarDeleteDialog(c *ui.Context) {
	ui.DialogBase(c, &a.sidebarUI.deleteOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, .35))
		panel.Width(400).Padding(24).Radius(12).Background(a.palette.Dialog)
		ui.Column(c).Gap(16).Children(func() {
			ui.Text(c, "Delete session?").FontSize(design.UISize(c, 18)).FontWeight(600)
			ui.Text(c, "This action cannot be undone.").TextColor(a.palette.Muted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				textButton(c, "Cancel").OnClick(func() { a.sidebarUI.deleteOpen = false })
				textButton(c, "Delete").TextColor(a.palette.Danger).OnClick(func() {
					id := a.sidebarUI.deleteID
					a.sidebarUI.deleteOpen = false
					a.dispatch(contracts.Command{Kind: "session.delete", ID: id})
					if a.sessionID == id {
						a.nextSidebarSession(id)
					}
				})
			})
		})
	})
}

func (a *App) reorderSidebarRows(rows []sidebarRow, from []int, to int) {
	if len(from) != 1 || from[0] < 0 || from[0] >= len(rows) {
		return
	}
	moved := rows[from[0]].Session
	if moved.ID == "" || moved.Archived {
		return
	}
	var pins []contracts.Session
	for _, session := range a.state.Sessions {
		if session.Pinned && session.ID != moved.ID {
			pins = append(pins, session)
		}
	}
	sort.SliceStable(pins, func(i, j int) bool { return pins[i].PinOrder < pins[j].PinOrder })
	anchor := ""
	after := false
	if to >= 0 && to < len(rows) && rows[to].Session.Pinned {
		anchor = rows[to].Session.ID
	} else {
		first, last := -1, -1
		for i, row := range rows {
			if row.Session.Pinned && !row.Session.Archived {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			return
		}
		if to <= first {
			anchor = rows[first].Session.ID
		} else if moved.Pinned || to <= last+1 {
			anchor = rows[last].Session.ID
			after = true
		} else {
			return
		}
	}
	if anchor == moved.ID {
		return
	}
	rank := 0
	for i, pin := range pins {
		if pin.ID == anchor {
			rank = i
			if after {
				rank++
			}
			break
		}
	}
	if moved.Pinned && rank == moved.PinOrder {
		return
	}
	pinned := true
	a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: moved.ID, Pinned: &pinned, PinOrder: &rank}})
}
