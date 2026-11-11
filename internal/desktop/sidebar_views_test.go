package desktop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// createSidebarGroup dispatches a real group.create and returns the new ID
// from the authoritative snapshot.
func createSidebarGroup(t *testing.T, a *App, title string) string {
	t.Helper()
	before, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, group := range before.SessionGroups {
		seen[group.ID] = true
	}
	a.dispatch(contracts.Command{Kind: "group.create", Text: title})
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range snapshot.SessionGroups {
		if !seen[group.ID] && group.Title == title {
			return group.ID
		}
	}
	t.Fatalf("group %q missing after create", title)
	return ""
}

func sessionRows(rows []sidebarRow) []contracts.Session {
	var out []contracts.Session
	for _, row := range rows {
		if row.Session.ID != "" {
			out = append(out, row.Session)
		}
	}
	return out
}

func TestSidebarGroupedViewKeepsManualGroups(t *testing.T) {
	a := testApp(t, "default")
	group := createSidebarGroup(t, a, "Research")
	sessions := a.visibleSessions()
	a.assignSidebarSessionGroup(sessions[2].ID, group)
	a.assignSidebarSessionGroup(sessions[3].ID, group)
	// Dispatched commands reapply the authoritative snapshot, so the view
	// switch must come after the group assignments.
	a.state.Settings.SidebarGrouping = "grouped"
	rows := a.sidebarRows()
	var header *sidebarRow
	for i := range rows {
		if rows[i].Key == "session-group:"+group {
			header = &rows[i]
		}
	}
	if header == nil || header.Count != 2 || header.Header != "Research" {
		t.Fatalf("group header missing or wrong: %+v", rows)
	}
	count := 0
	for _, row := range rows {
		if row.SessionGroup {
			continue
		}
		if row.Group == "session-group:"+group {
			if !row.Inset {
				t.Fatalf("group member not inset: %+v", row)
			}
			count++
		}
	}
	if count != 2 {
		t.Fatalf("group members missing: %d", count)
	}
	// Renaming goes through the shared inline editor.
	a.startSidebarGroupRename(group)
	a.sidebarUI.renameTitle = "Renamed group"
	a.commitSidebarRename()
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range snapshot.SessionGroups {
		if g.ID == group && g.Title == "Renamed group" {
			found = true
		}
	}
	if !found {
		t.Fatal("group rename did not persist")
	}
	// Deleting the group returns its conversations to the ungrouped list.
	a.dispatch(contracts.Command{Kind: "group.delete", ID: group})
	a.state.Settings.SidebarGrouping = "grouped"
	rows = a.sidebarRows()
	for _, row := range rows {
		if row.Key == "session-group:"+group {
			t.Fatal("deleted group header stayed in rows")
		}
	}
	for _, session := range sessionRows(rows) {
		if session.ID == sessions[2].ID && session.GroupID != "" {
			t.Fatal("group delete retained membership")
		}
	}
}

func TestSidebarTimelineHeadingsFollowCalendarBuckets(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.SidebarGrouping = "timeline"
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.state.Sessions = nil
	a.sidebarUI.limits = map[string]int{"timeline": 100}
	for i, offset := range []time.Duration{time.Hour, 26 * time.Hour, 3 * 24 * time.Hour, 4 * 24 * time.Hour, 10 * 24 * time.Hour, 20 * 24 * time.Hour, 40 * 24 * time.Hour, 100 * 24 * time.Hour} {
		a.state.Sessions = append(a.state.Sessions, contracts.Session{
			ID: fmt.Sprintf("timeline-%d", i), Title: fmt.Sprintf("Timeline %d", i),
			UpdatedAt: now.Add(-offset), CreatedAt: now.Add(-offset),
		})
	}
	var headings []string
	seen := map[string]int{}
	for _, row := range a.sidebarRows() {
		if row.Session.ID != "" {
			continue
		}
		seen[row.Key]++
		headings = append(headings, row.Key)
	}
	want := []string{"timeline:today", "timeline:yesterday", "timeline:daysAgo", "timeline:thisWeek", "timeline:lastWeek", "timeline:thisMonth", "timeline:lastMonth", "timeline:older"}
	if len(headings) != len(want) {
		t.Fatalf("timeline headings wrong: %v", headings)
	}
	for i, key := range want {
		if headings[i] != key || seen[key] != 1 {
			t.Fatalf("timeline heading %d = %s, want %s once", i, headings[i], key)
		}
	}
	if rows := sessionRows(a.sidebarRows()); len(rows) != 8 {
		t.Fatalf("timeline lost conversations: %d", len(rows))
	}
}

