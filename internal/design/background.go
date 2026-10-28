package design

import (
	"image"
	"image/color"
	"math"

	"github.com/egoist/mygo/ui"
)

// BackgroundAdjustment matches the viewport-independent framing in the pinned
// settings.rs and new_thread_background_mask.rs. It contains no native handles.
type BackgroundAdjustment struct{ FocalX, FocalY, Zoom float32 }

func DefaultBackgroundAdjustment() BackgroundAdjustment { return BackgroundAdjustment{.5, .5, 1} }

func NormalizeBackgroundAdjustment(a BackgroundAdjustment) BackgroundAdjustment {
	a.FocalX = finiteClamp(a.FocalX, 0, 1, .5)
	a.FocalY = finiteClamp(a.FocalY, 0, 1, .5)
	a.Zoom = finiteClamp(a.Zoom, 1, 4, 1)
	return a
}

func finiteClamp(v, low, high, fallback float32) float32 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return fallback
	}
	return min(high, max(low, v))
}

// BackgroundFit returns the exact covered source rectangle, before clipping.
func BackgroundFit(sw, sh float32, bounds ui.Rect, a BackgroundAdjustment) (ui.Rect, bool) {
	for _, value := range []float32{sw, sh, bounds.W, bounds.H} {
		if value <= 0 || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return ui.Rect{}, false
		}
	}
	a = NormalizeBackgroundAdjustment(a)
	cover := max(bounds.W/sw, bounds.H/sh)
	w, h := max(sw*cover*a.Zoom, bounds.W), max(sh*cover*a.Zoom, bounds.H)
	return ui.Rect{X: bounds.X - (w-bounds.W)*a.FocalX, Y: bounds.Y - (h-bounds.H)*a.FocalY, W: w, H: h}, true
}

func PanBackground(sw, sh float32, bounds ui.Rect, a BackgroundAdjustment, dx, dy float32) BackgroundAdjustment {
	a = NormalizeBackgroundAdjustment(a)
	fitted, ok := BackgroundFit(sw, sh, bounds, a)
	if !ok {
		return a
	}
	if fitted.W > bounds.W {
		a.FocalX -= dx / (fitted.W - bounds.W)
	}
	if fitted.H > bounds.H {
		a.FocalY -= dy / (fitted.H - bounds.H)
	}
	return NormalizeBackgroundAdjustment(a)
}

func ZoomBackground(sw, sh float32, bounds ui.Rect, a BackgroundAdjustment, zoom, x, y float32) BackgroundAdjustment {
	a = NormalizeBackgroundAdjustment(a)
	before, ok := BackgroundFit(sw, sh, bounds, a)
	if !ok {
		return a
	}
	next := a
	next.Zoom = zoom
	next = NormalizeBackgroundAdjustment(next)
	after, _ := BackgroundFit(sw, sh, bounds, next)
	sourceX, sourceY := (x-before.X)/before.W, (y-before.Y)/before.H
	if after.W > bounds.W {
		next.FocalX = (bounds.X - (x - sourceX*after.W)) / (after.W - bounds.W)
	}
	if after.H > bounds.H {
		next.FocalY = (bounds.Y - (y - sourceY*after.H)) / (after.H - bounds.H)
	}
	return NormalizeBackgroundAdjustment(next)
}

func BackgroundHeight(viewportHeight float32) float32 { return min(max(viewportHeight, 0)*.72, 760) }

func BackgroundPreviewSize(vw, vh, sidebarWidth float32, collapsed bool) (float32, float32) {
	if collapsed {
		sidebarWidth = 0
	}
	w, h := max(vw-sidebarWidth, 1), max(BackgroundHeight(vh), 1)
	scale := min(min(max(vw-84, 1), 556)/w, 258/h)
	return w * scale, h * scale
}

// BackgroundMaskAlpha ports GPUI's image_mask_alpha shader parameters from
// new_thread_background_mask.rs. Coordinates are logical pixels. The underlay
// and cutout are separate passes in the original compositor.
func BackgroundMaskAlpha(hero, composer ui.Rect, x, y float32, cutout bool) float32 {
	fade := smoothBackground((hero.Y + hero.H - y) / max(hero.H, 1))
	if !cutout {
		return fade
	}
	composer.H = max(composer.Y+composer.H, hero.Y+hero.H) - composer.Y
	halfW, halfH := composer.W*.5, composer.H*.5
	qx := float32(math.Abs(float64(x-composer.X-halfW))) - halfW + 26
	qy := float32(math.Abs(float64(y-composer.Y-halfH))) - halfH + 26
	px, py := max(qx, 0), max(qy, 0)
	distance := float32(math.Sqrt(float64(px*px+py*py))) + min(max(qx, qy), 0) - 26
	return min(fade, smoothBackground((distance-8)/min(max(hero.H*.52, 120), 280)))
}

// BackgroundCompositeAlpha combines the half-opacity reveal pass underneath
// the full masked pass, for a renderer preparing a single native bitmap.
func BackgroundCompositeAlpha(hero, composer ui.Rect, x, y, sourceAlpha float32) float32 {
	under := BackgroundMaskAlpha(hero, composer, x, y, false) * .5 * sourceAlpha
	over := BackgroundMaskAlpha(hero, composer, x, y, true) * sourceAlpha
	return over + under*(1-over)
}

