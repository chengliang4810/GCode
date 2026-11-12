package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestComposerLongDraftPaintClipped(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []float32{380, 736} {
			for _, hero := range []bool{false, true} {
				t.Run(fmt.Sprintf("dark=%v/width=%g/hero=%v", dark, width, hero), func(t *testing.T) {
					a := testApp(t, "default")
					a.palette = design.Palette("rcode", dark)
					a.draft = strings.Repeat("中文 internal/engine/files.go long wrapped paragraph ", 200)
					var input ui.Rect
					tt := ui.NewTester(func(c *ui.Context) {
						c.SetTheme(a.palette.UI())
						c.Root().Background(a.palette.Background)
						ui.Column(c).Absolute().Left(30).Top(80).Width(width).Children(func() {
							a.nativeComposerSurface(c, composerSurfaceProps{
								draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
								controls: a.centralControls(), hero: hero, widthHint: width,
								inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send",
								onSend: func(string) bool { return false }, onChooseAttach: func() {},
							})
						})
						input = a.composer.Bounds(c)
					}, int(width)+60, 500)
					tt.Advance(time.Second)
					tt.Frame()
					if input.H != 260 {
						t.Fatalf("long draft did not reach viewport cap: %v", input)
					}
					content := tt.Image()
					captureWorkbenchAudit(t, tt, fmt.Sprintf("composer-clip-%v-%g-%v", dark, width, hero))
					// Keep the same capped geometry and controls, with no visible
					// glyphs. Every pixel outside the editor must remain identical.
					a.draft = strings.Repeat(" \n", 100)
					for range 4 {
						tt.Frame()
					}
					blank := tt.Image()
					for y := 0; y < content.Bounds().Dy(); y++ {
						for x := 0; x < content.Bounds().Dx(); x++ {
							if float32(x) >= input.X && float32(x) < input.X+input.W &&
								float32(y) >= input.Y && float32(y) < input.Y+input.H {
								continue
							}
							if content.RGBAAt(x, y) != blank.RGBAAt(x, y) {
								t.Fatalf("draft painted outside editor %v at (%d, %d)", input, x, y)
							}
						}
					}
				})
			}
		}
	}
}
