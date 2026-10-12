package desktop

import (
	"strings"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func TestParseMarkdownDocumentBuildsNativeModel(t *testing.T) {
	source := strings.Join([]string{
		"# Heading",
		"",
		`Paragraph **bold _italic_** ~~removed~~ \*literal\* ` + "`code`" + ` and [link](https://example.com).`,
		"",
		"> quoted text",
		">",
		"> - nested item",
		">   - deeper item",
		"",
		"1. first",
		"2. second",
		"",
		"| Name | Count |",
		"| :--- | ---: |",
		"| Ada | 2 |",
		"",
		"~~~go",
		"package main",
		"~~~",
		"",
		"    indented code",
	}, "\n")

	document := parseMarkdownDocument(source)
	if len(document.blocks) != 7 {
		t.Fatalf("block count = %d, want 7", len(document.blocks))
	}
	if got := document.blocks[0].kind; got != markdownHeading {
		t.Fatalf("first block kind = %v, want heading", got)
	}
	paragraph := document.blocks[1]
	if paragraph.kind != markdownParagraph {
		t.Fatalf("second block kind = %v, want paragraph", paragraph.kind)
	}
	plain := plainMarkdownText(paragraph.inline)
	for _, want := range []string{"bold", "italic", "removed", "*literal*", "code", "link"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("paragraph text %q does not contain %q", plain, want)
		}
	}
	var bold, italic, strike, code, link bool
	for _, inline := range paragraph.inline {
		bold = bold || inline.bold
		italic = italic || inline.italic
		strike = strike || inline.strike
		code = code || inline.kind == markdownInlineCode
		link = link || inline.url == "https://example.com"
	}
	if !bold || !italic || !strike || !code || !link {
		t.Fatalf("inline styles missing: bold=%v italic=%v strike=%v code=%v link=%v", bold, italic, strike, code, link)
	}
	if document.blocks[2].kind != markdownBlockquote || len(document.blocks[2].children) != 2 {
		t.Fatalf("blockquote was not preserved: %#v", document.blocks[2])
	}
	if document.blocks[3].kind != markdownBlockList || !document.blocks[3].list.ordered || len(document.blocks[3].list.items) != 2 {
		t.Fatalf("ordered list was not preserved: %#v", document.blocks[3])
	}
	if document.blocks[4].kind != markdownBlockTable || len(document.blocks[4].table.columns) != 2 || len(document.blocks[4].table.rows) != 1 {
		t.Fatalf("table was not preserved: %#v", document.blocks[4])
	}
	if document.blocks[5].kind != markdownCodeBlock || document.blocks[5].language != "go" || document.blocks[5].code != "package main" {
		t.Fatalf("fenced code was not preserved: %#v", document.blocks[5])
	}
	if document.blocks[6].kind != markdownCodeBlock || document.blocks[6].code != "indented code" {
		t.Fatalf("indented code was not preserved: %#v", document.blocks[6])
	}
}

func TestMarkdownSafetyAndImagePlaceholder(t *testing.T) {
	document := parseMarkdownDocument(`Text <script>alert(1)</script> ![diagram](https://example.com/image.png) [unsafe](javascript:alert(1))`)
	if len(document.blocks) != 1 {
		t.Fatalf("block count = %d, want 1", len(document.blocks))
	}
	paragraph := document.blocks[0]
	plain := plainMarkdownText(paragraph.inline)
	if !strings.Contains(plain, "<script>alert(1)</script>") {
		t.Fatalf("raw HTML was not kept as literal text: %q", plain)
	}
	if !strings.Contains(plain, "[图片未加载：diagram]") {
		t.Fatalf("image placeholder missing: %q", plain)
	}
	for _, inline := range paragraph.inline {
		if strings.Contains(inline.text, "unsafe") && inline.url != "" {
			t.Fatalf("unsafe URL became clickable: %#v", inline)
		}
	}
	if got := safeMarkdownURL("javascript:alert(1)"); got != "" {
		t.Fatalf("unsafe URL = %q, want empty", got)
	}
}

