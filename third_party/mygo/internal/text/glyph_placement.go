package text

import "math"

// GlyphPlacement aliases the internal byte policy carrier. The public UI
// opt-in remains typed; no named policy type is stored in raster cache keys.
// Font selection, shaping, advances and offsets do not depend on it.
type GlyphPlacement = uint8

const (
	GlyphPlacementNative GlyphPlacement = iota
	GlyphPlacementWindowsClearTypeQuarter
)

// RasterGlyph is the opt-in entry; ordinary Glyph retains its native path.
// Unknown policies, disabled subpixel requests and nonfinite quarter pens use
// native placement immediately. Backend/AA/color checks run on a quarter
// placement-cache miss, before quarter insertion or glyph-cache lookup.
func RasterGlyph(s *System, f *Font, id uint32, scale, x float32, shade Shade, opaque bool, policy uint8) GlyphImage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f == nil {
		return GlyphImage{}
	}
	shade = f.shadeFor(shade)
	subpixel := opaque && s.subpixel
	if policy != GlyphPlacementWindowsClearTypeQuarter || !subpixel || !finitePen(x*4) {
		q, n, carry := s.place(f, scale, subpixel, x)
		return s.glyphAt(f, id, scale, shade, subpixel, q, n, carry, GlyphPlacementNative)
	}
	pk := placeKey{font: f, scale: uint32(scale*256 + 0.5), mode: quarterSubpixel}
	if _, ok := s.places[pk]; !ok {
		if !s.quarterEligible(f, scale, subpixel, policy) {
			q, n, carry := s.place(f, scale, subpixel, x)
			return s.glyphAt(f, id, scale, shade, subpixel, q, n, carry, GlyphPlacementNative)
		}
		s.places[pk] = placement{n: 4, round: true}
	}
	q, carry := quarterPosition(x)
	return s.glyphAt(f, id, scale, shade, subpixel, q, 4, carry, policy)
}

func finitePen(x float32) bool { return !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0) }

// quarterPosition keeps GPUI's float32 arithmetic and signed extraction:
// quantize, trunc integer origin, then signed fract cast to Rust u8. This is
// deliberately not floor-based quarter rounding for negative coordinates.
// Painter adds floor(x) itself; carry restores the source integer origin.
func quarterPosition(x float32) (q int, carry float32) {
	v := x * float32(4)
	magnitude := float32(math.Abs(float64(v))) - float32(.5)
	rounded := float32(math.Ceil(float64(magnitude)))
	rounded = float32(math.Copysign(float64(rounded), float64(v)))
	quantized := rounded / float32(4)
	integer := float32(math.Trunc(float64(quantized)))
	signed := (quantized - integer) * float32(4)
	q = max(0, int(signed)) // signed fract is [-1,1); negative Rust u8 =>0
	return q, integer - float32(math.Floor(float64(x)))
}
