package desktop

import (
	"gcode/internal/assets"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func icon(c *ui.Context, name string, size float32) {
	aliases := map[string]string{"sidebar-minimalistic": "split-columns", "menu-dots": "more-horizontal", "filter": "sort", "branch": "git-branch", "code": "file-code"}
	if mapped, ok := aliases[name]; ok {
		name = mapped
	}
	if svg := assets.Icon(name); svg != nil {
		glyph := ui.Icon(c, svg).Size(size, size)
		if name == "claude-mark" {
			glyph.TextColor(ui.Hex("#D97757"))
		}
		if name == "muse-mark" {
			glyph.TextColor(ui.Hex("#0081FB"))
		}
	}
}

func iconButton(c *ui.Context, name, label string) ui.Element {
	b := ui.ButtonBase(c).Size(28, 28).Radius(6).Label(label).Tooltip(label).Justify(ui.Center).AlignItems(ui.Center)
	if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() { icon(c, name, 16) })
	return b
}

func textButton(c *ui.Context, text string) ui.Element {
	b := ui.ButtonBase(c).Padding(5, 8).Radius(6).Label(text).AlignItems(ui.Center).Justify(ui.Start)
	if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() { ui.Text(c, text).SingleLine() })
	return b
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
	ui.Text(c, label).FontSize(design.UISize(c, 13)).TextColor(c.Theme().TextMuted).Margin(8, 0, 4, 0)
}

func (a *App) settingsCard(c *ui.Context, body func()) {
	ui.Column(c).FillWidth().Radius(12).Background(a.palette.Card).Children(body)
}

func line(c *ui.Context) {
	ui.Box(c).Height(1).AlignSelf(ui.Stretch).Margin(0, 16).Background(c.Theme().Border.Alpha(.6))
}
