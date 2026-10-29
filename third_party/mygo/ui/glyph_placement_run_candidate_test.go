//go:build windows && (amd64 || arm64)

package ui

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestQuarterRunGhostUsesCurrentGhostInsteadOfLiveReplacement(t *testing.T) {
	type observed struct {
		text  string
		color Color
		ghost bool
		ids   []uint32
	}
	first := true
	var seen []observed
	oldColor, newColor := RGB(220, 50, 60), RGB(45, 195, 120)
	tt, now := clockTester(func(c *context) {
		coreBox(c).Fill().Background(RGB(39, 39, 42)).Children(func() {
			value, key, ink := "OLD AV To", "old", oldColor
			if !first {
				value, key, ink = "NEW WWW", "replacement", newColor
			}
			e := coreText(c, value).Key(key).Absolute().Left(20.2).Top(10).Font("Microsoft YaHei UI").FontSize(10.5).
				FontWeight(400).FontFeatures("kern=0").TextColor(ink).Transition(ElementTransition{Exit: &Motion{Opacity: 0}, Ease: Linear, Duration: 200 * time.Millisecond})
			WithGlyphPlacement(wrapElement(e), GlyphPlacementWindowsClearTypeQuarter)
			base := e.ts.placeGlyph
			e.ts.placeGlyph = func(p *Painter, origin Rect) {
				current := p.inline
				record := observed{text: current.text, color: current.resolvedText().color, ghost: current.leaving != 0}
				for _, line := range current.tl.Lines {
					for _, glyph := range line.Glyphs {
						record.ids = append(record.ids, glyph.ID)
					}
				}
				seen = append(seen, record)
				base(p, origin)
			}
		})
	}, 280, 70)
	defer tt.rt.close()
	if len(seen) == 0 {
		t.Fatal("initial source text was not actually painted")
	}
	oldIDs := append([]uint32(nil), seen[len(seen)-1].ids...)
	first = false
	tt.Frame()
	*now = now.Add(50 * time.Millisecond)
	seen = nil
	tt.Frame()
	foundOld, foundNew := false, false
	for _, record := range seen {
		if record.ghost && record.text == "OLD AV To" && record.color == oldColor && reflect.DeepEqual(record.ids, oldIDs) {
			foundOld = true
		}
		if !record.ghost && record.text == "NEW WWW" && record.color == newColor && !reflect.DeepEqual(record.ids, oldIDs) {
			foundNew = true
		}
	}
	if !foundOld || !foundNew {
		t.Fatalf("paint callbacks did not independently read current ghost/live content: %+v", seen)
	}
	colors := map[uint32]bool{}
	for _, glyph := range tt.h.last.Glyphs {
		if glyph.Color.A != 0 {
			colors[uint32(glyph.Color.R)<<16|uint32(glyph.Color.G)<<8|uint32(glyph.Color.B)] = true
		}
	}
	if !colors[uint32(oldColor.R)<<16|uint32(oldColor.G)<<8|uint32(oldColor.B)] || !colors[uint32(newColor.R)<<16|uint32(newColor.G)<<8|uint32(newColor.B)] {
		t.Fatal("actual scene did not retain both fading ghost and live replacement colors")
	}
}

func TestQuarterRunTransformClipOpacityUsesExactNativeFallback(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			quarter := false
			tt := coreNewTester(func(c *context) {
				coreBox(c).Fill().Background(RGB(39, 39, 42)).Children(func() {
					parent := coreBox(c).Absolute().Left(35).Top(20).Size(130, 35).Clip().Rotate(9).Opacity(.6)
					if quarter {
						WithGlyphPlacement(wrapElement(parent), GlyphPlacementWindowsClearTypeQuarter)
					}
					parent.Children(func() {
						coreText(c, "AV To Tab02.txt clipped ending").Font("Microsoft YaHei UI").FontSize(10.5).TextColor(RGB(230, 230, 232)).
							TextBackground(RGB(30, 45, 60)).Underline().Strikethrough().DecorationColor(RGB(210, 80, 30)).Width(240)
					})
				})
			}, 280, 120)
			defer tt.rt.close()
			tt.SetScale(scale)
			tt.Frame()
			if len(tt.h.last.Glyphs) == 0 || clips(tt) == 0 {
				t.Fatal("no actual clipped text control")
			}
			native := bytes.Clone(tt.Image().Pix)
			before := quarterRunNonGlyphOps(tt)
			quarter = true
			tt.Frame()
			if !bytes.Equal(native, tt.Image().Pix) || !reflect.DeepEqual(before, quarterRunNonGlyphOps(tt)) {
				t.Fatal("opt-in changed native grayscale/transform/clip/opacity/decor fallback")
			}
		})
	}
}
