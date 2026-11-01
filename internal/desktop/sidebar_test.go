package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarToggleAllProjectsPreservesComposer(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	target := a.projectID
	a.draft = "Keep draft"
	if err := tt.Click("Collapse all projects"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarProjectsExpanded() || a.projectID != target || a.draft != "Keep draft" {
		t.Fatal("collapse all changed the composer or left a project open")
	}
	if err := tt.Click("Expand all projects"); err != nil {
		t.Fatal(err)
	}
	for _, project := range a.state.Projects {
		if a.expanded["project:"+project.ID] {
			t.Fatal("expand all left a project closed")
		}
	}
}

func TestSidebarVirtualizesStressRowsAndPreservesGroups(t *testing.T) {
	a := testApp(t, "stress")
	a.sidebarUI.limits = map[string]int{"project:rcode": 2000, "project:fieldnotes": 2000, "conversations": 2000}
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
	a.expanded["project:fieldnotes"] = true
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
