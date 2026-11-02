package desktop

import (
	"math"
	"strings"
	"time"

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

// transcriptRowLayout is how one pane frames its rows: the main conversation
// centers them in the transcript width, a side pane fills its column.
type transcriptRowLayout struct {
	width   float32 // the centered width; 0 fills the pane
	padding float32 // horizontal padding inside the row
	nested  bool    // renders inside another pane's tab
}

// transcriptPane is where a transcript is shown: the projection cache its rows
// came from, the scope its selection and hover belong to, and the session the
// working trailer speaks for.
type transcriptPane struct {
	cache   *transcriptCache
	scope   string
	session string
}

// transcriptRowView renders one row of a transcript list: a user message, one
// projected block of an assistant entry, or an assistant entry that projected
// no rows. The list builds only the rows in view, so a row carries everything
// it shows: its share of the message, the message's footer when it is the
// entry's last row, and the pane's working trailer on the last row of all.
func (a *App) transcriptRowView(c *ui.Context, pane transcriptPane, entries []contracts.Message, rows []conversationRow, index int, layout transcriptRowLayout) {
	row := rows[index]
	var message contracts.Message
	if row.Message >= 0 {
		message = entries[row.Message]
	}
	previousSelection, previousSource := a.linkSelection, a.linkSource
	if selection := a.transcriptSelections[pane.scope]; selection != nil {
		a.linkSelection = selection
	}
	if message.SessionID != "" {
		a.linkSource = message.SessionID
	}
	// A row shows a block of a side pane's tab inside the pane that opened it.
	nested := a.nestedMarkdownTable
	a.nestedMarkdownTable = layout.nested
	defer func() {
		a.linkSelection, a.linkSource = previousSelection, previousSource
		a.nestedMarkdownTable = nested
	}()
	// The row keeps its identity by the list row's key, so hover and its
	// callbacks outlive a rebuild of the row's content.
	wrapper := ui.Row(c).Key("row:" + row.Key).FillWidth().Justify(ui.Center)
	// The framework polls for the pointer: the row under it claims the hover,
	// and the row that had it gives it up.
	if wrapper.Hovered() {
		a.hoverTranscriptRow(pane.scope, row.Key, true)
	} else if a.transcriptHover[pane.scope] == row.Key {
		a.hoverTranscriptRow(pane.scope, row.Key, false)
	}
	wrapper.Children(func() {
		column := ui.Column(c).FillWidth()
		if layout.width > 0 {
			column.MaxWidth(layout.width)
		}
		column.Padding(0, layout.padding).Children(func() {
			if row.TopGap > 0 {
				ui.Box(c).Height(row.TopGap).Shrink(0)
			}
			switch {
			case row.User:
				a.userMessage(c, message)
			case row.Message < 0:
				// The pane's working row holds only the trailer below.
			case row.Bare:
				// An assistant entry with no rows holds only its footer.
			default:
				block := a.transcriptRowAt(pane.cache, message, row.Block)
				if len(block.Activity) > 0 {
					a.activityGroup(c, block, message)
				} else {
					c.Key(block.ID)
					a.transcriptBlock(c, message, block.Block)
				}
			}
			if row.Last && !row.User && row.Message >= 0 && !message.Streaming {
				a.transcriptFooter(c, message, a.transcriptEntryHovered(pane.scope, message.ID))
			}
			if index == len(rows)-1 {
				a.workingTrailer(c, pane.session)
			}
		})
	})
}

// transcriptRowAt returns the projected row the list row shows. The projection
// is the one the list was built from; an entry the cache no longer holds is
// projected again, which keeps a row from rendering nothing.
func (a *App) transcriptRowAt(cache *transcriptCache, message contracts.Message, index int) transcriptRow {
	rows := cache.rowsOf(message.ID)
	if index < 0 || index >= len(rows) {
		rows = cache.rowsFor(message, a.state.Settings.General.CompactMode)
	}
	if index < 0 || index >= len(rows) {
		return transcriptRow{}
	}
	return rows[index]
}

// hoverTranscriptRow records the row the pointer is in. A message's rows are
// separate list items, so its footer reads back whether any of them holds the
// pointer, rather than one message element answering for all of them.
func (a *App) hoverTranscriptRow(pane, key string, over bool) {
	if a.transcriptHover == nil {
		a.transcriptHover = map[string]string{}
	}
	switch {
	case over:
		a.transcriptHover[pane] = key
	case a.transcriptHover[pane] == key:
		delete(a.transcriptHover, pane)
	}
}

// transcriptEntryHovered reports whether the pointer is over any row of the
// entry: a row's key is the entry's, or the entry's and its block's.
func (a *App) transcriptEntryHovered(pane, entryID string) bool {
	key := a.transcriptHover[pane]
	return key != "" && (key == entryID || strings.HasPrefix(key, entryID+":"))
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
	// Rows streamed into the group that is still receiving the message's
	// latest work arrive with the reveal; history and settled work render
	// whole. The sweep starts with the group and keeps the summary legible
	// while its work runs. A frame with animations disabled (restoring
	// state, fixtures) renders every row settled and marks it seen.
	reduce := a.reduceMotion(c)
	still := reduce || c.Instant()
	now := c.Now()
	streaming := message.Streaming && len(row.Activity) > 0 && len(message.Blocks) > 0 &&
		row.Activity[len(row.Activity)-1].ID == message.Blocks[len(message.Blocks)-1].ID
	headerStart, starts := a.activityRevealStarts(row.ID, row.Activity, streaming, streaming && !still, now)
	headerProgress := activityRevealProgress(headerStart, now, still, activityRowRevealDuration, activityRowRevealEase)
	shimmerPhase := float32(-1)
	if streaming && !still && title != "" {
		shimmerStart := ui.Local(root, "shimmer-start", func() time.Time { return now })
		shimmerPhase = activityShimmerPhase(*shimmerStart, now)
		c.After(activityShimmerFrame)
	}
	motions := make([]activityRowMotion, len(row.Activity))
	animating := headerProgress < 1
	for i := range motions {
		progress := activityRevealProgress(starts[i], now, still, activityRowRevealDuration, activityRowRevealEase)
		connector := activityRevealProgress(starts[i], now, still, activityConnectorDuration, activityConnectorEase)
		incoming, branch := activityConnectorParts(connector, i > 0)
		next := float32(0)
		if i+1 < len(starts) {
			next = activityRevealProgress(starts[i+1], now, reduce, activityConnectorDuration, activityConnectorEase)
		}
		motions[i] = activityRowMotion{
			progress: progress, content: branch, incoming: incoming,
			continuation: activityContinuation(next),
			settled:      progress >= 1 && connector >= 1,
		}
		animating = animating || !motions[i].settled
	}
	if animating && !still {
		c.After(activityMotionFrame)
	}
	root.Children(func() {
		parts := ui.CollapsibleBaseWithAnimation(c.Key("activity-toggle:"+row.ID), open, activityFoldAnimation)
		trigger := parts.Trigger.FillWidth().Justify(ui.Start).Label(tr(c, "Activity: ") + summary)
		if headerProgress < 1 {
			trigger.Height(activityHeaderHeight * headerProgress).ClipY().AlignItems(ui.Start)
		} else {
			trigger.Height(activityHeaderHeight).AlignItems(ui.Center)
		}
		trigger.OnClick(a.manualActivityFold(c, row.ID, open, ""))
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
			titleRow := ui.Row(c).Grow(1).MinWidth(0).Height(activityHeaderHeight).AlignItems(ui.Center)
			titleRow.Children(func() {
				text := ui.Text(c, title).FontSize(design.TextSize(c, design.TextBase)).TextColor(color).MinWidth(0).Shrink(1).SingleLine()
				// Compact mode shows the worked duration instead of the
				// counts. Anchor the tip to the visible label so it points at
				// that text rather than the middle of the full-width row, and
				// omit it where the header already shows the same summary.
				if title != summary {
					text = text.Tooltip(summary)
				}
			})
			if shimmerPhase >= 0 {
				a.activityShimmerTitle(c, titleRow, title, shimmerPhase)
			}
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
					a.activityItem(c, row.ID+":"+block.ID, block, i+1 < len(row.Activity), live, motions[i])
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

// activityRevealStarts returns the group header's and each activity row's
// reveal start time. Rows streamed into a live group animate in with a
// stagger; history and settled work render whole. A streaming group read
// with animations disabled (animate false) marks its rows seen without
// starting them, so a snapshot restored mid-stream never replays.
func (a *App) activityRevealStarts(rowID string, blocks []contracts.Block, streaming, animate bool, now time.Time) (time.Time, []time.Time) {
	if a.activityReveals == nil {
		a.activityReveals = map[string]time.Time{}
	}
	starts := make([]time.Time, len(blocks))
	headerStart := a.activityReveals[rowID]
	hadAny := !headerStart.IsZero()
	if !hadAny {
		for _, block := range blocks {
			if _, ok := a.activityReveals[rowID+":"+block.ID]; ok {
				hadAny = true
				break
			}
		}
	}
	if !streaming {
		// Keep a reveal that is still running alive, and drop the entries
		// once it settles so replayed history and old sessions accumulate
		// nothing.
		for i, block := range blocks {
			key := rowID + ":" + block.ID
			if start, ok := a.activityReveals[key]; ok {
				starts[i] = start
				if now.Sub(start) > activityRevealSettleBuffer {
					delete(a.activityReveals, key)
				}
			}
		}
		if !headerStart.IsZero() && now.Sub(headerStart) > activityRevealSettleBuffer {
			delete(a.activityReveals, rowID)
			headerStart = time.Time{}
		}
		return headerStart, starts
	}
	if !hadAny && animate {
		headerStart = now
		a.activityReveals[rowID] = headerStart
	}
	delay := activityFirstRowDelay
	if hadAny {
		delay = 0
	}
	arrival := 0
	for i, block := range blocks {
		key := rowID + ":" + block.ID
		if start, ok := a.activityReveals[key]; ok {
			starts[i] = start
			continue
		}
		if !animate {
			// Mark the row seen without animating it: it reached the view
			// in a frame that renders changes settled.
			a.activityReveals[key] = now.Add(-activityRevealSettleBuffer)
			continue
		}
		start := now.Add(delay + time.Duration(arrival)*activityRowStagger)
		a.activityReveals[key] = start
		starts[i] = start
		arrival++
	}
	return headerStart, starts
}

// activityShimmerTitle sweeps a light band across a live group's summary by
// repainting the shaped line through narrow moving clips, one strip at a
// time, so the label stays one run of text to shaping and to assistive
// technology.
func (a *App) activityShimmerTitle(c *ui.Context, el ui.Element, title string, phase float32) {
	family, size := c.Theme().Font, design.TextSize(c, design.TextBase)
	span := ui.Span{Text: title, Font: family, Size: size}
	advance, _ := c.MeasureText(0, span)
	if advance <= 0 {
		return
	}
	base, peak := a.palette.Muted, a.palette.Text
	el.DrawOver(func(p *ui.Painter, r ui.Rect) {
		width := min(advance, r.W)
		_, height := p.MeasureText(0, span)
		y := r.Y + (r.H-height)/2
		for x := float32(0); x < width; x += activityShimmerStrip {
			amount := activityShimmerAmount((x+activityShimmerStrip/2)/max(width, 1), phase)
			if amount <= 0.001 {
				continue
			}
			left, right := r.X+x, min(r.X+x+activityShimmerStrip, r.X+width)
			p.Clip(ui.Rect{X: left, Y: r.Y, W: right - left, H: r.H}, 0, func() {
				p.RichText(r.X, y, 0, ui.Span{Text: title, Font: family, Size: size, Color: base.Mix(peak, amount)})
			})
		}
	})
}

func (a *App) activityItem(c *ui.Context, id string, block contracts.Block, continues, live bool, motion activityRowMotion) {
	root := ui.Column(c).FillWidth().Key(id)
	open := ui.Local(root, "open", func() bool { return false })
	a.activityFold(id, open, false)
	label, detail, glyph := activityTitle(block)
	if block.Kind == "reasoning" && !*open {
		detail = reasoningPreview(block.Text, live)
	}
	color := a.palette.Muted
	if block.Tool != nil && block.Tool.Failed() {
		color = a.palette.Danger
	}
	root.Draw(func(p *ui.Painter, r ui.Rect) {
		if motion.settled {
			p.FillPath(activityRailPath(r.X, r.Y, r.H, continues), a.activityInk(.12))
			return
		}
		p.FillPath(activityRailRevealPath(r.X, r.Y, r.H, continues, motion.incoming, motion.content, motion.continuation), a.activityInk(.12))
	})
	root.Children(func() {
		parts := ui.CollapsibleBaseWithAnimation(c.Key("activity-item-toggle:"+id), open, activityFoldAnimation)
		accessibleDetail := firstNonempty(detail, block.ID)
		if block.Kind == "reasoning" {
			accessibleDetail = block.ID
		}
		trigger := parts.Trigger.FillWidth().Justify(ui.Start).Label(tr(c, label) + ": " + accessibleDetail)
		if motion.settled {
			trigger.Height(activityRowHeight).AlignItems(ui.Center)
		} else {
			// The row grows in while its content fades and lifts with the
			// branch leg of the slower connector draw.
			trigger.Height(activityRowHeight*motion.progress).ClipY().AlignItems(ui.Start).
				Opacity(motion.content).Margin(activityRevealLift*(1-motion.content), 0, 0, 0)
		}
		trigger.OnClick(a.manualActivityFold(c, id, open, strings.TrimSuffix(id, ":"+block.ID)))
		trigger.Children(func() {
			ui.Box(c).Width(activityGutter).Height(activityRowHeight).Shrink(0).Children(func() {
				ui.Row(c).Absolute().Left(activityIconLeft).Top(8).Size(activityIconSize, activityIconSize).TextColor(color).Children(func() { icon(c, glyph, activityIconSize) })
			})
			ui.Row(c).Grow(1).MinWidth(0).Height(30).Margin(0, 0, 0, activityTextGap).AlignItems(ui.Center).Gap(8).TextColor(color).Children(func() {
				text := color
				if trigger.Hovered() && (block.Tool == nil || !block.Tool.Failed()) {
					text = a.palette.Text
				}
				ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).TextColor(text).SingleLine().Shrink(0)
				if activityFile(block) {
					a.activityFileBadge(c, block.Tool.Path, color)
				} else if detail != "" {
					ui.Text(c, detail).MinWidth(0).Shrink(1).FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).TextColor(text).SingleLine()
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
				detail := block
				detail.ID = id
				a.activityDetail(c, detail, live)
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
		ui.Text(c, activityBasename(path)).SingleLine().FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).TextColor(color)
	})
}

func (a *App) activityDetail(c *ui.Context, block contracts.Block, live bool) {
	if block.Subagent != nil {
		ui.Text(c, block.Subagent.Task).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		ui.Text(c, block.Subagent.Summary).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		return
	}
	if block.Tool != nil && block.Tool.Diff != nil {
		tool := *block.Tool
		tool.ID = block.ID
		a.activityDiff(c, tool)
		return
	}
	a.activityTextDetail(c, block, live)
}

func (a *App) activityDiff(c *ui.Context, tool contracts.ToolCall) {
	change := *tool.Diff
	lineHeight := float32(a.state.Settings.Appearance.CodeFontSize) * 1.5
	container := ui.ScrollBoth(c.Key("activity-diff-viewport:"+tool.ID)).FillWidth().
		MaxHeight(activityDetailLines*lineHeight+12).Padding(6, 0).Label(tr(c, "Activity diff"))
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
	state.Font = ui.Font{Family: design.MonoFont(a.state.Settings.Appearance.CodeFont), Size: float32(a.state.Settings.Appearance.CodeFontSize)}
	state.LineHeight = 1.5
	state.Theme = &editor.Theme{Text: a.palette.Muted, Background: ui.Color{}, Added: a.palette.Success, Deleted: a.palette.Danger}
	container.Children(func() {
		source := detailSource(contracts.Block{Tool: &tool}, false)
		font := design.MonoFont(a.state.Settings.Appearance.CodeFont)
		size := float32(a.state.Settings.Appearance.CodeFontSize)
		for _, line := range detailLines(source) {
			ui.RichText(c, detailSpans(line, font, size, a.palette.Faint)...).
				SingleLine().Height(lineHeight).FontSize(size).Selectable()
		}
		editor.DiffView(c, state).FillWidth()
	})
}

type activityDiffCache struct {
	old, current string
	state        editor.DiffState
}

func (a *App) transcriptFooter(c *ui.Context, message contracts.Message, hovered bool) {
	services := c.Services()
	ui.Row(c).FillWidth().MinHeight(max(16, design.TextSize(c, design.TextSmall)*1.5)).Margin(8, 0, 0, 0).Gap(6).AlignItems(ui.Center).Children(func() {
		row := ui.Row(c).Gap(6).AlignItems(ui.Center)
		if !hovered {
			row.Opacity(0)
		}
		row.Children(func() {
			ui.Text(c, date(c, message.CreatedAt.Local(), "Jan 2, 3:04 PM")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Faint)
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
	return activityRailRevealPath(x, y, height, continues, 1, 1, 1)
}

// activityRailRevealPath draws the trunk, bend and branch ribbons
// progressively: incoming grows the trunk to the bend, continuation extends
// it toward the next arrival, and branch reveals the elbow and tip by
// distance along their path, so a changing branch length never speeds up.
func activityRailRevealPath(x, y, height float32, continues bool, incoming, branch, continuation float32) *ui.Path {
	var path ui.Path
	trunk := x + activityTrunkX
	bendY := y + activityRowHeight/2 - activityBend
	if incoming > 0 {
		bottom := y + (activityRowHeight/2-activityBend)*incoming
		if incoming >= 1 && continues && continuation > 0 {
			bottom = bendY + max(height-(activityRowHeight/2-activityBend), 0)*continuation
		}
		activityRibbon(&path, [][2]float32{{trunk, y}, {trunk, bottom}})
	}
	if branch > 0 {
		relative := activityBranchPoints(branch)
		points := make([][2]float32, len(relative))
		for i, point := range relative {
			points[i] = [2]float32{trunk + point[0], bendY + point[1]}
		}
		activityRibbon(&path, points)
	}
	return &path
}

func activityBranchPoints(progress float32) [][2]float32 {
	points := make([][2]float32, 0, 26)
	for i := 0; i <= 24; i++ {
		t := float32(i) / 24
		points = append(points, [2]float32{activityBend * t * t, activityBend * (2*t - t*t)})
	}
	points = append(points, [2]float32{activityBranchEnd - activityTrunkX, activityBend})
	if progress >= 1 {
		return points
	}
	lengths := make([]float32, len(points)-1)
	total := float32(0)
	for i := 1; i < len(points); i++ {
		dx, dy := points[i][0]-points[i-1][0], points[i][1]-points[i-1][1]
		lengths[i-1] = float32(math.Hypot(float64(dx), float64(dy)))
		total += lengths[i-1]
	}
	remaining := total * max(0, min(progress, 1))
	visible := make([][2]float32, 0, len(points))
	visible = append(visible, points[0])
	for i := 1; i < len(points) && remaining > 0; i++ {
		t := min(remaining/lengths[i-1], 1)
		visible = append(visible, [2]float32{
			points[i-1][0] + (points[i][0]-points[i-1][0])*t,
			points[i-1][1] + (points[i][1]-points[i-1][1])*t,
		})
		remaining -= lengths[i-1]
	}
	return visible
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
