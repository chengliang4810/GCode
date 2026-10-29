package desktop

import (
	"fmt"
	"math"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	activityHeaderHeight = float32(26)
	activityRowHeight    = float32(32)
	activityGutter       = float32(48)
	activityTextGap      = float32(8)
	activityTrunkX       = float32(12.5)
	activityBend         = float32(6)
	activityBranchEnd    = float32(28)
	activityIconLeft     = float32(32)
	activityIconSize     = float32(16)
	activityDetailLines  = 24
)

func (a *App) assistantTranscript(c *ui.Context, message contracts.Message) {
	rows := transcriptRows(message, a.state.Settings.General.CompactMode)
	root := ui.Column(c).FillWidth().Selectable()
	root.Children(func() {
		for i, row := range rows {
			gap := float32(8)
			if i > 0 && (len(row.Activity) > 0 || len(rows[i-1].Activity) > 0) {
				gap = 12
			}
			if i > 0 {
				ui.Box(c).Height(gap).Shrink(0)
			}
			if len(row.Activity) > 0 {
				a.activityGroup(c, row, message)
			} else {
				c.Key(row.ID)
				a.transcriptBlock(c, message, row.Block)
			}
		}
		if !message.Streaming {
			a.transcriptFooter(c, message, root.Hovered())
		}
	})
}

// Native CollapsibleBase supplies pointer/keyboard toggling, focus, expanded
// accessibility state and reduced-motion handling. Only the source-specific
// geometry, typography and connector rail are painted by the application.
func (a *App) activityGroup(c *ui.Context, row transcriptRow, message contracts.Message) {
	root := ui.Column(c).FillWidth().Key(row.ID)
	open := ui.Local(root, "open", func() bool { return row.AutoOpen })
	a.activityFold(row.ID, open, row.AutoOpen)
	summary := a.activitySummary(contracts.SummarizeActivity(row.Activity))
	title := summary
	if row.Compact && !message.Streaming {
		duration := message.DurationMS
		for _, run := range a.state.Runs {
			if run.ID == message.TurnID {
				duration = max(duration, run.DurationMS)
			}
		}
		if duration > 0 {
			title = a.workedFor(duration)
		}
	}
	root.Children(func() {
		parts := ui.CollapsibleBase(c, open)
		trigger := parts.Trigger.FillWidth().Height(activityHeaderHeight).AlignItems(ui.Center).Justify(ui.Start).Label(tr(c, "Activity: ") + summary).Tooltip(summary)
		trigger.OnClick(func() { a.rememberActivityFold(row.ID, *open) })
		trigger.Children(func() {
			ui.Box(c).Size(22, activityHeaderHeight).Shrink(0).Draw(func(p *ui.Painter, bounds ui.Rect) {
				if *open {
					p.Fill(ui.Rect{X: bounds.X + activityTrunkX - .5, Y: bounds.Y + activityHeaderHeight/2 + 6, W: 1, H: activityHeaderHeight/2 - 6}, a.activityInk(.12), 0)
				}
			})
			color := a.palette.Muted
			if trigger.Hovered() || trigger.FocusVisible() {
				color = a.palette.Text
			}
			ui.Text(c, title).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(color).Grow(1).SingleLine()
		})
		progress, tint := parts.Progress(), a.palette.Muted
		trigger.DrawOver(func(p *ui.Painter, bounds ui.Rect) { activityChevron(p, bounds.X+5.5, bounds.Y+6, 14, progress, tint) })
		parts.Panel(func() {
			// Source top pad continues the trunk instead of leaving a 2px gap.
			ui.Box(c).FillWidth().Height(2).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(ui.Rect{X: r.X + activityTrunkX - .5, Y: r.Y, W: 1, H: r.H}, a.activityInk(.12), 0)
			})
			for i, block := range row.Activity {
				live := message.Streaming && i == len(row.Activity)-1 && block.ID == message.Blocks[len(message.Blocks)-1].ID
				if block.Subagent != nil {
					ui.Column(c).FillWidth().Height(38).Padding(4, 0).Children(func() { a.transcriptSubagent(c, *block.Subagent) })
				} else {
					a.activityItem(c, row.ID+":"+block.ID, block, i+1 < len(row.Activity), live && !row.Compact)
				}
			}
		})
	})
}

