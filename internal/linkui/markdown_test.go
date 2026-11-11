package linkui

import (
	"strings"
	"testing"

	"gcode/internal/design"
	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func resolveTest(raw string) (Destination, bool) {
	switch {
	case strings.HasPrefix(raw, "https://"):
		return Destination{Raw: raw, Kind: linktarget.Web}, true
	case raw == "/workspace/docs/guide.md:9":
		return Destination{Raw: raw, Tooltip: raw, Label: "guide.md", Kind: linktarget.File, KnownFile: true, Local: true}, true
	}
	return Destination{}, false
}

func TestBlockPreservesLabelsStylesAndKnownFileCode(t *testing.T) {
	const source = "Read [**original label**](https://example.com/very/long/path), `/workspace/docs/guide.md:9`, and `ordinary_code()`."
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			th := design.Palette("rcode", dark)
			var opened string
			cfg := Config{Resolve: resolveTest, Internal: true, Open: func(raw string) { opened = raw }}
			options := markdown.Options{Block: func(c *ui.Context, document *parser.Document, index int) bool {
				return Block(c, document, index, cfg, nil)
			}}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(th.UI())
				c.Root().Background(c.Theme().Background)
				ui.Column(c).Fill().Padding(24).Children(func() { markdown.View(c, source, &options) })
			}, 420, 300)
			if !tt.HasText("original label") || !tt.HasText("guide.md") || !tt.HasText("ordinary_code()") {
				t.Fatalf("missing content: %v", tt.Texts())
			}
			if err := tt.Click("original label"); err != nil {
				t.Fatal(err)
			}
			if opened != "https://example.com/very/long/path" {
				t.Fatalf("link route=%q", opened)
			}
			if err := tt.Click("guide.md"); err != nil {
				t.Fatal(err)
			}
			if opened != "/workspace/docs/guide.md:9" {
				t.Fatalf("known file route=%q", opened)
			}
			writeEvidence(t, tt, map[bool]string{false: "light", true: "dark"}[dark]+"-markdown")
			r, _ := tt.Find("original label")
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			writeEvidence(t, tt, map[bool]string{false: "light", true: "dark"}[dark]+"-markdown-hover")
			tt.Press(r.X+1, r.Y+r.H/2)
			tt.Move(r.X+r.W-1, r.Y+r.H/2)
			tt.Release(r.X+r.W-1, r.Y+r.H/2)
			tt.Key(ui.Cmd, ui.KeyC)
			if tt.Clipboard() != "original label" {
				t.Fatalf("label selection copy=%q", tt.Clipboard())
			}
		})
	}
}

func TestBlockUsesExistingRendererForUnsupportedExtensionsAndOrdinaryCode(t *testing.T) {
	for _, source := range []string{
		"ordinary `code`",
		"[link](https://example.com) <b>HTML</b>",
		"[link](https://example.com) ![image](picture.png)",
		"[link](https://example.com) $x^2$",
		"[link](https://example.com) [^note]\n\n[^note]: Footnote",
		"[link](https://example.com) [local heading](#heading)",
	} {
		t.Run(source, func(t *testing.T) {
			document := parser.New(parser.Default | parser.ExtDollarMath)
			document.Set(source)
			handled := false
			ui.NewTester(func(c *ui.Context) {
				handled = Block(c, document, 1, Config{Resolve: resolveTest}, nil)
			}, 320, 180)
			if handled {
				t.Fatal("custom link renderer swallowed unsupported content")
			}
		})
	}
}

func TestBlockRunsInsideListAndKeepsHeadingGeometry(t *testing.T) {
	const source = "# [Heading](https://example.com/heading)\n\n- [Item](https://example.com/item)"
	cfg := Config{Resolve: resolveTest}
	handled := 0
	options := markdown.Options{Block: func(c *ui.Context, document *parser.Document, index int) bool {
		ok := Block(c, document, index, cfg, nil)
		if ok {
			handled++
		}
		return ok
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		markdown.View(c, source, &options)
	}, 360, 200)
	heading, h := tt.Find("Heading")
	item, i := tt.Find("Item")
	if !h || !i || handled < 2 || heading.H <= item.H {
		t.Fatalf("heading=%v item=%v handled=%d texts=%v", heading, item, handled, tt.Texts())
	}
}