func TestMarkdownInlineCodeUsesCodeSurfaceContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := newPalette(dark)
		span := markdownSpan(markdownInline{kind: markdownInlineCode, text: "settings_controller.go"}, p)
		if span.Color != p.codeText {
			t.Fatalf("dark=%v inline code color = %#v, want code text %#v", dark, span.Color, p.codeText)
		}
		if span.Background != p.code {
			t.Fatalf("dark=%v inline code background = %#v, want code surface %#v", dark, span.Background, p.code)
		}
	}
}

func TestMarkdownLinkDoesNotOpenUntilUserAction(t *testing.T) {
	app := Open(nil, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{{
		ID: "markdown-link", Title: "Markdown", Messages: []appmodel.Message{{
			ID: "message", Role: "assistant", Text: "See [the guide](https://example.com/guide)",
		}},
	}}}})
	t.Cleanup(func() { _ = app.Close() })
	tester := ui.NewTester(app.view, 900, 640)
	if got := tester.OpenedURLs(); len(got) != 0 {
		t.Fatalf("rendering opened URLs: %v", got)
	}
	if err := tester.Click("the guide"); err != nil {
		t.Fatalf("click link: %v", err)
	}
	if got := tester.OpenedURLs(); len(got) != 1 || got[0] != "https://example.com/guide" {
		t.Fatalf("clicked URLs = %v", got)
	}
}

func TestNativeMarkdownFeaturesRenderInDesktopView(t *testing.T) {
	source := "# Heading\n\n- first\n  - nested\n\n> quote\n\n| Name | Count |\n| :--- | ---: |\n| Ada | 2 |\n\n```go\npackage main\n```\n\n![diagram](https://example.com/image.png)"
	app := Open(nil, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{{
		ID: "markdown-view", Title: "Markdown", Messages: []appmodel.Message{{
			ID: "message", Role: "assistant", Text: source,
		}},
	}}}})
	t.Cleanup(func() { _ = app.Close() })
	tester := ui.NewTester(app.view, 900, 720)
	for _, want := range []string{"Heading", "first", "nested", "quote", "Name", "Count", "Ada", "package main", "图片未加载"} {
		if !tester.HasText(want) {
			t.Fatalf("native Markdown render missing %q: %v", want, tester.Texts())
		}
	}
	if got := tester.OpenedURLs(); len(got) != 0 {
		t.Fatalf("rendering Markdown opened URLs: %v", got)
	}
}

func TestMarkdownAsyncResultCannotReplaceNewerSource(t *testing.T) {
	app := Open(nil, OpenOptions{})
	t.Cleanup(func() { _ = app.Close() })
	app.SetDispatch(func(fn func()) { fn() })
	_, ready := app.markdownDocumentForRender("message", "old **value**")
	if ready {
		t.Fatal("first dispatched parse completed on the UI path")
	}
	_, ready = app.markdownDocumentForRender("message", "new _value_")
	if ready {
		t.Fatal("new dispatched parse completed on the UI path")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		app.mu.RLock()
		cached := app.markdownCache["message"]
		pending := app.markdownPending["message"]
		app.mu.RUnlock()
		if cached.ready && cached.text == "new _value_" {
			return
		}
		if !pending && cached.text != "new _value_" {
			t.Fatalf("stale cache state: text=%q pending=%v", cached.text, pending)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the newest Markdown parse")
}

func TestMarkdownAppendKeepsParsedBodyWhileCompletionWaits(t *testing.T) {
	const initial = "已排版 **正文**。\n\n尾部令牌"
	app := Open(nil, OpenOptions{})
	t.Cleanup(func() { _ = app.Close() })
	source := initial
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(300).Children(func() {
			app.renderTextBlocks(c, newPalette(true), "append", source)
		})
	}, 320, 300)
	completed := make(chan func(), 4)
	app.SetDispatch(func(fn func()) { completed <- fn })
	source += "\n\n第一段新内容"
	tester.Frame()
	if !tester.HasText("已排版 正文。") || tester.HasText("**正文**") {
		t.Fatalf("pending append replaced parsed body with raw Markdown: %v", tester.Texts())
	}
	var finish func()
	select {
	case finish = <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("first append was not parsed")
	}
	// The source advances before the first completion reaches the UI queue.
	source += "\n\n第二段新内容"
	tester.Frame()
	finish()
	tester.Frame()
	if !tester.HasText("第一段新内容") || tester.HasText("第二段新内容") {
		t.Fatalf("valid parsed prefix was not published while newest parse waits: %v", tester.Texts())
	}
	select {
	case finish = <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("latest append was not parsed")
	}
	finish()
	tester.Frame()
	if !tester.HasText("第二段新内容") {
		t.Fatalf("latest completion is absent: %v", tester.Texts())
	}
	// A replacement cannot reuse the old message's parsed prefix.
	source = "完全替换的正文"
	tester.Frame()
	if tester.HasText("已排版 正文。") || !tester.HasText(source) {
		t.Fatalf("replacement retained an unrelated parsed body: %v", tester.Texts())
	}
	select {
	case finish = <-completed:
		finish()
	case <-time.After(2 * time.Second):
		t.Fatal("replacement was not parsed")
	}
}

