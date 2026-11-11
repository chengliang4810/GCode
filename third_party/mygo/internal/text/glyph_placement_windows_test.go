//go:build windows && (amd64 || arm64)

package text

import (
	"bytes"
	"math"
	"reflect"
	"testing"
	"unsafe"
)

func renderPlacementTest(t *testing.T, s *System, f *Font, id uint32, scale, x float32, opaque bool, policy GlyphPlacement) GlyphImage {
	t.Helper()
	for range 8 {
		g := RasterGlyph(s, f, id, scale, x, 0, opaque, policy)
		if !s.Full() {
			return g
		}
		s.MakeRoom()
	}
	t.Fatal("atlas did not settle")
	return GlyphImage{}
}

func TestQuarterPositionRecordedSignedAndTieVectors(t *testing.T) {
	oracle := loadQuarterOracle(t)
	for _, v := range oracle.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			x := math.Float32frombits(v.PenBits)
			q, carry := quarterPosition(x)
			origin := float32(math.Floor(float64(x))) + carry
			if q != v.PhaseIndex || origin != float32(v.IntegerOrigin) {
				t.Fatalf("pen=%v got phase=%v integer=%v, external source phase=%v integer=%v", x, q, origin, v.PhaseIndex, v.IntegerOrigin)
			}
		})
	}
}

func TestQuarterCachePolicyCollisionOrderAndNormalization(t *testing.T) {
	for _, quarterFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "native-first", true: "quarter-first"}[quarterFirst], func(t *testing.T) {
			s, f, oracle := quarterTestSystem(t)
			glyph := s.Layout(Params{Text: "a", Style: Style{Family: oracle.FontFamily, Size: f.Size}}).Lines[0].Glyphs[0]
			// At .4 the native and quarter phase indices are both2, with
			// different denominators6/4. An index-only cache aliases them.
			first, second := GlyphPlacementNative, GlyphPlacementWindowsClearTypeQuarter
			if quarterFirst {
				first, second = second, first
			}
			for _, policy := range []GlyphPlacement{first, second} {
				g := renderPlacementTest(t, s, f, glyph.ID, 2, .4, true, policy)
				if !g.OK || !g.Subpixel {
					t.Fatalf("no actual ClearType glyph: %+v", g)
				}
			}
			var native, quarter *atlasEntry
			for key, entry := range s.glyphs {
				if key.subX == 2 {
					if key.mode == nativeSubpixel {
						native = entry
					} else if key.mode == quarterSubpixel {
						quarter = entry
					}
				}
			}
			if native == nil || quarter == nil || native == quarter {
				t.Fatal("native/quarter phase2 did not retain separate cache identities")
			}
			if bytes.Equal(quarterImageRGB(s, native.GlyphImage), quarterImageRGB(s, quarter.GlyphImage)) {
				t.Fatal("collision control failed to distinguish actual .333/.5 coverage")
			}
			before := len(s.glyphs)
			for _, policy := range []GlyphPlacement{GlyphPlacementNative, GlyphPlacementWindowsClearTypeQuarter, GlyphPlacement(2), GlyphPlacement(255)} {
				renderPlacementTest(t, s, f, glyph.ID, 2, .4, true, policy)
			}
			if len(s.glyphs) != before {
				t.Fatal("repeat/unknown policy made a duplicate atlas entry")
			}
			if len(s.places) != 2 {
				t.Fatalf("place identities=%d, want native+quarter", len(s.places))
			}
		})
	}
}

func TestQuarterPreservesShapeAndFontCaches(t *testing.T) {
	s, _, oracle := quarterTestSystem(t)
	p := Params{Text: "AV To fi 123 中文 e\u0301", Style: Style{Family: oracle.FontFamily, Size: 10.5, Features: "kern=0"}}
	layout := s.Layout(p)
	before := append([]Glyph(nil), layout.Lines[0].Glyphs...)
	fonts, made := len(s.fonts), s.LayoutsMade()
	for _, g := range before {
		renderPlacementTest(t, s, g.Font, g.ID, 2, 40+g.X*2, true, GlyphPlacementWindowsClearTypeQuarter)
	}
	if s.Layout(p) != layout || !reflect.DeepEqual(before, layout.Lines[0].Glyphs) || len(s.fonts) != fonts || s.LayoutsMade() != made {
		t.Fatal("paint policy changed shaped output, font identity or layout cache")
	}
}

