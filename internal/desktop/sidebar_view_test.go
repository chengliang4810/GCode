package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarProjectNewConversationHoverAndTarget(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []float32{264, 350} {
			t.Run(fmt.Sprintf("dark-%v-width-%g", dark, width), func(t *testing.T) {
				a := sidebarLayoutApp(t)
				a.sidebarWidth = width
				a.state.Settings.Appearance.Mode = "light"
				if dark {
					a.state.Settings.Appearance.Mode = "dark"
					a.state.Settings.Appearance.FontSize = 20
				}
				target := &a.state.Projects[1]
				target.DeviceID, target.Branch = "macbook", "project-branch"
				stabilizeFixtureSnapshot(t, a)
				// Opening the old conversation bookmarks another canvas target.
				a.projectID = "2"
				a.selectSession("layout-0")
				a.draft = "Keep the existing conversation draft"
				a.attachments = []contracts.Attachment{{Name: "existing.txt"}}
				a.expanded["project:1"] = true
				count := len(a.state.Sessions)
				tt := ui.NewTester(a.View, 1320, 1000)
				label := "在 bzaqsc-new 中新建对话"
				if _, ok := tt.Find(label); ok {
					t.Fatal("project action is visible without hover or focus")
				}
				project, ok := tt.Find("bzaqsc-new")
				if !ok {
					t.Fatal("target project missing")
				}
				tt.Move(project.X+40, project.Y+project.H/2)
				tt.Frame()
				action, ok := tt.Find(label)
				if !ok || action.X+action.W > project.X+project.W || action.X < project.X+project.W/2 {
					t.Fatalf("hover action missing or not at the row's end: row=%+v action=%+v", project, action)
				}
				tt.Move(action.X+action.W/2, action.Y+action.H/2)
				tt.Frame()
				if _, ok := tt.Find(label); !ok {
					t.Fatal("action vanished while moving onto it")
				}
				tt.Move(800, 120)
				tt.Frame()
				if _, ok := tt.Find(label); ok {
					t.Fatal("action remained visible after leaving the row")
				}
				tt.Move(project.X+40, project.Y+project.H/2)
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if a.sessionID != "" || a.route != "chat" || a.projectID != "1" || a.deviceID != "macbook" || a.branch != "project-branch" {
					t.Fatalf("new canvas used the old destination: session=%q project=%q device=%q branch=%q",
						a.sessionID, a.projectID, a.deviceID, a.branch)
				}
				if a.draft != "" || len(a.attachments) != 0 || a.workspace.ProjectID != "1" || a.bottom.ProjectID != "1" {
					t.Fatal("new canvas did not clear or retarget its workspace")
				}
				if !a.expanded["project:1"] || len(a.state.Sessions) != count {
					t.Fatal("new button toggled the project or prematurely created a session")
				}
				old := a.sessionStates["layout-0"]
				if old.Draft != "Keep the existing conversation draft" || len(old.Attachments) != 1 {
					t.Fatal("new project conversation discarded the old conversation's draft")
				}
			})
		}
	}
}