func TestMarkdownReplacementWithSharedPrefixClearsParsedBody(t *testing.T) {
	const initial = "已排版 **正文**。"
	app := Open(nil, OpenOptions{})
	t.Cleanup(func() { _ = app.Close() })
	source := initial
	tester := ui.NewTester(func(c *ui.Context) {
		app.renderTextBlocks(c, newPalette(true), "replace-prefix", source)
	}, 320, 240)
	completed := make(chan func(), 4)
	app.SetDispatch(func(fn func()) { completed <- fn })
	source = initial + "\n\n第一条续写"
	tester.Frame()
	source = initial + "\n\n已替换的续写"
	tester.Frame()
	if !tester.HasText("**正文**") || !tester.HasText("已替换的续写") {
		t.Fatalf("replacement reused the old parsed body: %v", tester.Texts())
	}
	for range 2 {
		select {
		case finish := <-completed:
			finish()
		case <-time.After(2 * time.Second):
			t.Fatal("replacement completion is absent")
		}
	}
}

func TestEmptyMessageInvalidatesParsedBody(t *testing.T) {
	const initial = "已排版 **正文**。"
	app := Open(nil, OpenOptions{})
	t.Cleanup(func() { _ = app.Close() })
	message := appmodel.Message{ID: "reuse", Role: "assistant", Text: initial}
	tester := ui.NewTester(func(c *ui.Context) {
		app.renderMessage(c, newPalette(true), "session", "reuse", message)
	}, 400, 240)
	completed := make(chan func(), 4)
	app.SetDispatch(func(fn func()) { completed <- fn })
	message.Text = ""
	tester.Frame()
	if !tester.HasText("正在生成") {
		t.Fatal("empty message did not show the waiting state")
	}
	message.Text = initial + "\n\n重建后的正文"
	tester.Frame()
	if !tester.HasText("**正文**") || !tester.HasText("重建后的正文") {
		t.Fatalf("empty transition retained the old parsed body: %v", tester.Texts())
	}
	select {
	case finish := <-completed:
		finish()
	case <-time.After(2 * time.Second):
		t.Fatal("rebuilt message completion is absent")
	}
}

func TestEmptyMessageRejectsEarlierCompatibleParse(t *testing.T) {
	const initial = "已排版 **正文**。"
	completed := make(chan func(), 4)
	app := Open(nil, OpenOptions{Dispatch: func(fn func()) { completed <- fn }})
	t.Cleanup(func() { _ = app.Close() })
	message := appmodel.Message{ID: "reuse", Role: "assistant", Text: initial}
	tester := ui.NewTester(func(c *ui.Context) {
		app.renderMessage(c, newPalette(true), "session", "reuse", message)
	}, 400, 240)
	var oldCompletion func()
	select {
	case oldCompletion = <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("original parse completion is absent")
	}
	message.Text = ""
	tester.Frame()
	message.Text = initial + "\n\n重建后的正文"
	tester.Frame()
	oldCompletion()
	tester.Frame()
	if !tester.HasText("**正文**") || !tester.HasText("重建后的正文") {
		t.Fatalf("a completion from before reset published an old prefix: %v", tester.Texts())
	}
	select {
	case finish := <-completed:
		finish()
	case <-time.After(2 * time.Second):
		t.Fatal("rebuilt parse completion is absent")
	}
}
