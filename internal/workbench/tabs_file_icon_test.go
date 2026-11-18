package workbench

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTabStripFitsFileNames(t *testing.T) {
	for _, tc := range []struct {
		name, mode, fileName string
		width                int
		font, scale          float32
		middle               bool
	}{
		{"short-markdown", "light", "SKILL.md", 280, 20, 2, false},
		{"html-preview", "dark", "preview-report.html", 280, 20, 2, false},
		{"long-go-file", "dark", "session_persistence_test.go", 520, 14, 1, false},
		{"cjk-file", "light", "\u4f1a\u8bdd\u6062\u590d\u62a5\u544a.md", 280, 18, 1, false},
		{"over-limit", "dark", "desktop-session-persistence-recovery-investigation-notes.md", 320, 18, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			name := tc.fileName
			path := filepath.Join(t.TempDir(), name)
			snapshot.Settings.Appearance.Mode = tc.mode
			snapshot.Settings.Appearance.ThemeID = "rcode-" + tc.mode
			snapshot.Files = append(snapshot.Files, contracts.File{ID: "name-fit", Name: name, Path: path, Loaded: true})
			s.Update(snapshot)
			s.OnToggleExpand = func() {}
			if tc.name == "html-preview" {
				if err := s.OpenFilePreview(path, name); err != nil {
					t.Fatal(err)
				}
			} else {
				s.OpenFile("name-fit")
			}
			theme := s.palette().UI()
			theme.FontSize = tc.font
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme)
				ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
			}, tc.width, 80)
			tt.SetScale(tc.scale)
			chip, ok := tt.Find(name)
			if !ok {
				t.Fatalf("missing file tab %q", name)
			}
			close, ok := tt.Find("Close tab " + name)
			if !ok || close.X+close.W > chip.X+chip.W {
				t.Fatalf("close control is not inside its file tab: close=%+v chip=%+v", close, chip)
			}
			x, y := int((chip.X+25)*tc.scale), int(chip.Y*tc.scale)
			plate := tt.Image().RGBAAt(x-1, y+int(12*tc.scale))
			room := close.X - (chip.X + 25) - 3
			var textWidth float32
			ref := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme)
				c.Root().Background(ui.RGB(plate.R, plate.G, plate.B))
				size := design.TextSize(c, design.TextBase)
				textWidth, _ = c.MeasureText(0, ui.Span{Text: name, Size: size})
				ui.Row(c).Height(24).Children(func() {
					label := ui.Text(c, name).FontSize(size).TextColor(s.palette().Text).SingleLine()
					if tc.middle {
						label.Width(room).TruncateMiddle()
					}
				})
			}, tc.width, 24)
			ref.SetScale(tc.scale)
			if !tc.middle && room < textWidth {
				t.Fatalf("file name %q needs %.2f DIP, label only has %.2f DIP", name, textWidth, room)
			}
			if tc.middle && (room >= textWidth || chip.W != 240) {
				t.Fatalf("over-limit file should be elided in a 240 DIP tab: text=%.2f room=%.2f chip=%+v", textWidth, room, chip)
			}
			got, want := tt.Image(), ref.Image()
			for py := 0; py < want.Bounds().Dy(); py++ {
				for px := 0; px < int(min(room, textWidth+1)*tc.scale); px++ {
					if got.RGBAAt(x+px, y+py) != want.RGBAAt(px, py) {
						t.Fatalf("tab did not render name %q as expected at pixel (%d,%d)", name, px, py)
					}
				}
			}
			if tc.middle {
				tt.Move(chip.X+10, chip.Y+chip.H/2)
				tt.Advance(time.Second)
				if !tt.HasText(path) {
					t.Fatalf("elided tab lost its full path tooltip: %v", tt.Texts())
				}
			}
		})
	}
}

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
