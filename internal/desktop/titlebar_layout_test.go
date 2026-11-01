package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The source shell overlays its title bar over a full-height content row, so
// the right pane's surface reaches the window's top edge. GCode's panes do
// the same: the surface starts at y=0, the workspace strip lives inside the
// titlebar band, and only the pane content keeps clear of that band.
func TestRightPaneSurfaceReachesTheWindowTop(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.right = true
	tt := ui.NewTester(a.View, 1600, 1000)
	// The first frame binds the workspace to the selected conversation;
	// surfaces opened before it belong to the previous scope.
	a.workspace.OpenSurface("changes")
	tt.Frame()

	pane, ok := tt.Find("Workspace surface")
	if !ok {
		t.Fatal("right pane surface missing")
	}
	if pane.Y != 0 {
		t.Fatalf("right pane surface starts at y=%v, want 0 (flush with the window top)", pane.Y)
	}
	tabs, ok := tt.Find("Workspace tabs")
	if !ok {
		t.Fatal("workspace tabs missing")
	}
	if band := titlebarBandHeight(); tabs.Y < 0 || tabs.Y+tabs.H > band {
		t.Fatalf("workspace tabs at %+v, want inside the %vpx titlebar band", tabs, band)
	}
	heading, ok := tt.Find("Changes")
	if !ok {
		t.Fatal("changes surface content missing")
	}
	if band := titlebarBandHeight(); heading.Y < band {
		t.Fatalf("pane content starts at y=%v, behind the %vpx titlebar band", heading.Y, band)
	}
}

func TestFilesDockSurfaceReachesTheWindowTop(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.right, a.files = true, true
	tt := ui.NewTester(a.View, 1700, 1000)

	dock, ok := tt.Find("Files explorer")
	if !ok {
		t.Fatal("files dock surface missing")
	}
	if dock.Y != 0 {
		t.Fatalf("files dock surface starts at y=%v, want 0 (flush with the window top)", dock.Y)
	}
	search, ok := tt.Find("Search files")
	if !ok {
		t.Fatal("files dock search missing")
	}
	if band := titlebarBandHeight(); search.Y < band {
		t.Fatalf("files dock content starts at y=%v, behind the %vpx titlebar band", search.Y, band)
	}
}

// The overlay must stay above the surfaces for input: the titlebar's pane
// toggle still works with the pane content under it.
func TestTitlebarOverlayStaysClickable(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.right = true
	tt := ui.NewTester(a.View, 1600, 1000)
	if err := tt.Click("Toggle right pane"); err != nil {
		t.Fatal(err)
	}
	if a.right {
		t.Fatal("titlebar pane toggle did not reach the button over the pane surface")
	}
}
