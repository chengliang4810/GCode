package desktop

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type transcriptTableState struct {
	id               string
	source           string
	root, frame      ui.Handle
	scroll, preview  ui.ScrollState
	widths           []float32
	measurement      ui.Rect
	expanded, opened bool
	position         float64
	lastScroll       float32
}

func (a *App) transcriptMarkdownBlock(c *ui.Context, document *parser.Document, index int) bool {
	if a.transcriptTableBlock(c, document, index) {
		return true
	}
	if a.state.Settings.Appearance.RenderMath && a.mathMarkdownBlock(c, document, index) {
		return true
	}
	return a.linkMarkdownBlock(c, document, index, a.md.Theme)
}

func (a *App) transcriptTableBlock(c *ui.Context, document *parser.Document, index int) bool {
	if document.Block(index).Kind != parser.Table {
		return false
	}
	table := readTranscriptTable(document, index)
	root := ui.Column(c.Key(fmt.Sprintf("markdown-table:%d", document.Block(index).Start))).FillWidth().MinWidth(0).Shrink(0).Gap(8).
		Label(tr(c, "Markdown table"))
	state := ui.Local(root, "state", func() transcriptTableState { return transcriptTableState{} })
	state.id = strconv.FormatUint(root.ID(), 10)
	root.Bind(&state.root)
	bounds := state.root.Bounds(c)
	source := document.Source()[document.Block(index).Start:document.Block(index).End]
	if bounds.W <= 0 || state.measurement != bounds || state.source != source {
		state.measurement = bounds
		state.source = source
		c.After(time.Millisecond)
	}
	windowWidth, _ := c.Size()
	width := bounds.W
	if width <= 0 {
		width = max(64, min(736, windowWidth-64))
	}
	natural := a.measureTranscriptTable(c, table, state)
	viewport := a.conversationViewport.Bounds(c)
	if viewport.W <= 0 {
		viewport = ui.Rect{W: windowWidth}
	}
	reserve, _ := a.infoVisibility()
	viewport.W = max(float32(0), viewport.W-reserve)
	left, expandedWidth := tableExpandedGeometry(bounds, viewport)
	canExpand := !a.nestedMarkdownTable && sumTableWidths(natural) > width+1 && expandedWidth > width+16
	drawWidth := width
	if state.expanded && canExpand {
		drawWidth = expandedWidth
	} else {
		left = 0
	}
	widths := fitTranscriptTableWidths(natural, drawWidth)
	root.Children(func() {
		a.transcriptTableActions(c, table, state, canExpand)
		ui.Column(c).Width(drawWidth).Shrink(0).Margin(0, 0, 0, -left).Children(func() {
			frame := ui.Column(c.Key(state.key("table-frame"))).Bind(&state.frame).FillWidth().MinWidth(0).Shrink(0).
				Radius(12).Border(1, a.palette.Border).Background(a.palette.Background).Clip().Role(ui.RoleTable).Label(tr(c, "Table frame"))
			frame.Children(func() {
				scroll := ui.ScrollHorizontal(c.Key(state.key("table-content"))).TrackScroll(&state.scroll).Scrollbars(false).Shrink(0).
					FillWidth().Label(tr(c, "Table contents"))
				scroll.Children(func() {
					a.transcriptTableRows(c, table, widths, true)
				})
			})
			a.tableEdgeShadows(frame, state.scroll.X, state.scroll.MaxX)
			sticky, y := a.tableStickyScrollbar(c, state)
			sticky = sticky && !a.nestedMarkdownTable
			if state.scroll.MaxX > 1 {
				if sticky && !state.opened {
					// Keep its normal slot, so scrolling cannot change message height.
					ui.Box(c).Height(design.Controls().ScrollbarHeight)
					ui.Overlay(c, func() {
						ui.Column(c.Key(state.key("table-sticky-scrollbar"))).Absolute().Left(bounds.X - left).Top(y).Width(drawWidth).
							Background(a.palette.Background).Children(func() { a.transcriptTableScrollbar(c, state, drawWidth) })
					})
				} else {
					a.transcriptTableScrollbar(c, state, drawWidth)
				}
			}
		})
	})
	a.transcriptTablePreview(c, table, state, natural)
	return true
}

func (state *transcriptTableState) key(name string) string { return name + ":" + state.id }

func tableExpandedGeometry(frame, viewport ui.Rect) (left, width float32) {
	if frame.W <= 0 || viewport.W <= 0 {
		return 0, frame.W
	}
	left = max(0, frame.X-viewport.X-16)
	right := max(0, viewport.X+viewport.W-frame.X-frame.W-16)
	return left, frame.W + left + right
}

