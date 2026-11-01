package desktop

import (
	"fmt"
	"math"
	"testing"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTranscriptTypographyFollowsUIAndIndependentCodeSizes(t *testing.T) {
	for _, size := range []float32{12, 14, 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			app := testApp(t, "transcript")
			app.state.Settings.Appearance.FontSize = float64(size)
			app.state.Settings.Appearance.CodeFontSize = 17
			app.state.Settings.Appearance.TerminalFontSize = 19
			tt := ui.NewTester(func(c *ui.Context) {
				app.View(c)
				app.prepareTranscriptMarkdown(c)
				theme := app.md.Theme
				if theme.FontSize != size || theme.FixedLineHeight != size*1.75 {
					t.Fatalf("chat font differs from preference: %+v", theme)
				}
				if math.Abs(float64(theme.FontSize*theme.CodeSize-17)) > .001 ||
					theme.CodeFixedLineHeight != 25.5 {
					t.Fatalf("UI font changed the independent code size: %+v", theme)
				}
				for i, offset := range []float32{4, 2, 0, 0, 0, 0} {
					if math.Abs(float64(theme.Headings[i].Size*size-(size+offset))) > .001 {
						t.Fatalf("heading %d does not use the shared UI scale", i+1)
					}
				}
				if c.Theme().FontSize != size || design.Controls().IconButtonSize != 28 ||
					app.state.Settings.Appearance.TerminalFontSize != 19 {
					t.Fatal("UI preference must affect text without changing icons or terminal size")
				}
			}, 900, 700)
			tt.Frame()
		})
	}
}
