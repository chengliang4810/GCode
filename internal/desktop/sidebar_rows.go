package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"sort"
	"strings"
	"time"
)

// visibleSessions lists active top-level conversations ordered by last update.
// Archived conversations stay out of every sidebar view; they live in settings.
func (a *App) visibleSessions() []contracts.Session {
	var result []contracts.Session
	query := strings.ToLower(strings.TrimSpace(a.search))
	for _, session := range a.state.Sessions {
		if session.Archived || session.ParentSessionID != "" || session.SideChat {
			continue
		}
		text := session.Title + " " + session.Branch + " " + a.projectNameFor(session.ProjectID) + " " + a.deviceNameFor(session.DeviceID)
		if query != "" && !strings.Contains(strings.ToLower(text), query) {
			continue
		}
		result = append(result, session)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Pinned != right.Pinned {
			return left.Pinned
		}
		if left.Pinned && left.PinOrder != right.PinOrder {
			return left.PinOrder < right.PinOrder
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		return left.ID < right.ID
	})
	return result
}

type sidebarRow struct {
	Key, Header, ProjectID string
	Group                  string
	GroupID                string
	Count                  int
	More                   bool
	Empty                  bool
	Project                bool
	SessionGroup           bool
	Inset                  bool
	Session                contracts.Session
}

// sidebarRows projects the active conversations into the configured view:
// the project accordion, user-defined groups, the update timeline, or device
// groups. Archived data never renders here.
func (a *App) sidebarRows() []sidebarRow {
	switch a.state.Settings.SidebarGrouping {
	case "grouped":
		return a.groupedSidebarRows()
	case "timeline":
		return a.timelineSidebarRows()
	case "device":
		return a.deviceSidebarRows()
	default:
		return a.projectSidebarRows()
	}
}

func (a *App) projectSidebarRows() []sidebarRow {
	rows := []sidebarRow{}
	byProject := make(map[string][]contracts.Session)
	var pinned, unassigned []contracts.Session
	projects := make(map[string]bool, len(a.state.Projects))
	for _, project := range a.state.Projects {
		projects[project.ID] = true
	}
	for _, session := range a.visibleSessions() {
		switch {
		case session.Pinned:
			pinned = append(pinned, session)
		case projects[session.ProjectID]:
			byProject[session.ProjectID] = append(byProject[session.ProjectID], session)
		default:
			// A removed project must not make its retained conversations vanish.
			unassigned = append(unassigned, session)
		}
	}
	rows = a.appendSidebarPinned(rows, pinned)
	rows = append(rows, sidebarRow{Key: "section:projects", Header: "Projects"})
	if !a.sidebarUI.projectsClosed {
		for _, project := range a.state.Projects {
			key := "project:" + project.ID
			sessions := byProject[project.ID]
			rows = append(rows, sidebarRow{
				Key: "sidebar-project:" + project.ID, Header: project.Name, ProjectID: project.ID,
				Group: key, Project: true, Count: len(sessions),
			})
			if !a.expanded[key] {
				rows = a.appendSidebarConversations(rows, key, sessions, true)
			}
		}
	}
	rows = append(rows, sidebarRow{Key: "section:conversations", Header: "Conversations"})
	if !a.sidebarUI.conversationsClosed {
		rows = a.appendSidebarConversations(rows, "conversations", unassigned, false)
	}
	return rows
}