func TestSidebarDeviceViewPutsCurrentDeviceFirst(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.SidebarGrouping = "device"
	a.state.Devices = []contracts.Device{
		{ID: "remote", Name: "Studio", Status: "online"},
		{ID: "local", Name: "This device", Current: true},
	}
	a.state.Sessions = []contracts.Session{
		{ID: "s-remote", Title: "Remote talk", DeviceID: "remote", UpdatedAt: a.now()},
		{ID: "s-local", Title: "Local talk", DeviceID: "local", UpdatedAt: a.now()},
		{ID: "s-unknown", Title: "Ghost talk", DeviceID: "gone", UpdatedAt: a.now()},
	}
	var headers []string
	memberOf := map[string]int{}
	for _, row := range a.sidebarRows() {
		if row.Session.ID == "" {
			if row.Key != "device:remote" && row.Key != "device:local" && row.Key != "device:gone" {
				t.Fatalf("unexpected sidebar row %+v", row)
			}
			headers = append(headers, row.Key)
			continue
		}
		memberOf[row.Group]++
	}
	if len(headers) != 3 || headers[0] != "device:local" || headers[1] != "device:remote" || headers[2] != "device:gone" {
		t.Fatalf("device order wrong: %v", headers)
	}
	if memberOf["device:local"] != 1 || memberOf["device:remote"] != 1 || memberOf["device:gone"] != 1 {
		t.Fatalf("device membership wrong: %v", memberOf)
	}
}

func TestSidebarGroupedDropResolvesDestinationGroup(t *testing.T) {
	a := testApp(t, "default")
	group := createSidebarGroup(t, a, "Research")
	sessions := a.visibleSessions()
	a.assignSidebarSessionGroup(sessions[2].ID, group)
	a.assignSidebarSessionGroup(sessions[3].ID, group)
	a.state.Settings.SidebarGrouping = "grouped"
	rows := a.sidebarRows()
	indexOf := func(id string) int {
		for i, row := range rows {
			if row.Session.ID == id {
				return i
			}
		}
		t.Fatalf("row %s missing", id)
		return -1
	}
	// Dropping on a group's first member joins that group before it.
	if got, target, after, ok := a.sidebarGroupDrop(rows, indexOf(sessions[2].ID)); !ok || got != group || target != sessions[2].ID || after {
		t.Fatalf("group member drop resolved to %q %q after=%v ok=%v", got, target, after, ok)
	}
	// Dropping on the group header inserts at the top of that group.
	header := indexOf(sessions[2].ID) - 1
	if got, target, _, ok := a.sidebarGroupDrop(rows, header); !ok || got != group || target != sessions[2].ID {
		t.Fatalf("group header drop resolved to %q %q ok=%v", got, target, ok)
	}
	// Dropping on the ungrouped section stays in the ungrouped root.
	if got, target, _, ok := a.sidebarGroupDrop(rows, indexOf(sessions[5].ID)); !ok || got != "" || target != sessions[5].ID {
		t.Fatalf("ungrouped drop resolved to %q %q ok=%v", got, target, ok)
	}
	// The resolved commands move the conversation between groups for real.
	if got, target, after, ok := a.sidebarGroupDrop(rows, indexOf(sessions[2].ID)); ok {
		a.dispatch(contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{
			GroupID: got, SourceID: sessions[4].ID, TargetID: target, After: after,
		}})
	}
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if session.ID == sessions[4].ID && session.GroupID != group {
			t.Fatalf("drag into group did not persist: %q", session.GroupID)
		}
	}
	// Dragging the same conversation onto the ungrouped root detaches it.
	snapshot, _ = a.service.Snapshot(context.Background())
	a.apply(snapshot)
	a.state.Settings.SidebarGrouping = "grouped"
	rows = a.sidebarRows()
	ungrouped := -1
	for i, row := range rows {
		if row.Session.ID != "" && row.Group == "grouped" {
			ungrouped = i
			break
		}
	}
	if ungrouped < 0 {
		t.Fatal("no ungrouped conversation row")
	}
	if got, target, after, ok := a.sidebarGroupDrop(rows, ungrouped); ok {
		a.dispatch(contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{
			GroupID: got, SourceID: sessions[4].ID, TargetID: target, After: after,
		}})
	}
	snapshot, _ = a.service.Snapshot(context.Background())
	for _, session := range snapshot.Sessions {
		if session.ID == sessions[4].ID && session.GroupID != "" {
			t.Fatalf("drag out of group kept membership %q", session.GroupID)
		}
	}
}

