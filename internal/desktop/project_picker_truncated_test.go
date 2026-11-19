package desktop

import (
	"context"
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type truncatedDirectoryService struct {
	contracts.Service
	listing   contracts.DirectoryListing
	locations contracts.DirectoryLocations
}

func (s *truncatedDirectoryService) ListDirectories(context.Context, contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	return s.listing, nil
}

func (s *truncatedDirectoryService) ListDirectoryLocations(context.Context, contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	return s.locations, nil
}

func TestProjectPickerTruncatedStatus(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{560, 1320} {
			t.Run(fmt.Sprintf("%s/%d", theme, width), func(t *testing.T) {
				a := testApp(t, "default")
				a.state.Settings.Appearance.Mode = theme
				a.refreshBackground()
				service := &truncatedDirectoryService{
					Service: a.service,
					locations: contracts.DirectoryLocations{DeviceID: "local", Truncated: true, Locations: []contracts.DirectoryLocation{
						{Name: "Home", Path: "/host", Kind: contracts.LocationHome},
					}},
					listing: contracts.DirectoryListing{DeviceID: "local", Path: "/host", Home: "/host", Parent: "/", Truncated: true, Entries: []contracts.DirectoryEntry{
						{Name: "work", Path: "/host/work"},
					}},
				}
				a.service = service
				a.openNewProject()
				a.chooseProjectDevice("local")
				tt := ui.NewTester(a.View, width, 880)
				if !tt.HasText("Partial list") {
					t.Fatal("truncated locations were silently presented as complete")
				}
				tt.Key(0, ui.KeyEnter)
				if a.newProject.step != 2 || !tt.HasText("Partial list") {
					t.Fatal("truncated directory status missing or Home navigation blocked")
				}
				if _, ok := tt.Find("Project folder: work"); !ok {
					t.Fatal("truncation hid available directory rows")
				}
				service.listing.Entries = nil
				a.projectBrowse("/host")
				tt.Frame()
				if !tt.HasText("Folder list is incomplete") || !tt.HasText("Enter a path to browse directly.") || tt.HasText("No folders here") {
					t.Fatal("incomplete empty listing was confused with a known-empty directory")
				}
				if err := tt.Click("Edit path"); err != nil {
					t.Fatal(err)
				}
				if !a.newProject.pathEdit {
					t.Fatal("truncation disabled direct path input")
				}
				service.listing.Truncated = false
				a.projectBrowse("/host")
				tt.Frame()
				if tt.HasText("Partial list") || !tt.HasText("No folders here") {
					t.Fatal("a complete subsequent result retained stale truncation status")
				}
			})
		}
	}
}
