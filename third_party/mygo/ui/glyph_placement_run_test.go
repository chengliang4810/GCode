package ui

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

func TestQuarterRunCallbackReusesExistingDrawType(t *testing.T) {
	actual := reflect.TypeFor[glyphPlacementPaint]()
	want := reflect.TypeFor[func(*Painter, Rect)]()
	if actual != want || actual.Name() != "" {
		t.Fatalf("callback retained a distinct named/type identity: got %v (%q), want %v", actual, actual.Name(), want)
	}
}

func TestQuarterRunDispatchesOncePerActualTextRun(t *testing.T) {
	if !glyphPlacementTestSupported() {
		t.Skip("Windows64 candidate")
	}
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			calls := 0
			tt := coreNewTester(func(c *context) {
				coreBox(c).Fill().Background(RGB(39, 39, 42)).Children(func() {
					e := coreText(c, "AV To Tab02.txt").Font("Microsoft YaHei UI").FontSize(10.5).
						FontWeight(400).FontFeatures("kern=0").FixedLineHeight(17).Absolute().Left(20.2).Top(10)
					WithGlyphPlacement(wrapElement(e), GlyphPlacementWindowsClearTypeQuarter)
					base := reflect.ValueOf(e.ts.placeGlyph)
					// This instrumentation also compiles against the previous7arg
					// callback, so the old per-glyph dispatch is a real before failure.
					e.ts.placeGlyph = reflect.MakeFunc(base.Type(), func(args []reflect.Value) []reflect.Value {
						calls++
						return base.Call(args)
					}).Interface().(glyphPlacementPaint)
				})
			}, 260, 60)
			defer tt.rt.close()
			tt.SetScale(scale)
			calls = 0
			tt.Frame()
			if calls != 1 {
				t.Fatalf("actual run invoked callback%d times; want1 per text run", calls)
			}
		})
	}
}

func TestQuarterRunUnsupportedTextPathsPaintNative(t *testing.T) {
	if !glyphPlacementTestSupported() {
		t.Skip("Windows64 candidate")
	}
	for _, scale := range []float32{1, 1.25, 2} {
		for _, light := range []bool{false, true} {
			for _, path := range []string{"font-only-spans", "color-spans", "inline", "content", "input", "painter-text", "painter-rich"} {
				t.Run(fmt.Sprintf("scale%v/light%v/%s", scale, light, path), func(t *testing.T) {
					quarter := false
					bg, ink := RGB(39, 39, 42), RGB(225, 225, 227)
					if light {
						bg, ink = RGB(244, 244, 245), RGB(35, 35, 38)
					}
					draft := "AV To Tab02.txt"
					var probe *node
					tt := coreNewTester(func(c *context) {
						root := coreBox(c).Fill().Background(bg).Font("Microsoft YaHei UI").FontSize(10.5).
							FontWeight(400).FontFeatures("kern=0").FixedLineHeight(17).TextColor(ink)
						if quarter {
							WithGlyphPlacement(wrapElement(root), GlyphPlacementWindowsClearTypeQuarter)
						}
						root.Children(func() {
							makeContext(c).Key("target")
							var e *node
							switch path {
							case "font-only-spans":
								e = coreRichText(c, Span{Text: draft, Font: "Microsoft YaHei UI"})
							case "color-spans":
								e = coreRichText(c, Span{Text: "AV To", Color: RGB(220, 80, 35), Underline: true}, Span{Text: " Tab02.txt", Weight: 500})
							case "inline":
								e = coreText(c, "").Children(func() { coreText(c, "AV To "); coreText(c, "Tab02.txt").FontWeight(500) })
							case "content":
								e = coreText(c, draft).ContentTransition(ContentTransition{})
							case "input":
								e = coreTextInputBase(c, &draft)
							case "painter-text":
								e = coreBox(c).Draw(func(p *Painter, r Rect) { p.Text(r.X, r.Y, draft, 10.5, ink) })
							case "painter-rich":
								e = coreBox(c).Draw(func(p *Painter, r Rect) {
									p.RichText(r.X, r.Y, 220, Span{Text: draft, Size: 10.5, Color: ink, Underline: true})
								})
							}
							probe = e.Absolute().Left(20.2).Top(10).Size(220, 40)
						})
					}, 280, 80)
					defer tt.rt.close()
					tt.SetScale(scale)
					tt.Frame()
					if path == "font-only-spans" {
						var sp spanPaint
						if probe.spans == nil || probe.paintSpans(&sp) != nil {
							t.Fatal("control must have font-only spans with nil spanPaint")
						}
					}
					native := bytes.Clone(tt.Image().Pix)
					quarter = true
					tt.Frame()
					if !bytes.Equal(native, tt.Image().Pix) {
						t.Fatal("narrowed unsupported path did not retain exact native frame")
					}
				})
			}
		}
	}
}

func quarterRunNonGlyphOps(tt *Tester) []scene.Op {
	var ops []scene.Op
	for _, op := range tt.h.last.Ops {
		if op.Kind != scene.OpGlyphs {
			ops = append(ops, op)
		}
	}
	return ops
}