func TestSidebarGroupedDragReordersMembers(t *testing.T) {
	a := testApp(t, "default")
	group := createSidebarGroup(t, a, "Research")
	sessions := a.visibleSessions()
	a.assignSidebarSessionGroup(sessions[2].ID, group)
	a.assignSidebarSessionGroup(sessions[3].ID, group)
	a.state.Settings.SidebarGrouping = "grouped"
	tt := ui.NewTester(a.View, 1320, 880)
	first, _ := tt.Find(sessions[2].Title)
	second, _ := tt.Find(sessions[3].Title)
	tt.Press(second.X+80, second.Y+second.H/2)
	tt.Move(second.X+90, second.Y+second.H/2)
	tt.Move(first.X+80, first.Y+3)
	tt.Release(first.X+80, first.Y+3)
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ranks := map[string]int{}
	for _, session := range snapshot.Sessions {
		if session.GroupID == group {
			ranks[session.ID] = session.GroupOrder
		}
	}
	if ranks[sessions[3].ID] >= ranks[sessions[2].ID] {
		t.Fatalf("in-group drag did not reorder members: %v", ranks)
	}
}

func TestSidebarProjectDragReordersConversations(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			a := testApp(t, "default")
			settings := a.state.Settings
			settings.SidebarGrouping = "project"
			settings.Appearance.Mode = appearance
			a.dispatch(contracts.Command{Kind: "settings.update", Settings: &settings})
			var members []contracts.Session
			for _, row := range a.sidebarRows() {
				if row.Group == "project:fieldnotes" && row.Session.ID != "" {
					members = append(members, row.Session)
				}
			}
			if len(members) < 2 {
				t.Fatal("fixture needs two conversations in the same project")
			}
			first, second := members[0], members[1]
			tt := ui.NewTester(a.View, 900, 700)
			top, ok := tt.Find(first.Title)
			if !ok {
				t.Fatal("first conversation missing")
			}
			bottom, ok := tt.Find(second.Title)
			if !ok {
				t.Fatal("second conversation missing")
			}
			tt.Press(bottom.X+80, bottom.Y+bottom.H/2)
			tt.Move(bottom.X+90, bottom.Y+bottom.H/2)
			tt.Move(top.X+80, top.Y+3)
			tt.Release(top.X+80, top.Y+3)
			var order []string
			for _, row := range a.sidebarRows() {
				if row.Group == "project:fieldnotes" && row.Session.ID != "" {
					order = append(order, row.Session.ID)
					if row.Session.Pinned {
						t.Fatal("project reorder pinned a conversation")
					}
				}
			}
			if len(order) < 2 || order[0] != second.ID || order[1] != first.ID {
				t.Fatalf("project drag did not persist display order: %v, error=%s", order, a.errorText)
			}
		})
	}
}

func TestSidebarGroupHeaderDragReordersGroups(t *testing.T) {
	a := testApp(t, "default")
	alpha := createSidebarGroup(t, a, "Alpha")
	beta := createSidebarGroup(t, a, "Beta")
	a.state.Settings.SidebarGrouping = "grouped"
	rows := a.sidebarRows()
	headerIndex := func(id string) int {
		for i, row := range rows {
			if row.SessionGroup && row.GroupID == id {
				return i
			}
		}
		t.Fatalf("group header %s missing", id)
		return -1
	}
	from := headerIndex(beta)
	to := headerIndex(alpha)
	a.reorderSidebarRows(rows, []int{from}, to)
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ranks := map[string]int{}
	for _, group := range snapshot.SessionGroups {
		ranks[group.ID] = group.Order
	}
	if ranks[beta] >= ranks[alpha] {
		t.Fatalf("dragging a header did not move the group: %v", ranks)
	}
}
