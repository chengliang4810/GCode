//go:build windows && (amd64 || arm64)

package ui

import (
	"math"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/text"
)

func glyphPlacementFor(policy GlyphPlacement) glyphPlacementPaint {
	if policy == GlyphPlacementWindowsClearTypeQuarter {
		return paintQuarterText
	}
	return nil
}

// paintQuarterText runs only from the current kindText paint branch. Ghosts
// pass their own current node as p.inline; no live-node closure is retained.
func paintQuarterText(p *Painter, origin Rect) {
	e := p.inline
	if e == nil {
		return
	}
	l := e.tl
	ts := e.resolvedText()
	var spans spanPaint
	sp := e.paintSpans(&spans)
	if l == nil || e.kind != kindText || e.spans != nil || e.first != nil || e.isInline() || e.contentSet || sp != nil {
		// textLayout never dispatches this hook: unsupported paths paint once,
		// with their original span colors/decorations and immediately return.
		p.textLayout(l, origin.X, origin.Y, ts.color, ts, sp)
		return
	}
	x, y, color := origin.X, origin.Y, ts.color
	p.textBackgrounds(l, x, y, ts, sp)
	// Content transitions also paint the old layout; only the current text
	// carries its selection, in the same position, clip and opacity as its glyphs.
	if e := p.inline; e != nil && e.tl == l {
		if ed := e.st.editor; ed != nil && e.flags&flagSelectable != 0 && p.rt.textSelectionVisible(e.st) {
			if a, b := ed.selection(); a != b {
				for _, r := range l.Selection(a, b) {
					p.Fill(Rect{x + r.X, y + r.Y, r.W, r.H}, ts.selectionColor(e.c.theme), 0)
				}
			}
		}
	}
	sys := p.rt.text
	s := p.scale
	deco := decoration{underline: ts.underline, wavy: ts.wavy, dotted: ts.dotted, strike: ts.strike, color: ts.decoColor, thick: ts.decoThick}
	shade := text.ShadeOf(color.R, color.G, color.B)
	start := int32(len(p.s.Glyphs))
	// Inline transforms do not change while the text paints; look for them once.
	moves := p.inlineTransforms()
	clips := !p.transform.Set && !moves
	var run *glyphRun
	if sys.JoinsGlyphs() && !moves {
		// The engine's, whose buffers each text reuses.
		run = &p.rt.glyphRun
		run.ids, run.pens, run.glyphs = run.ids[:0], run.pens[:0], run.glyphs[:0]
	}
	for li := range l.Lines {
		line := &l.Lines[li]
		if clips && (y+line.Y > p.clip.Y+p.clip.H || y+line.Y+line.Height < p.clip.Y) {
			continue
		}
		baseline := sys.Baseline((y + line.Baseline) * s)
		for _, g := range line.Glyphs {
			pen := (x + g.X) * s
			if g.Object || clips && (pen > (p.clip.X+p.clip.W)*s || pen+(g.Advance+g.Size)*s < p.clip.X*s) {
				continue
			}
			ix := float32(math.Floor(float64(pen)))
			glyphColor, glyphShade := color, shade
			if sp != nil {
				if glyphColor = sp.color(sp.at(g.Cluster), color); glyphColor != color {
					glyphShade = text.ShadeOf(glyphColor.R, glyphColor.G, glyphColor.B)
				}
			}
			gi := text.RasterGlyph(sys, g.Font, g.ID, s, pen, glyphShade, p.opaque && !moves, text.GlyphPlacementWindowsClearTypeQuarter)
			if !gi.OK {
				continue
			}
			glyphColor = glyphColor.Alpha(p.opacity)
			sg := scene.Glyph{
				X: ix + gi.Left, Y: baseline + gi.Top, W: float32(gi.W), H: float32(gi.H),
				U: gi.X, V: gi.Y, UW: gi.W, VH: gi.H,
				Transform: p.inlineTransform(g.Cluster).Pixels(s),
				Color:     glyphColor.scene(), Wide: p.glyphWide(glyphColor), Colored: gi.Colored, Subpixel: gi.Subpixel, Thin: gi.Thin,
			}
			if run != nil {
				run.add(p, g, gi, pen, sg, glyphShade, baseline)
				continue
			}
			p.s.Glyphs = append(p.s.Glyphs, sg)
		}
		if run != nil {
			run.flush(p, baseline)
		}
		if sp != nil {
			sp.lines(p, l, li, x, y, color, deco)
		}
		if deco.underline || deco.strike {
			p.decorations(l, li, 0, len(line.Glyphs), x, y, deco, color)
		}
	}
	if end := int32(len(p.s.Glyphs)); end > start {
		p.add(scene.Op{Kind: scene.OpGlyphs, Start: start, End: end})
	}
}
