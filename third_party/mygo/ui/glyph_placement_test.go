package ui

import (
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/text"
)

func glyphPlacementTestSupported() bool {
	return runtime.GOOS == "windows" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")
}

func TestGlyphPlacementRepaintLayoutAndRestoration(t *testing.T) {
	if !glyphPlacementTestSupported() {
		t.Skip("Windows ClearType candidate")
	}
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			policy := GlyphPlacementNative
			apply := true
			var current *node
			tt := coreNewTester(func(c *context) {
				coreBox(c).Fill().Background(RGB(39, 39, 42)).Children(func() {
					current = coreText(c, "AV To Tab02.txt 中文 e\u0301").Key("run").Label("run").Font("Microsoft YaHei UI").FontSize(10.5).
						FontWeight(400).FontFeatures("kern=0").FixedLineHeight(17).Absolute().Left(20.2).Top(10)
					if apply {
						WithGlyphPlacement(wrapElement(current), policy)
					}
					coreText(c, "unrelated").Label("sibling").Absolute().Left(20).Top(45)
				})
			}, 400, 90)
			tt.SetScale(scale)
			tt.Frame()
			bounds, _ := tt.Find("run")
			params, layout := current.textParams(0), current.tl
			glyphs := append([]text.Glyph(nil), layout.Lines[0].Glyphs...)
			native := bytes.Clone(tt.Image().Pix)
			policy = GlyphPlacementWindowsClearTypeQuarter
			tt.Frame()
			quarter := bytes.Clone(tt.Image().Pix)
			if current.resolvedText().placeGlyph == nil {
				t.Fatal("public opt-in was not wired into actual text paint")
			}
			if current.textParams(0) != params || current.tl != layout || !reflect.DeepEqual(glyphs, current.tl.Lines[0].Glyphs) {
				t.Fatal("paint-only policy changed text params, retained layout or glyph shapes/advances")
			}
			if after, _ := tt.Find("run"); after != bounds {
				t.Fatalf("paint policy moved layout: before=%v after=%v", bounds, after)
			}
			if bytes.Equal(native, quarter) {
				t.Fatal("test did not observe changed actual glyph placement/coverage")
			}
			policy = GlyphPlacementNative
			tt.Frame()
			if !bytes.Equal(native, tt.Image().Pix) || current.resolvedText().placeGlyph != nil {
				t.Fatal("explicit Native did not restore exact original frame")
			}
			policy = GlyphPlacementWindowsClearTypeQuarter
			tt.Frame()
			apply = false
			tt.Frame()
			if !bytes.Equal(native, tt.Image().Pix) || current.resolvedText().placeGlyph != nil {
				t.Fatal("rebuilt keyed node retained an omitted policy")
			}
		})
	}
}

func TestGlyphPlacementInheritanceAndPerWindowIsolation(t *testing.T) {
	var inherited, overridden, unknown, sibling *node
	build := func(c *context) {
		root := coreColumn(c)
		WithGlyphPlacement(wrapElement(root), GlyphPlacementWindowsClearTypeQuarter)
		root.Children(func() {
			inherited = coreText(c, "inherited")
			overridden = coreText(c, "native")
			WithGlyphPlacement(wrapElement(overridden), GlyphPlacementNative)
			unknown = coreText(c, "unknown")
			WithGlyphPlacement(wrapElement(unknown), GlyphPlacement(255))
		})
		sibling = coreText(c, "outside")
	}
	a := coreNewTester(build, 200, 100)
	a.Frame()
	if glyphPlacementTestSupported() && inherited.resolvedText().placeGlyph == nil {
		t.Fatal("descendant did not inherit the opt-in")
	}
	if overridden.resolvedText().placeGlyph != nil || unknown.resolvedText().placeGlyph != nil || sibling.resolvedText().placeGlyph != nil {
		t.Fatal("native override or unrelated branch inherited the opt-in")
	}
	var other *node
	b := coreNewTester(func(c *context) { other = coreText(c, "independent window") }, 200, 100)
	b.Frame()
	if other.resolvedText().placeGlyph != nil {
		t.Fatal("policy leaked across windows")
	}
	a.Frame()
	if glyphPlacementTestSupported() && inherited.resolvedText().placeGlyph == nil {
		t.Fatal("other window reset the original opt-in")
	}
}

func TestGlyphPlacementCheckedLifetimeAndZero(t *testing.T) {
	if got := WithGlyphPlacement(Element{}, GlyphPlacementWindowsClearTypeQuarter); got.Valid() {
		t.Fatal("absent element became valid")
	}
	var expired Element
	tt := coreNewTester(func(c *context) { expired = wrapElement(coreText(c, "lifetime")) }, 100, 50)
	tt.Frame()
	defer func() {
		if recover() == nil {
			t.Fatal("expired checked Element did not diagnose use")
		}
	}()
	WithGlyphPlacement(expired, GlyphPlacementWindowsClearTypeQuarter)
}

func TestGlyphPlacementBuildStateSizes(t *testing.T) {
	t.Logf("node=%d textStyle=%d optionalFunctionWord=%d", unsafe.Sizeof(node{}), unsafe.Sizeof(textStyle{}), unsafe.Sizeof(glyphPlacementPaint(nil)))
}
