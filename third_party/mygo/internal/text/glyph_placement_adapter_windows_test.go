//go:build windows && (amd64 || arm64)

package text

func quarterRequestedForTest(s *System, f *Font, id uint32, scale, x float32, shade Shade, opaque bool) GlyphImage {
	return RasterGlyph(s, f, id, scale, x, shade, opaque, GlyphPlacementWindowsClearTypeQuarter)
}
