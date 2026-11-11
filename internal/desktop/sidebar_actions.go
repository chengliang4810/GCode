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
			a.revealSidebarSession(session)
			a.sidebarUI.renameID = id
			a.sidebarUI.renameTitle = session.Title
			a.sidebarUI.focusRename = true
			a.sidebarUI.focusList = false
			return
		}
	}
}

func (a *App) startSidebarGroupRename(id string) {
	for _, group := range a.state.SessionGroups {
		if group.ID == id {
			a.sidebarUI.renameID = "group:" + id
			a.sidebarUI.renameTitle = group.Title
			a.sidebarUI.focusRename = true
			a.sidebarUI.focusList = false
			return
		}
	}
}

func (a *App) revealSidebarSession(session contracts.Session) {
	a.search = ""
	a.sidebarUI.revealID = session.ID
	a.sidebarUI.revealGroup = "conversations"
	if session.Pinned {
		a.expanded["group:pinned"] = false
		a.sidebarUI.revealGroup = "group:pinned"
		return
	}
	switch a.state.Settings.SidebarGrouping {
	case "timeline":
		a.sidebarUI.revealGroup = "timeline"
	case "device":
		a.expanded["device:"+session.DeviceID] = false
		a.sidebarUI.revealGroup = "device:" + session.DeviceID
	case "grouped":
		a.sidebarUI.conversationsClosed = false
		if session.GroupID != "" {
			a.expanded["session-group:"+session.GroupID] = false
			a.sidebarUI.revealGroup = "session-group:" + session.GroupID
		}
	default:
		a.sidebarUI.projectsClosed = false
		a.sidebarUI.conversationsClosed = false
		a.expanded["project:"+session.ProjectID] = false
		for _, project := range a.state.Projects {
			if project.ID == session.ProjectID {
				a.sidebarUI.revealGroup = "project:" + project.ID
				break
			}
		}
	}
}

func (a *App) commitSidebarRename() {
	id, title := a.sidebarUI.renameID, strings.TrimSpace(a.sidebarUI.renameTitle)
	a.sidebarUI.renameID = ""
	if id == "" || title == "" {
		return
	}
	if groupID, ok := strings.CutPrefix(id, "group:"); ok {
		a.dispatch(contracts.Command{Kind: "group.rename", ID: groupID, Text: title})
		return
	}
	for _, session := range a.state.Sessions {
		if session.ID == id && session.Title == title {
			return
		}
	}
	a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: id, Title: &title}})
}

// archiveSidebarSession archives a conversation. The archived data leaves the
// sidebar immediately; restoring and deleting it happens in settings.
func (a *App) archiveSidebarSession(id string) {
	a.dispatch(contracts.Command{Kind: "session.archive", ID: id, Enabled: true})
	if a.sessionID == id {
		a.nextSidebarSession(id)
	}
}

// assignSidebarSessionGroup moves one conversation into a session group, or
// back to the ungrouped root with an empty ID.
func (a *App) assignSidebarSessionGroup(sessionID, groupID string) {
	a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: sessionID, GroupID: &groupID}})
}

func (a *App) nextSidebarSession(skip string) {
	for _, session := range a.visibleSessions() {
		if session.ID != skip {
			a.navigate("chat/" + session.ID)
			return
		}
	}
	a.newConversation()
}

