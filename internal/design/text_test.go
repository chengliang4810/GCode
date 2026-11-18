package design

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Interface levels keep their pixel distance from the 14 px default and stop
// at the 12 px floor, so a small interface font shares the floor instead of
// shrinking the lower levels past it.
func TestInterfaceFontUsesPixelOffsets(t *testing.T) {
	for _, base := range []float32{12, 14, 20} {
		t.Run(fmt.Sprint(base), func(t *testing.T) {
			tt := ui.NewTester(func(c *ui.Context) {
				theme := Palette("rcode", false).UI()
				theme.FontSize = base
				c.SetTheme(theme)
				for _, role := range []struct{ size, offset float32 }{
					{TextSmall, -2}, {TextCaption, -1}, {TextBase, 0},
					{TextLarge, 2}, {TextXL, 4}, {TextTitle, 10}, {TextTitleLarge, 16},
				} {
					want := max(base+role.offset, MinTextSize)
					if got := TextSize(c, role.size); got != want {
						t.Fatalf("role %g at base %g: got %g, want %g", role.size, base, got, want)
					}
				}
			}, 100, 100)
			tt.Frame()
		})
	}
}