func TestInlinePiecesKeepsNonFileCodeAndExplicitLabels(t *testing.T) {
	doc := parser.Parse("[visible **label**](https://example.com) `/workspace/docs/guide.md:9` `plain()`")
	th := markdown.DefaultTheme(design.Palette("rcode", false).UI())
	pieces, linked := inlinePieces(doc.Inlines(1), Config{Resolve: resolveTest}, th, ui.Span{Size: 14})
	if !linked {
		t.Fatal("links were not recognized")
	}
	var label string
	foundCode, foundFile, foundStrong := false, false, false
	for _, pc := range pieces {
		for _, span := range pc.spans {
			label += span.Text
			if span.Text == "plain()" {
				foundCode = span.Font == th.CodeFont && span.Background == th.CodeBackground && pc.destination == nil
			}
			if span.Text == "guide.md" {
				foundFile = pc.destination != nil && pc.destination.Kind == linktarget.File && span.Background == (ui.Color{})
			}
			if span.Text == "label" {
				foundStrong = span.Weight >= 600
			}
		}
	}
	if label != "visible label guide.md plain()" || !foundCode || !foundFile || !foundStrong {
		t.Fatalf("label=%q code=%v file=%v strong=%v pieces=%+v", label, foundCode, foundFile, foundStrong, pieces)
	}
}

func TestGeneratedAbsoluteFileCodeResolvesWithoutProjectIndex(t *testing.T) {
	for _, raw := range []string{
		"/Users/example/.gcode/workspaces/task/巴中市城乡供水一体化管理平台_平台与设备清单.xlsx",
		"/Users/example/My Reports/设备清单.xlsx",
		`C:\Users\example\Reports\设备清单.xlsx`,
		"file:///Users/example/Reports/report.xlsx",
	} {
		t.Run(raw, func(t *testing.T) {
			cfg := Config{
				Resolve: func(value string) (Destination, bool) {
					target, err := linktarget.Parse(value)
					return Destination{Raw: value, Tooltip: target.Path, Label: "generated.xlsx", Kind: target.Kind, Local: true}, err == nil
				},
			}
			document := parser.Parse("**`" + raw + "`**")
			th := markdown.DefaultTheme(design.Palette("rcode", false).UI())
			pieces, linked := inlinePieces(document.Inlines(1), cfg, th, ui.Span{Size: 14})
			if !linked || len(pieces) != 1 || pieces[0].destination == nil {
				t.Fatalf("generated absolute file is not a link: %+v", pieces)
			}
			pc := pieces[0]
			if pc.destination.Raw != raw || pc.destination.KnownFile || pc.copySource != raw ||
				len(pc.spans) != 1 || pc.spans[0].Text != "generated.xlsx" {
				t.Fatalf("generated file lost its original target or label: %+v", pc)
			}
		})
	}
}

func TestUnknownRelativeCodeKeepsCodeStyle(t *testing.T) {
	th := markdown.DefaultTheme(design.Palette("rcode", false).UI())
	cfg := Config{Resolve: func(raw string) (Destination, bool) {
		target, err := linktarget.Parse(raw)
		return Destination{Raw: raw, Kind: target.Kind}, err == nil
	}}
	for _, raw := range []string{"report.xlsx", "reports/report.xlsx", "plain()", "../report.xlsx", `C:report.xlsx`, `\\server\share\report.xlsx`} {
		t.Run(raw, func(t *testing.T) {
			doc := parser.Parse("`" + raw + "`")
			_, linked := inlinePieces(doc.Inlines(1), cfg, th, ui.Span{Size: 14})
			if linked {
				t.Fatalf("ordinary or ambiguous code became a link: %q", raw)
			}
		})
	}
}