func (a *App) measureTranscriptTable(c *ui.Context, table transcriptTableData, state *transcriptTableState) []float32 {
	widths := make([]float32, len(table.align))
	if table.live {
		copy(widths, state.widths)
	}
	font, size := design.Font(a.state.Settings.Appearance.Font), design.TextSize(c, design.TextBase)
	for _, row := range table.rows {
		for col, cell := range row {
			width, _ := c.MeasureText(0, ui.Span{Text: cell.text, Font: font, Size: size})
			widths[col] = max(widths[col], min(float32(448), max(float32(64), width+24)))
		}
	}
	state.widths = append(state.widths[:0], widths...)
	return widths
}

func sumTableWidths(widths []float32) float32 {
	var total float32
	for _, width := range widths {
		total += width
	}
	return total
}

func fitTranscriptTableWidths(natural []float32, available float32) []float32 {
	widths := append([]float32(nil), natural...)
	total := sumTableWidths(widths)
	if total > 0 && total < available-2 {
		for i := range widths {
			widths[i] *= (available - 2) / total
		}
	}
	return widths
}

func (a *App) transcriptTableRows(c *ui.Context, table transcriptTableData, widths []float32, header bool) {
	ui.Column(c).Width(sumTableWidths(widths)).Shrink(0).Children(func() {
		for i, row := range table.rows {
			if i == 0 && !header {
				continue
			}
			c.Key(i)
			a.transcriptTableRow(c, row, widths, table.align, i == 0, i == len(table.rows)-1)
		}
	})
}

func (a *App) transcriptTableRow(c *ui.Context, cells []transcriptTableCell, widths []float32, aligns []parser.Align, header, last bool) {
	row := ui.Row(c).Width(sumTableWidths(widths)).Shrink(0).AlignItems(ui.Stretch).Role(ui.RoleRow)
	if !last {
		row.BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border)
	}
	if row.Hovered() && !header {
		row.Background(a.palette.Hover.Alpha(.2))
	}
	theme := *a.md.Theme
	if header {
		theme.Text, theme.Strong, theme.Link = a.palette.Muted, a.palette.Muted, a.palette.Muted
	}
	sourceSession := a.linkSource
	options := &markdown.Options{Theme: &theme, Extensions: a.md.Extensions, ShareSelection: true,
		OnLink: func(raw string) { a.openTranscriptLink(sourceSession, raw) }, LoadImage: a.md.LoadImage, Math: a.md.Math}
	options.Block = func(c *ui.Context, document *parser.Document, index int) bool {
		previous := a.linkSource
		a.linkSource = sourceSession
		defer func() { a.linkSource = previous }()
		if a.state.Settings.Appearance.RenderMath && a.mathMarkdownBlock(c, document, index) {
			return true
		}
		return a.linkMarkdownBlock(c, document, index, &theme)
	}
	row.Children(func() {
		for col, cell := range cells {
			c.Key(col)
			role, color := ui.RoleCell, a.palette.Text
			if header {
				role, color = ui.RoleColumnHeader, a.palette.Muted
			}
			box := ui.Column(c).Width(widths[col]).Shrink(0).Padding(8, 12).TextColor(color).Role(role).Label(cell.text)
			align := ui.Start
			if aligns[col] == parser.AlignCenter {
				align = ui.Center
			} else if aligns[col] == parser.AlignRight {
				align = ui.End
			}
			box.Children(func() {
				markdown.View(c, cell.markdown, options).FillWidth().TextAlign(align)
			})
		}
	})
}

func (a *App) transcriptTableActions(c *ui.Context, table transcriptTableData, state *transcriptTableState, expandable bool) {
	services := c.Services()
	ui.Row(c).FillWidth().Justify(ui.End).Gap(2).Children(func() {
		iconButton(c.Key(state.key("table-copy")), "copy", tr(c, "Copy table as Markdown")).OnClick(func() {
			services.WriteClipboard(table.markdownText())
			a.notification(a.t("Table copied"))
		})
		iconButton(c.Key(state.key("table-export")), "arrow-down", tr(c, "Download table as CSV")).OnClick(func() { a.exportTranscriptTable(table) })
		iconButton(c.Key(state.key("table-preview")), "window-maximize", tr(c, "Expand table")).OnClick(func() {
			state.opened = true
			state.preview.X, state.preview.Y = 0, 0
		})
		if expandable {
			label, glyph := "Use conversation width", "arrow-right"
			if state.expanded {
				label, glyph = "Restore table width", "arrow-left"
			}
			iconButton(c.Key(state.key("table-expand-width")), glyph, tr(c, label)).OnClick(func() {
				state.expanded = !state.expanded
				state.measurement = ui.Rect{}
			})
		}
	})
}

