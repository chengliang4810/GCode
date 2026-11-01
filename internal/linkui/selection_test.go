package linkui

import (
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestSelectionOriginalKnownFilePathAndWholeMarkdown(t *testing.T) {
	const source = "Read `/workspace/docs/guide.md:9` then [original label](https://example.com).\n\nSecond [paragraph](https://example.com/two)."
	cfg := Config{Resolve: resolveTest, Internal: true}
	var selection *Selection
	options := markdown.Options{ShareSelection: true}
	options.Block = func(c *ui.Context, document *parser.Document, index int) bool {
		cfg.Selection = selection
		return Block(c, document, index, cfg, nil)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		scope := ui.Column(c).Fill().Padding(24).Selectable()
		selection = BeginSelection(scope)
		scope.Children(func() { markdown.View(c, source, &options) })
		selection.Menu(c, scope)
	}, 500, 260)
	file, ok := tt.Find("guide.md")
	if !ok {
		t.Fatal("file link missing")
	}
	tt.Press(file.X+1, file.Y+file.H/2)
	tt.Move(file.X+file.W-1, file.Y+file.H/2)
	tt.Release(file.X+file.W-1, file.Y+file.H/2)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "/workspace/docs/guide.md:9" {
		t.Fatalf("basename copy did not restore full original path: %q", tt.Clipboard())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	want := "Read /workspace/docs/guide.md:9 then original label.\nSecond paragraph."
	if tt.Clipboard() != want {
		t.Fatalf("cross paragraph plain copy=%q, want=%q", tt.Clipboard(), want)
	}
	md, err := tt.ClipboardData().Read(transfer.Markdown)
	if err != nil || string(md) != source {
		t.Fatalf("whole paragraph Markdown=%q error=%v", md, err)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if tt.Clipboard() != source {
		t.Fatalf("Copy as Markdown did not preserve source: %q", tt.Clipboard())
	}
}

func TestSelectionPartialLabelsAndSourceOwnership(t *testing.T) {
	selection := &Selection{paragraphs: map[uint64]selectionParagraph{}}
	const display = "Inspect guide.md before continuing."
	const source = "Inspect `/workspace/docs/guide.md:9` before continuing."
	selection.register(1, display, source, []selectionReplacement{{8, 16, "/workspace/docs/guide.md:9"}})
	for _, tc := range []struct {
		name     string
		part     ui.SelectedText
		text, md string
	}{
		{"whole file", ui.SelectedText{ID: 1, Text: display, Start: 8, End: 16}, "/workspace/docs/guide.md:9", "/workspace/docs/guide.md:9"},
		{"partial file", ui.SelectedText{ID: 1, Text: display, Start: 10, End: 15}, "ide.m", "ide.m"},
		{"whole paragraph", ui.SelectedText{ID: 1, Text: display, Start: 0, End: utf8.RuneCountInString(display)}, "Inspect /workspace/docs/guide.md:9 before continuing.", source},
		{"changed paragraph", ui.SelectedText{ID: 1, Text: "unrelated", Start: 0, End: 9}, "unrelated", "unrelated"},
		{"other paragraph", ui.SelectedText{ID: 2, Text: "ordinary text", Start: 0, End: 8}, "ordinary", "ordinary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := selection.CopyData([]ui.SelectedText{tc.part})
			text, _ := data.Read(transfer.Text)
			md, _ := data.Read(transfer.Markdown)
			if string(text) != tc.text || string(md) != tc.md {
				t.Fatalf("text=%q markdown=%q", text, md)
			}
		})
	}
}
