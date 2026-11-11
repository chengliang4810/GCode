package design

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

// Keep tabs in the source and let the framework shape their tab stops. The
// same language definition drives the file editor and conversation code.
func MarkdownCodeBlock(c *ui.Context, block *markdown.CodeBlockView) {
	tabSize := 4
	if language := editor.LanguageNamed(block.Language); language != nil && language.TabSize > 0 {
		tabSize = language.TabSize
	}
	ui.Column(c).AlignSelf(ui.Stretch).TabSize(float32(tabSize)).Children(func() {
		markdown.DefaultCodeBlock(c, block)
	})
}
