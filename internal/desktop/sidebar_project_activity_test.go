package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarProjectActivityIncludesHiddenConversations(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		pinned, child, archived bool
		status                  contracts.SessionStatus
		want                    bool
	}{
		{name: "working", status: contracts.Working, want: true},
		{name: "pinned", pinned: true, status: contracts.Working, want: true},
		{name: "child", child: true, status: contracts.Working, want: true},
		{name: "archived", archived: true, status: contracts.Working},
		{name: "idle", status: contracts.Idle},
		{name: "awaiting input", status: contracts.AwaitingInput},
		{name: "error", status: contracts.Errored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := sidebarLayoutApp(t)
			a.state.Sessions = []contracts.Session{{ID: "activity", ProjectID: "0", Status: tc.status, Pinned: tc.pinned, Archived: tc.archived}}
			if tc.child {
				a.state.Sessions[0].ParentSessionID = "parent"
			}
			a.search = "does not match any conversation"
			for _, row := range a.projectSidebarRows() {
				if row.Project {
					want := tc.want && row.ProjectID == "0"
					if row.Working != want {
						t.Fatalf("project %s activity=%v want=%v", row.ProjectID, row.Working, want)
					}
				}
			}
		})
	}
}

func TestSidebarCollapsedProjectActivityLifecycle(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.state.Sessions = []contracts.Session{{ID: "first", ProjectID: "0", Status: contracts.Working}, {ID: "second", ProjectID: "0", Status: contracts.Working}}
	a.expanded["project:0"] = true
	tt := ui.NewTester(func(c *ui.Context) {
		for _, row := range a.projectSidebarRows() {
			if row.ProjectID == "0" {
				a.sidebarProjectRow(c, row)
				return
			}
		}
	}, 350, 80)
	assertRunning := func(want bool) {
		t.Helper()
		tt.Frame()
		if _, got := tt.Find("Running"); got != want {
			t.Fatalf("indicator=%v want=%v", got, want)
		}
	}
	assertRunning(true)
	if err := tt.Click("fnbot"); err != nil {
		t.Fatal(err)
	}
	assertRunning(false)
	if err := tt.Click("fnbot"); err != nil {
		t.Fatal(err)
	}
	assertRunning(true)
	a.state.Sessions[0].Status = contracts.Idle
	assertRunning(true)
	a.state.Sessions[1].Status = contracts.Idle
	assertRunning(false)
	a.state.Sessions[0].Status = contracts.Working
	assertRunning(true)
}