func (a *App) activityFold(id string, open *bool, automatic bool) {
	if a.expanded["activity-explicit:"+id] {
		*open = a.expanded["activity-open:"+id]
	} else {
		*open = automatic
	}
}

func (a *App) rememberActivityFold(id string, open bool) {
	a.expanded["activity-explicit:"+id] = true
	a.expanded["activity-open:"+id] = open
}

func (a *App) activityItem(c *ui.Context, id string, block contracts.Block, continues, live bool) {
	root := ui.Column(c).FillWidth().Key(id)
	open := ui.Local(root, "open", func() bool { return live && (block.Kind == "reasoning" || block.Kind == "text") })
	a.activityFold(id, open, live && (block.Kind == "reasoning" || block.Kind == "text"))
	label, detail, glyph := activityTitle(block)
	color := a.palette.Muted
	if block.Tool != nil && block.Tool.Failed() {
		color = a.palette.Danger
	}
	root.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillPath(activityRailPath(r.X, r.Y, r.H, continues), a.activityInk(.12))
	})
	root.Children(func() {
		parts := ui.CollapsibleBase(c, open)
		trigger := parts.Trigger.FillWidth().Height(activityRowHeight).AlignItems(ui.Center).Justify(ui.Start).Label(tr(c, label) + ": " + firstNonempty(detail, block.ID))
		trigger.OnClick(func() { a.rememberActivityFold(id, *open) })
		trigger.Children(func() {
			ui.Box(c).Width(activityGutter).Height(activityRowHeight).Shrink(0).Children(func() {
				ui.Row(c).Absolute().Left(activityIconLeft).Top(8).Size(activityIconSize, activityIconSize).TextColor(color).Children(func() { icon(c, glyph, activityIconSize) })
			})
			ui.Row(c).Grow(1).MinWidth(0).Height(30).Margin(0, 0, 0, activityTextGap).AlignItems(ui.Center).Gap(8).TextColor(color).Children(func() {
				text := color
				if trigger.Hovered() && (block.Tool == nil || !block.Tool.Failed()) {
					text = a.palette.Text
				}
				ui.Text(c, tr(c, label)).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(text).SingleLine().Shrink(0)
				if activityFile(block) {
					a.activityFileBadge(c, block.Tool.Path, color)
				} else if detail != "" {
					ui.Text(c, detail).MinWidth(0).Shrink(1).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(text).SingleLine()
				}
				progress := parts.Progress()
				tile := ui.Box(c).Size(18, 18).Shrink(0)
				if !trigger.Hovered() && !trigger.FocusVisible() {
					tile.Opacity(0)
				}
				tile.Draw(func(p *ui.Painter, r ui.Rect) { activityChevron(p, r.X+3, r.Y+3, 12, progress, color) })
			})
		})
		parts.Panel(func() {
			ui.Column(c).FillWidth().Padding(0, 0, 1, activityGutter+activityTextGap).Gap(0).Children(func() {
				a.activityDetail(c, block, live)
			})
		})
	})
}

func (a *App) activityFileBadge(c *ui.Context, path string, color ui.Color) {
	file := contracts.File{Path: path, Name: activityBasename(path), Kind: "file"}
	for _, candidate := range a.state.Files {
		if candidate.ProjectID == a.projectID && strings.EqualFold(strings.ReplaceAll(candidate.Path, "\\", "/"), strings.ReplaceAll(path, "\\", "/")) {
			file = candidate
			break
		}
	}
	badge := ui.ButtonBase(c).Height(22).MinWidth(0).Shrink(1).Padding(0, 6, 0, 1).Gap(2).Radius(5).Background(a.activityInk(.06)).TextColor(color).AlignItems(ui.Center).Justify(ui.Start).Tooltip(path).Label(tr(c, "Open file: ") + path)
	if file.ID != "" {
		badge.OnClick(func() { a.workspace.OpenFile(file.ID) })
	}
	badge.Children(func() {
		ui.Row(c).Size(20, 20).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Icon(c, workbench.FileIcon(file, a.palette.Dark)).Size(14, 14) })
		ui.Text(c, activityBasename(path)).SingleLine().FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(color)
	})
}

