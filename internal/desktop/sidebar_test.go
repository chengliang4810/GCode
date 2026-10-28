package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarProjectFilterDoesNotChangeComposerProject(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	composerProject := a.projectID
	if err := tt.Click("All projects"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebarProjectMenu || a.projectMenu {
		t.Fatal("sidebar opened composer project picker")
	}
	// Filtering owns its own project ID and leaves the new-session target alone.
	a.sidebarProjectID = "zeron"
	a.sidebarProjectMenu = false
	tt.Frame()
	if a.projectID != composerProject {
		t.Fatal("sidebar filtering changed composer target")
	}
	for _, session := range a.visibleSessions(false) {
		if session.ProjectID != "zeron" {
			t.Fatal("project filter retained an unrelated session")
		}
	}
}

func TestSidebarVirtualizesStressRowsAndPreservesGroups(t *testing.T) {
	a := testApp(t, "stress")
	a.state.Settings.SidebarGrouping = "project"
	tt := ui.NewTester(a.View, 1320, 880)
	rows := a.sidebarRows()
	if len(rows) < 990 {
		t.Fatal("stress rows are not in the model")
	}
	visible := 0
	for _, text := range tt.Texts() {
		if strings.Contains(text, "Workspace review") {
			visible++
		}
	}
	if visible >= 100 {
		t.Fatalf("sidebar eagerly rendered %d stress rows", visible)
	}
	a.expanded["group:project:fieldnotes"] = true
	collapsed := a.sidebarRows()
	for _, row := range collapsed {
		if row.Header == "" && !row.Session.Pinned && !row.Session.Archived && row.Session.ProjectID == "fieldnotes" {
			t.Fatal("collapsed group retained visible child rows")
		}
	}
	zero := 0
	pin := true
	a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: "chat-1", Pinned: &pin, PinOrder: &zero}})
	tt.Frame()
	pinned := []string{}
	for _, row := range a.sidebarRows() {
		if row.Header == "" && row.Session.Pinned {
			pinned = append(pinned, row.Session.ID)
		}
	}
	if len(pinned) < 2 || pinned[0] != "chat-1" || pinned[1] != "chat-0" {
		t.Fatalf("pin reorder lost after flattening: %v", pinned)
	}
}