// groupedSidebarRows mirrors ZCode's group view: pinned conversations stay on
// top, user groups follow in their manual order, ungrouped conversations keep
// a root list. Groups are empty containers until conversations move in.
func (a *App) groupedSidebarRows() []sidebarRow {
	rows := []sidebarRow{}
	var pinned, ungrouped []contracts.Session
	members := make(map[string][]contracts.Session)
	groups := make(map[string]bool, len(a.state.SessionGroups))
	for _, group := range a.state.SessionGroups {
		groups[group.ID] = true
	}
	for _, session := range a.visibleSessions() {
		switch {
		case session.Pinned:
			pinned = append(pinned, session)
		case groups[session.GroupID]:
			members[session.GroupID] = append(members[session.GroupID], session)
		default:
			ungrouped = append(ungrouped, session)
		}
	}
	rows = a.appendSidebarPinned(rows, pinned)
	for _, group := range a.orderedSessionGroups() {
		key := "session-group:" + group.ID
		sortSidebarGroupMembers(members[group.ID])
		rows = append(rows, sidebarRow{Key: key, Header: group.Title, Group: key, GroupID: group.ID, SessionGroup: true, Count: len(members[group.ID])})
		if !a.expanded[key] {
			rows = a.appendSidebarConversations(rows, key, members[group.ID], true)
		}
	}
	sortSidebarGroupMembers(ungrouped)
	rows = append(rows, sidebarRow{Key: "section:grouped", Header: "Conversations"})
	if !a.sidebarUI.conversationsClosed {
		rows = a.appendSidebarConversations(rows, "grouped", ungrouped, false)
	}
	return rows
}

// sortSidebarGroupMembers orders a group's conversations by their manual
// GroupOrder, keeping the update order for members never dragged.
func sortSidebarGroupMembers(sessions []contracts.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].GroupOrder != sessions[j].GroupOrder {
			return sessions[i].GroupOrder < sessions[j].GroupOrder
		}
		if !sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		}
		return sessions[i].ID < sessions[j].ID
	})
}

// timelineSidebarRows shows every active conversation in one update-ordered
// list with localized date headings between day buckets.
func (a *App) timelineSidebarRows() []sidebarRow {
	rows := []sidebarRow{}
	sessions := a.visibleSessions()
	limit := max(design.Sidebar().PageSize, a.sidebarUI.limits["timeline"])
	now := a.now()
	previous := ""
	shown := 0
	for _, session := range sessions {
		if shown >= limit && a.search == "" && session.ID != a.sessionID && session.ID != a.sidebarUI.renameID {
			break
		}
		key, header := timelineHeading(session.UpdatedAt, now, a.state.Settings.General.Language)
		if key != previous {
			rows = append(rows, sidebarRow{Key: "timeline:" + key, Header: header})
			previous = key
		}
		rows = append(rows, sidebarRow{Key: "session:" + session.ID, Group: "timeline", Session: session})
		shown++
	}
	if len(sessions) > shown && a.search == "" {
		rows = append(rows, sidebarRow{Key: "timeline:more", Group: "timeline", More: true})
	}
	return rows
}

// deviceSidebarRows groups conversations by their host device; the current
// device keeps its first position and unknown hosts share a trailing bucket.
func (a *App) deviceSidebarRows() []sidebarRow {
	rows := []sidebarRow{}
	var pinned []contracts.Session
	members := make(map[string][]contracts.Session)
	for _, session := range a.visibleSessions() {
		if session.Pinned {
			pinned = append(pinned, session)
			continue
		}
		members[session.DeviceID] = append(members[session.DeviceID], session)
	}
	rows = a.appendSidebarPinned(rows, pinned)
	known := make(map[string]bool, len(a.state.Devices))
	appendDevice := func(id, header string) {
		key := "device:" + id
		rows = append(rows, sidebarRow{Key: key, Header: header, Group: key, Count: len(members[id])})
		if !a.expanded[key] {
			rows = a.appendSidebarConversations(rows, key, members[id], true)
		}
	}
	for _, device := range a.state.Devices {
		if device.Current {
			known[device.ID] = true
			appendDevice(device.ID, a.deviceDisplayName(device))
		}
	}
	for _, device := range a.state.Devices {
		if device.Current {
			continue
		}
		known[device.ID] = true
		appendDevice(device.ID, a.deviceDisplayName(device))
	}
	for id := range members {
		if !known[id] {
			appendDevice(id, a.t("This device"))
		}
	}
	return rows
}

