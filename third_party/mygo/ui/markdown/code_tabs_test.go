package markdown

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A Go code block shows its tabs at the editor's four-column stops and
// copies its source back with the tabs.
func TestCodeBlockTabsShowAndCopy(t *testing.T) {
	src := "```go\nfunc main() {\n\tif x {\n\t\tpanic(1)\n\t}\n}\n```\n"
	tt := ui.NewTester(func(c *ui.Context) { View(c, src, nil) }, 700, 400)
	if !tt.HasText("    if x {") || !tt.HasText("        panic(1)") {
		t.Fatalf("tabs not expanded at four columns: %q", tt.Texts())
	}
	if tt.HasText("\tif x {") {
		t.Fatalf("the text kept a tab: %q", tt.Texts())
	}
	r, ok := tt.Find("func main() {\n    if x {\n        panic(1)\n    }\n}")
	if !ok {
		t.Fatal("code block missing")
	}
	tt.ClickAt(r.X+2, r.Y+r.H/2)
	tt.Command("selectAll")
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if got, want := tt.Clipboard(), strings.TrimSuffix(src, "\n"); got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// A second code block beside the first keeps its own language's tab size.
func TestCodeBlockTabsUseTheLanguageSize(t *testing.T) {
	src := "```js\nif (x) {\n\tgo();\n}\n```\n"
	tt := ui.NewTester(func(c *ui.Context) { View(c, src, nil) }, 700, 300)
	if !tt.HasText("if (x) {\n  go();\n}") {
		t.Fatalf("JavaScript tabs not expanded at two columns: %q", tt.Texts())
	}
}

// A selection over the spaces showing a tab copies the source's tab, and
// one ending inside them takes the whole tab.
func TestReviewCopyMarkdownCodeTabs(t *testing.T) {
	src := "```go\n\tif x {\n\t\tpanic(1)\n\t}\n```\n"
	_, get := rvView(&src, nil)
	st := get()
	k := rvElemOf(t, st, 1)
	line, _ := st.doc.Line(1, 0)
	disp, _ := codeDisplay(line, codeTabSize("go"))
	if disp != "    if x {" {
		t.Fatalf("display line %q", disp)
	}
	part := rvPart(st, k, disp, 2, 6) // inside the tab, through "if"
	if got, want := st.copyMarkdown([]ui.SelectedText{part}), "\tif"; got != want {
		t.Errorf("copied Markdown %q, want %q", got, want)
	}
	if got, want := st.copyText([]ui.SelectedText{part}), "\tif"; got != want {
		t.Errorf("copied text %q, want %q", got, want)
	}
	part = rvPart(st, k, disp, 0, 2) // only spaces of the tab
	if got, want := st.copyMarkdown([]ui.SelectedText{part}), "\t"; got != want {
		t.Errorf("copied part of a tab %q, want %q", got, want)
	}
}
