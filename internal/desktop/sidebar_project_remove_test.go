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
