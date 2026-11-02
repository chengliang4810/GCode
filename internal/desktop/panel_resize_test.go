package desktop

import (
	"context"
	"testing"

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

// A drag left past the sidebar minimum stops there, and the next frame keeps
// the limit: the split size the drag left is not mistaken for a keyboard step.
func TestSidebarDragStopsAtItsMinimum(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	tt.Press(divider.X, 200)
	tt.Move(divider.X-80, 200)
	tt.Release(divider.X-80, 200)
	if a.sidebarWidth != design.SidebarMin {
		t.Fatalf("the drag stopped at %g, want %g", a.sidebarWidth, design.SidebarMin)
	}
	tt.Frame()
	if a.sidebarWidth != design.SidebarMin {
		t.Fatalf("the next frame moved the width to %g", a.sidebarWidth)
	}
	if a.state.Settings.SidebarWidth != float64(design.SidebarMin) {
		t.Fatalf("the limit did not reach preferences: %g", a.state.Settings.SidebarWidth)
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
