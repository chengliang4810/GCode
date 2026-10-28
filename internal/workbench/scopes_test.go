package workbench

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSessionWorkspaceRestoresTabsAndUnsavedEditorIndependently(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	fileID := ""
	for _, file := range snapshot.Files {
		if file.Kind == "file" && !file.ReadOnly {
			fileID = file.ID
			break
		}
	}
	s.SwitchSession("first")
	s.OpenFile(fileID)
	firstDocument := s.documents[fileID]
	firstDocument.editor.Buffer.Set("first unsaved text")
	s.fileSearch = "first search"
	s.SwitchSession("second")
	if s.TabCount() != 0 || s.fileSearch != "" {
		t.Fatal("previous conversation's workspace leaked")
	}
	s.OpenFile(fileID)
	if s.documents[fileID] == firstDocument || s.documents[fileID].editor.Buffer.String() == "first unsaved text" {
		t.Fatal("new conversation shares unsaved editor state")
	}
	s.documents[fileID].editor.Buffer.Set("second unsaved text")
	s.OpenSurface("history")
	s.SwitchSession("first")
	if s.TabCount() != 1 || s.documents[fileID] != firstDocument || s.documents[fileID].editor.Buffer.String() != "first unsaved text" || s.fileSearch != "first search" {
		t.Fatal("first conversation's workspace did not restore")
	}
	s.SwitchSession("second")
	if s.TabCount() != 2 || s.ActiveKind() != "history" || s.documents[fileID].editor.Buffer.String() != "second unsaved text" {
		t.Fatal("second conversation's workspace did not restore")
	}
}

func TestSideSessionRendererAndCentralFallback(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	id := snapshot.Sessions[0].ID
	central := ""
	s.OnOpenSession = func(id string) { central = id }
	s.OpenSession(id)
	if central != id || s.TabCount() != 0 {
		t.Fatal("missing side renderer did not use central navigation")
	}
	rendered := ""
	s.RenderSession = func(c *ui.Context, id string) { rendered = id; ui.Text(c, "Side transcript "+id) }
	s.OpenSession(id)
	tt := ui.NewTester(s.View, 520, 600)
	if rendered != id || s.ActiveKind() != "session" || !tt.HasText("Side transcript "+id) {
		t.Fatal("native side transcript was not rendered")
	}
	if central != id {
		t.Fatal("side renderer changed central navigation")
	}
}
