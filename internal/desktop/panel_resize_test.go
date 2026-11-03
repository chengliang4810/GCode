package desktop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gcode/internal/contracts"
	"gcode/internal/design"
)

func TestResizeBounceOffsetPulsesAtTheEdge(t *testing.T) {
	if got := resizeBounceOffset(resizeEdgeMax, 0); got != 0 {
		t.Fatalf("the bounce starts at rest, got %g", got)
	}
	peak := resizeBounceOffset(resizeEdgeMax, resizeBounceOutShare)
	if peak < resizeBounceTravel-0.01 || peak > resizeBounceTravel+0.01 {
		t.Fatalf("the bounce should reach its travel, got %g", peak)
	}
	if got := resizeBounceOffset(resizeEdgeMax, 1); got != 0 {
		t.Fatalf("the bounce comes home, got %g", got)
	}
	if got := resizeBounceOffset(resizeEdgeMin, resizeBounceOutShare); got > -resizeBounceTravel+0.01 {
		t.Fatalf("the minimum edge bounces the other way, got %g", got)
	}
}

func TestSplitResizeKeepsTheWidthInsideItsLimits(t *testing.T) {
	var resize splitResize
	request := float32(500)
	var width, painted float32
	tt := ui.NewTester(func(c *ui.Context) {
		width = resize.observe(c, request, design.SidebarMin, design.SidebarMax, false)
		// The split lays the pane out at the painted size, as the app does.
		painted = resize.paint(c, width, false)
	}, 400, 300)
	tt.Frame()
	if width != design.SidebarMax {
		t.Fatalf("a push past the maximum keeps the width at its limit, got %g", width)
	}
	if resize.held != resizeEdgeMax || resize.bounced != resizeEdgeMax || resize.started.IsZero() {
		t.Fatalf("the limit the pointer crossed should be latched, got %v", resize.held)
	}
	started := resize.started
	request = 520
	tt.Frame()
	if !resize.started.Equal(started) {
		t.Fatal("a held edge restarted the bounce")
	}
	// Back inside: the seam takes the pointer's width at once, and the pulse
	// already running plays out rather than snapping by its remaining travel.
	request = 300
	tt.Frame()
	if resize.held != resizeEdgeNone {
		t.Fatalf("the held edge should clear, got %v", resize.held)
	}
	if width != 300 {
		t.Fatalf("the width inside the limits is the one the pointer asked for, got %g", width)
	}
	// The pulse started at zero travel; it still owes its travel a moment on.
	tt.Advance(resizeBounceDuration / 3)
	tt.Frame()
	if painted <= 300 || resize.started.IsZero() {
		t.Fatalf("the running pulse should still show, painted %g", painted)
	}
	tt.Advance(resizeBounceDuration)
	request = 310
	tt.Frame()
	if painted != 310 {
		t.Fatalf("the pulse should have finished, painted %g", painted)
	}
}

type paneSaveRecordingService struct {
	contracts.Service
	saved []contracts.Settings
}

func (s *paneSaveRecordingService) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "settings.update" && command.Settings != nil {
		s.saved = append(s.saved, *command.Settings)
	}
	return s.Service.Execute(ctx, command)
}

func TestSidebarDragSavesOnceTheDragRests(t *testing.T) {
	a := testApp(t, "default")
	recorder := &paneSaveRecordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1320, 880)
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	tt.Press(divider.X, 200)
	tt.Move(divider.X+30, 200)
	tt.Move(divider.X+50, 200)
	tt.Move(divider.X+70, 200)
	tt.Release(divider.X+70, 200)
	width := a.sidebarWidth
	if width < 316 || width > 336 {
		t.Fatalf("sidebar drag did not move the width: %g", width)
	}
	if len(recorder.saved) != 0 {
		t.Fatalf("the drag saved on its way: %d settings writes", len(recorder.saved))
	}
	tt.Advance(paneWidthSaveDelay)
	if len(recorder.saved) != 1 {
		t.Fatalf("the resting drag should save once, got %d", len(recorder.saved))
	}
	if recorder.saved[0].SidebarWidth != float64(width) {
		t.Fatalf("the resting width did not reach the store: %g", recorder.saved[0].SidebarWidth)
	}
}

