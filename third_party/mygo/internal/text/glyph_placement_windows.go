//go:build windows && (amd64 || arm64)

package text

func (s *System) quarterEligible(f *Font, scale float32, subpixel bool, policy uint8) bool {
	if policy != GlyphPlacementWindowsClearTypeQuarter || !subpixel {
		return false
	}
	e, ok := s.engine().(*dwrite)
	if !ok {
		return false
	}
	e.settings()
	if !e.clearType || e.aliased || e.isColor(f.native) {
		return false
	}
	mode, _ := e.mode(f, scale)
	return quarterRenderingMode(mode)
}

func quarterRenderingMode(mode uintptr) bool {
	return mode == renderingModeNatural || mode == renderingModeNaturalSymmetric || mode == renderingModeDownsampled
}
