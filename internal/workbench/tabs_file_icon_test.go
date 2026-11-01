package workbench

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestTabStripFileImageOpacityFollowsSelection(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("dark%t-scale%g", dark, scale), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				var files []contracts.File
				names := map[string]bool{}
				for _, file := range snapshot.Files {
					if !file.ReadOnly && file.Kind != "directory" && file.Kind != "image" && !names[file.Name] {
						files = append(files, file)
						names[file.Name] = true
						if len(files) == 2 {
							break
						}
					}
				}
				if len(files) != 2 {
					t.Fatal("two distinct real editable file resources are required")
				}
				for _, file := range files {
					s.OpenFile(file.ID)
					if s.documents[file.ID] == nil {
						t.Fatal("real file resource did not open")
					}
				}
				snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", "rcode-light"
				if dark {
					snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "dark", "rcode-dark"
				}
				s.Update(snapshot)
				s.selectTab(0)
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(s.palette().UI())
					c.Root().Background(ui.RGB(70, 80, 90))
					ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
				}, 360, 80)
				tt.SetScale(scale)
				geometry := map[string]ui.Rect{}
				for _, file := range files {
					geometry[file.ID], _ = tt.Find(file.Name)
				}
				for step, selected := range []int{0, 1, 0} {
					if step > 0 {
						if err := tt.Click(files[selected].Name); err != nil {
							t.Fatal(err)
						}
						tt.Move(350, 70)
					}
					if s.tabs[s.active].id != files[selected].ID {
						t.Fatal("file image intercepted chip selection")
					}
					for i, file := range files {
						chip, ok := tt.Find(file.Name)
						if !ok || chip != geometry[file.ID] || chip.W != 112 || chip.H != 24 {
							t.Fatalf("file opacity changed chip geometry: %+v", chip)
						}
						plate := tt.Image().RGBAAt(int((chip.X+3)*scale), int((chip.Y+chip.H/2)*scale))
						opacity := float32(.78)
						if i == selected {
							opacity = 1
						}
						reference := ui.NewTester(func(c *ui.Context) {
							c.SetTheme(s.palette().UI())
							c.Root().Background(ui.RGB(plate.R, plate.G, plate.B))
							ui.Image(c, sourceFileIcon(file, false, dark)).Size(14, 14).Opacity(opacity)
						}, 14, 14)
						reference.SetScale(scale)
						actual, expected := tt.Image(), reference.Image()
						for y := 0; y < int(14*scale); y++ {
							for x := 0; x < int(14*scale); x++ {
								got := actual.RGBAAt(int((chip.X+6)*scale)+x, int((chip.Y+5)*scale)+y)
								if want := expected.RGBAAt(x, y); got != want {
									t.Fatalf("selection pass%d file%s opacity%g pixel%d,%d got%v want%v", step, file.Name, opacity, x, y, got, want)
								}
							}
						}
						if s.documents[file.ID].editor.Buffer.String() != file.Content || s.documents[file.ID].editor.Buffer.Modified() {
							t.Fatal("selection changed the real file buffer")
						}
					}
				}
				if err := tt.Click("Close tab " + files[1].Name); err != nil {
					t.Fatal(err)
				}
				if len(s.tabs) != 1 || s.tabs[0].id != files[0].ID || s.documents[files[1].ID] != nil {
					t.Fatal("image opacity intercepted the file close action")
				}
			})
		}
	}
}
