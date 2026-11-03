package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarStatusSemanticColors(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, status := range []contracts.SessionStatus{contracts.Working, contracts.AwaitingInput, contracts.Errored, contracts.Idle} {
			t.Run(fmt.Sprintf("dark-%v-%s", dark, status), func(t *testing.T) {
				a := sidebarLayoutApp(t)
				a.state.Settings.Appearance.Mode = "light"
				if dark {
					a.state.Settings.Appearance.Mode = "dark"
				}
				ui.NewTester(a.View, 900, 1000)
				expected := a.palette.Accent
				if status == contracts.AwaitingInput {
					expected = a.palette.Warning
				}
				if status == contracts.Errored {
					expected = a.palette.Danger
				}
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Box(c).Size(24, 24).Center().Children(func() { a.sidebarStatusGlyph(c, contracts.Session{Status: status, Unread: true}) })
				}, 24, 24)
				img := tt.Image()
				for y := 0; y < 24; y++ {
					for x := 0; x < 24; x++ {
						p := img.RGBAAt(x, y)
						if p.A > 0 && absByte(p.R, expected.R) < 65 && absByte(p.G, expected.G) < 65 && absByte(p.B, expected.B) < 65 {
							return
						}
					}
				}
				t.Fatalf("status %s did not render theme color %+v", status, expected)
			})
		}
	}
}
