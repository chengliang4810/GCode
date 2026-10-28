package workbench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestSystemThemeUsesHostAppearanceForAllWorkspaceSurfaces(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode = "system"
			snapshot.Settings.Appearance.ThemeID = ""
			snapshot.Settings.Appearance.LightThemeID = "zeron-light"
			snapshot.Settings.Appearance.DarkThemeID = "zeron-dark"
			s.Update(snapshot)
			host := design.Palette("zeron", dark)
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(host.UI()); s.View(c) }, 520, 600)
			if s.palette().Dark != dark || s.palette().Background != host.Background {
				t.Fatal("workspace ignored the host's resolved system appearance")
			}
			s.OpenSurface("changes")
			tt.Frame()
			if s.palette().Dark != dark {
				t.Fatal("tab navigation changed resolved appearance")
			}
			tt = ui.NewTester(func(c *ui.Context) { c.SetTheme(host.UI()); s.FilesView(c) }, 286, 600)
			if s.palette().Dark != dark {
				t.Fatal("explorer ignored host appearance")
			}
			if _, ok := tt.Find("Search files"); !ok {
				t.Fatal("file search absent")
			}
		})
	}
}

func TestExplorerPreservesHostRootMaterialAndTheme(t *testing.T) {
	s, _, _ := testWorkspace(t)
	host := design.Palette("zeron-light", false).UI()
	rootColor := ui.RGB(23, 45, 67)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(host)
		c.Root().Background(rootColor)
		ui.Box(c).Width(100).Height(100).Children(func() { s.FilesView(c) })
		if c.Theme() != host {
			t.Fatal("explorer replaced host theme")
		}
	}, 300, 250)
	pixel := tt.Image().RGBAAt(250, 200)
	if pixel.R != rootColor.R || pixel.G != rootColor.G || pixel.B != rootColor.B {
		t.Fatalf("explorer overwrote host material: %+v", pixel)
	}
}

func TestOverflowTabsKeepFixedGeometryAndKeyboardReveal(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for i := 0; i < 14; i++ {
		s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Long workspace tab %02d", i))
	}
	tt := ui.NewTester(s.View, 360, 600)
	if s.tabScroll.MaxX <= 0 {
		t.Fatal("tabs were squeezed instead of scrolling")
	}
	last, ok := tt.Find(s.tabs[s.active].title)
	if !ok || last.W != 112 || last.H != 24 || last.X < 0 || last.X+last.W > 340 {
		t.Fatalf("active tab was not revealed: %+v", last)
	}
	plus, _ := tt.Find("New surface")
	if plus.X+plus.W > 360 || plus.W != 24 {
		t.Fatalf("add button overflowed %+v", plus)
	}
	s.selectTab(0)
	tt.Frame()
	if err := tt.Click(s.tabs[0].title); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	if s.active != len(s.tabs)-1 {
		t.Fatalf("End did not select last tab: %d", s.active)
	}
	last, _ = tt.Find(s.tabs[s.active].title)
	if last.X < 0 || last.X+last.W > 340 {
		t.Fatalf("keyboard selection did not scroll into view: %+v", last)
	}
}

func TestReorderAndClosingInactiveTabsPreserveActiveIdentity(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for _, title := range []string{"First", "Second", "Third"} {
		s.choose("changes", title, title)
	}
	active := s.tabs[s.active]
	s.moveTab(tabDrag{"changes", "Third"}, tabDrag{"changes", "First"})
	if s.tabs[s.active] != active || s.active != 0 {
		t.Fatal("moving tabs changed the active surface")
	}
	tt := ui.NewTester(s.View, 520, 600)
	if err := tt.Click("Close tab Second"); err != nil {
		t.Fatal(err)
	}
	if s.tabs[s.active] != active || len(s.tabs) != 2 {
		t.Fatal("closing inactive tab changed the active surface")
	}
}

func TestTabsDragAndWheelKeepSelection(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for _, title := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		s.choose("changes", title, title)
	}
	s.selectTab(0)
	tt := ui.NewTester(s.View, 520, 600)
	first, _ := tt.Find("First")
	second, _ := tt.Find("Second")
	x, y := first.X+30, first.Y+12
	tt.Press(x, y)
	tt.Move(x+12, y)
	tt.Move(second.X+50, y)
	tt.Release(second.X+50, y)
	if s.tabs[0].title != "Second" || s.tabs[1].title != "First" {
		t.Fatalf("native tab drag did not reorder: %+v", s.tabs)
	}
	active := s.tabs[s.active]
	tt.Scroll(180, 20, 180, 0)
	tt.Advance(time.Second)
	if s.tabScroll.X <= 0 || s.tabs[s.active] != active {
		t.Fatal("overflow wheel did not scroll independently of active selection")
	}
}

