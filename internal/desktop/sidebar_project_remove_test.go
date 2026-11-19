package desktop

import (
	"gcode/internal/contracts"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarRemoveProjectKeepsChatsAndDraft(t *testing.T) {
	for _, tc := range []struct {
		theme         string
		width, height int
	}{{"light", 1320, 880}, {"dark", 900, 720}} {
		t.Run(tc.theme, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.Appearance.ThemeID = "rcode-" + tc.theme
			project := a.state.Projects[0]
			a.newProjectConversation(project.ID)
			a.draft = "Keep this draft"
			sessions := len(a.state.Sessions)
			tt := ui.NewTester(a.View, tc.width, tc.height)
			open := func() {
				t.Helper()
				row, ok := tt.Find(project.Name)
				if !ok {
					t.Fatal("project row missing")
				}
				tt.Move(row.X+row.W/2, row.Y+row.H/2)
				if err := tt.Click("Project actions"); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem("Remove project"); err != nil {
					t.Fatal(err)
				}
			}
			open()
			if err := tt.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
			open()
			if err := tt.Click("Remove"); err != nil {
				t.Fatal(err)
			}
			if slices.ContainsFunc(a.state.Projects, func(p contracts.Project) bool { return p.ID == project.ID }) {
				t.Fatal("project remains in the list")
			}
			if len(a.state.Sessions) != sessions || a.projectID != "" || a.draft != "Keep this draft" {
				t.Fatal("removal did not preserve chats and the draft while clearing the project target")
			}
		})
	}
}

func TestSidebarProjectRename(t *testing.T) {
	a := testApp(t, "default")
	a.state.Runtime = nil
	a.state.Projects[0].DeviceID = "local"
	a.state.Projects[0].Path = t.TempDir()
	project := a.state.Projects[0]
	tt := ui.NewTester(a.View, 900, 720)
	row, ok := tt.Find(project.Name)
	if !ok {
		t.Fatal("project row missing")
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	if err := tt.Click("Project actions"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Open project folder") {
		t.Fatal("folder action missing")
	}
	if err := tt.ChooseMenuItem("Rename project"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarUI.renameProjectName != project.Name {
		t.Fatal("rename dialog did not preload the current name")
	}
	a.sidebarUI.renameProjectName = "  Renamed project  "
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(a.state.Projects, func(p contracts.Project) bool {
		return p.ID == project.ID && p.Name == "Renamed project"
	}) {
		t.Fatal("rename did not preserve the project ID and path")
	}
}

func TestSidebarRemoteProjectHidesFolderAction(t *testing.T) {
	a := testApp(t, "default")
	a.state.Runtime = &contracts.RuntimeInfo{Remote: true}
	a.state.Projects[0].DeviceID = "local"
	a.state.Projects[0].Path = t.TempDir()
	project := a.state.Projects[0]
	tt := ui.NewTester(a.View, 1320, 880)
	row, ok := tt.Find(project.Name)
	if !ok {
		t.Fatal("project row missing")
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	if err := tt.Click("Project actions"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	if slices.Contains(menu, "Open project folder") || !slices.Contains(menu, "Rename project") || !slices.Contains(menu, "Remove project") {
		t.Fatalf("unexpected remote project menu: %v", menu)
	}
}
