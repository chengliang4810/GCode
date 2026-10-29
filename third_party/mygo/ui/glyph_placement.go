package ui

// GlyphPlacement selects a paint-only horizontal glyph-placement policy.
// It changes neither font selection, shaping, advances, nor layout geometry.
type GlyphPlacement uint8

const (
	// GlyphPlacementNative retains the platform's normal glyph placement.
	GlyphPlacementNative GlyphPlacement = iota
	// GlyphPlacementWindowsClearTypeQuarter requests float32 quarter-device-
	// pixel placement with half ties toward zero on Windows ClearType text.
	// Negative coordinates retain signed-fraction/truncation extraction.
	// Transparent/grayscale/aliased text and color-capable font faces keep
	// native placement, as do all other platforms.
	GlyphPlacementWindowsClearTypeQuarter
)

// An alias reuses the existing Draw/DrawOver function identity.
// The Rect carries the current text origin in X/Y; W/H are unused.
type glyphPlacementPaint = func(*Painter, Rect)

// WithGlyphPlacement sets an inheritable paint-only policy for plain Text
// leaves. Only the current kindText layout, without spans, inline children,
// an inline parent or ContentTransition, uses the requested placement. Rich
// text, font-only spans, inputs and Painter's custom text keep native painting.
// Backgrounds, selection and decorations retain their normal paint order.
// Native (the default), including unknown values, restores native placement.
// This unpublished opt-in is narrower than the earlier per-glyph experiment.
// This checked function adds no method to Element or private node.
func WithGlyphPlacement(e Element, policy GlyphPlacement) Element {
	if n := e.node(); n != nil {
		n.ts.placeGlyph = glyphPlacementFor(policy)
		n.ts.set |= setGlyphPlacement
	}
	return e
}
