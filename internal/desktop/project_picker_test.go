package desktop

import (
	"context"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestNewProjectDeviceFolderKeyboardAndSubmit(t *testing.T) {
	a := testApp(t, "default")
	before := len(a.state.Projects)
	a.sidebarUI.projectsClosed = true
	a.draft = "Keep project draft"
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
	if a.sidebarUI.projectsClosed || a.draft != "Keep project draft" {
		t.Fatal("creating a project left it hidden or changed the draft")
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
func TestProjectPickerListsHostLocations(t *testing.T) {
	a := testApp(t, "default")
	a.openNewProject()
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Key(0, ui.KeyEnter)
	if a.newProject.step != 1 || a.newProject.loading {
		t.Fatalf("device choice: %+v", a.newProject)
	}
	rows := a.projectPickerRows()
	if len(rows) == 0 || rows[0].key != "home" || rows[0].glyph != "home" {
		t.Fatalf("home row missing: %+v", rows)
	}
	// Every advertised location must be a row the picker can open, and the
	// backend must serve its listing.
	if len(rows) != len(a.newProject.locations.Locations) {
		t.Fatalf("row/location mismatch: %d rows, %d locations", len(rows), len(a.newProject.locations.Locations))
	}
	for _, row := range rows {
		if err := tt.Click("Project folder: " + row.title); err != nil {
			t.Fatalf("row %q: %v", row.key, err)
		}
		if a.newProject.step != 2 || a.newProject.error != "" || a.newProject.listing.Path == "" {
			t.Fatalf("opening %q: %+v", row.key, a.newProject)
		}
		a.projectPickerBack()
		if a.newProject.step != 1 {
			t.Fatalf("back from %q did not return to locations", row.key)
		}
	}
}
func TestProjectPickerLocationsComeFromTheService(t *testing.T) {
	a := testApp(t, "default")
	service := &locationRecordingService{Service: a.service, locations: contracts.DirectoryLocations{
		DeviceID: "local",
		Locations: []contracts.DirectoryLocation{
			{Name: "Home", Path: "~/projects", Kind: contracts.LocationHome},
			{Name: "E:", Path: "E:/", Kind: contracts.LocationVolume},
		},
	}}
	a.service = service
	a.openNewProject()
	tt := ui.NewTester(a.View, 900, 600)
	tt.Key(0, ui.KeyEnter)
	if service.requested != "local" {
		t.Fatalf("location query device: %q", service.requested)
	}
	if !tt.HasText("E:") {
		t.Fatal("offered drive is not drawn")
	}
	if err := tt.Click("Project folder: E:"); err != nil {
		t.Fatal(err)
	}
	if service.browsed != "E:/" {
		t.Fatalf("drive row did not browse its root: %q", service.browsed)
	}
}

// locationRecordingService answers the picker with fixed locations so the test
// proves the UI renders backend data instead of its own guess.
type locationRecordingService struct {
	contracts.Service
	locations contracts.DirectoryLocations
	requested string
	browsed   string
}

func (s *locationRecordingService) ListDirectoryLocations(_ context.Context, request contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	s.requested = request.DeviceID
	return s.locations, nil
}

func (s *locationRecordingService) ListDirectories(_ context.Context, request contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	s.browsed = request.Path
	return contracts.DirectoryListing{DeviceID: request.DeviceID, Path: request.Path, Parent: "/", Home: "~/projects"}, nil
}

func TestSidebarNewProjectAndOfflineProject(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 1100)
	if !tt.HasText("Device offline") {
		t.Fatal("offline project state is missing")
	}
	if err := tt.Click("New project"); err != nil {
		t.Fatal(err)
	}
	if !a.projectOpen {
		t.Fatal("project section action did not open the project picker")
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
		if err := tt.Click("New project"); err != nil {
			t.Fatal(err)
		}
		captureWorkbenchAudit(t, tt, "sidebar-add-project-"+name)
	}
	a := testApp(t, "default")
	a.openNewProject()
	tt := ui.NewTester(a.View, 900, 600)
	tt.Type("Build server")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	captureWorkbenchAudit(t, tt, "project-offline-error-900")
}
