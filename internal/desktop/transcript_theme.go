package desktop

import (
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *App) prepareTranscriptMarkdown(c *ui.Context) {
	a.md.ShareSelection = true
	a.userMD.ShareSelection = true
	fontSize := design.TextSize(c, design.TextBase)
	codeFont := design.MonoFont(a.state.Settings.Appearance.CodeFont)
	a.md.Theme = &markdown.Theme{
		FontSize: fontSize, Font: design.Font(a.state.Settings.Appearance.Font),
		CodeFont: codeFont, CodeSize: float32(a.state.Settings.Appearance.CodeFontSize) / fontSize,
		Link: a.palette.Accent, LinkWeight: 500, BlockSpacing: 12 / fontSize, CodeBlockRadius: 10,
		FixedLineHeight:     fontSize * 1.75,
		CodeFixedLineHeight: float32(a.state.Settings.Appearance.CodeFontSize) * 1.5,
	}
	for i, size := range []float32{design.TextXL, design.TextLarge, design.TextBase, design.TextBase, design.TextBase, design.TextBase} {
		size = design.TextSize(c, size)
		weight := 600
		if i == 4 {
			weight = 500
		} else if i == 5 {
			weight = 400
		}
		a.md.Theme.Headings[i] = markdown.HeadingStyle{
			Size: size / fontSize, Weight: weight, Top: 24 / fontSize, Bottom: 16 / fontSize,
			FixedLineHeight: size * 1.5,
		}
	}
	code := editor.DefaultTheme(c.Theme())
	code.Background = a.palette.Card
	code.Text = a.palette.Text
	for kind, role := range map[editor.TokenKind]string{editor.TokenKeyword: "keyword", editor.TokenString: "string", editor.TokenComment: "comment", editor.TokenNumber: "number", editor.TokenFunction: "function", editor.TokenType: "type"} {
		if color, ok := a.palette.Syntax[role]; ok {
			code.Tokens[kind].Color = color
		}
	}
	a.md.Theme.CodeColors = code
	a.md.Theme.CodeBlockBackground = a.palette.Card
	a.md.Theme.CodeBlockBorder = a.palette.Border
	a.md.Theme.CodeBackground = a.palette.CodeWash
	a.md.Theme.TableBorder = a.palette.Border
	a.md.Theme.TableHeader = a.palette.Card
	userTheme := *a.md.Theme
	a.userMD.Theme = &userTheme
	a.configureMathMarkdown()
	a.userMD.Block = a.userMarkdownBlock
}
