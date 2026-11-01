package desktop

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestFitPanels(t *testing.T) {
	all := [3]bool{true, true, true}
	order := [3]uint64{1, 2, 3}
	if f := fitPanels(1320, 256, all, order, false); !f.Sidebar || !f.Right || !f.Files {
		t.Fatalf("wide %+v", f)
	}
	if f := fitPanels(900, 256, all, order, false); f.Sidebar || !f.Right || !f.Files {
		t.Fatalf("oldest yields %+v", f)
	}
	if f := fitPanels(500, 256, all, order, true); f.Sidebar || !f.Right || f.Files {
		t.Fatalf("takeover %+v", f)
	}
	if f := fitPanels(900, 256, all, [3]uint64{3, 2, 1}, false); !f.Sidebar || f.Right || f.Files {
		t.Fatalf("recency %+v", f)
	}
}

func TestFilesWorkspaceAndDockCanRenderTogether(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.workspace.OpenSurface("files")
	a.files, a.right = true, true
	a.panelOpened = [3]uint64{1, 2, 3}
	tt := ui.NewTester(a.View, 1600, 1000)
	for range 3 {
		tt.Frame()
	}
	if !a.fit.Right || !a.fit.Files {
		t.Fatal("regression must render both the workspace and Files dock")
	}
	searches := 0
	for _, label := range tt.Texts() {
		if label == "Search files" {
			searches++
		}
	}
	if searches != 2 {
		t.Fatalf("expected two usable file explorers, found %d", searches)
	}
}

