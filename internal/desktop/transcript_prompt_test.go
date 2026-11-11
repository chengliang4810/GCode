package desktop

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestTranscriptInlineObjectsCopySource(t *testing.T) {
	review := (contracts.PromptReference{Kind: "command", ID: "review", Label: "review"}).Markdown()
	docs := (contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}).Markdown()
	for _, scene := range []struct{ name, role, source, label, plain string }{
		{"references", "user", review + " 请检查 " + docs, "review 请检查 文档\n", "review 请检查 文档"},
		{"formula", "assistant", `公式 \(x^2\) 与 [说明](https://example.invalid)。`, `公式 \(x^2\) 与 [说明](https://example.invalid)。`, `公式 \(x^2\) 与 说明。`},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a := linkTestApp(t)
			a.state.Settings.Appearance.RenderMath = true
			message := contracts.Message{ID: "copy-inline", SessionID: a.sessionID, Role: scene.role,
				Blocks: []contracts.Block{{ID: "body", Kind: "text", Text: scene.source}}}
			tt := linkMessageTester(t, a, message, false)
			row, ok := tt.Find(scene.label)
			if !ok {
				t.Fatalf("inline paragraph is missing: %v", tt.Texts())
			}
			tt.ClickAt(row.X+2, row.Y+row.H/2)
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Key(ui.Cmd, ui.KeyC)
			if !strings.HasPrefix(tt.Clipboard(), scene.plain) || strings.ContainsAny(tt.Clipboard(), "\ufffc\u2060") {
				t.Fatalf("decorative inline objects leaked into plain copy: %q", tt.Clipboard())
			}
			tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
			if !strings.HasPrefix(tt.Clipboard(), scene.source) {
				t.Fatalf("Markdown copy lost its original source: %q", tt.Clipboard())
			}
		})
	}
}

func TestPromptReferenceIconAlignsWithLabelInk(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, size := range []float32{14, 20} {
			for _, ref := range []contracts.PromptReference{
				{Kind: "command", ID: "review", Label: "review"},
				{Kind: "skill", ID: "docs", Label: "文档"},
			} {
				t.Run(fmt.Sprintf("dark=%t/size=%.0f/%s", dark, size, ref.Kind), func(t *testing.T) {
					a := testApp(t, "default")
					a.palette = design.Palette("rcode", dark)
					tt := ui.NewTester(func(c *ui.Context) {
						theme := a.palette.UI()
						theme.FontSize = size
						c.SetTheme(theme)
						c.Root().Background(a.palette.Background)
						a.prepareTranscriptMarkdown(c)
						ui.Column(c).Padding(24).Children(func() { markdown.View(c, ref.Markdown(), &a.userMD) })
					}, 400, 120)
					row, ok := tt.Find(ref.Label + "\n")
					if !ok {
						t.Fatal("reference paragraph is missing")
					}
					bounds := func(left, right float32) image.Rectangle {
						b := image.Rectangle{}
						for y := int(row.Y); y < int(row.Y+row.H); y++ {
							for x := int(left); x < int(right); x++ {
								p := tt.Image().RGBAAt(x, y)
								dr := int(p.R) - int(a.palette.Background.R)
								dg := int(p.G) - int(a.palette.Background.G)
								db := int(p.B) - int(a.palette.Background.B)
								if p.A > 0 && dr*dr+dg*dg+db*db > 900 {
									b = b.Union(image.Rect(x, y, x+1, y+1))
								}
							}
						}
						return b
					}
					icon, label := bounds(row.X, row.X+size), bounds(row.X+size+3, row.X+row.W)
					if icon.Empty() || label.Empty() {
						t.Fatalf("reference has no icon or label ink: icon=%v label=%v", icon, label)
					}
					iconCenter := float64(icon.Min.Y+icon.Max.Y) / 2
					labelCenter := float64(label.Min.Y+label.Max.Y) / 2
					if math.Abs(iconCenter-labelCenter) > 1.5 {
						t.Fatalf("icon and label are misaligned: icon=%v label=%v, center delta=%.1fpx", icon, label, iconCenter-labelCenter)
					}
					if directory := os.Getenv("RCODE_PROMPT_EVIDENCE"); directory != "" {
						if err := os.MkdirAll(directory, 0755); err != nil {
							t.Fatal(err)
						}
						file, err := os.Create(filepath.Join(directory, fmt.Sprintf("%t-%.0f-%s.png", dark, size, ref.Kind)))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(file, tt.Image())
						closeErr := file.Close()
						if err != nil || closeErr != nil {
							t.Fatalf("save rendered reference: %v %v", err, closeErr)
						}
					}
				})
			}
		}
	}
}

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

func TestPromptMarkdownDrawObjectsPaintNonInteractiveReferenceIcons(t *testing.T) {
	a := testApp(t, "default")
	a.palette = design.Palette("rcode", false)
	for _, reference := range []contracts.PromptReference{
		{Kind: "skill", ID: "docs", Label: "文档"},
		{Kind: "command", ID: "goal", Label: "目标"},
	} {
		t.Run(reference.Kind, func(t *testing.T) {
			source := reference.Markdown()
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				c.Root().Background(a.palette.Background)
				a.prepareTranscriptMarkdown(c)
				ui.Column(c).Padding(24).Children(func() {
					markdown.View(c, source, &a.userMD)
					ui.Button(c, "After reference")
				})
			}, 600, 180)
			r, ok := tt.Find(reference.Label + "\n")
			if !ok {
				t.Fatalf("reference label %q is missing: %v", reference.Label, tt.Texts())
			}
			ink := 0
			image := tt.Image()
			left := max(0, int(r.X))
			right := min(image.Bounds().Max.X, int(r.X+a.userMD.Theme.FontSize+3))
			top := max(0, int(r.Y))
			bottom := min(image.Bounds().Max.Y, int(r.Y+r.H))
			for y := top; y < bottom; y++ {
				for x := left; x < right; x++ {
					pixel := image.RGBAAt(x, y)
					if pixel.A > 0 && (pixel.R != a.palette.Background.R || pixel.G != a.palette.Background.G || pixel.B != a.palette.Background.B) {
						ink++
					}
				}
			}
			if ink == 0 {
				t.Fatalf("reference icon before %q did not paint pixels", reference.Label)
			}
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			if tt.Cursor() == ui.CursorPointer {
				t.Fatalf("reference %q became a pointer link", reference.Label)
			}
			tt.ClickAt(r.X+5, r.Y+r.H/2)
			if len(tt.OpenedURLs()) != 0 {
				t.Fatalf("clicking a reference opened URLs: %v", tt.OpenedURLs())
			}
			tt.Key(0, ui.KeyTab)
			if !tt.Focused("After reference") {
				t.Fatalf("reference %q added a keyboard link stop", reference.Label)
			}
		})
	}
}
