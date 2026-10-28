package design

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func backgroundReferenceSource(w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(x, y, color.NRGBA{uint8((x*19 + y*7) % 256), uint8((x*11 + y*23) % 256), uint8((x*31 + y*5) % 256), uint8((x*13 + y*17) % 256)})
		}
	}
	return out
}

// These bytes come from the unmodified original Rust effect methods, not from
// this Go implementation. They verify pixels, not native screenshot parity.
func TestBackgroundSourceRasterReferences(t *testing.T) {
	for _, effect := range []string{"none", "dither", "ascii", "halftone", "scanlines"} {
		for _, light := range []bool{false, true} {
			name := effect + "-dark"
			if light {
				name = effect + "-light"
			}
			t.Run(name, func(t *testing.T) {
				want, err := os.ReadFile("testdata/background/" + name + ".rgba")
				if err != nil {
					t.Fatal(err)
				}
				got := BackgroundEffectPixels(backgroundReferenceSource(33, 19), effect, light)
				if !bytes.Equal(got.Pix, want) {
					n, maxDelta := 0, 0
					for i, v := range got.Pix {
						d := int(v) - int(want[i])
						if d < 0 {
							d = -d
						}
						if d > 0 {
							n++
						}
						maxDelta = max(maxDelta, d)
					}
					t.Fatalf("%d channel differences, maximum delta%d against source raster", n, maxDelta)
				}
			})
		}
	}
}

func TestBackgroundThumbnailSourceReferences(t *testing.T) {
	for _, tc := range []struct{ w, h, limit int }{{3, 2, 8}, {19, 11, 7}, {1, 1, 9}, {9, 3, 5}, {2, 9, 7}} {
		name := fmt.Sprintf("thumbnail-%d-%d-%d", tc.w, tc.h, tc.limit)
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile("testdata/background/" + name + ".rgba")
			if err != nil {
				t.Fatal(err)
			}
			got := BackgroundThumbnail(backgroundReferenceSource(tc.w, tc.h), tc.limit)
			if !bytes.Equal(got.Pix, want) {
				t.Fatalf("thumbnail differs from original image crate: got%d bytes,want%d", len(got.Pix), len(want))
			}
		})
	}
}

func TestBackgroundFramingPreservesSourcePixelAtZoomAnchor(t *testing.T) {
	bounds := ui.Rect{X: 40, Y: 24, W: 1000, H: 500}
	fit, ok := BackgroundFit(1600, 900, bounds, DefaultBackgroundAdjustment())
	if !ok || fit.W != 1000 || fit.H != 562.5 || fit.X != 40 || fit.Y != -7.25 {
		t.Fatalf("source centered crop changed: %+v", fit)
	}
	pan := PanBackground(1600, 900, bounds, DefaultBackgroundAdjustment(), 10, 10)
	if pan.FocalX != .5 || math.Abs(float64(pan.FocalY-.34)) > .00001 {
		t.Fatalf("pan does not use overflowing axes: %+v", pan)
	}
	a := BackgroundAdjustment{.37, .61, 2.2}
	x, y := float32(380), float32(200)
	before, _ := BackgroundFit(1600, 900, bounds, a)
	zoom := ZoomBackground(1600, 900, bounds, a, 3, x, y)
	after, _ := BackgroundFit(1600, 900, bounds, zoom)
	if math.Abs(float64((x-before.X)/before.W-(x-after.X)/after.W)) > .000001 || math.Abs(float64((y-before.Y)/before.H-(y-after.Y)/after.H)) > .000001 {
		t.Fatal("zoom moved the source pixel under the anchor")
	}
	normalized := NormalizeBackgroundAdjustment(BackgroundAdjustment{float32(math.NaN()), float32(math.Inf(1)), 12})
	if normalized != (BackgroundAdjustment{.5, .5, 4}) {
		t.Fatalf("invalid geometry normalization %+v", normalized)
	}
}

func TestBackgroundMaskKeepsComposerCutoutOpenToHeroBottom(t *testing.T) {
	hero := ui.Rect{W: 1000, H: 600}
	composer := ui.Rect{X: 200, Y: 300, W: 600, H: 150}
	if BackgroundMaskAlpha(hero, composer, 500, 550, true) != 0 {
		t.Fatal("artwork fades back beneath composer lower edge")
	}
	if BackgroundMaskAlpha(hero, composer, 500, 600, false) != 0 {
		t.Fatal("bottom fade not transparent")
	}
	for _, alpha := range []float32{0, .2, .8, 1} {
		want := BackgroundMaskAlpha(hero, composer, 500, 350, false) * .5 * alpha
		got := BackgroundCompositeAlpha(hero, composer, 500, 350, alpha)
		if math.Abs(float64(got-want)) > .00001 {
			t.Fatal("composer reveal pass differs from source composition")
		}
	}
	if BackgroundHeight(400) != 288 || BackgroundHeight(1200) != 760 {
		t.Fatal("hero viewport height changed")
	}
}