func TestQuarterRunPreservesActualSelectionBackgroundAndDecorations(t *testing.T) {
	if !glyphPlacementTestSupported() {
		t.Skip("Windows64 candidate")
	}
	for _, scale := range []float32{1, 1.25, 2} {
		for _, light := range []bool{false, true} {
			t.Run(fmt.Sprintf("scale%v/light%v", scale, light), func(t *testing.T) {
				quarter := false
				var current *node
				bg, ink := RGB(39, 39, 42), RGB(230, 230, 232)
				if light {
					bg, ink = RGB(244, 244, 245), RGB(25, 25, 28)
				}
				textBG, selected, decoration := RGB(29, 45, 68), RGB(70, 105, 180), RGB(210, 85, 45)
				tt := coreNewTester(func(c *context) {
					coreBox(c).Fill().Background(bg).Children(func() {
						current = coreText(c, "AV To Tab02.txt").Key("selectable").Font("Microsoft YaHei UI").FontSize(10.5).
							FontWeight(400).FontFeatures("kern=0").FixedLineHeight(17).TextColor(ink).TextBackground(textBG).
							SelectionColor(selected).Underline().Strikethrough().DecorationColor(decoration).DecorationThickness(1.25).
							Absolute().Left(20.2).Top(12).Selectable()
						if quarter {
							WithGlyphPlacement(wrapElement(current), GlyphPlacementWindowsClearTypeQuarter)
						}
					})
				}, 280, 70)
				defer tt.rt.close()
				tt.SetScale(scale)
				tt.Frame()
				point := func(index int) (float32, float32) {
					ed := current.st.editor
					x, y, h := ed.layout.Caret(index)
					return current.LocalToWindow(ed.originX+x, ed.originY+y+h/2)
				}
				tt.Press(point(3))
				tt.Move(point(9))
				tt.Release(point(9))
				tt.Frame()
				a, b := current.st.editor.selection()
				if a == b || !tt.rt.textSelectionVisible(current.st) {
					t.Fatal("control has no actual visible text selection")
				}
				before := quarterRunNonGlyphOps(tt)
				counts := [3]int{}
				for _, op := range before {
					for i, want := range []Color{textBG, selected, decoration} {
						if op.Kind == scene.OpFill && op.Color == want.scene() {
							counts[i]++
						}
					}
				}
				if counts[0] != 1 || counts[1] == 0 || counts[2] < 2 {
					t.Fatalf("background/selection/deco control missing or duplicated: %v", counts)
				}
				quarter = true
				tt.Frame()
				if after := quarterRunNonGlyphOps(tt); !reflect.DeepEqual(before, after) {
					t.Fatal("optional foreground changed/doubled/reordered actual background, selection, decorations or clips")
				}
				if x, y := current.st.editor.selection(); x != a || y != b {
					t.Fatal("painting changed actual selection")
				}
			})
		}
	}
}

func TestQuarterRunContentTransitionKeepsOldColorAndLayout(t *testing.T) {
	if !glyphPlacementTestSupported() {
		t.Skip("Windows64 candidate")
	}
	for _, numeric := range []bool{false, true} {
		for _, scale := range []float32{1, 1.25, 2} {
			t.Run(fmt.Sprintf("numeric%v/scale%v", numeric, scale), func(t *testing.T) {
				quarter := false
				value := "120"
				ink := RGB(225, 50, 60)
				tt, now := clockTester(func(c *context) {
					coreBox(c).Fill().Background(RGB(39, 39, 42)).Children(func() {
						e := coreText(c, value).Key("changing").Absolute().Left(20.2).Top(10).Font("Microsoft YaHei UI").FontSize(10.5).
							TextColor(ink).ContentTransition(ContentTransition{Numeric: numeric, Animation: &Animation{Duration: time.Second, Ease: Linear}})
						if quarter {
							WithGlyphPlacement(wrapElement(e), GlyphPlacementWindowsClearTypeQuarter)
						}
					})
				}, 280, 70)
				defer tt.rt.close()
				tt.SetScale(scale)
				tt.Frame()
				value = "130"
				tt.Frame()
				*now = now.Add(450 * time.Millisecond)
				// contentStep captures oldColor when the text changes. Change
				// the current color later so old/current colors really differ.
				ink = RGB(40, 200, 120)
				tt.Frame()
				native := bytes.Clone(tt.Image().Pix)
				before := quarterRunNonGlyphOps(tt)
				colors := map[scene.Color]bool{}
				for _, glyph := range tt.h.last.Glyphs {
					if glyph.Color.A != 0 {
						c := glyph.Color
						c.A = 255
						colors[c] = true
					}
				}
				if !colors[RGB(225, 50, 60).scene()] || !colors[RGB(40, 200, 120).scene()] {
					t.Fatal("transition control did not retain both old/current colors")
				}
				quarter = true
				tt.Frame()
				if !bytes.Equal(native, tt.Image().Pix) || !reflect.DeepEqual(before, quarterRunNonGlyphOps(tt)) {
					t.Fatal("opt-in replaced transition's old color/layout or numeric bands")
				}
			})
		}
	}
}
