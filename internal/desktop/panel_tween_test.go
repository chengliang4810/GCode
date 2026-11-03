package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/design"

	"github.com/egoist/mygo/ui"
)

// Width tweens must preserve the pane's full-height layout on every frame,
// including the footer's bottom anchor, rather than expanding vertically only
// after the width has settled.
func TestPanelToggleKeepsContentHeight(t *testing.T) {
	for _, panel := range []struct {
		name, toggle, content string
		index                 int
	}{
		{"sidebar", "Toggle sidebar", "Session sidebar", 0},
		{"workspace", "Toggle right pane", "Tools workspace", 1},
		{"files", "Files", "Files explorer", 2},
	} {
		for _, mode := range []string{"light", "dark"} {
			for _, size := range []struct{ width, height int }{{900, 600}, {1600, 1000}} {
				t.Run(fmt.Sprintf("%s/%s/%dx%d", panel.name, mode, size.width, size.height), func(t *testing.T) {
					a := testApp(t, "default")
					a.selectSession("chat-0")
					a.state.Settings.Appearance.Mode = mode
					a.state.Settings.Appearance.ThemeID = "rcode-" + mode
					a.workspace.OpenSurface("changes")
					a.sidebar, a.right, a.files = panel.index == 0, panel.index == 1, panel.index == 2
					tt := ui.NewTester(a.View, size.width, size.height)
					initial, ok := tt.Find(panel.content)
					if !ok || initial.H < float32(size.height)-1 {
						t.Fatalf("initial pane does not fill the window: %+v", initial)
					}
					footer, _ := tt.Find("Settings")
					check := func(stage string) {
						t.Helper()
						if a.panelPainted[panel.index] <= 0.5 {
							return
						}
						rect, found := tt.Find(panel.content)
						if !found || rect.Y != initial.Y || rect.H != initial.H {
							t.Errorf("%s: pane compressed vertically: got %+v, want y=%g h=%g", stage, rect, initial.Y, initial.H)
						}
						if panel.index == 0 {
							button, found := tt.Find("Settings")
							if !found || button.Y != footer.Y {
								t.Errorf("%s: footer left the bottom: got %+v, want y=%g", stage, button, footer.Y)
							}
						}
					}
					for _, direction := range []string{"close", "open"} {
						if err := tt.Click(panel.toggle); err != nil {
							t.Fatal(err)
						}
						for frame := range 12 {
							tt.Advance(16 * time.Millisecond)
							check(fmt.Sprintf("%s/frame-%d", direction, frame))
						}
						tt.Advance(300 * time.Millisecond)
						check(direction + "/settled")
					}
				})
			}
		}
	}
}

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
	if got := a.panelPainted[0]; got < openWidth-1 || got > openWidth+1 {
		t.Fatalf("the reopened column settled at %g, want its original width %g", got, openWidth)
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
