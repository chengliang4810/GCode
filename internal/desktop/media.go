package desktop

import (
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"strings"
	"sync"
)

type media struct {
	bitmap *ui.Bitmap
	svg    *ui.SVG
}

var mediaCache sync.Map

func loadMedia(ref string) media {
	if cached, ok := mediaCache.Load(ref); ok {
		return cached.(media)
	}
	data, err := assets.Read(strings.TrimPrefix(ref, "asset://"))
	if err != nil {
		return media{}
	}
	result := media{}
	if strings.HasSuffix(strings.ToLower(ref), ".svg") {
		result.svg, _ = ui.ParseSVG(data)
	} else {
		result.bitmap, _ = ui.DecodeBitmap(data)
	}
	mediaCache.Store(ref, result)
	return result
}
func drawMedia(c *ui.Context, image media, width, height float32) ui.Element {
	if image.svg != nil {
		return ui.Image(c, image.svg).Size(width, height).Fit(ui.Contain)
	}
	if image.bitmap != nil {
		return ui.Image(c, image.bitmap).Size(width, height).Fit(ui.Contain)
	}
	return ui.Text(c, "Preview unavailable").TextColor(c.Theme().TextMuted)
}
func (a *App) imageAttachment(c *ui.Context, item contracts.Attachment) {
	image := loadMedia(item.AssetRef)
	w, h := float32(item.Width), float32(item.Height)
	if w <= 0 || h <= 0 {
		w, h = 240, 180
	}
	scale := min(float32(1), min(360/w, 260/h))
	w *= scale
	h *= scale
	ui.Column(c).Gap(6).Children(func() {
		drawMedia(c, image, w, h).Radius(10).Label("Open " + item.Name).OnClick(func() { a.imageRef = item.AssetRef; a.imageZoom = 1; a.imageOpen = true })
		ui.Text(c, item.Name).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
	})
}
func (a *App) imageDialog(c *ui.Context) {
	ui.DialogBase(c, &a.imageOpen, func(back, panel ui.Element) {
		width, height := c.Size()
		back.Background(ui.RGBA(0, 0, 0, .8))
		panel.Width(max(320, width-100)).Height(max(280, height-100)).Padding(12).Radius(12).Background(a.palette.Background)
		ui.Column(c).Fill().Gap(8).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				iconButton(c, "minus", "Zoom out").OnClick(func() { a.imageZoom = max(.25, a.imageZoom-.25) })
				textButton(c, "Reset zoom").OnClick(func() { a.imageZoom = 1 })
				iconButton(c, "plus", "Zoom in").OnClick(func() { a.imageZoom = min(4, a.imageZoom+.25) })
				iconButton(c, "close", "Close image").OnClick(func() { a.imageOpen = false })
			})
			ui.Scroll(c).Grow(1).Children(func() { drawMedia(c, loadMedia(a.imageRef), (width-160)*a.imageZoom, (height-190)*a.imageZoom) })
		})
	})
}
