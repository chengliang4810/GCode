package richtext

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEditorPaintStaysInsideViewport(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("中文 long wrapped paragraph ", 200),
		strings.Repeat("short paragraph\n", 100),
	} {
		doc := NewDocument(NewText(text))
		var handle ui.Handle
		var bounds ui.Rect
		tt := ui.NewTester(func(c *ui.Context) {
			c.Root().Background(ui.RGB(255, 255, 255))
			EditorBase(c, doc, EditorOptions{
				Label: "Draft", NoMarkdown: true,
				Font:  ui.Font{Size: 16},
				Theme: &Theme{Text: ui.RGB(0, 0, 0)},
			}).Bind(&handle).Absolute().Left(40).Top(80).Width(240).Height(100)
			bounds = handle.Bounds(c)
		}, 320, 300)
		for _, caret := range []int{0, doc.Text().Len() / 2, doc.Text().Len()} {
			doc.SetSelection(caret, caret)
			tt.Frame()
			tt.Frame()
			img := tt.Image()
			background := img.RGBAAt(0, 0)
			for y := 0; y < img.Bounds().Dy(); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					if float32(x) >= bounds.X && float32(x) < bounds.X+bounds.W &&
						float32(y) >= bounds.Y && float32(y) < bounds.Y+bounds.H {
						continue
					}
					if img.RGBAAt(x, y) != background {
						t.Fatalf("caret %d: editor painted outside %v at (%d, %d)", caret, bounds, x, y)
					}
				}
			}
		}
	}
}
