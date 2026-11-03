package desktop

import (
	"gcode/internal/assets"
	"gcode/internal/design"
	"gcode/internal/providerlogo"
	"github.com/egoist/mygo/ui"
)

func icon(c *ui.Context, name string, size float32) {
	aliases := map[string]string{"sidebar-minimalistic": "split-columns", "menu-dots": "more-horizontal", "filter": "sort", "branch": "git-branch", "code": "file-code"}
	if mapped, ok := aliases[name]; ok {
		name = mapped
	}
	svg := assets.Icon(name)
	if svg == nil {
		svg = assets.Glyph(name)
	}
	if svg != nil {
		glyph := ui.Icon(c, svg).Size(size, size)
		if name == "claude-mark" {
			glyph.TextColor(ui.Hex("#D97757"))
		}
		if name == "muse-mark" {
			glyph.TextColor(ui.Hex("#0081FB"))
		}
	}
}

// Shared by provider settings and the selected model in every composer.
func modelProviderIcon(c *ui.Context, size float32) {
	color := c.Theme().Text
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		x, y, s := r.X, r.Y, r.W
		path := (&ui.Path{}).MoveTo(x+s*.5, y+s*.04).LineTo(x+s*.9, y+s*.26).LineTo(x+s*.9, y+s*.73).
			LineTo(x+s*.5, y+s*.97).LineTo(x+s*.1, y+s*.73).LineTo(x+s*.1, y+s*.26).Close()
		p.StrokePath(path, 1.2, color)
		lines := (&ui.Path{}).MoveTo(x+s*.1, y+s*.26).LineTo(x+s*.5, y+s*.49).LineTo(x+s*.9, y+s*.26).
			MoveTo(x+s*.5, y+s*.49).LineTo(x+s*.5, y+s*.97).MoveTo(x+s*.3, y+s*.15).LineTo(x+s*.7, y+s*.38)
		p.StrokePath(lines, 1.1, color)
	})
}

func providerLogo(c *ui.Context, logo string, size float32) {
	if providerlogo.IsBuiltin(logo) {
		icon(c, logo, size)
		return
	}
	if logo != "" {
		image := loadMedia(logo)
		if image.svg != nil || image.bitmap != nil {
			drawMedia(c, image, size, size).Shrink(0)
			return
		}
	}
	modelProviderIcon(c, size)
}

func menuChoice(c *ui.Context, label string, selected bool, action func()) {
	b := ui.ButtonBase(c).FillWidth().Padding(7, 10).Radius(6).Label(label).Justify(ui.Start)
	if selected {
		b.Background(c.Theme().SurfacePressed)
	} else if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	b.OnClick(action)
}

func sectionTitle(c *ui.Context, label string) {
	ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted).Margin(8, 0, 4, 0)
}

func (a *App) settingsCard(c *ui.Context, body func()) {
	ui.Column(c).FillWidth().Radius(12).Background(a.palette.Card).Children(body)
}

func line(c *ui.Context) {
	ui.Box(c).Height(1).AlignSelf(ui.Stretch).Margin(0, 16).Background(c.Theme().Border.Alpha(.6))
}