func (a *App) orderedSessionGroups() []contracts.SessionGroup {
	groups := append([]contracts.SessionGroup(nil), a.state.SessionGroups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Order < groups[j].Order })
	return groups
}

func (a *App) appendSidebarPinned(rows []sidebarRow, pinned []contracts.Session) []sidebarRow {
	if len(pinned) == 0 {
		return rows
	}
	rows = append(rows, sidebarRow{Key: "group:pinned", Header: "Pinned", Count: len(pinned)})
	if !a.expanded["group:pinned"] {
		for _, session := range pinned {
			rows = append(rows, sidebarRow{Key: "session:" + session.ID, Group: "group:pinned", Session: session})
		}
	}
	return rows
}

// timelineHeading buckets one instant into ZCode's timeline sections. Days are
// calendar days, so DST-shortened days do not collapse into their neighbours.
func timelineHeading(at, now time.Time, language string) (string, string) {
	day := func(t time.Time) int {
		t = t.Local()
		return t.Year()*10000 + int(t.Month())*100 + t.Day()
	}
	today, atDay := day(now), day(at)
	daysAgo := today - atDay
	weekdayOffset := int(now.Local().Weekday())
	if i18n.Normalize(language) != i18n.English {
		weekdayOffset = (weekdayOffset + 6) % 7
	}
	weekStart := today - weekdayOffset
	monthStart := day(time.Date(now.Local().Year(), now.Local().Month(), 1, 0, 0, 0, 0, now.Local().Location()))
	lastMonthStart := day(time.Date(now.Local().Year(), now.Local().Month()-1, 1, 0, 0, 0, 0, now.Local().Location()))
	switch {
	case daysAgo <= 0:
		return "today", aLanguage(language, "Today")
	case daysAgo == 1:
		return "yesterday", aLanguage(language, "Yesterday")
	case daysAgo <= 3:
		return "daysAgo", aLanguage(language, "%d days ago", daysAgo)
	case atDay >= weekStart:
		return "thisWeek", aLanguage(language, "This week")
	case atDay >= weekStart-7:
		return "lastWeek", aLanguage(language, "Last week")
	case atDay >= monthStart:
		return "thisMonth", aLanguage(language, "This month")
	case atDay >= lastMonthStart:
		return "lastMonth", aLanguage(language, "Last month")
	default:
		return "older", aLanguage(language, "Earlier")
	}
}

func aLanguage(language, message string, args ...any) string {
	return i18n.In(language, message, args...)
}

func (a *App) appendSidebarConversations(rows []sidebarRow, group string, sessions []contracts.Session, inset bool) []sidebarRow {
	if len(sessions) == 0 {
		return append(rows, sidebarRow{Key: group + ":empty", Group: group, Empty: true, Inset: inset})
	}
	limit := max(design.Sidebar().PageSize, a.sidebarUI.limits[group])
	if a.sidebarUI.revealID != "" && a.sidebarUI.revealGroup == group {
		for i, session := range sessions {
			if session.ID == a.sidebarUI.revealID {
				// A completed organization action must remain visible after its
				// one-shot scroll request is consumed, even beyond the first page.
				limit = max(limit, (i/design.Sidebar().PageSize+1)*design.Sidebar().PageSize)
				if a.sidebarUI.limits == nil {
					a.sidebarUI.limits = make(map[string]int)
				}
				a.sidebarUI.limits[group] = limit
				break
			}
		}
	}
	for i, session := range sessions {
		if i >= limit && a.search == "" && session.ID != a.sessionID && session.ID != a.sidebarUI.renameID {
			continue
		}
		rows = append(rows, sidebarRow{Key: "session:" + session.ID, Group: group, Session: session, Inset: inset})
	}
	if len(sessions) > limit && a.search == "" {
		rows = append(rows, sidebarRow{Key: group + ":more", Group: group, More: true, Inset: inset})
	}
	return rows
}