// Reaching the minimum keeps the pane open. Continuing the same drag closes
// only that pane, and reopening restores its minimum width.
func TestPanelDragPastMinimumCloses(t *testing.T) {
	for _, panel := range []struct {
		name, label string
		index       int
		minimum     float32
	}{
		{"sidebar", "Session sidebar", 0, design.SidebarMin},
		{"workspace", "Tools workspace", 1, design.RightMin},
		{"files", "Files explorer", 2, design.FilesMin},
	} {
		for _, reduced := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reduced=%t", panel.name, reduced), func(t *testing.T) {
				a := testApp(t, "default")
				recorder := &paneSaveRecordingService{Service: a.service}
				a.service = recorder
				a.selectSession("chat-0")
				a.state.Settings.Appearance.ReduceMotion = reduced
				a.workspace.OpenSurface("changes")
				a.sidebar, a.right, a.files = true, true, true
				tt := ui.NewTester(a.View, 1800, 880)
				pane, ok := tt.Find(panel.label)
				if !ok {
					t.Fatal("pane missing")
				}
				width := func() float32 {
					return [3]float32{a.sidebarWidth, a.rightWidth, a.filesWidth}[panel.index]
				}
				open := func() [3]bool { return [3]bool{a.sidebar, a.right, a.files} }
				seam, direction := pane.X-1, float32(1)
				if panel.index == 0 {
					seam, direction = pane.X+pane.W, -1
				}
				target := seam + direction*(width()-panel.minimum)
				tt.Press(seam, 200)
				tt.Move(target, 200)
				if !open()[panel.index] || width() != panel.minimum {
					t.Fatalf("at minimum: open=%v width=%g, want open at %g", open(), width(), panel.minimum)
				}
				tt.Move(target+direction*12, 200)
				if open()[panel.index] {
					t.Fatal("continuing past the minimum did not close the pane before release")
				}
				for i, otherOpen := range open() {
					if i != panel.index && !otherOpen {
						t.Fatalf("drag closed neighboring pane %d", i)
					}
				}
				tt.Release(target+direction*12, 200)
				tt.Advance(300 * time.Millisecond)
				if _, visible := tt.Find(panel.label); visible || a.panelPainted[panel.index] != 0 {
					t.Fatal("closed pane remained visible")
				}
				a.setPanel(panel.index, true)
				tt.Frame()
				tt.Advance(300 * time.Millisecond)
				if _, visible := tt.Find(panel.label); !visible || width() != panel.minimum {
					t.Fatalf("reopened pane: visible=%t width=%g, want %g", visible, width(), panel.minimum)
				}
				if len(recorder.saved) == 0 {
					t.Fatal("dragged width was not saved")
				}
				saved := recorder.saved[len(recorder.saved)-1]
				if got := [3]float64{saved.SidebarWidth, saved.RightPaneWidth, saved.FilesWidth}[panel.index]; got != float64(panel.minimum) {
					t.Fatalf("saved width=%g, want %g", got, panel.minimum)
				}
				pane, _ = tt.Find(panel.label)
				seam = pane.X - 1
				if panel.index == 0 {
					seam = pane.X + pane.W
				}
				tt.Press(seam, 200)
				tt.Move(seam+direction*80, 200)
				tt.Release(seam+direction*80, 200)
				if open()[panel.index] {
					t.Fatal("a fast drag after reopening did not close the pane")
				}
			})
		}
	}
}

// An arrow step on the focused divider lands after the frame that built the
// split; the next frame adopts and keeps it.
func TestSidebarArrowStepPersists(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	start := a.sidebarWidth
	tt.ClickAt(divider.X, 200)
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if a.sidebarWidth <= start {
		t.Fatalf("Right did not widen the sidebar: %g from %g", a.sidebarWidth, start)
	}
	want := a.sidebarWidth
	tt.Frame()
	if a.sidebarWidth != want {
		t.Fatalf("the adopted step moved again: %g from %g", a.sidebarWidth, want)
	}
	if a.state.Settings.SidebarWidth != float64(want) {
		t.Fatalf("the step did not reach preferences: %g", a.state.Settings.SidebarWidth)
	}
}

func TestSidebarDragBouncesPastItsMaximum(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	tt.Press(divider.X, 200)
	tt.Move(divider.X+300, 200)
	tt.Release(divider.X+300, 200)
	if a.sidebarWidth != design.SidebarMax {
		t.Fatalf("the width stays at its limit, got %g", a.sidebarWidth)
	}
	tt.Advance(resizeBounceDuration / 3)
	if a.visibleSidebarWidth <= design.SidebarMax {
		t.Fatalf("the seam did not bounce past the limit, got %g", a.visibleSidebarWidth)
	}
	tt.Advance(resizeBounceDuration)
	if a.visibleSidebarWidth != design.SidebarMax {
		t.Fatalf("the bounce did not come home, got %g", a.visibleSidebarWidth)
	}
}

// A pane hidden and shown again must bounce at its limit: a latched edge
// from before it was hidden would swallow the second push.
func TestSplitResizeBouncesAgainAfterReset(t *testing.T) {
	var resize splitResize
	request := float32(500)
	tt := ui.NewTester(func(c *ui.Context) {
		resize.observe(c, request, design.SidebarMin, design.SidebarMax, false)
	}, 400, 300)
	tt.Frame()
	first := resize.started
	if first.IsZero() {
		t.Fatal("the first push past the limit did not bounce")
	}
	tt.Advance(resizeBounceDuration)
	tt.Frame()
	resize.reset()
	tt.Frame()
	if resize.started.IsZero() {
		t.Fatal("a shown pane kept the edge latched from before it was hidden")
	}
}

// Dragging one pane must not write widths the user never dragged back over a
// width another surface changed.
func TestPaneSaveKeepsWidthsTheUserDidNotDrag(t *testing.T) {
	a := testApp(t, "default")
	recorder := &paneSaveRecordingService{Service: a.service}
	a.service = recorder
	a.state.Settings.FilesWidth = 300
	a.filesWidth = 286 // as the window read it when it opened
	tt := ui.NewTester(a.View, 1320, 880)
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	tt.Press(divider.X, 200)
	tt.Move(divider.X+40, 200)
	tt.Release(divider.X+40, 200)
	tt.Advance(paneWidthSaveDelay)
	if len(recorder.saved) != 1 {
		t.Fatalf("the resting drag should save once, got %d", len(recorder.saved))
	}
	if recorder.saved[0].FilesWidth != 300 {
		t.Fatalf("a sidebar drag rewrote the files width: %g", recorder.saved[0].FilesWidth)
	}
	if recorder.saved[0].SidebarWidth != float64(a.sidebarWidth) {
		t.Fatalf("the dragged width did not reach the store: %g", recorder.saved[0].SidebarWidth)
	}
}
