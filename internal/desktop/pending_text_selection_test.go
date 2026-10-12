package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPendingMessageSelectionPreservesSource(t *testing.T) {
	for name, source := range map[string]string{
		"unicode-markdown": "中文与 emoji 🧪\n\n```go\nvar value = 1\n```\n\n\n尾行\n",
		"crlf":             "CRLF 第一行\r\n\r\n末行 🧪\r\n",
		"long-line":        "第一行\n" + strings.Repeat("很长的行🧪", 40) + "\n末行",
	} {
		t.Run(name, func(t *testing.T) {
			app := Open(nil, OpenOptions{Dispatch: func(func()) {}})
			t.Cleanup(func() { _ = app.Close() })
			tester := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Width(280).Children(func() {
					app.renderTextBlocks(c, newPalette(true), "pending-selection", source)
				})
			}, 320, 1000)
			first := strings.Split(source, "\n")[0]
			if err := tester.Click(first); err != nil {
				t.Fatal(err)
			}
			tester.Key(ui.Cmd, ui.KeyA)
			tester.Key(ui.Cmd, ui.KeyC)
			if got := tester.Clipboard(); got != source {
				t.Fatalf("pending Select All copied %q, want original source %q", got, source)
			}
		})
	}
}

func TestPendingMessageDragAcrossBlankLines(t *testing.T) {
	const source = "第一行 🧪\n\n\n最后一行"
	app := Open(nil, OpenOptions{Dispatch: func(func()) {}})
	t.Cleanup(func() { _ = app.Close() })
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(280).Children(func() {
			app.renderTextBlocks(c, newPalette(true), "pending-drag", source)
		})
	}, 320, 240)
	first, ok := tester.Find("第一行 🧪")
	if !ok {
		t.Fatal("first line is absent")
	}
	last, ok := tester.Find("最后一行")
	if !ok {
		t.Fatal("last line is absent")
	}
	tester.Press(first.X, first.Y+first.H/2)
	tester.Move(last.X+last.W-1, last.Y+last.H/2)
	tester.Release(last.X+last.W-1, last.Y+last.H/2)
	tester.Key(ui.Cmd, ui.KeyC)
	if got := tester.Clipboard(); got != source {
		t.Fatalf("cross-line drag copied %q, want %q", got, source)
	}
}
