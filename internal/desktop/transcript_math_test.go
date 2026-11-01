package desktop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

const mathMessageSource = "An inline formula \\(x^2+\\alpha\\) appears inside a real Markdown paragraph.\n\n$$\n\\frac{x^2 + \\alpha}{\\sqrt{y}} = \\sum_{i=1}^{n} i\n$$\n\n```math\n\\int_0^\\infty e^{-x}\\,dx=1\n```"

func mathMessageTester(t *testing.T, a *App, width int, dark bool) *ui.Tester {
	t.Helper()
	a.palette = design.Palette("rcode", dark)
	message := contracts.Message{ID: "math-message", Role: "assistant"}
	block := contracts.Block{ID: "math-text", Kind: "text", Text: mathMessageSource}
	return ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		c.Root().Background(a.palette.Background).TextColor(a.palette.Text).Font("Geist")
		a.prepareTranscriptMarkdown(c)
		ui.Column(c).Fill().Padding(24).Gap(14).Children(func() { a.transcriptBlock(c, message, block) })
	}, width, 420)
}

func TestMathFontChangesActualMessageGlyphsAndSize(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.Appearance.RenderMath = true
			a.state.Settings.Appearance.MathScale = 100
			seen := make(map[[32]byte]string)
			for _, family := range []string{"Fira Math", "STIX Two", "Computer Modern", "Libertinus", "Palatino", "Euler"} {
				a.state.Settings.Appearance.MathFont = family
				tt := mathMessageTester(t, a, 680, dark)
				if !tt.HasText("appears inside a real Markdown paragraph.") {
					t.Fatalf("%s dropped surrounding prose: %v", family, tt.Texts())
				}
				var encoded bytes.Buffer
				if err := png.Encode(&encoded, tt.Image()); err != nil {
					t.Fatal(err)
				}
				hash := sha256.Sum256(encoded.Bytes())
				if previous := seen[hash]; previous != "" {
					t.Fatalf("%s reused %s rendering", family, previous)
				}
				seen[hash] = family
				formula := a.math.cache.entries[mathViewKey{source: `\frac{x^2 + \alpha}{\sqrt{y}} = \sum_{i=1}^{n} i`, family: family}]
				if formula == nil || formula.result.svg == nil || formula.result.err != nil {
					t.Fatalf("%s did not render the actual message expression: %+v", family, formula)
				}
				if directory := os.Getenv("RCODE_MATH_EVIDENCE"); directory != "" {
					if err := os.MkdirAll(directory, 0755); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(directory, map[bool]string{false: "light-", true: "dark-"}[dark]+family+".png")
					if err := os.WriteFile(path, encoded.Bytes(), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			a.state.Settings.Appearance.MathScale = 150
			tt := mathMessageTester(t, a, 420, dark)
			large, ok := tt.Find(`\frac{x^2 + \alpha}{\sqrt{y}} = \sum_{i=1}^{n} i`)
			if !ok || large.H <= 0 || large.W <= 0 {
				t.Fatalf("large formula has no geometry: %+v", large)
			}
			a.state.Settings.Appearance.MathScale = 100
			tt = mathMessageTester(t, a, 420, dark)
			normal, ok := tt.Find(`\frac{x^2 + \alpha}{\sqrt{y}} = \sum_{i=1}^{n} i`)
			if !ok || normal.W >= large.W || normal.H >= large.H {
				t.Fatalf("math size did not affect real formula: normal=%+v large=%+v", normal, large)
			}
		})
	}
}

func TestMathMarkdownPreservesNativeFallbackAndLinkedObjects(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.RenderMath = true
	a.state.Settings.Appearance.MathFont = "STIX Two"
	for _, source := range []string{`Formula \(x^2\) and ![picture](file:///fixture.png)`, `Formula \(x^2\) and <br>`, `Formula \(x^2\) and [^note]` + "\n\n[^note]: note"} {
		document := parser.Parse(source)
		for index := range document.Children(0) {
			if document.Block(index).Kind != parser.Paragraph {
				continue
			}
			ui.NewTester(func(c *ui.Context) {
				a.prepareTranscriptMarkdown(c)
				if a.mathMarkdownBlock(c, document, index) {
					t.Fatalf("custom formula renderer consumed native-only content in %q", source)
				}
			}, 600, 180)
			break
		}
	}
	source := `[cost \(x^2\)](https://example.invalid)`
	tt := ui.NewTester(func(c *ui.Context) {
		a.prepareTranscriptMarkdown(c)
		c.Root().Font("Geist")
		markdown.View(c, source, &a.md)
	}, 600, 180)
	if !tt.HasText("cost ") {
		t.Fatalf("link prose disappeared: %v", tt.Texts())
	}
	if result := a.math.cache.entries[mathViewKey{source: "x^2", family: "STIX Two"}]; result == nil || result.result.svg == nil {
		t.Fatal("linked formula did not retain its inline object")
	}
	a.state.Settings.Appearance.RenderMath = false
	ui.NewTester(func(c *ui.Context) {
		a.prepareTranscriptMarkdown(c)
		if a.md.Math != nil || a.md.Block == nil || a.md.Code != nil || a.userMD.Math != nil {
			t.Fatal("Render math off retained formulas or disabled other Markdown block renderers")
		}
		markdown.View(c, source, &a.md)
	}, 600, 180)
}

func TestMathViewCacheWorkerCompletesAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := newMathViewCache(ctx, true)
	key := mathViewKey{source: `x^2`, family: "Fira Math"}
	_ = cache.lookup(key, ui.Services{})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		result := cache.lookup(key, ui.Services{})
		if result.svg != nil {
			cancel()
			select {
			case <-cache.done:
			case <-time.After(time.Second):
				t.Fatal("formula worker did not stop after cancellation")
			}
			return
		}
		if result.err != nil {
			t.Fatal(result.err)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("formula worker did not publish an immutable result")
}

func TestMathOversizedNativeImageShowsSource(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.RenderMath = true
	a.state.Settings.Appearance.MathFont = "Fira Math"
	source := strings.Repeat("x", 1000)
	tt := ui.NewTester(func(c *ui.Context) {
		a.prepareTranscriptMarkdown(c)
		result := a.mathFormula(c, source)
		if result.svg != nil || result.err == nil {
			t.Fatal("oversized formula retained an invisible SVG")
		}
		a.mathView(c, source)
	}, 600, 200)
	if !tt.HasText(source) {
		t.Fatal("oversized formula lost its source fallback")
	}
}
