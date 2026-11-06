package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarProjectTreeMembershipAndIndependentPagination(t *testing.T) {
	a := sidebarLayoutApp(t)
	base := a.state.Sessions[0]
	for i := range 7 {
		s := base
		s.ID, s.Title, s.ProjectID = fmt.Sprintf("second-%d", i), fmt.Sprintf("Second %d", i), "1"
		a.state.Sessions = append(a.state.Sessions, s)
	}
	pin := base
	pin.ID, pin.Pinned = "pin", true
	child := base
	child.ID, child.ParentSessionID = "child", base.ID
	side := base
	side.ID, side.SideChat = "side", true
	orphan := base
	orphan.ID, orphan.ProjectID = "removed-project", "removed"
	a.state.Sessions = append(a.state.Sessions, pin, child, side, orphan)
	check := func(wantFirst, wantSecond int) {
		t.Helper()
		counts, seen := map[string]int{}, map[string]bool{}
		group := ""
		for _, row := range a.sidebarRows() {
			if row.Project {
				group = row.ProjectID
			} else if row.Header != "" {
				group = row.Key
			}
			if row.Session.ID == "" {
				continue
			}
			s := row.Session
			if seen[s.ID] || s.ParentSessionID != "" || s.SideChat {
				t.Fatalf("duplicate or nested conversation in tree: %+v", s)
			}
			seen[s.ID] = true
			wantGroup := s.ProjectID
			if s.Pinned {
				wantGroup = "group:pinned"
			} else if s.ProjectID == "" || s.ProjectID == "removed" {
				wantGroup = "section:conversations"
			}
			if group != wantGroup {
				t.Fatalf("%s belongs to %s, rendered under %s", s.ID, wantGroup, group)
			}
			counts[group]++
		}
		if counts["0"] != wantFirst || counts["1"] != wantSecond || !seen["pin"] || !seen["removed-project"] {
			t.Fatalf("incorrect project pagination or missing retained conversation: %v", counts)
		}
	}
	check(5, 5)
	a.sidebarUI.limits = map[string]int{"project:0": 10}
	check(7, 5)
	a.expanded["project:0"] = true
	check(0, 5)
	a.expanded["project:0"] = false
	check(7, 5)
}

func TestSidebarExplicitNavigationRevealsSameConversationAndDeepProject(t *testing.T) {
	a := sidebarLayoutApp(t)
	tt := ui.NewTester(a.View, 900, 600)
	target := a.state.Sessions[6]
	a.selectSession(target.ID)
	tt.Frame()
	a.expanded["project:0"] = true
	a.sidebarUI.projectsClosed = true
	tt.Frame()
	a.selectSession(target.ID)
	tt.Frame()
	if a.expanded["project:0"] || a.sidebarUI.projectsClosed {
		t.Fatal("explicit navigation to the same conversation did not open its ancestors")
	}
	r, ok := tt.Find(target.Title)
	if !ok || r.X >= a.sidebarWidth || r.Y < 0 || r.Y+r.H > 600-54 {
		t.Fatalf("selected conversation did not scroll into the list viewport: %+v", r)
	}
	// Move the target far beyond the visible viewport using real project IDs.
	for i := range 30 {
		a.state.Projects = append(a.state.Projects, contracts.Project{ID: fmt.Sprintf("deep-%d", i), Name: fmt.Sprintf("Deep project %d", i)})
	}
	a.state.Sessions[6].ProjectID = "deep-29"
	a.expanded["project:deep-29"] = true
	a.selectSession(target.ID)
	tt.Frame()
	r, ok = tt.Find(target.Title)
	if !ok || r.X >= a.sidebarWidth || r.Y < 0 || r.Y+r.H > 600-54 {
		t.Fatalf("deep project selection stayed outside viewport: %+v", r)
	}
	a.expanded["project:deep-29"] = true
	tt.Frame()
	if r, ok := tt.Find(target.Title); !a.expanded["project:deep-29"] || (ok && r.X < a.sidebarWidth) {
		t.Fatalf("ordinary frames reopened a manually collapsed selected project: closed=%v bounds=%+v", a.expanded["project:deep-29"], r)
	}
}

func TestSidebarRenameOnlyRevealsTargetProject(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.sidebarUI.projectsClosed = true
	a.expanded["project:0"], a.expanded["project:1"] = true, true
	tt := ui.NewTester(a.View, 900, 600)
	a.startSidebarRename("layout-6")
	tt.Frame()
	if a.sidebarUI.projectsClosed || a.expanded["project:0"] || !a.expanded["project:1"] {
		t.Fatal("rename did not exclusively reveal the owning project")
	}
	if !tt.Focused("对话名称") {
		t.Fatal("hidden paginated conversation rename did not receive focus")
	}
	tt.Key(0, ui.KeyEscape)
	if a.sidebarUI.renameID != "" {
		t.Fatal("Escape left the rename editor open")
	}
}

func TestSidebarExcludesArchivedConversations(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.state.Sessions = nil
	for i := range 25 {
		a.state.Sessions = append(a.state.Sessions, contracts.Session{
			ID: fmt.Sprintf("archive-%02d", i), Title: fmt.Sprintf("Archived %02d", i), Archived: true,
			UpdatedAt: time.Date(2026, 10, 30, 0, i, 0, 0, time.UTC),
		})
	}
	stabilizeFixtureSnapshot(t, a)
	a.selectSession("archive-00")
	for _, row := range a.sidebarRows() {
		if row.Key == "group:archived" || row.Session.Archived {
			t.Fatalf("archived conversation leaked into the sidebar: %+v", row)
		}
	}
	if len(a.visibleSessions()) != 0 {
		t.Fatal("archived conversations counted as visible")
	}
}