func TestFileRenameUpdatesTabAndRetainsUnsavedBuffer(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	var file contracts.File
	for _, candidate := range snapshot.Files {
		if candidate.Kind == "file" && !candidate.ReadOnly {
			file = candidate
			break
		}
	}
	s.OpenFile(file.ID)
	s.documents[file.ID].editor.Buffer.Set("unsaved edit")
	if err := backend.Execute(context.Background(), contracts.Command{Kind: "file.rename", ID: file.ID, Name: "renamed.go"}); err != nil {
		t.Fatal(err)
	}
	next, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.Update(next)
	if s.tabs[s.active].title != "renamed.go" || s.documents[file.ID].file.Name != "renamed.go" || s.documents[file.ID].editor.Buffer.String() != "unsaved edit" {
		t.Fatal("rename lost the document identity or its unsaved edits")
	}
}

func TestExplorerFiltersHiddenAncestorsAndSortsDirectoriesFirst(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	snapshot.Files = []contracts.File{{ID: "z", Name: "z.go", Path: "z.go", Kind: "file"}, {ID: "folder", Name: "src", Path: "src", Kind: "directory"}, {ID: "a", Name: "a.go", Path: "a.go", Kind: "file"}, {ID: "hidden", Name: "config", Path: ".private/config", Kind: "file"}}
	snapshot.Settings.Files.ShowHidden = false
	s.Update(snapshot)
	got := s.filesIn("")
	if len(got) != 3 || got[0].ID != "folder" || got[1].ID != "a" {
		t.Fatalf("file tree order/filter: %+v", got)
	}
	s.fileSearch = "config"
	tt := ui.NewTester(s.FilesView, 286, 600)
	if tt.HasText(".private/config") {
		t.Fatal("search leaked a hidden ancestor")
	}
	if err := tt.Click("Search files"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if s.fileSearch != "" {
		t.Fatal("Escape did not close search")
	}
}

func TestExplorerDirectoryToggleAndRowGeometry(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	var dir, child contracts.File
	for _, candidate := range snapshot.Files {
		if candidate.Kind != "directory" || candidate.ParentID != "" {
			continue
		}
		for _, next := range snapshot.Files {
			if next.ParentID == candidate.ID {
				dir, child = candidate, next
				break
			}
		}
		if dir.ID != "" {
			break
		}
	}
	if dir.ID == "" {
		t.Fatal("fixture has no directory with children")
	}
	tt := ui.NewTester(s.FilesView, 286, 600)
	row, ok := tt.Find(dir.Name)
	if !ok || row.H != 27 {
		t.Fatalf("directory row differs from source geometry: %+v", row)
	}
	if _, ok := tt.Find(child.Name); !ok {
		t.Fatal("expanded directory omitted children")
	}
	if err := tt.Click(dir.Name); err != nil {
		t.Fatal(err)
	}
	if *s.folders[dir.ID] {
		t.Fatal("clicking directory name did not collapse it")
	}
	if _, ok := tt.Find(child.Name); ok {
		t.Fatal("collapsed directory retained child widgets")
	}
	tt.Key(0, ui.KeyRight)
	if !*s.folders[dir.ID] {
		t.Fatal("native Tree Right key did not expand directory")
	}
}

func TestNewFileEditorReceivesFocusAndCommittedIMEInput(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	var file contracts.File
	for _, candidate := range snapshot.Files {
		if candidate.Kind == "file" && !candidate.ReadOnly {
			file = candidate
			break
		}
	}
	s.OpenFile(file.ID)
	tt := ui.NewTester(s.View, 720, 560)
	document := s.documents[file.ID]
	before := document.editor.Buffer.String()
	tt.Compose("你好", 2)
	// As the official editor composition test does, commit through TextInput.
	// Tester.Key sends an unfiltered raw key; native IMM32/NSTextInput filters
	// candidate-selection Enter before this custom editor receives the key.
	tt.Type("您好")
	if document.editor.Buffer.String() == before || !document.editor.Buffer.Modified() {
		t.Fatal("newly opened file did not receive committed input")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if document.editor.Buffer.String() != before {
		t.Fatal("committed IME edit did not undo as one operation")
	}
}

func TestWorkspaceThemeCatalogAndConfiguredEditorFont(t *testing.T) {
	for _, theme := range design.Themes() {
		t.Run(theme.ID, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode = theme.Appearance
			snapshot.Settings.Appearance.ThemeID = theme.ID
			snapshot.Settings.Appearance.CodeFont = "Geist Mono"
			snapshot.Settings.Appearance.CodeFontSize = 20
			s.Update(snapshot)
			s.OpenChange(snapshot.Changes[0].ID)
			tt := ui.NewTester(s.View, 520, 600)
			p := design.Palette(theme.ID, theme.Appearance == "dark")
			if s.palette().Background != p.Background || s.diffs[snapshot.Changes[0].ID].Theme.Background != p.Background {
				t.Fatal("workspace/diff palette differs from selected theme")
			}
			if s.diffs[snapshot.Changes[0].ID].Font.Size != 20*13/12.5 {
				t.Fatal("diff ignored code font size")
			}
			if _, ok := tt.Find("Workspace tabs"); !ok {
				t.Fatal("theme removed tabs")
			}
		})
	}
}