func (a *App) activityDetail(c *ui.Context, block contracts.Block, live bool) {
	if block.Kind == "reasoning" || block.Kind == "text" {
		container := ui.Column(c).FillWidth().Padding(6, 0).Gap(0)
		cache := ui.Local(container, "thought", func() thoughtCache { return thoughtCache{} })
		if cache.text != block.Text || cache.live != live || cache.lines == nil {
			cache.text, cache.live = block.Text, live
			cache.lines, cache.truncated = thoughtLines(block.Text, live)
		}
		container.Children(func() {
			for _, line := range cache.lines {
				var spans []ui.Span
				for _, run := range line {
					span := ui.Span{Text: run.text, Color: a.palette.Faint, Italic: run.style.italic, Strikethrough: run.style.strike}
					if run.style.bold {
						span.Weight = 600
					}
					if run.style.code {
						span.Font = design.MonoFont(a.state.Settings.Appearance.CodeFont)
					}
					if run.style.link {
						span.Underline = true
					}
					spans = append(spans, span)
				}
				if len(spans) == 0 {
					spans = []ui.Span{{Text: " "}}
				}
				ui.RichText(c, spans...).SingleLine().FillWidth().Height(design.UISize(c, 18)).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(a.palette.Faint).Selectable()
			}
			a.activityTruncated(c, cache.truncated)
		})
		return
	}
	if block.Subagent != nil {
		ui.Text(c, block.Subagent.Task).FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted)
		ui.Text(c, block.Subagent.Summary).FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted)
		return
	}
	if block.Tool == nil {
		return
	}
	t := block.Tool
	invocation := firstNonempty(t.Command, t.Args, t.Path)
	if invocation != "" {
		ui.Column(c).FillWidth().Padding(6, 0).Gap(0).Children(func() {
			for _, line := range activityInvocation(invocation) {
				ui.Text(c, line).SingleLine().Height(design.UISize(c, 18)).Font(design.MonoFont(a.state.Settings.Appearance.CodeFont)).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(a.palette.Faint).Selectable()
			}
		})
	}
	if invocation != "" && (t.Diff != nil || t.Output != "") {
		ui.Box(c).Height(1).Shrink(0)
	}
	if t.Diff != nil {
		a.activityDiff(c, *t.Diff)
		return
	}
	if t.Output != "" {
		text, truncated := activityLines(t.Output, false, activityDetailLines)
		ui.Column(c).FillWidth().Padding(6, 0).Gap(0).Children(func() {
			for _, line := range strings.Split(text, "\n") {
				ui.Text(c, line).SingleLine().Height(design.UISize(c, 18)).Font(design.MonoFont(a.state.Settings.Appearance.CodeFont)).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(a.palette.Faint).Selectable()
			}
			a.activityTruncated(c, truncated)
		})
	}
}

func (a *App) activityTruncated(c *ui.Context, lines int) {
	if lines > 0 {
		ui.Text(c, fmt.Sprintf(tr(c, "… %d more lines"), lines)).FontSize(design.UISize(c, 12)).FixedLineHeight(design.UISize(c, 18)).TextColor(a.palette.Faint)
	}
}

func (a *App) activityDiff(c *ui.Context, change contracts.Change) {
	container := ui.Column(c).FillWidth().MaxHeight(600 * 18).Clip()
	cache := ui.Local(container, "diff", func() activityDiffCache {
		return activityDiffCache{old: change.OldContent, current: change.NewContent, state: editor.DiffState{Old: editor.NewBuffer(change.OldContent), New: editor.NewBuffer(change.NewContent), Layout: editor.DiffUnified, FitContent: true, Wrap: false, NoLineNumbers: true}}
	})
	if cache.old != change.OldContent {
		cache.old = change.OldContent
		cache.state.Old.Reset(change.OldContent)
	}
	if cache.current != change.NewContent {
		cache.current = change.NewContent
		cache.state.New.Reset(change.NewContent)
	}
	state := &cache.state
	state.Font = ui.Font{Family: design.MonoFont(a.state.Settings.Appearance.CodeFont), Size: design.UISize(c, 12)}
	state.LineHeight = 1.5
	state.Theme = &editor.Theme{Text: a.palette.Muted, Background: ui.Color{}, Added: a.palette.Success, Deleted: a.palette.Danger}
	container.Children(func() { editor.DiffView(c, state).FillWidth() })
}

type activityDiffCache struct {
	old, current string
	state        editor.DiffState
}

