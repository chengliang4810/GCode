package desktop

import (
	"fmt"
	"math"
	"testing"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTranscriptScrollbarTracksConversationPaneEdge(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scene := range []struct {
			name  string
			width int
			info  bool
		}{
			{"info", 1500, true},
			{"info-hidden", 1500, false},
			{"narrow", 900, false},
			{"wide", 1800, true},
		} {
			t.Run(fmt.Sprintf("%s/dark=%v", scene.name, dark), func(t *testing.T) {
				a := testApp(t, "transcript")
				a.selectSession("chat-0")
				a.right, a.files = false, false
				a.expanded["session-info-user"], a.expanded["session-info"] = true, scene.info
				a.state.Settings.Appearance.ReduceMotion = true
				a.state.Messages[a.sessionID] = railFixtureMessages(3)
				a.state.Revision++
				a.palette = design.Palette("rcode", dark)
				var viewport ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(a.palette.UI())
					a.View(c)
					viewport = a.messages.Bounds(c)
				}, scene.width, 800)
				tt.Frame()
				pane, ok := tt.Find("Conversation pane")
				if !ok || viewport.H <= 0 {
					t.Fatal("conversation viewport missing")
				}
				if math.Abs(float64(viewport.X-pane.X)) > 1 ||
					math.Abs(float64(viewport.X+viewport.W-pane.X-pane.W)) > 1 {
					t.Fatalf("scroll viewport is inset from pane: viewport=%+v pane=%+v", viewport, pane)
				}
				reserve, _ := a.infoVisibility()
				if scene.name == "info" && reserve <= 0 {
					t.Fatal("fixture did not reserve room for the information card")
				}
				// Moving the scrollbar must not move text under the information
				// card, or shift the centered reading column.
				text, ok := tt.Find("Reply 02 paragraph 11")
				wantLeft := pane.X + max((pane.W-reserve-736)/2, 0) + 24
				if !ok || math.Abs(float64(text.X-wantLeft)) > 1 {
					t.Fatalf("reading column moved: text=%+v expected x=%v reserve=%v", text, wantLeft, reserve)
				}
				before, _ := a.messages.Visible()
				tt.Scroll(viewport.X+viewport.W-12, viewport.Y+viewport.H/2, 0, -180)
				after, _ := a.messages.Visible()
				if after >= before {
					t.Fatal("scrolling near the pane edge did not reach the transcript")
				}
			})
		}
	}
}
