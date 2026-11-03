package desktop

import (
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/providerlogo"
	"github.com/egoist/mygo/ui"
	"strings"
	"sync"
)

type media struct {
	bitmap *ui.Bitmap
	svg    *ui.SVG
}

// mediaCache keeps recently shown attachment images decodable once. Entries
// hold full decoded bitmaps, so the cache is bounded: the oldest image falls
// out past mediaCacheLimit and decodes again when it is shown later.
var mediaKeys []string
var mediaCache = map[string]media{}
var mediaCacheMu sync.Mutex

const mediaCacheLimit = 16

func loadMedia(ref string) media {
	mediaCacheMu.Lock()
	if hit, ok := mediaCache[ref]; ok {
		for i, key := range mediaKeys {
			if key == ref {
				mediaKeys = append(mediaKeys[:i], mediaKeys[i+1:]...)
				break
			}
		}
		mediaKeys = append(mediaKeys, ref)
		mediaCacheMu.Unlock()
		return hit
	}
	mediaCacheMu.Unlock()
	var data []byte
	var err error
	svg := strings.HasSuffix(strings.ToLower(ref), ".svg")
	if strings.HasPrefix(ref, "data:image/") {
		data, svg, err = providerlogo.ImageData(ref)
	} else {
		data, err = assets.Read(strings.TrimPrefix(ref, "asset://"))
	}
	if err != nil {
		return media{}
	}
	result := media{}
	if svg {
		result.svg, _ = ui.ParseSVG(data)
	} else {
		result.bitmap, _ = ui.DecodeBitmap(data)
	}
	storeMedia(ref, result)
	return result
}

// hasMedia reports whether a decoded image for ref is cached, for preload
// loops that would otherwise decode it again.
func hasMedia(ref string) bool {
	mediaCacheMu.Lock()
	defer mediaCacheMu.Unlock()
	_, ok := mediaCache[ref]
	return ok
}

// storeMedia caches a decoded image, evicting the least recently shown one
// past the limit.
func storeMedia(ref string, result media) {
	mediaCacheMu.Lock()
	defer mediaCacheMu.Unlock()
	for i, key := range mediaKeys {
		if key == ref {
			mediaKeys = append(mediaKeys[:i], mediaKeys[i+1:]...)
			break
		}
	}
	mediaKeys = append(mediaKeys, ref)
	mediaCache[ref] = result
	for len(mediaKeys) > mediaCacheLimit {
		delete(mediaCache, mediaKeys[0])
		mediaKeys = mediaKeys[1:]
	}
}
func drawMedia(c *ui.Context, image media, width, height float32) ui.Element {
	if image.svg != nil {
		return ui.Image(c, image.svg).Size(width, height).Fit(ui.Contain)
	}
	if image.bitmap != nil {
		return ui.Image(c, image.bitmap).Size(width, height).Fit(ui.Contain)
	}
	return ui.Text(c, tr(c, "Preview unavailable")).TextColor(c.Theme().TextMuted)
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
		drawMedia(c, image, w, h).Radius(10).Label(tr(c, "Open ") + item.Name).OnClick(func() { a.imageRef = item.AssetRef; a.imageZoom = 1; a.imageOpen = true })
		ui.Text(c, item.Name).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
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
				iconButton(c, "minus", tr(c, "Zoom out")).OnClick(func() { a.imageZoom = max(.25, a.imageZoom-.25) })
				textButton(c, tr(c, "Reset zoom")).OnClick(func() { a.imageZoom = 1 })
				iconButton(c, "plus", tr(c, "Zoom in")).OnClick(func() { a.imageZoom = min(4, a.imageZoom+.25) })
				iconButton(c, "close", tr(c, "Close image")).OnClick(func() { a.imageOpen = false })
			})
			ui.Scroll(c).Grow(1).Children(func() { drawMedia(c, loadMedia(a.imageRef), (width-160)*a.imageZoom, (height-190)*a.imageZoom) })
		})
	})
}
