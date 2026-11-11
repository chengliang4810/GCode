package design

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func TestTranscriptCodeTabStopsPreserveSource(t *testing.T) {
	for _, test := range []struct {
		language string
		spaces   int
	}{
		{"go", 4}, {"js", 2}, {"ts", 2}, {"json", 2}, {"unknown", 4},
	} {
		t.Run(test.language, func(t *testing.T) {
			const suffix = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
			line := "\t" + suffix
			source := fmt.Sprintf("```%s\n%s\n```", test.language, line)
			var expected float32
			tt := ui.NewTester(func(c *ui.Context) {
				font := markdown.DefaultTheme(c.Theme())
				font.CodeFont = "monospace"
				options := &markdown.Options{Theme: font, CodeBlock: MarkdownCodeBlock}
				ui.Column(c).Fill().Children(func() { markdown.View(c, source, options) })
				expected, _ = c.MeasureText(0, ui.Span{
					Text: strings.Repeat(" ", test.spaces) + suffix,
					Font: font.CodeFont, Size: font.FontSize * font.CodeSize,
				})
			}, 1400, 220)
			code, ok := tt.Find(line)
			if !ok {
				t.Fatalf("code did not retain source tabs: %v", tt.Texts())
			}
			if math.Abs(float64(code.W-expected)) > 1 {
				t.Fatalf("%s tab layout width=%v, want %v (%d spaces)", test.language, code.W, expected, test.spaces)
			}
			tt.ClickAt(code.X+code.W/2, code.Y+code.H/2)
			tt.Command("selectAll")
			tt.Command("copy")
			if got := tt.Clipboard(); got != line {
				t.Fatalf("selected code copied %q, want original %q", got, line)
			}
			advance := expected / float32(len(suffix)+test.spaces)
			y := code.Y + code.H/2
			tt.Press(code.X+advance/4, y)
			tt.Move(code.X+advance*float32(test.spaces+2)+advance/8, y)
			tt.Release(code.X+advance*float32(test.spaces+2)+advance/8, y)
			tt.Command("copy")
			if got := tt.Clipboard(); got != "\txx" {
				t.Fatalf("partial code selection copied %q, want original tab and two letters", got)
			}
			if err := tt.Click("Copy code"); err != nil {
				t.Fatal(err)
			}
			if got := tt.Clipboard(); got != line+"\n" {
				t.Fatalf("code button copied %q, want original line", got)
			}
		})
	}
}
