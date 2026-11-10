package desktop

import (
	"fmt"
	"math"
	"testing"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestComposerWidthFollowsConversationSetting(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, windowWidth := range []int{900, 1680} {
			t.Run(fmt.Sprintf("dark=%t/window=%d", dark, windowWidth), func(t *testing.T) {
				a := testApp(t, "default")
				a.palette = design.Palette("rcode", dark)
				a.selectSession("chat-0")
				a.expanded["session-info-user"] = true
				a.expanded["session-info"] = false
				var surface, viewport ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					a.View(c)
					surface = a.composerSurface.Bounds(c)
					viewport = a.conversationViewport.Bounds(c)
				}, windowWidth, 880)
				for _, width := range []float64{560, 1200, 736, 560} {
					a.state.Settings.Appearance.TranscriptWidth = width
					tt.Frame()
					tt.Advance(time.Second)
					if _, ok := tt.Find("Message"); !ok {
						t.Fatal("composer missing")
					}
					want := min(viewport.W, float32(width)+32) - 32
					center := viewport.X + viewport.W/2
					if math.Abs(float64(surface.W-want)) > 1 ||
						math.Abs(float64(surface.X+surface.W/2-center)) > 1 {
						t.Fatalf("conversation width %v: surface=%+v, want width=%v centered at %v", width, surface, want, center)
					}
				}
			})
		}
	}
}
