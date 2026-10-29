package ui

// AttachAtPoint takes the element out of its parent's layout and places its
// margin box gap logical pixels right of and below (x, y), in window coordinates.
// It measures this layout's own content, padding, border and min/max constraints
// before positioning. An overflowing right or bottom edge flips that axis to
// gap before the point; a negative origin then moves to zero. An oversized box
// keeps its measured size. There is no implicit window-edge inset or rounding.
//
// gap is at least zero. The coordinates are physical directions, independent of
// text direction. Build in an Overlay to avoid ancestor clipping. This replaces
// Attach/AttachTo positioning; SnapToWindow does not alter this placement.
func (e *node) AttachAtPoint(x, y, gap float32) *node {
	e.flags |= flagAbsolute
	e.attach, e.popover = 0, nil
	e.pointX, e.pointY, e.pointGap = x, y, max(gap, 0)
	e.pointAttached = true
	// The constructor reaches the implementation so applications that do not
	// use this option do not link its layout code through the shared engine.
	e.c.placePoint = layoutAtPoint
	return e
}

func layoutAtPoint(c, parent *node, pw, ph float32) {
	var w float32
	if v, ok := c.width.resolve(pw); ok {
		w = c.clampW(v, pw)
	} else {
		// Available viewport room must not shrink a naturally sized tooltip
		// before the flip. Percentage constraints still resolve against pw.
		w = fitWidth(c, inf, pw)
	}
	var h float32
	if v, ok := c.height.resolve(ph); ok {
		h = c.clampH(v, ph)
	} else {
		h = heightAt(c, w, ph)
	}
	x := pointAxis(c.pointX, w+c.marginX(), c.c.w, c.pointGap) + c.m(3)
	y := pointAxis(c.pointY, h+c.marginY(), c.c.h, c.pointGap) + c.m(0)
	ox, oy := laidOutOrigin(parent)
	c.x, c.y = x-ox, y-oy
	layoutBox(c, w, h)
}

func pointAxis(point, size, limit, gap float32) float32 {
	pos := point + gap
	if pos+size > limit {
		pos = point - gap - size
	}
	return max(pos, 0)
}