func (a *App) tableEdgeShadows(frame ui.Element, offset, maximum float32) {
	color := a.palette.Text.Alpha(.1)
	frame.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if offset > 1 {
			p.FillGradient(ui.Rect{X: r.X + 1, Y: r.Y + 1, W: 14, H: max(0, r.H-2)},
				ui.LinearGradient{Angle: 0, From: color, To: color.Alpha(0)}, 0)
		}
		if maximum-offset > 1 {
			p.FillGradient(ui.Rect{X: r.X + r.W - 15, Y: r.Y + 1, W: 14, H: max(0, r.H-2)},
				ui.LinearGradient{Angle: 0, From: color.Alpha(0), To: color}, 0)
		}
	})
}

func (a *App) tableStickyScrollbar(c *ui.Context, state *transcriptTableState) (bool, float32) {
	bounds := state.frame.Bounds(c)
	viewport := a.conversationViewport.Bounds(c)
	if viewport.H <= 0 || bounds.H <= 0 {
		return false, 0
	}
	bottom := viewport.Y + viewport.H - 16
	if composer := a.composerSurface.Bounds(c); composer.H > 0 {
		bottom = min(bottom, composer.Y-8)
	}
	height := design.Controls().ScrollbarHeight
	return bounds.Y < bottom-height && bounds.Y+bounds.H > bottom && bounds.H > bottom-viewport.Y,
		bottom - height
}

func (a *App) transcriptTableScrollbar(c *ui.Context, state *transcriptTableState, width float32) {
	state.scroll.X = clampTableScroll(state.scroll.X, state.scroll.MaxX)
	if state.scroll.X != state.lastScroll {
		state.position = float64(state.scroll.X)
	}
	state.lastScroll = state.scroll.X
	controlHorizontalScrollbar(c.Key(state.key("table-horizontal-scroll")), &state.position, state.scroll.MaxX, width).
		FillWidth().Label(tr(c, "Scroll table horizontally")).OnChange(func() {
		state.scroll.X = float32(state.position)
		state.lastScroll = state.scroll.X
	})
}

func (a *App) transcriptTablePreview(c *ui.Context, table transcriptTableData, state *transcriptTableState, natural []float32) {
	ui.DialogBase(c.Key(state.key("table-preview-dialog")), &state.opened, func(backdrop, panel ui.Element) {
		width, height := c.Size()
		backdrop.Background(ui.RGBA(0, 0, 0, .45))
		panel.Width(max(0, width-48)).Height(max(0, height-48)).Padding(16).Gap(12).Radius(12).
			Background(a.palette.Background).Border(1, a.palette.Border).Label(tr(c, "Table preview"))
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, tr(c, "Table preview")).Grow(1).FontWeight(600)
			iconButton(c.Key(state.key("table-preview-close")), "close", tr(c, "Close table preview")).OnClick(func() { state.opened = false })
		})
		widths := fitTranscriptTableWidths(natural, width-82)
		ui.Column(c).FillWidth().Grow(1).MinHeight(0).Border(1, a.palette.Border).Radius(12).Clip().Role(ui.RoleTable).Children(func() {
			if len(table.rows) > 0 {
				ui.Column(c.Key(state.key("table-preview-header"))).FillWidth().ClipX().Label(tr(c, "Table preview header")).Children(func() {
					ui.Column(c).Width(sumTableWidths(widths)).Shrink(0).Margin(0, 0, 0, -state.preview.X).Children(func() {
						a.transcriptTableRow(c, table.rows[0], widths, table.align, true, false)
					})
				})
			}
			ui.ScrollBoth(c.Key(state.key("table-preview-content"))).TrackScroll(&state.preview).FillWidth().Grow(1).MinHeight(0).
				Label(tr(c, "Table preview contents")).Children(func() { a.transcriptTableRows(c, table, widths, false) })
		})
	})
}

func (a *App) exportTranscriptTable(table transcriptTableData) {
	window, choose, title := a.win, a.chooseTableExportPath, a.t("Download table as CSV")
	content := []byte(table.csvText())
	a.backgroundTask(func() func() {
		var path string
		var err error
		if choose != nil {
			path, err = choose()
		} else if window != nil {
			path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: window, Title: title, DefaultPath: "table.csv",
				Filters: []mygo.FileFilter{{Name: "CSV", Extensions: []string{"csv"}}}})
		}
		if err == nil && path != "" {
			err = os.WriteFile(path, content, 0600)
		}
		return func() {
			if err != nil {
				a.errorText = err.Error()
			} else if path != "" {
				a.notification(a.t("Table exported"))
			}
		}
	})
}

// Keep floating point scroll positions finite when content shrinks mid-stream.
func clampTableScroll(value, maximum float32) float32 {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return 0
	}
	return min(max(float32(0), value), max(float32(0), maximum))
}
