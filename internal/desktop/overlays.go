package desktop

import (
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"strings"
	"sync"
)

func (a *App) overlays(c *ui.Context) {
	ui.DialogBase(c, &a.renameOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, .35))
		panel.Width(400).Padding(24).Radius(12).Background(a.palette.Raised)
		ui.Column(c).Gap(16).Children(func() {
			ui.Text(c, tr(c, "Rename session")).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600)
			ui.TextInput(c, &a.renameText).Label(tr(c, "Session name")).AutoFocus()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				textButton(c, tr(c, "Cancel")).OnClick(func() { a.renameOpen = false })
				textButton(c, tr(c, "Rename")).OnClick(func() {
					title := a.renameText
					a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: a.renameID, Title: &title}})
					a.renameOpen = false
				})
			})
		})
	})
	a.newProjectDialog(c)
	a.commandPalette(c)

	if a.errorText != "" {
		opened := true
		ui.DialogBase(c, &opened, func(back, panel ui.Element) {
			back.Background(ui.RGBA(0, 0, 0, .35))
			panel.Width(440).Padding(24).Radius(12).Background(a.palette.Raised)
			ui.Column(c).Gap(16).Children(func() {
				ui.Text(c, tr(c, "Unable to complete the operation")).FontWeight(600)
				ui.Text(c, a.errorText).Selectable().TextColor(a.palette.Muted)
				textButton(c, tr(c, "Dismiss")).OnClick(func() { a.errorText = "" })
			})
		})
		if !opened {
			a.errorText = ""
		}
	}
}

var artifactCache sync.Map

func (a *App) artifactView(c *ui.Context, artifact contracts.Artifact, kind string) {
	if kind == "math" {
		if a.state.Settings.Appearance.RenderMath {
			a.mathView(c, artifact.Source)
		} else {
			fence := strings.Repeat("`", 3)
			markdown.View(c, fence+"math\n"+artifact.Source+"\n"+fence, &a.md)
		}
		return
	}
	for _, variant := range design.Themes() {
		suffix := "-" + variant.ID + ".svg"
		if strings.HasSuffix(artifact.AssetRef, suffix) {
			artifact.AssetRef = strings.TrimSuffix(artifact.AssetRef, suffix) + "-" + a.palette.ID + ".svg"
			break
		}
	}
	var svg *ui.SVG
	if cached, ok := artifactCache.Load(artifact.AssetRef); ok {
		svg = cached.(*ui.SVG)
	} else if data, err := assets.Read(strings.TrimPrefix(artifact.AssetRef, "asset://")); err == nil {
		if parsed, err := ui.ParseSVG(data); err == nil {
			svg = parsed
			artifactCache.Store(artifact.AssetRef, parsed)
		}
	}
	services := c.Services()
	show := func() {
		if svg != nil {
			w, h := float32(artifact.Width), float32(artifact.Height)
			ui.ScrollHorizontal(c).FillWidth().Children(func() {
				reserve, _ := a.infoVisibility()
				columnWidth := max(float32(0), min(float32(a.state.Settings.Appearance.TranscriptWidth), a.chatWidth-reserve)-48)
				if kind == "diagram" {
					columnWidth = max(0, columnWidth-24)
				}
				row := ui.Row(c).Width(max(w, columnWidth)).Justify(ui.Center)
				row.Children(func() {
					ui.Image(c, svg).Size(w, h).Fit(ui.Contain).Label(artifact.Source).Menu(func(menu *ui.Menu) {
						if menu.Item(tr(c, "Copy")).Chosen() {
							services.WriteClipboard(artifact.Source)
						}
					}).OnClick(func() {
						if kind == "diagram" {
							a.imageRef = artifact.AssetRef
							a.imageZoom = 1
							a.imageOpen = true
						}
					})
				})
			})
		} else {
			ui.Text(c, tr(c, "Preview unavailable")).TextColor(a.palette.Muted)
		}
	}
	key := "diagram-source:" + artifact.Source
	ui.Column(c).FillWidth().Radius(10).Border(1, a.palette.Border).Background(a.palette.Card).Children(func() {
		ui.Row(c).FillWidth().Padding(4, 10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, "mermaid")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			ui.Spacer(c)
			glyph, label := "file-code", "Show source"
			if a.expanded[key] {
				glyph, label = "eye", "Show diagram"
			}
			inlineIconButton(c, glyph, tr(c, label)).OnClick(func() { a.expanded[key] = !a.expanded[key] })
			inlineIconButton(c, "copy", tr(c, "Copy diagram source")).OnClick(func() { services.WriteClipboard(artifact.Source) })
		})
		ui.Column(c).FillWidth().Padding(12).Children(func() {
			if a.expanded[key] {
				ui.Text(c, artifact.Source).Font("Geist Mono").FontSize(float32(a.state.Settings.Appearance.CodeFontSize)).Selectable()
			} else {
				show()
			}
		})
	})
}