func TestPanelWidthsPreserveConversationAndPreferences(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		viewport, sidebar, files, right float32
		expanded                        bool
		want                            panelWidths
	}{
		{"default", 1320, 256, 286, 520, false, panelWidths{256, 300, 478, 286}},
		{"large", 1600, 256, 286, 520, false, panelWidths{256, 538, 520, 286}},
		{"narrow", 900, 256, 440, 520, false, panelWidths{0, 300, 360, 240}},
		{"maximum preferences", 1320, 400, 440, 800, false, panelWidths{400, 300, 360, 260}},
		{"takeover", 1320, 256, 286, 520, true, panelWidths{256, 0, 778, 286}},
		{"narrow takeover", 500, 256, 286, 520, true, panelWidths{0, 0, 500, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fit := fitPanels(tc.viewport, tc.sidebar, [3]bool{true, true, true}, [3]uint64{1, 2, 3}, tc.expanded)
			if got := allocatePanels(tc.viewport, tc.sidebar, tc.files, tc.right, fit, tc.expanded); got != tc.want {
				t.Fatalf("widths %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWorkspaceRenderedColumnOrderAndResizeRestoration(t *testing.T) {
	for _, size := range []struct{ width, height int }{{1320, 880}, {900, 600}, {1600, 1000}} {
		for _, mode := range []string{"light", "dark"} {
			t.Run(fmt.Sprintf("%dx%d-%s", size.width, size.height, mode), func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession(a.state.Sessions[0].ID)
				a.state.Settings.Appearance.Mode = mode
				a.state.Settings.Appearance.ThemeID = "rcode-" + mode
				a.workspace.Update(a.state)
				a.workspace.OpenSurface("changes")
				a.files, a.right = true, true
				a.panelOpened = [3]uint64{1, 2, 3}
				a.filesWidth, a.rightWidth = 440, 520
				tt := ui.NewTester(a.View, size.width, size.height)
				chat, chatOK := tt.Find("Conversation pane")
				tools, toolsOK := tt.Find("Tools workspace")
				files, filesOK := tt.Find("Files explorer")
				if !chatOK || !toolsOK || !filesOK {
					t.Fatalf("missing columns: chat %v tools %v files %v", chatOK, toolsOK, filesOK)
				}
				if chat.W < design.ChatMin-1 || tools.W < design.RightMin-1 || files.W < design.FilesMin-1 || chat.X+chat.W > tools.X || tools.X+tools.W > files.X || files.X+files.W != float32(size.width) {
					t.Fatalf("bad rendered geometry chat=%+v tools=%+v files=%+v", chat, tools, files)
				}
				if a.filesWidth != 440 || a.rightWidth != 520 {
					t.Fatal("window fitting overwrote preferred widths")
				}
				captureWorkbenchAudit(t, tt, fmt.Sprintf("columns-%dx%d-%s", size.width, size.height, mode))
				tt.SetSize(1600, 1000)
				if a.filesWidth != 440 || a.rightWidth != 520 {
					t.Fatal("growing viewport changed preferences")
				}
				if !a.fit.Sidebar || !a.fit.Files || !a.fit.Right {
					t.Fatal("fit-hidden panel did not return")
				}
			})
		}
	}
}

func captureWorkbenchAudit(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("RCODE_WORKBENCH_EVIDENCE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, tt.Image())
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	geometry := map[string]ui.Rect{}
	for _, label := range []string{"Conversation pane", "Tools workspace", "Files explorer", "Bottom terminal", "Workspace tabs", "Scroll workspace tabs", "New surface"} {
		if rect, ok := tt.Find(label); ok {
			geometry[label] = rect
		}
	}
	data, err := json.MarshalIndent(struct {
		Kind          string `json:"kind"`
		Width, Height int
		Geometry      map[string]ui.Rect `json:"geometry"`
	}{"MyGo ui.NewTester implementation render", tt.Image().Bounds().Dx(), tt.Image().Bounds().Dy(), geometry}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspacePaneDragAndTakeoverLifecycle(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.workspace.OpenSurface("changes")
	a.files = true
	tt := ui.NewTester(a.View, 1600, 1000)
	files, _ := tt.Find("Files explorer")
	x := files.X - 1
	tt.Press(x, 220)
	tt.Move(x-80, 220)
	tt.Release(x-80, 220)
	if a.filesWidth < 356 || a.filesWidth > 376 {
		t.Fatalf("files drag did not persist: %g", a.filesWidth)
	}
	tools, _ := tt.Find("Tools workspace")
	x = tools.X - 1
	tt.Press(x, 220)
	tt.Move(x-30, 220)
	tt.Release(x-30, 220)
	if a.rightWidth < 540 || a.rightWidth > 560 {
		t.Fatalf("workspace drag did not persist: %g", a.rightWidth)
	}
	a.rightExpanded = true
	tt.Frame()
	if _, ok := tt.Find("Conversation pane"); ok {
		t.Fatal("takeover retained conversation")
	}
	tools, _ = tt.Find("Tools workspace")
	files, _ = tt.Find("Files explorer")
	if tools.X < 255 || tools.X+tools.W > files.X {
		t.Fatalf("takeover overlapped adjacent columns: %+v %+v", tools, files)
	}
	captureWorkbenchAudit(t, tt, "takeover-1600-light")
	a.workspace.CloseActive()
	tt.Frame()
	if a.right || a.rightExpanded || !a.files {
		t.Fatal("last surface close did not collapse only the tools host")
	}
	if _, ok := tt.Find("Files explorer"); !ok {
		t.Fatal("closing tools removed explorer")
	}
	a.workspace.OpenSurface("history")
	tt.Frame()
	if !a.right || a.rightExpanded {
		t.Fatal("reopening tools unexpectedly restored takeover")
	}
}

func TestWorkspaceTerminalDrawerDoesNotShortenOtherColumns(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.workspace.OpenSurface("changes")
	a.files = true
	a.toggleTerminal()
	tt := ui.NewTester(a.View, 1600, 1000)
	bottom, ok := tt.Find("Bottom terminal")
	if !ok {
		t.Fatal("bottom terminal absent")
	}
	chat, _ := tt.Find("Conversation pane")
	tools, _ := tt.Find("Tools workspace")
	files, _ := tt.Find("Files explorer")
	if bottom.X != chat.X || bottom.W != chat.W || tools.Y+tools.H != 1000 || files.Y+files.H != 1000 || bottom.X+bottom.W >= tools.X+tools.W {
		t.Fatalf("drawer displaced other columns: bottom=%+v chat=%+v tools=%+v files=%+v", bottom, chat, tools, files)
	}
	captureWorkbenchAudit(t, tt, "terminal-combination-1600")
}

func TestWorkspaceOverflowEvidence(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession(a.state.Sessions[0].ID)
			a.state.Settings.Appearance.Mode = mode
			a.state.Settings.Appearance.ThemeID = "rcode-" + mode
			for i := 0; i < 18; i++ {
				a.state.Files = append(a.state.Files, contracts.File{ID: fmt.Sprintf("audit-file-%d", i), ProjectID: a.projectID, Name: fmt.Sprintf("workspace-file-%02d.go", i), Path: fmt.Sprintf("src/workspace-file-%02d.go", i), Kind: "file", Content: "package workspace\n", Version: 1})
			}
			a.workspace.Update(a.state)
			for i := 0; i < 18; i++ {
				a.workspace.OpenFile(fmt.Sprintf("audit-file-%d", i))
			}
			a.files = true
			tt := ui.NewTester(a.View, 1320, 880)
			// The tools surface spans the full window height and the tab
			// strip lives in the titlebar band over it; the body starts
			// below the band, so the strip is not an extra row.
			strip, _ := tt.Find("Workspace tabs")
			tools, _ := tt.Find("Tools workspace")
			surface, _ := tt.Find("Workspace surface")
			if strip.Y != 0 || strip.H != 38 || tools.Y != 0 || tools.H != 880 || surface.Y != 0 || surface.H != 880 {
				t.Fatalf("tools surface does not span the window: strip=%+v host=%+v surface=%+v", strip, tools, surface)
			}
			captureWorkbenchAudit(t, tt, "overflow-1320-"+mode)
		})
	}
}

func TestWorkspacePanelCombinationMatrix(t *testing.T) {
	for _, viewport := range []int{900, 1320, 1600} {
		for mask := 0; mask < 8; mask++ {
			t.Run(fmt.Sprintf("width-%d-panels-%03b", viewport, mask), func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession(a.state.Sessions[0].ID)
				a.workspace.OpenSurface("changes")
				a.sidebar, a.right, a.files = mask&1 != 0, mask&2 != 0, mask&4 != 0
				a.panelOpened = [3]uint64{1, 2, 3}
				tt := ui.NewTester(a.View, viewport, 880)
				var previous float32
				for _, label := range []string{"Conversation pane", "Tools workspace", "Files explorer"} {
					r, visible := tt.Find(label)
					if !visible {
						continue
					}
					if r.X < previous || r.X+r.W > float32(viewport) || r.W < 219 {
						t.Fatalf("%s violates column geometry: %+v after %g", label, r, previous)
					}
					previous = r.X + r.W
				}
				if a.sidebar != (mask&1 != 0) || a.right != (mask&2 != 0) || a.files != (mask&4 != 0) {
					t.Fatal("fit changed explicit open flags")
				}
			})
		}
	}
}

func TestFitHiddenPanelTogglePromotesRatherThanCloses(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.workspace.OpenSurface("changes")
	a.files = true
	a.panelOpened = [3]uint64{3, 2, 1}
	tt := ui.NewTester(a.View, 900, 600)
	if a.fit.Files {
		t.Fatal("fixture must begin with hidden files")
	}
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	if !a.files || !a.fit.Files {
		t.Fatal("hidden explorer toggle closed it instead of promoting it")
	}
	opened := a.panelOpened
	a.setPanel(2, true)
	tt.Frame()
	if a.panelOpened != opened {
		t.Fatal("opening an already visible panel changed fit priority")
	}
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	if a.files {
		t.Fatal("visible explorer toggle did not close it")
	}
}

func TestConversationSwitchRestoresOwnedWorkspaceAndTakeover(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.workspace.OpenSurface("history")
	a.rightExpanded = true
	firstOrder := a.panelOpened
	a.selectSession("chat-6")
	if a.workspace.TabCount() != 0 || a.rightExpanded {
		t.Fatal("first conversation's workspace/takeover leaked into second")
	}
	a.workspace.OpenSurface("changes")
	a.selectSession("chat-0")
	if a.workspace.TabCount() != 1 || a.workspace.ActiveKind() != "history" || !a.rightExpanded || a.panelOpened != firstOrder {
		t.Fatal("first conversation's workspace/takeover did not restore")
	}
	a.selectSession("chat-6")
	if a.workspace.ActiveKind() != "changes" || a.rightExpanded {
		t.Fatal("second conversation's workspace did not restore independently")
	}
}

func TestSidebarDragPersistsLogicalWidth(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	if divider.X != 255 {
		t.Fatalf("source sidebar border should end at 256, got %+v", divider)
	}
	tt.Press(divider.X, 200)
	tt.Move(divider.X+70, 200)
	tt.Release(divider.X+70, 200)
	if a.sidebarWidth < 316 || a.sidebarWidth > 336 {
		t.Fatalf("sidebar drag did not persist: %g", a.sidebarWidth)
	}
	tt.Frame()
	if a.state.Settings.SidebarWidth != float64(a.sidebarWidth) {
		t.Fatal("resized width did not reach preferences")
	}
}
