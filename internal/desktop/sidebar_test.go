package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarProjectSectionCollapsesChildrenAndPreservesComposer(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.draft = "Keep draft"
	target := a.projectID
	tt := ui.NewTester(a.View, 1440, 1400)
	if _, ok := tt.Find("Collapse all projects"); ok {
		t.Fatal("removed bulk toggle remains")
	}
	if err := tt.Click("项目"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebarUI.projectsClosed || a.projectID != target || a.draft != "Keep draft" {
		t.Fatal("section collapse changed composer state")
	}
	for _, project := range a.state.Projects {
		if !a.expanded["project:"+project.ID] {
			t.Fatal("project children remain expanded")
		}
	}
	if err := tt.Click("项目"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("第二项目的对话") || tt.HasText("了解对话输出目录内容") {
		t.Fatal("reopening section reopened child conversations")
	}
	if err := tt.Click("bzaqsc-new"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("第二项目的对话") || tt.HasText("了解对话输出目录内容") {
		t.Fatal("project did not expand independently")
	}
	if a.projectID != target || a.draft != "Keep draft" {
		t.Fatal("individual expansion changed composer")
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

func TestSidebarConversationHeadingNewEntry(t *testing.T) {
	a := sidebarLayoutApp(t)
	a.selectSession(a.state.Sessions[0].ID)
	a.draft = "old draft"
	tt := ui.NewTester(func(c *ui.Context) {
		a.sidebarSection(c, sidebarRow{Key: "section:conversations", Header: "Conversations"})
	}, 350, 100)
	if err := tt.Click("New conversation"); err != nil {
		t.Fatal(err)
	}
	if a.sessionID != "" || a.draft != "" {
		t.Fatal("new conversation entry did not open a blank draft")
	}
}
