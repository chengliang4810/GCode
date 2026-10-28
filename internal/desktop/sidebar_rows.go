package desktop

import (
	"gcode/internal/contracts"
	"sort"
	"strings"
)

func (a *App) visibleSessions(archived bool) []contracts.Session {
	var result []contracts.Session
	query := strings.ToLower(strings.TrimSpace(a.search))
	for _, session := range a.state.Sessions {
		if session.Archived != archived || session.ParentSessionID != "" || session.SideChat {
			continue
		}
		if a.sidebarProjectID != "" && session.ProjectID != a.sidebarProjectID {
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
		if !archived && left.Pinned != right.Pinned {
			return left.Pinned
		}
		if !archived && left.Pinned && left.PinOrder != right.PinOrder {
			return left.PinOrder < right.PinOrder
		}
		first, second := left.UpdatedAt, right.UpdatedAt
		if a.state.Settings.SidebarSort == "created" {
			first, second = left.CreatedAt, right.CreatedAt
		}
		if !first.Equal(second) {
			return first.After(second)
		}
		return left.ID < right.ID
	})
	return result
}

type sidebarRow struct {
	Key, Header, ProjectID string
	Count                  int
	More                   bool
	Session                contracts.Session
}

func (a *App) sidebarRows() []sidebarRow {
	sessions := a.visibleSessions(false)
	type group struct {
		key, label, projectID string
		sessions              []contracts.Session
	}
	var groups []group
	positions := map[string]int{}
	for _, session := range sessions {
		key, label, projectID := "group:sessions", "Sessions", ""
		if session.Pinned {
			key, label = "group:pinned", "Pinned"
		} else {
			switch sidebarGrouping(a.state.Settings.SidebarGrouping) {
			case "project":
				key, label, projectID = "group:project:"+session.ProjectID, a.projectNameFor(session.ProjectID), session.ProjectID
			case "device":
				key, label = "group:device:"+session.DeviceID, a.deviceNameFor(session.DeviceID)
			}
		}
		at, ok := positions[key]
		if !ok {
			at = len(groups)
			positions[key] = at
			groups = append(groups, group{key: key, label: label, projectID: projectID})
		}
		groups[at].sessions = append(groups[at].sessions, session)
	}
	if sidebarGrouping(a.state.Settings.SidebarGrouping) == "device" {
		for _, device := range a.state.Devices {
			if device.Current {
				for i, g := range groups {
					if g.key == "group:device:"+device.ID {
						to := 0
						if len(groups) > 0 && groups[0].key == "group:pinned" {
							to = 1
						}
						if i > to {
							copy(groups[to+1:i+1], groups[to:i])
							groups[to] = g
						}
						break
					}
				}
			}
		}
	}
	var rows []sidebarRow
	for _, group := range groups {
		rows = append(rows, sidebarRow{Key: group.key, Header: group.label, ProjectID: group.projectID, Count: len(group.sessions)})
		if !a.expanded[group.key] {
			for _, session := range group.sessions {
				rows = append(rows, sidebarRow{Key: "session:" + session.ID, Session: session})
			}
		}
	}
	if a.state.Settings.SidebarShowArchived {
		archived := a.visibleSessions(true)
		if len(archived) > 0 {
			rows = append(rows, sidebarRow{Key: "group:archived", Header: "Archived", Count: len(archived)})
			if a.sidebarUI.archivedOpen {
				limit := max(10, a.sidebarUI.archiveLimit)
				for _, session := range archived[:min(limit, len(archived))] {
					rows = append(rows, sidebarRow{Key: "session:" + session.ID, Session: session})
				}
				if len(archived) > limit {
					rows = append(rows, sidebarRow{Key: "archived:more", More: true})
				}
			}
		}
	}
	return rows
}
