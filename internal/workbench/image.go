package workbench

import (
	"gcode/internal/design"
	"math"

	"github.com/egoist/mygo/ui"
)

// imageGeometry ports the bounded fit, anchor-preserving zoom and pan policy
// from the frozen reference's crates/ui/src/image_viewer.rs.
type imageGeometry struct {
	naturalW, naturalH, width, height, scale, panX, panY float32
	fitted                                               bool
}

func finite(v float32) bool                { return !float32NaN(v) && !math.IsInf(float64(v), 0) }
func float32NaN(v float32) bool            { return v != v }
func (g *imageGeometry) fitScale() float32 { return min(g.width/g.naturalW, g.height/g.naturalH, 1) }
func (g *imageGeometry) resize(nw, nh, w, h float32) {
	if !finite(nw) || !finite(nh) || !finite(w) || !finite(h) || min(nw, nh, w, h) <= 0 {
		return
	}
	if g.scale <= 0 {
		g.fitted = true
	}
	g.naturalW, g.naturalH, g.width, g.height = nw, nh, w, h
	if g.fitted {
		g.fit()
	} else {
		g.clampPan()
	}
}
func (g *imageGeometry) fit() { g.scale = g.fitScale(); g.panX, g.panY = 0, 0; g.fitted = true }
func (g *imageGeometry) clampPan() {
	x := max((g.naturalW*g.scale-g.width)/2, 0)
	y := max((g.naturalH*g.scale-g.height)/2, 0)
	g.panX = max(-x, min(x, g.panX))
	g.panY = max(-y, min(y, g.panY))
}
func (g *imageGeometry) zoom(scale, x, y float32) {
	if !finite(scale) || !finite(x) || !finite(y) || scale <= 0 || g.scale <= 0 {
		return
	}
	maximum := max(min(131072/max(g.naturalW, g.naturalH), 32), g.fitScale())
	scale = max(min(g.fitScale(), .01), min(maximum, scale))
	ratio := scale / g.scale
	x -= g.width / 2
	y -= g.height / 2
	g.panX = x - (x-g.panX)*ratio
	g.panY = y - (y-g.panY)*ratio
	g.scale = scale
	g.fitted = false
	g.clampPan()
}
func (g *imageGeometry) pan(dx, dy float32) {
	if finite(dx) && finite(dy) {
		g.panX += dx
		g.panY += dy
		g.clampPan()
	}
}
func (g *imageGeometry) origin() (float32, float32) {
	return (g.width-g.naturalW*g.scale)/2 + g.panX, (g.height-g.naturalH*g.scale)/2 + g.panY
}

// ImageViewport is persistent, native image zoom/pan state. The returned element
// is frame-local; callers must not retain it. Images themselves are decoded once.
type ImageViewport struct {
	geometry     imageGeometry
	handle       ui.Handle
	dragging     bool
	lastX, lastY float32
}

func (v *ImageViewport) View(c *ui.Context, image ui.ImageSource) ui.Element {
	nw, nh := float32(1), float32(1)
	switch image := image.(type) {
	case *ui.SVG:
		nw, nh = image.Size()
	case *ui.Bitmap:
		w, h := image.Size()
		nw, nh = float32(w), float32(h)
	}
	bounds := v.handle.Bounds(c)
	v.geometry.resize(nw, nh, bounds.W, bounds.H)
	if bounds.W <= 0 || bounds.H <= 0 {
		c.Invalidate()
	}
	viewport := ui.Box(c).Grow(1).FillWidth().Clip().Focusable().Bind(&v.handle).Label(tr(c, "Image preview")).Cursor(ui.CursorGrab)
	if v.dragging {
		viewport.Cursor(ui.CursorGrabbing)
	}
	viewport.HandleInput(func(e ui.InputEvent) bool {
		switch e.Kind {
		case ui.InputZoom:
			v.geometry.zoom(v.geometry.scale*e.Scale, e.X, e.Y)
			return true
		case ui.InputScroll:
			if e.Mods&ui.Ctrl != 0 {
				v.geometry.zoom(v.geometry.scale*float32(math.Exp(float64(-e.DY*.0025))), e.X, e.Y)
			} else {
				v.geometry.pan(-e.DX, -e.DY)
			}
			return true
		case ui.InputPointerDown:
			if e.Button == 0 {
				v.dragging = true
				v.lastX, v.lastY = e.X, e.Y
				return true
			}
		case ui.InputPointerMove:
			if v.dragging {
				v.geometry.pan(e.X-v.lastX, e.Y-v.lastY)
				v.lastX, v.lastY = e.X, e.Y
				return true
			}
		case ui.InputPointerUp:
			if v.dragging {
				v.dragging = false
				return true
			}
		}
		return false
	})
	viewport.Children(func() {
		x, y := v.geometry.origin()
		ui.Image(c, image).Absolute().Left(x).Top(y).Size(nw*v.geometry.scale, nh*v.geometry.scale).Fit(ui.Contain)
	})
	return viewport
}

func (s *State) imageDialog(c *ui.Context) {
	p := s.palette()
	width, height := c.Size()
	ui.DialogBase(c, &s.imageOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .8))
		panel.Width(max(300, width-64)).Height(max(200, height-64)).Radius(10).Background(p.Background).Padding(8)
		d := s.documents[s.imageID]
		if d == nil || d.image == nil {
			return
		}
		ui.Row(c).Height(30).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, d.file.Name).FontSize(design.UISize(c, 12)).TextColor(p.Muted)
			ui.Spacer(c)
			s.iconButton(c, "close", "Close image", func() { s.imageOpen = false })
		})
		s.imageViewer.View(c, d.image)
	})
}
