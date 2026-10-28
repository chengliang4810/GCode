package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNewProjectDeviceFolderKeyboardAndSubmit(t *testing.T) {
	a := testApp(t, "default")
	before := len(a.state.Projects)
	a.openNewProject()
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Key(0, ui.KeyEnter)
	if a.newProject.step != 1 || a.newProject.deviceID != "local" {
		t.Fatal("device choice failed")
	}
	tt.Key(0, ui.KeyEnter)
	if a.newProject.step != 2 || a.newProject.listing.Path == "" {
		t.Fatalf("Home choice: %+v", a.newProject)
	}
	tt.Type("fieldnotes/")
	if !strings.HasSuffix(a.newProject.listing.Path, "/fieldnotes") {
		t.Fatal("slash did not descend into named folder")
	}
	tt.Type("internal")
	tt.Key(0, ui.KeyEnter)
	if !strings.HasSuffix(a.newProject.listing.Path, "/internal") {
		t.Fatal("folder keyboard activation failed")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	if a.projectOpen || len(a.state.Projects) != before+1 {
		t.Fatalf("project submit: open %t, error %s, count %d", a.projectOpen, a.newProject.error, len(a.state.Projects))
	}
	project := a.state.Projects[len(a.state.Projects)-1]
	if project.DeviceID != "local" || project.Name != "internal" {
		t.Fatalf("create discarded selection: %+v", project)
	}
}
func TestNewProjectOfflineAndTypedPathRecoverWithoutInventingFolders(t *testing.T) {
	a := testApp(t, "default")
	a.openNewProject()
	tt := ui.NewTester(a.View, 900, 600)
	tt.Type("Build server")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	if a.newProject.error == "" || a.newProject.loading {
		t.Fatal("offline device did not produce recoverable error")
	}
	a.openNewProject()
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	if err := tt.Click("Edit path"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("/missing-virtual-folder")
	tt.Key(0, ui.KeyEnter)
	if a.newProject.error == "" || !a.projectOpen {
		t.Fatal("invalid typed path accepted")
	}
	a.projectBrowse("~/projects/fieldnotes")
	tt.Frame()
	if a.newProject.error != "" {
		t.Fatal(a.newProject.error)
	}
	if err := tt.Click("Add project"); err != nil {
		t.Fatal(err)
	}
	if !a.projectOpen || !strings.Contains(a.newProject.error, "already added") {
		t.Fatal("duplicate path did not stay in picker with error")
	}
}
func TestProjectFilterSourceRowsOfflineGlyphAndPinnedFooter(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("All projects"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Device offline"); !ok {
		t.Fatal("offline glyph missing")
	}
	tt.Type("no matching project")
	if tt.HasText("All projects") { // Sidebar trigger remains; check selectable list instead.
		if len(a.sidebarProjects()) != 0 {
			t.Fatal("unexpected project matches")
		}
	}
	tt.Key(0, ui.KeyEnter)
	if !a.projectOpen || a.sidebarProjectMenu {
		t.Fatal("pinned footer not keyboard selectable when list empty")
	}
}
func TestProjectPickerEvidence(t *testing.T) {
	a := testApp(t, "default")
	a.openNewProject()
	tt := ui.NewTester(a.View, 1320, 880)
	captureWorkbenchAudit(t, tt, "project-devices")
	tt.Key(0, ui.KeyEnter)
	captureWorkbenchAudit(t, tt, "project-locations")
	tt.Key(0, ui.KeyEnter)
	captureWorkbenchAudit(t, tt, "project-folders")
	tt.Type("fieldnotes/")
	captureWorkbenchAudit(t, tt, "project-nested-folders")
}

func TestExplorerAndOfflinePickerEvidence(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := testApp(t, "default")
		a.selectSession("chat-0")
		a.files = true
		a.right = false
		a.state.Settings.Appearance.Mode = "light"
		if dark {
			a.state.Settings.Appearance.Mode = "dark"
		}
		a.refreshBackground()
		tt := ui.NewTester(a.View, 1320, 880)
		if err := tt.Click("Search files"); err != nil {
			t.Fatal(err)
		}
		tt.Type("layout")
		name := "light"
		if dark {
			name = "dark"
		}
		captureWorkbenchAudit(t, tt, "files-search-"+name)
		tt.Key(0, ui.KeyEscape)
		if err := tt.Click("All projects"); err != nil {
			t.Fatal(err)
		}
		captureWorkbenchAudit(t, tt, "project-filter-"+name)
	}
	a := testApp(t, "default")
	a.openNewProject()
	tt := ui.NewTester(a.View, 900, 600)
	tt.Type("Build server")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	captureWorkbenchAudit(t, tt, "project-offline-error-900")
}
