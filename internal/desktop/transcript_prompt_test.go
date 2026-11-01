package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestPromptMarkdownPreservesEscapedNamesAndSurroundingText(t *testing.T) {
	a := testApp(t, "default")
	name := "[文档]\\ *name* & <正文>"
	raw := (contracts.PromptReference{Kind: "skill", ID: "docs", Label: name}).Markdown()
	source := "**重点** " + raw + " [外链](https://example.invalid)\n\n```text\n" + raw + "\n```"
	tt := ui.NewTester(func(c *ui.Context) {
		a.prepareTranscriptMarkdown(c)
		markdown.View(c, source, &a.userMD)
	}, 600, 400)
	for _, text := range []string{"重点", name, "外链", raw} {
		if !tt.HasText(text) {
			t.Fatalf("reference rendering lost %q: %v", text, tt.Texts())
		}
	}
	document := parser.Parse(raw)
	for index := range document.Children(0) {
		for _, run := range mathInlineRuns(document.Inlines(index), ui.Span{}, "") {
			if run.link == "gcode://skill/docs" && run.span.Text != name {
				t.Fatalf("escaped label split into inconsistent tokens: %+v", run)
			}
		}
	}
}