func (a *App) transcriptFooter(c *ui.Context, message contracts.Message, hovered bool) {
	services := c.Services()
	ui.Row(c).FillWidth().Height(16).Margin(8, 0, 0, 0).Gap(6).AlignItems(ui.Center).Children(func() {
		row := ui.Row(c).Gap(6).AlignItems(ui.Center)
		if !hovered {
			row.Opacity(0)
		}
		row.Children(func() {
			ui.Text(c, date(c, message.CreatedAt.Local(), "Jan 2, 3:04 PM")).FontSize(design.UISize(c, 11)).TextColor(a.palette.Faint)
			ui.ButtonBase(c).Size(16, 16).Label(tr(c, "Copy message")).Tooltip(tr(c, "Copy message")).AlignItems(ui.Center).Justify(ui.Center).OnClick(func() {
				var text []string
				for _, b := range message.Blocks {
					if b.Kind == "text" && b.Text != "" {
						text = append(text, b.Text)
					}
				}
				services.WriteClipboard(strings.Join(text, "\n\n"))
			}).Children(func() { icon(c, "copy", 12) })
		})
	})
}

func (a *App) activityInk(opacity float32) ui.Color {
	if a.palette.Dark {
		return ui.Hex("#ffffff").Alpha(opacity)
	}
	return ui.Hex("#000000").Alpha(opacity)
}

func activityChevron(p *ui.Painter, x, y, size, progress float32, color ui.Color) {
	angle := float64(progress) * math.Pi / 2
	s, co := float32(math.Sin(angle)), float32(math.Cos(angle))
	point := func(dx, dy float32) (float32, float32) { return x + size/2 + dx*co - dy*s, y + size/2 + dx*s + dy*co }
	var path ui.Path
	path.MoveTo(point(-size/8, -size/4)).LineTo(point(size/8, 0)).LineTo(point(-size/8, size/4))
	p.StrokePath(&path, 1.3, color)
}

// The source unions clockwise 1px ribbons to avoid double-alpha intersections.
// FillPath's nonzero contour fill gives the same single contribution at forks.
func activityRailPath(x, y, height float32, continues bool) *ui.Path {
	var path ui.Path
	trunk := x + activityTrunkX
	bendY := y + activityRowHeight/2 - activityBend
	bottom := bendY
	if continues {
		bottom = y + height
	}
	activityRibbon(&path, [][2]float32{{trunk, y}, {trunk, bottom}})
	points := make([][2]float32, 0, 26)
	for i := 0; i <= 24; i++ {
		t := float32(i) / 24
		points = append(points, [2]float32{trunk + activityBend*t*t, bendY + activityBend*(2*t-t*t)})
	}
	points = append(points, [2]float32{x + activityBranchEnd, y + activityRowHeight/2})
	activityRibbon(&path, points)
	return &path
}

func activityRibbon(path *ui.Path, points [][2]float32) {
	left, right := make([][2]float32, len(points)), make([][2]float32, len(points))
	for i, point := range points {
		a, b := points[max(0, i-1)], points[min(len(points)-1, i+1)]
		dx, dy := b[0]-a[0], b[1]-a[1]
		length := max(float32(math.Hypot(float64(dx), float64(dy))), .0001)
		nx, ny := -dy/length*.5, dx/length*.5
		left[i], right[i] = [2]float32{point[0] + nx, point[1] + ny}, [2]float32{point[0] - nx, point[1] - ny}
	}
	path.MoveTo(left[0][0], left[0][1])
	for _, point := range left[1:] {
		path.LineTo(point[0], point[1])
	}
	for i := len(right) - 1; i >= 0; i-- {
		path.LineTo(right[i][0], right[i][1])
	}
	path.Close()
}

func activityLines(text string, tail bool, limit int) (string, int) {
	lines := strings.Split(strings.TrimRight(text, "\r\n \t"), "\n")
	truncated := max(len(lines)-limit, 0)
	if truncated > 0 {
		if tail {
			lines = lines[truncated:]
		} else {
			lines = lines[:limit]
		}
	}
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) > 8192 {
			lines[i] = string(runes[:8192]) + "…"
		}
	}
	return strings.Join(lines, "\n"), truncated
}

func activityInvocation(text string) []string {
	var lines []string
	for _, raw := range strings.Split(text, "\n") {
		runes := []rune(raw)
		for len(runes) > 80 {
			lines = append(lines, string(runes[:80]))
			runes = runes[80:]
		}
		lines = append(lines, string(runes))
	}
	return lines
}
