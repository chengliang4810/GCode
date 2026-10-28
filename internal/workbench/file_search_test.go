package workbench

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestExplorerSearchUsesServiceHierarchyKeyboardAndScope(t *testing.T) {
	s, _, _ := testWorkspace(t)
	s.ProjectID = "fieldnotes"
	s.SwitchSession("first")
	tt := ui.NewTester(s.FilesView, 286, 600)
	if err := tt.Click("Search files"); err != nil {
		t.Fatal(err)
	}
	tt.Type("layout")
	if len(s.search.rows) != 2 || s.search.rows[0].ID != "dir-internal" || s.search.rows[1].ID != "file-layout" {
		t.Fatalf("ancestor context missing: %+v", s.search.rows)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if s.activeFile() != "file-layout" {
		t.Fatal("Enter did not open selected file")
	}
	s.SwitchSession("second")
	if s.fileSearch != "" || len(s.search.rows) > 0 {
		t.Fatal("search leaked into another conversation")
	}
	s.SwitchSession("first")
	tt.Frame()
	if s.fileSearch != "layout" || len(s.search.rows) != 2 {
		t.Fatal("conversation search state was discarded")
	}
	if err := tt.Click("Search files"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if s.fileSearch != "" || s.search.cancel != nil {
		t.Fatal("Escape retained query task")
	}
}
func TestOpenSearchLocationUsesUTF8ByteColumnAndRecentOrder(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	s.ProjectID = "fieldnotes"
	s.SwitchSession("one")
	for i := range snapshot.Files {
		if snapshot.Files[i].ID == "file-layout" {
			snapshot.Files[i].Content = "package layout\n// 中文 example\n"
		}
	}
	s.Update(snapshot)
	s.OpenFileAt("file-layout", 2, 10)
	if got := s.documents["file-layout"].editor.CursorPos(); got != (editor.Pos{Line: 1, Col: 10}) {
		t.Fatalf("search caret: %+v", got)
	}
	s.OpenFile("file-readme")
	s.OpenFile("file-layout")
	recent := s.RecentFiles()
	if len(recent) != 2 || recent[0].FileID != "file-layout" {
		t.Fatalf("recent access order: %+v", recent)
	}
	s.SwitchSession("two")
	if len(s.RecentFiles()) != 0 {
		t.Fatal("recent paths leaked across conversations")
	}
}
func TestRevealDirectoryClearsSearchAndExpandsAncestors(t *testing.T) {
	s, _, _ := testWorkspace(t)
	s.fileSearch = "layout"
	closed := false
	s.folders["dir-cmd"] = &closed
	s.RevealFile("dir-app")
	if s.fileSearch != "" || !(*s.folders["dir-cmd"]) || !(*s.folders["dir-app"]) {
		t.Fatal("directory result was not revealed")
	}
}
func TestClosingSideSessionCallsOnlyItsLifecycleHook(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	s.RenderSession = func(*ui.Context, string) {}
	called := ""
	s.OnCloseSession = func(id string) { called = id }
	s.OpenSession("agent")
	if called != "" {
		t.Fatal("opening a session called the close hook")
	}
	s.CloseActive()
	if called != "agent" {
		t.Fatal("session lifecycle hook missing")
	}
	s.OpenFile(snapshot.Files[0].ID)
	called = ""
	s.CloseActive()
	if called != "" {
		t.Fatal("file close called session hook")
	}
}
func TestExplorerSearchNeverUsesContentAsFilename(t *testing.T) {
	s, _, _ := testWorkspace(t)
	s.ProjectID = "fieldnotes"
	s.fileSearch = "SidebarWidth"
	s.refreshFileSearch()
	if len(s.search.rows) != 0 {
		t.Fatal("file explorer rendered content matches as filename results")
	}
	s.search.result = contracts.WorkspaceSearchResult{}
}
