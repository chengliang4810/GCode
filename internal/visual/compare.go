// Package visual compares equal-size reference and implementation captures.
package visual

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

type Result struct {
	Width                int     `json:"width"`
	Height               int     `json:"height"`
	SSIM                 float64 `json:"ssim"`
	ChangedPixels        int     `json:"changedPixels"`
	MaxChannelDifference uint8   `json:"maxChannelDifference"`
}

// Compare uses local8x8 luminance SSIM, without resizing, registration or masks.
func Compare(reference, actual image.Image) (Result, *image.RGBA, *image.RGBA, *image.RGBA, error) {
	if reference.Bounds().Size() != actual.Bounds().Size() {
		return Result{}, nil, nil, nil, fmt.Errorf("capture dimensions differ: reference %v, actual %v", reference.Bounds().Size(), actual.Bounds().Size())
	}
	size := reference.Bounds().Size()
	r := Result{Width: size.X, Height: size.Y}
	diff := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	pair := image.NewRGBA(image.Rect(0, 0, size.X*2, size.Y))
	overlay := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			a := color.NRGBAModel.Convert(reference.At(reference.Bounds().Min.X+x, reference.Bounds().Min.Y+y)).(color.NRGBA)
			b := color.NRGBAModel.Convert(actual.At(actual.Bounds().Min.X+x, actual.Bounds().Min.Y+y)).(color.NRGBA)
			dr, dg, db, da := delta(a.R, b.R), delta(a.G, b.G), delta(a.B, b.B), delta(a.A, b.A)
			maximum := max(dr, dg, db, da)
			r.MaxChannelDifference = max(r.MaxChannelDifference, maximum)
			if maximum > 0 {
				r.ChangedPixels++
			}
			diff.Set(x, y, color.NRGBA{R: dr, G: dg, B: db, A: 255})
			pair.Set(x, y, a)
			pair.Set(x+size.X, y, b)
			overlay.Set(x, y, color.NRGBA{R: uint8((uint16(a.R) + uint16(b.R)) / 2), G: uint8((uint16(a.G) + uint16(b.G)) / 2), B: uint8((uint16(a.B) + uint16(b.B)) / 2), A: 255})
		}
	}
	var total float64
	windows := 0
	for y := 0; y < size.Y; y += 8 {
		for x := 0; x < size.X; x += 8 {
			var aa, bb, a2, b2, ab, n float64
			for yy := y; yy < min(y+8, size.Y); yy++ {
				for xx := x; xx < min(x+8, size.X); xx++ {
					a := luminance(reference.At(reference.Bounds().Min.X+xx, reference.Bounds().Min.Y+yy))
					b := luminance(actual.At(actual.Bounds().Min.X+xx, actual.Bounds().Min.Y+yy))
					aa += a
					bb += b
					a2 += a * a
					b2 += b * b
					ab += a * b
					n++
				}
			}
			ma, mb := aa/n, bb/n
			va, vb, cov := math.Max(0, a2/n-ma*ma), math.Max(0, b2/n-mb*mb), ab/n-ma*mb
			total += ((2*ma*mb + 6.5025) * (2*cov + 58.5225)) / ((ma*ma + mb*mb + 6.5025) * (va + vb + 58.5225))
			windows++
		}
	}
	if windows > 0 {
		r.SSIM = total / float64(windows)
	}
	return r, diff, pair, overlay, nil
}
func delta(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return .2126*float64(r)/257 + .7152*float64(g)/257 + .0722*float64(b)/257
}