func TestSidebarPinUnpinAndRestoreRevealDestination(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 1100)
	session := sidebarSession(t, a, "chat-0")
	target := a.projectID
	a.draft = "Keep this draft"
	a.sidebarUI.projectsClosed = true
	a.expanded["project:"+session.ProjectID] = true
	tt.Frame()
	if err := tt.RightClick(session.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Unpin"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarUI.projectsClosed || a.expanded["project:"+session.ProjectID] || !tt.HasText(session.Title) {
		t.Fatal("unpin did not reveal the destination project")
	}
	a.expanded["group:pinned"] = true
	tt.Frame()
	if err := tt.RightClick(session.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Pin"); err != nil {
		t.Fatal(err)
	}
	if a.expanded["group:pinned"] || !tt.HasText(session.Title) {
		t.Fatal("pin did not reveal the pinned section")
	}
	pinned := sidebarSession(t, a, session.ID)
	a.archiveSidebarSession(session.ID)
	// Settings restores without forcing the pinned group open; the restored
	// conversation must return under its pinned identity.
	a.dispatch(contracts.Command{Kind: "session.archive", ID: session.ID, Enabled: false})
	tt.Frame()
	restored := sidebarSession(t, a, session.ID)
	if restored.Archived || !restored.Pinned || restored.PinOrder != pinned.PinOrder {
		t.Fatal("restore lost pin identity/order")
	}
	restoredRow := false
	for _, row := range a.sidebarRows() {
		if row.Session.ID == session.ID {
			restoredRow = true
			if row.Group != "group:pinned" {
				t.Fatalf("restored conversation left the pinned group: %s", row.Group)
			}
		}
	}
	if !restoredRow {
		t.Fatal("restored pin missing from sidebar rows")
	}
	if a.projectID != target || a.draft != "Keep this draft" {
		t.Fatal("sidebar organization changed the composer target or draft")
	}
}

func TestSidebarRelativeAgeUsesCurrentClock(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		want string
	}{{-time.Hour, "now"}, {30 * time.Second, "now"}, {17 * time.Minute, "17m"}, {6 * time.Hour, "6h"}, {72 * time.Hour, "3d"}} {
		if got := relativeAge(now.Add(-tc.age), now); got != tc.want {
			t.Fatalf("age %s: got %q, want %q", tc.age, got, tc.want)
		}
	}
}

func TestSidebarNewProjectRevealWaitsForAuthoritativeSnapshot(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.sidebarUI.projectsClosed = true
	a.draft = "Keep pending project draft"
	request := contracts.ProjectCreate{DeviceID: "local", Name: "Created after reply", Path: "/pending-project"}
	a.newProject.submission, a.newProject.submissionID = &request, "pending-project"
	a.projectOpen = true
	a.projectCommandFinished(contracts.Command{Kind: "project.create", ID: "pending-project", ProjectCreate: &request}, nil)
	tt := ui.NewTester(a.View, 900, 600)
	if a.sidebarUI.revealProject == nil || a.sidebarUI.projectsClosed || a.projectOpen {
		t.Fatal("command reply discarded the pending project reveal before its snapshot")
	}
	a.state.Projects = append(a.state.Projects, contracts.Project{
		ID: "created", Name: request.Name, Path: request.Path, DeviceID: request.DeviceID,
	})
	a.apply(a.state)
	tt.Frame()
	r, ok := tt.Find(request.Name)
	if !ok || r.X >= a.sidebarWidth || r.Y < 0 || r.Y+r.H > 600-54 || a.sidebarUI.revealProject != nil {
		t.Fatalf("authoritative project did not become visible: %+v", r)
	}
	if a.projectID != "created" || a.draft != "Keep pending project draft" {
		t.Fatal("asynchronous project creation did not select the project and preserve the draft")
	}
}

func TestSidebarOrganizationRevealWaitsForDestinationAndKeepsOlderPage(t *testing.T) {
	t.Run("unpin", func(t *testing.T) {
		a := sidebarLayoutApp(t)
		target := a.state.Sessions[6]
		a.state.Sessions[6].Pinned = true
		stabilizeFixtureSnapshot(t, a)
		a.selectSession("extra-0")
		tt := ui.NewTester(a.View, 900, 600)
		a.draft = "Keep the other conversation draft"
		a.revealSidebarSession(target)
		// A background command can finish after frames using the old
		// snapshot. Finding the same ID at its old location is insufficient.
		tt.Frame()
		if a.sidebarUI.revealID != target.ID {
			t.Fatal("old snapshot consumed the destination reveal")
		}
		a.state.Sessions[6] = target
		for range 3 {
			tt.Frame()
			r, ok := tt.Find(target.Title)
			if !ok || r.X >= a.sidebarWidth || r.Y < 0 || r.Y+r.H > 600-54 {
				t.Fatalf("older non-current conversation disappeared after unpin: %+v", r)
			}
		}
		if a.sessionID != "extra-0" || a.draft != "Keep the other conversation draft" {
			t.Fatal("organization reveal navigated away from the current draft")
		}
		if a.sidebarUI.revealID != "" {
			t.Fatal("committed destination did not consume the scroll request")
		}
	})
}