func smoothBackground(v float32) float32 { v = min(max(v, 0), 1); return v * v * (3 - 2*v) }

// BackgroundEffect ports the original source-space pixels, including its
// image-rs thumbnail proxy and alpha handling. Unlike RenderImage, Go images use
// RGBA channel order, so the original final BGRA upload swizzle is omitted.
// Run this once per source/effect/appearance off the UI thread, never per draw.
func BackgroundEffect(src image.Image, effect string, light bool) *image.NRGBA {
	return BackgroundEffectPixels(BackgroundThumbnail(src, 2048), effect, light)
}

// BackgroundEffectPixels applies the authored effect to an already sized proxy.
func BackgroundEffectPixels(src *image.NRGBA, effect string, light bool) *image.NRGBA {
	if src == nil || src.Bounds().Empty() {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	pixel := func(x, y int) color.NRGBA { return src.NRGBAAt(src.Bounds().Min.X+x, src.Bounds().Min.Y+y) }
	luma := func(c color.NRGBA) uint8 {
		// image 0.25.10 casts through its sRGB CICP matrix (moxcms0.8.1),
		// rather than the older integer Pixel::to_luma coefficients.
		const scale float32 = 1.0 / 255
		v := float32(float32(c.R)*scale) * float32(.21264784)
		v = float32(v + float32(float32(c.G)*scale)*float32(.7151792))
		v = float32(v + float32(float32(c.B)*scale)*float32(.07217304))
		return uint8(min(max(float32(math.Round(float64(v*255))), 0), 255))
	}
	if effect == "halftone" {
		paper := float32(0)
		if light {
			paper = 255
		}
		for y := 0; y < h; y += 4 {
			for x := 0; x < w; x += 4 {
				density := luma(pixel(x, y))
				if light {
					density = 255 - density
				}
				radius := float32(2) * (float32(.3) + float32(.7)*float32(math.Sqrt(float64(float32(density)/255))))
				ink := pixel(min(x+2, w-1), min(y+2, h-1))
				for dy := 0; dy < min(4, h-y); dy++ {
					for dx := 0; dx < min(4, w-x); dx++ {
						vx, vy := float32(dx)-1.5, float32(dy)-1.5
						distance := float32(math.Sqrt(float64(vx*vx + vy*vy)))
						coverage := min(max(radius+.5-distance, 0), 1) * float32(ink.A) / 255
						c := pixel(x+dx, y+dy)
						blend := func(base, dot uint8) uint8 {
							return uint8(float32(base)*.60 + (float32(dot)*coverage+paper*(1-coverage))*.40)
						}
						out.SetNRGBA(x+dx, y+dy, color.NRGBA{blend(c.R, ink.R), blend(c.G, ink.G), blend(c.B, ink.B), c.A})
					}
				}
			}
		}
		return out
	}
	bayer := [4][4]uint8{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	glyphs := [10][7]uint8{{}, {0, 0, 0, 0, 0, 4, 0}, {0, 4, 0, 0, 4, 0, 0}, {0, 0, 0, 14, 0, 0, 0}, {0, 0, 14, 0, 14, 0, 0}, {0, 4, 4, 31, 4, 4, 0}, {0, 21, 14, 31, 14, 21, 0}, {10, 10, 31, 10, 31, 10, 10}, {17, 2, 4, 4, 8, 16, 17}, {14, 17, 23, 21, 23, 16, 14}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := pixel(x, y)
			switch effect {
			case "dither":
				c = pixel(min(x/2*2+1, w-1), min(y/2*2+1, h-1))
				peak := float32(max(c.R, c.G, c.B))
				gain := float32(.08)
				if peak/255 > (float32(bayer[y/2%4][x/2%4])+.5)/16 {
					gain = 255 / max(peak, 1)
				}
				quantize := func(v uint8) uint8 { return uint8(min(float32(math.Round(float64(float32(v)*gain))), 255)) }
				c.R, c.G, c.B = quantize(c.R), quantize(c.G), quantize(c.B)
			case "ascii":
				inkColor := pixel(min(x/6*6+3, w-1), min(y/8*8+4, h-1))
				density := luma(inkColor)
				if light {
					density = 255 - density
				}
				index := int(float32(math.Sqrt(float64(float32(density)/255))) * 9)
				ink := x%6 < 5 && y%8 < 7 && glyphs[index][y%8]&(1<<uint(4-x%6)) != 0
				mix := func(base, glyph uint8) uint8 {
					paper := float32(0)
					if light {
						paper = 255
					}
					if ink {
						paper = float32(glyph)
					}
					return uint8(float32(base)*.60 + paper*.40)
				}
				c.R, c.G, c.B = mix(c.R, inkColor.R), mix(c.G, inkColor.G), mix(c.B, inkColor.B)
			case "scanlines":
				if y%3 == 0 {
					channel := func(v uint8) uint8 {
						if light {
							return uint8(float32(v) + (255-float32(v))*(1-float32(.52)))
						}
						return uint8(float32(v) * .52)
					}
					c.R, c.G, c.B = channel(c.R), channel(c.G), channel(c.B)
				}
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}
