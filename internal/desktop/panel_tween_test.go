package desktop

import (
	"testing"
	"time"

	"gcode/internal/design"

	"github.com/egoist/mygo/ui"
)

// A panel toggle eases its column between the painted and settled widths
// instead of flashing in or out; the content keeps the width it had so the
// moving edge reveals it rather than reflowing it every frame.
func TestPanelColumnsEaseOpenAndClose(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	openWidth := a.panelPainted[0]
	if openWidth < design.SidebarMin-1 {
		t.Fatalf("the sidebar started at %g", openWidth)
	}
	if err := tt.Click("Toggle sidebar"); err != nil {
		t.Fatal(err)
	}
	if !a.panelAnimating(0) {
		t.Fatal("closing did not start a width tween")
	}
	if got := a.panelPainted[0]; got < openWidth-1 {
		t.Fatalf("the close snapped to %g instead of easing", got)
	}
	if got := a.panelContent[0]; got < openWidth-1 {
		t.Fatalf("the closing content was reflowed to %g", got)
	}
	tt.Advance(90 * time.Millisecond)
	mid := a.panelPainted[0]
	if mid <= 0 || mid >= openWidth-1 {
		t.Fatalf("halfway through the close the column was %g, want between", mid)
	}
	tt.Advance(300 * time.Millisecond)
	if got := a.panelPainted[0]; got != 0 {
		t.Fatalf("the closed column settled at %g", got)
	}
	if err := tt.Click("Toggle sidebar"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if got := a.panelPainted[0]; got < design.SidebarMin-1 {
		t.Fatalf("the reopened column settled at %g", got)
	}
}

// Reduced motion snaps the toggle in one frame, as the reference does.
func TestPanelToggleSnapsUnderReducedMotion(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Settings.Appearance.ReduceMotion = true
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if a.panelPainted[0] < design.SidebarMin-1 {
		t.Fatal("the sidebar did not open")
	}
	if err := tt.Click("Toggle sidebar"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panelAnimating(0) || a.panelPainted[0] != 0 {
		t.Fatalf("reduced motion eased the close: animating=%v width=%g", a.panelAnimating(0), a.panelPainted[0])
	}
}

// The columns beside the sidebar move with the seam in the same frame: a drag
// never leaves the conversation and workspace a frame behind the divider.
func TestSidebarDragMovesColumnsInTheSameFrame(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("sidebar divider missing")
	}
	tt.Press(divider.X, 300)
	defer tt.Release(divider.X, 300)
	for _, dx := range []float32{40, 80, 40, 0, -30} {
		tt.Move(divider.X+dx, 300)
		tt.Frame()
		chat, _ := tt.Find("Conversation pane")
		if diff := chat.W - a.chatWidth; diff > 1 || diff < -1 {
			t.Fatalf("dx=%g: the conversation column lagged the seam: pane=%g model=%g", dx, chat.W, a.chatWidth)
		}
	}
}
