package design

import (
	"image"
	"image/color"
	"math"
)

// BackgroundThumbnail ports image 0.25.10 DynamicImage::thumbnail, used by the
// reference before all artwork effects. Its integer box means and fractional
// interpolation are intentionally preserved, including enlargement of small
// images. Upstream: image/src/imageops/sample.rs (MIT or Apache-2.0).
func BackgroundThumbnail(src image.Image, limit int) *image.NRGBA {
	if src == nil || src.Bounds().Empty() || limit < 1 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	ratio := min(float64(limit)/float64(w), float64(limit)/float64(h))
	nw, nh := max(1, int(math.Round(float64(w)*ratio))), max(1, int(math.Round(float64(h)*ratio)))
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	get := func(x, y int) color.NRGBA {
		return color.NRGBAModel.Convert(src.At(x+src.Bounds().Min.X, y+src.Bounds().Min.Y)).(color.NRGBA)
	}
	channels := func(c color.NRGBA) [4]uint64 { return [4]uint64{uint64(c.R), uint64(c.G), uint64(c.B), uint64(c.A)} }
	fract := func(v float32) float32 { return v - float32(math.Trunc(float64(v))) }
	ceil := func(v float32) int { return int(math.Ceil(float64(v))) }
	xRatio, yRatio := float32(w)/float32(nw), float32(h)/float32(nh)
	for y := 0; y < nh; y++ {
		bottomF := float32(y) * yRatio
		topF := bottomF + yRatio
		bottom, top := min(max(ceil(bottomF), 0), h-1), min(max(ceil(topF), 0), h)
		for x := 0; x < nw; x++ {
			leftF := float32(x) * xRatio
			rightF := leftF + xRatio
			left, right := min(max(ceil(leftF), 0), w-1), min(max(ceil(rightF), 0), w)
			var values [4]uint8
			switch {
			case bottom != top && left != right:
				var sum [4]uint64
				for sy := bottom; sy < top; sy++ {
					for sx := left; sx < right; sx++ {
						c := channels(get(sx, sy))
						for i := range sum {
							sum[i] += c[i]
						}
					}
				}
				n := uint64((right - left) * (top - bottom))
				for i := range values {
					values[i] = uint8((sum[i] + n/2) / n)
				}
			case bottom != top:
				var a, b [4]uint64
				for sy := bottom; sy < top; sy++ {
					ca, cb := channels(get(right-1, sy)), channels(get(right, sy))
					for i := range a {
						a[i] += ca[i]
						b[i] += cb[i]
					}
				}
				f := (fract(leftF) + fract(rightF)) / 2
				fa, fb := (1-f)/float32(top-bottom), f/float32(top-bottom)
				for i := range values {
					values[i] = uint8(fa*float32(a[i]) + fb*float32(b[i]))
				}
			case left != right:
				var a, b [4]uint64
				for sx := left; sx < right; sx++ {
					ca, cb := channels(get(sx, top-1)), channels(get(sx, top))
					for i := range a {
						a[i] += ca[i]
						b[i] += cb[i]
					}
				}
				f := (fract(topF) + fract(bottomF)) / 2
				fa, fb := (1-f)/float32(right-left), f/float32(right-left)
				for i := range values {
					values[i] = uint8(fa*float32(a[i]) + fb*float32(b[i]))
				}
			default:
				fx, fy := (fract(leftF)+fract(rightF))/2, (fract(topF)+fract(bottomF))/2
				bl, tl := channels(get(right-1, top-1)), channels(get(right-1, top))
				br, tr := channels(get(right, top-1)), channels(get(right, top))
				ftr, ftl, fbr, fbl := fy*fx, fy*(1-fx), (1-fy)*fx, (1-fy)*(1-fx)
				for i := range values {
					values[i] = uint8(fbr*float32(br[i]) + ftr*float32(tr[i]) + fbl*float32(bl[i]) + ftl*float32(tl[i]))
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{values[0], values[1], values[2], values[3]})
		}
	}
	return out
}
