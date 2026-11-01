package design

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestInterfaceFontUsesPixelOffsets(t *testing.T) {
	for _, base := range []float32{12, 14, 20} {
		t.Run(fmt.Sprint(base), func(t *testing.T) {
			tt := ui.NewTester(func(c *ui.Context) {
				theme := Palette("rcode", false).UI()
				theme.FontSize = base
				c.SetTheme(theme)
				for _, role := range []struct{ size, offset float32 }{
					{TextXS, -4}, {TextSmall, -2}, {TextCaption, -1}, {TextBase, 0},
					{TextLarge, 2}, {TextXL, 4}, {TextTitle, 10}, {TextTitleLarge, 16},
				} {
					if got := TextSize(c, role.size); got != base+role.offset {
						t.Fatalf("role %g at base %g: got %g, want %g", role.size, base, got, base+role.offset)
					}
				}
			}, 100, 100)
			tt.Frame()
		})
	}
}