func TestQuarterUnsupportedPathsNormalizeToNative(t *testing.T) {
	for _, name := range []string{"transparent", "grayscale-setting", "aliased-setting", "color-face", "unknown-policy"} {
		t.Run(name, func(t *testing.T) {
			s, f, oracle := quarterTestSystem(t)
			opaque, policy := true, GlyphPlacementWindowsClearTypeQuarter
			e := s.engine().(*dwrite)
			switch name {
			case "transparent":
				opaque = false
			case "grayscale-setting":
				e.clearType = false
				s.subpixel = false
			case "aliased-setting":
				e.aliased = true
				s.subpixel = false
			case "color-face":
				g := s.Layout(Params{Text: "🎉", Style: Style{Size: 16}}).Lines[0].Glyphs[0]
				f = g.Font
				if !isColor(f.native) {
					t.Fatal("color exclusion control did not resolve a color face")
				}
			case "unknown-policy":
				policy = 255
			}
			g := s.Layout(Params{Text: "a", Style: Style{Family: oracle.FontFamily, Size: f.Size}}).Lines[0].Glyphs[0]
			if name == "color-face" {
				g = s.Layout(Params{Text: "🎉", Style: Style{Size: 16}}).Lines[0].Glyphs[0]
			}
			native := renderPlacementTest(t, s, f, g.ID, 2, .4, opaque, GlyphPlacementNative)
			count := len(s.glyphs)
			other := renderPlacementTest(t, s, f, g.ID, 2, .4, opaque, policy)
			if native != other || len(s.glyphs) != count {
				t.Fatalf("unsupported policy did not reuse exact native output/cache: native=%+v requested=%+v", native, other)
			}
			wantMode := nativeNonSubpixel
			if opaque && s.subpixel {
				wantMode = nativeSubpixel
			}
			for key := range s.glyphs {
				if key.mode != wantMode {
					t.Fatal("unsupported request left a non-native glyph key")
				}
			}
			for key := range s.places {
				if key.mode != wantMode {
					t.Fatal("unsupported request left a non-native placement key")
				}
			}
		})
	}
}

func TestQuarterRecommendedRenderingModeEligibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode uintptr
		want bool
	}{
		{"default", 0, false}, {"aliased", renderingModeAliased, false},
		{"gdi-classic", renderingModeGDIClassic, false}, {"gdi-natural", renderingModeGDINatural, false},
		{"natural", renderingModeNatural, true}, {"symmetric", renderingModeNaturalSymmetric, true},
		{"outline", renderingModeOutline, false}, {"downsampled", renderingModeDownsampled, true},
		{"unknown", 255, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := quarterRenderingMode(tc.mode); got != tc.want {
				t.Fatalf("mode %v eligible=%v want=%v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestQuarterCacheKeyPaddingCost(t *testing.T) {
	// Explicit native baseline layouts: no hidden extra key allocation/word.
	type nativeGlyphKey struct {
		font      *Font
		id, scale uint32
		subX      uint8
		shade     Shade
		subpixel  bool
	}
	type nativePlaceKey struct {
		font     *Font
		scale    uint32
		subpixel bool
	}
	if unsafe.Sizeof(glyphKey{}) != unsafe.Sizeof(nativeGlyphKey{}) || unsafe.Sizeof(placeKey{}) != unsafe.Sizeof(nativePlaceKey{}) {
		t.Fatal("normalized mode grew glyph or placement keys from the native baseline")
	}
	t.Logf("glyphKey=%d placeKey=%d", unsafe.Sizeof(glyphKey{}), unsafe.Sizeof(placeKey{}))
}

// All three normalized requests must keep their own raster and placement
// identities, regardless of which request warms the caches first.
func TestQuarterThreeRasterModeRequestOrders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order [3]uint8
	}{
		{"non-subpixel-native-quarter", [3]uint8{0, 1, 2}},
		{"non-subpixel-quarter-native", [3]uint8{0, 2, 1}},
		{"native-non-subpixel-quarter", [3]uint8{1, 0, 2}},
		{"native-quarter-non-subpixel", [3]uint8{1, 2, 0}},
		{"quarter-non-subpixel-native", [3]uint8{2, 0, 1}},
		{"quarter-native-non-subpixel", [3]uint8{2, 1, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, oracle := quarterTestSystem(t)
			glyph := s.Layout(Params{Text: "a", Style: Style{Family: oracle.FontFamily, Size: f.Size}}).Lines[0].Glyphs[0]
			var first [3]GlyphImage
			for _, mode := range tc.order {
				policy := GlyphPlacementNative
				if mode == quarterSubpixel {
					policy = GlyphPlacementWindowsClearTypeQuarter
				}
				g := renderPlacementTest(t, s, f, glyph.ID, 2, .4, mode != nativeNonSubpixel, policy)
				if !g.OK || g.Subpixel != (mode != nativeNonSubpixel) || g.Colored {
					t.Fatalf("mode=%d actual glyph flags=%+v", mode, g)
				}
				first[mode] = g
			}
			for _, mode := range tc.order {
				policy := GlyphPlacementNative
				if mode == quarterSubpixel {
					policy = GlyphPlacementWindowsClearTypeQuarter
				}
				if got := renderPlacementTest(t, s, f, glyph.ID, 2, .4, mode != nativeNonSubpixel, policy); got != first[mode] {
					t.Fatalf("mode=%d repeat changed glyph: got=%+v first=%+v", mode, got, first[mode])
				}
			}
			glyphModes, placeModes := [3]int{}, [3]int{}
			for key := range s.glyphs {
				if key.mode > quarterSubpixel {
					t.Fatalf("non-normalized glyph mode %d", key.mode)
				}
				glyphModes[key.mode]++
			}
			for key := range s.places {
				if key.mode > quarterSubpixel {
					t.Fatalf("non-normalized placement mode %d", key.mode)
				}
				placeModes[key.mode]++
			}
			if glyphModes != [3]int{1, 1, 1} || placeModes != [3]int{1, 1, 1} {
				t.Fatalf("three states aliased or duplicated: glyphs=%v places=%v", glyphModes, placeModes)
			}
		})
	}
}
