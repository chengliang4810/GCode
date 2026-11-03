package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestProjectSelectSearchKeyboardAndFixedActions(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			a := testApp(t, "default")
			a.newConversation()
			a.state.Projects = nil
			for i := range 18 {
				a.state.Projects = append(a.state.Projects, contracts.Project{
					ID: fmt.Sprint(i), Name: fmt.Sprintf("Project %02d", i), DeviceID: "local",
				})
			}
			a.state.Projects[16].Name = "中文项目"
			a.projectID, a.draft = "0", "Keep this draft"
			tt := ui.NewTester(func(c *ui.Context) {
				a.palette = design.Palette("rcode-"+appearance, appearance == "dark")
				c.SetTheme(a.palette.UI())
				c.DisableAnimations()
				design.ApplyReferenceTypography(c)
				ui.Column(c).Fill().Padding(20).Justify(ui.End).Children(func() { a.projectPicker(c) })
			}, 380, 600)
			if err := tt.Click("Select project: Project 00"); err != nil {
				t.Fatal(err)
			}
			panel, ok := tt.Find("Select project")
			if !ok || panel.X < 0 || panel.Y < 0 || panel.X+panel.W > 380 || panel.Y+panel.H > 600 {
				t.Fatalf("project popup outside narrow window: %+v", panel)
			}
			picker, _ := tt.Find("Select project: Project 00")
			if panel.Y+panel.H > picker.Y {
				t.Fatal("project popup did not open above the selector")
			}
			footer, ok := tt.Find("New project…")
			if !ok {
				t.Fatal("new project action missing")
			}
			if _, ok := tt.Find("Don't work in a project"); !ok {
				t.Fatal("no-project action missing")
			}
			tt.Key(0, ui.KeyDown)
			for range 15 {
				tt.Key(0, ui.KeyDown)
			}
			if after, ok := tt.Find("New project…"); !ok || after != footer {
				t.Fatal("scrolling projects moved the fixed footer")
			}
			tt.Type("中文")
			if _, ok := tt.Find("中文项目"); !ok {
				t.Fatal("Chinese search did not reveal matching project")
			}
			if _, ok := tt.Find("Project 01"); ok {
				t.Fatal("nonmatching project remained after search")
			}
			tt.Key(0, ui.KeyEnter)
			if a.projectMenu || a.projectID != "16" || a.workspace.ProjectID != "16" || a.draft != "Keep this draft" {
				t.Fatal("keyboard selection did not preserve the draft and update the target")
			}
			if err := tt.Click("Select project: 中文项目"); err != nil {
				t.Fatal(err)
			}
			if a.projectSelect.query != "" {
				t.Fatal("reopened popup retained its search")
			}
			tt.Type("no-such-project")
			if _, ok := tt.Find("No results"); !ok {
				t.Fatal("empty search state missing")
			}
			tt.Key(0, ui.KeyEnter)
			if a.projectID != "16" || !a.projectMenu {
				t.Fatal("empty search selected another project")
			}
			if err := tt.Click("Don't work in a project"); err != nil {
				t.Fatal(err)
			}
			if a.projectID != "" || a.workspace.ProjectID != "" || a.draft != "Keep this draft" {
				t.Fatal("no-project action retained target or lost the draft")
			}
			if err := tt.Click("Select project: Don't work in a project"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("New project…"); err != nil {
				t.Fatal(err)
			}
			if !a.projectOpen || a.projectMenu {
				t.Fatal("new-project action did not replace project selection with creation")
			}
		})
	}
}

func TestProjectSelectPlacementAndHoverStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		x, y  float32
		below bool
	}{
		{name: "prefer above with room on both sides", x: 100, y: 440},
		{name: "shrink at right edge", x: 680, y: 440},
		{name: "flip only when near top", x: 100, y: 30, below: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.newConversation()
			a.projectID = a.state.Projects[0].ID
			tt := ui.NewTester(func(c *ui.Context) {
				c.DisableAnimations()
				ui.Column(c).Fill().Padding(tc.y, 0, 0, tc.x).Children(func() {
					ui.Row(c).Children(func() { a.projectPicker(c) })
				})
			}, 900, 900)
			label := "Select project: " + a.projectLabel()
			picker, _ := tt.Find(label)
			tt.Move(picker.X+picker.W/2, picker.Y+picker.H/2)
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			panel, ok := tt.Find("Select project")
			if !ok || panel.X != tc.x || panel.X+panel.W > 892 {
				t.Fatalf("popup left alignment / right edge: %+v, left=%v", panel, tc.x)
			}
			if tc.below {
				if panel.Y < picker.Y+picker.H {
					t.Fatal("popup did not flip below the top-edge selector")
				}
			} else if panel.Y+panel.H > picker.Y {
				t.Fatal("popup preferred below despite room above")
			}
			// The menu action spans the card. A leaked hover shortcut would
			// be found first and occupy only the 12px icon slot.
			if action, ok := tt.Find("Don't work in a project"); !ok || action.W < 30 {
				t.Fatal("open popup retained the hover clear shortcut")
			}
			tt.Key(0, ui.KeyEscape)
			if a.projectMenu {
				t.Fatal("Escape did not close the project popup")
			}
			if action, ok := tt.Find("Don't work in a project"); !ok || action.W > 30 {
				t.Fatal("closing popup did not restore the hover clear shortcut")
			}
		})
	}
}
