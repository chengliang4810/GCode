package desktop

import (
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *App) prepareTranscriptMarkdown(c *ui.Context) {
	a.md.ShareSelection = true
	fontScale := float32(a.state.Settings.Appearance.FontSize) / 16
	if fontScale <= 0 {
		fontScale = 1
	}
	codeFont := design.MonoFont(a.state.Settings.Appearance.CodeFont)
	a.md.Theme = &markdown.Theme{FontSize: 14 * fontScale, Font: design.Font(a.state.Settings.Appearance.Font), CodeFont: codeFont, CodeSize: float32(a.state.Settings.Appearance.CodeFontSize) / (14 * fontScale), Link: a.palette.Accent, BlockSpacing: 12 / (14 * fontScale), CodeBlockRadius: 10, FixedLineHeight: 22 * fontScale, CodeFixedLineHeight: 18 * float32(a.state.Settings.Appearance.CodeFontSize) / 12.5}
	headingLines := []float32{27, 24, 22, 22, 22, 22}
	for i, size := range []float32{19, 16, 15, 14, 14, 14} {
		a.md.Theme.Headings[i] = markdown.HeadingStyle{Size: size / 14, Weight: 600, Top: 12 / (14 * fontScale), Bottom: 12 / (14 * fontScale), FixedLineHeight: headingLines[i] * fontScale}
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
	a.md.Theme.CodeBackground = a.palette.Input
	a.md.Theme.TableBorder = a.palette.Border
	a.md.Theme.TableHeader = a.palette.Card
	userTheme := *a.md.Theme
	userTheme.FontSize = 14 * fontScale
	userTheme.CodeSize = float32(a.state.Settings.Appearance.CodeFontSize) / userTheme.FontSize
	a.userMD.Theme = &userTheme
}
