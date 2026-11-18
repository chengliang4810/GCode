package desktop

import (
	"testing"

	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestSidebarProjectActivityIncludesHiddenConversations(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		pinned, child, archived bool
		status                  contracts.SessionStatus
		want                    sidebarActivity
	}{
		{name: "working", status: contracts.Working, want: sidebarActivityWorking},
		{name: "pinned", pinned: true, status: contracts.Working, want: sidebarActivityWorking},
		{name: "child", child: true, status: contracts.Working, want: sidebarActivityWorking},
		{name: "archived", archived: true, status: contracts.Working},
		{name: "idle", status: contracts.Idle},
		{name: "awaiting input", status: contracts.AwaitingInput, want: sidebarActivityAwaiting},
		{name: "error", status: contracts.Errored, want: sidebarActivityErrored},
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
					want := sidebarActivityNone
					if row.ProjectID == "0" {
						want = tc.want
					}
					if row.Activity != want {
						t.Fatalf("project %s activity=%v want=%v", row.ProjectID, row.Activity, want)
					}
				}
			}
		})
	}
}

// A folded project group speaks with one voice: approval outranks an error,
// which outranks plain work, and the unread count never joins that badge.
func TestSidebarCollapsedProjectActivityPriority(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.state.Sessions = []contracts.Session{
		{ID: "work", ProjectID: "0", Status: contracts.Working},
		{ID: "gone", ProjectID: "0", Status: contracts.Errored, Unread: true},
		{ID: "ask", ProjectID: "0", Status: contracts.AwaitingInput},
	}
	activity := func() sidebarActivity {
		t.Helper()
		for _, row := range a.projectSidebarRows() {
			if row.ProjectID == "0" {
				return row.Activity
			}
		}
		t.Fatal("project row missing")
		return sidebarActivityNone
	}
	if got := activity(); got != sidebarActivityAwaiting {
		t.Fatalf("activity=%v want approval", got)
	}
	a.state.Sessions[2].Status = contracts.Idle
	if got := activity(); got != sidebarActivityErrored {
		t.Fatalf("activity=%v want error", got)
	}
	a.state.Sessions[1].Status = contracts.Idle
	if got := activity(); got != sidebarActivityWorking {
		t.Fatalf("activity=%v want working", got)
	}
	a.state.Sessions[0].Status = contracts.Idle
	if got := activity(); got != sidebarActivityNone {
		t.Fatalf("activity=%v want none", got)
	}
}

func TestSidebarCollapsedProjectActivityLifecycle(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.state.Sessions = []contracts.Session{{ID: "first", ProjectID: "0", Status: contracts.Working}, {ID: "second", ProjectID: "0", Status: contracts.Working}}
	a.expanded["project:0"] = true
	tt := ui.NewTester(func(c *ui.Context) {
		localize.Set(c, a.state.Settings.General.Language)
		for _, row := range a.projectSidebarRows() {
			if row.ProjectID == "0" {
				a.sidebarProjectRow(c, row)
				return
			}
		}
	}, 350, 80)
	assertActivity := func(want, replace string) {
		t.Helper()
		tt.Frame()
		if _, got := tt.Find(want); got == false {
			t.Fatalf("indicator %q missing: %v", want, tt.Texts())
		}
		if replace != "" {
			if _, got := tt.Find(replace); got {
				t.Fatalf("stale %q indicator remained", replace)
			}
		}
	}
	assertActivity("运行中", "")
	if err := clickAccessible(tt, ui.RoleButton, "fnbot, 运行中"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, got := tt.Find("运行中"); got {
		t.Fatal("collapsed badge survived expansion")
	}
	if err := clickAccessible(tt, ui.RoleButton, "fnbot"); err != nil {
		t.Fatal(err)
	}
	assertActivity("运行中", "")
	a.state.Sessions[0].Status = contracts.Idle
	assertActivity("运行中", "")
	a.state.Sessions[1].Status = contracts.AwaitingInput
	assertActivity("等待授权", "运行中")
	a.state.Sessions[1].Status = contracts.Errored
	assertActivity("失败", "等待授权")
	a.state.Sessions[1].Status = contracts.Idle
	tt.Frame()
	if _, got := tt.Find("失败"); got {
		t.Fatal("error badge survived recovery")
	}
	a.state.Sessions[0].Status = contracts.Working
	assertActivity("运行中", "")
}
