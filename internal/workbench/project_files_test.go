package workbench

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestProjectFileLoadingStatusKeepsExistingFilesVisible(t *testing.T) {
	for _, status := range []contracts.LoadStatus{contracts.LoadLoading, contracts.LoadError} {
		t.Run(string(status), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			s.ProjectID = snapshot.Projects[0].ID
			snapshot.Projects[0].FilesState = status
			snapshot.Projects[0].FilesError = "fixture enumeration failed"
			s.Update(snapshot)
			tt := ui.NewTester(s.FilesView, 500, 600)
			label := "Loading project files…"
			if status == contracts.LoadError {
				label = "File loading incomplete: fixture enumeration failed"
			}
			// The status must coexist with the tree; the project is already open.
			if _, ok := tt.Find(label); !ok {
				t.Fatalf("missing project file status: %s", label)
			}
			if _, ok := tt.Find("Search files"); !ok {
				t.Fatal("project file status replaced the file explorer")
			}
		})
	}
}
