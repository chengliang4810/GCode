//go:build !windows || (!amd64 && !arm64)

package ui

func glyphPlacementFor(GlyphPlacement) glyphPlacementPaint { return nil }