// sessionDeleteDialog confirms deletion of a conversation from the sidebar or
// the settings archived page; both feed sidebarUI.deleteID.
func (a *App) sessionDeleteDialog(c *ui.Context) {
	ui.DialogBase(c.Key("session-delete-dialog"), &a.sidebarUI.deleteOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, .35))
		panel.Width(400).Padding(24).Radius(12).Label(tr(c, "Delete session?"))
		a.floatingSurface(c, panel, 12)
		ui.Column(c).Gap(16).Children(func() {
			ui.Text(c, tr(c, "Delete session?")).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600)
			ui.Text(c, tr(c, "Once deleted, this conversation cannot be recovered. To hide it from the list instead, use Archive.")).FillWidth().TextColor(a.palette.Muted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				textButton(c.Key("session-delete-cancel"), tr(c, "Cancel")).OnClick(func() { a.sidebarUI.deleteOpen = false })
				dangerButton(c.Key("session-delete-confirm"), tr(c, "Delete")).OnClick(func() {
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

// reorderSidebarRows handles a native list drop. Pinned rows keep the pin
// order path; project rows reorder inside their project, while group-view
// conversations may also join the group under the pointer.
func (a *App) reorderSidebarRows(rows []sidebarRow, from []int, to int) {
	if len(from) != 1 || from[0] < 0 || from[0] >= len(rows) {
		return
	}
	row := rows[from[0]]
	if row.SessionGroup {
		a.reorderSessionGroups(rows, row.GroupID, to)
		return
	}
	moved := row.Session
	if moved.ID == "" || moved.Archived {
		return
	}
	if !moved.Pinned && a.state.Settings.SidebarGrouping == "grouped" {
		if groupID, targetID, after, ok := a.sidebarGroupDrop(rows, to); ok {
			a.dispatch(contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{
				GroupID: groupID, SourceID: moved.ID, TargetID: targetID, After: after,
			}})
			return
		}
	}
	if !moved.Pinned && strings.HasPrefix(row.Group, "project:") {
		if target, after, ok := sidebarProjectDrop(rows, row.Group, to); ok {
			if target != moved.ID {
				a.dispatch(contracts.Command{Kind: "session.project.reorder", Reorder: &contracts.ReorderInput{
					SourceID: moved.ID, TargetID: target, After: after,
				}})
			}
			return
		}
		// A project drop may enter the explicit pinned section, but cannot
		// reorder or reassign a conversation in a different project.
		if to < 0 || to >= len(rows) || rows[to].Group != "group:pinned" {
			return
		}
	}
	a.reorderPinnedSession(rows, moved, to)
}

func sidebarProjectDrop(rows []sidebarRow, group string, to int) (string, bool, bool) {
	if to < 0 || to > len(rows) {
		return "", false, false
	}
	if to < len(rows) && rows[to].Group == group {
		if rows[to].Session.ID != "" {
			return rows[to].Session.ID, false, true
		}
		if rows[to].Project {
			for i := to + 1; i < len(rows) && rows[i].Group == group; i++ {
				if rows[i].Session.ID != "" {
					return rows[i].Session.ID, false, true
				}
			}
		}
	}
	if to > 0 && rows[to-1].Group == group && rows[to-1].Session.ID != "" {
		return rows[to-1].Session.ID, true, true
	}
	return "", false, false
}

// reorderSessionGroups moves one group header to where it was dropped. Only
// other group headers anchor the move; member rows use their own reorder.
func (a *App) reorderSessionGroups(rows []sidebarRow, sourceID string, to int) {
	if to < 0 || to > len(rows) {
		return
	}
	targetID, after := "", false
	if to < len(rows) && rows[to].SessionGroup {
		targetID = rows[to].GroupID
	} else {
		for i := to - 1; i >= 0; i-- {
			if rows[i].SessionGroup {
				targetID, after = rows[i].GroupID, true
				break
			}
		}
		if targetID == "" {
			for i := to; i < len(rows); i++ {
				if rows[i].SessionGroup {
					targetID = rows[i].GroupID
					break
				}
			}
		}
	}
	if targetID == "" || targetID == sourceID {
		return
	}
	a.dispatch(contracts.Command{Kind: "group.reorder", Reorder: &contracts.ReorderInput{
		SourceID: sourceID, TargetID: targetID, After: after,
	}})
}

// sidebarGroupDrop resolves a list drop into the destination session group
// and the row the conversation goes before. An empty target appends to the
// group; an empty group ID is the ungrouped root. Only the group view offers
// these destinations.
func (a *App) sidebarGroupDrop(rows []sidebarRow, to int) (groupID, targetID string, after, ok bool) {
	if to < 0 || to > len(rows) {
		return "", "", false, false
	}
	// The destination is the group of the row at the drop point, else that of
	// the nearest row on either side of it.
	group, found := "", false
	for _, index := range []int{to, to - 1, to + 1} {
		if index < 0 || index >= len(rows) {
			continue
		}
		if id, ok := sidebarRowGroupID(rows[index]); ok {
			group, found = id, true
			break
		}
	}
	if !found {
		return "", "", false, false
	}
	inGroup := func(row sidebarRow) bool {
		id, ok := sidebarRowGroupID(row)
		return ok && id == group && row.Session.ID != ""
	}
	// A conversation dropped on a group's own header joins the top of that
	// group; otherwise it takes the place of the row under the pointer.
	if to < len(rows) && rows[to].SessionGroup {
		for i := to + 1; i < len(rows); i++ {
			if inGroup(rows[i]) {
				return group, rows[i].Session.ID, false, true
			}
		}
		return group, "", false, true
	}
	for i := min(to, len(rows)); i < len(rows); i++ {
		if inGroup(rows[i]) {
			return group, rows[i].Session.ID, false, true
		}
	}
	for i := min(to, len(rows)) - 1; i >= 0; i-- {
		if inGroup(rows[i]) {
			return group, rows[i].Session.ID, true, true
		}
	}
	return group, "", false, true
}

// sidebarRowGroupID reports the session group a row belongs to, including the
// section heading that introduces that group's conversations. Pinned,
// timeline and device rows own their own ordering and are not destinations.
func sidebarRowGroupID(row sidebarRow) (string, bool) {
	switch {
	case row.SessionGroup:
		return row.GroupID, true
	case row.Key == "section:grouped":
		return "", true
	case strings.HasPrefix(row.Group, "session-group:"):
		return strings.TrimPrefix(row.Group, "session-group:"), true
	case row.Group == "grouped":
		return "", true
	}
	return "", false
}

// reorderPinnedSession keeps the pin-order path for pinned conversations.
func (a *App) reorderPinnedSession(rows []sidebarRow, moved contracts.Session, to int) {
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
