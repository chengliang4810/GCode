package desktop

import (
	"fmt"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowRunTimeline draws the horizontal stage line: the rail with its
// lamps and arcs, then the station row under it with each station's name and
// the roster of instances that ran in it. The line scrolls sideways when the
// run has more stations than the card is wide; the scrollbar is the shared
// thin one, not the native bar.
func (a *App) workflowRunTimeline(c *ui.Context, view workflow.RunView, timeline workflow.Timeline, expanded bool, viewport float32) {
	railHeight := workflow.TrackRowHeight
	arcHeight := workflow.ArcBase + workflow.ArcGap*float32(maxArcLane(timeline))
	width := workflow.TrackWidth(len(timeline.Stations))
	if timeline.TailStub != "" {
		width += workflow.TailStubWidth
	}
	state, position := a.workflowTimelineScroll(view.RunID)
	// 滚到视口外的站折到那一侧的边檐上：同一枚小灯、点一下把它带回来。
	fold := a.workflowTimelineFold(timeline, state, viewport)
	ui.Column(c.Key("workflow-timeline:" + view.RunID)).FillWidth().Gap(0).Children(func() {
		if fold.Folded() {
			a.workflowTimelineLedge(c, view, timeline, fold, state)
		}
		ui.ScrollHorizontal(c.Key("workflow-timeline-scroll:" + view.RunID)).TrackScroll(state).
			Scrollbars(false).FillWidth().MinWidth(0).Children(func() {
			ui.Column(c).Width(width).Shrink(0).Gap(0).Children(func() {
				// 轨道行只画轨道、灯和弧，不装内容：名字与名册落在它下面的站台行，
				// 两张面因此始终对齐，也不会压到卡上的其他部分。
				ui.Box(c.Key("workflow-timeline-rail:" + view.RunID)).FillWidth().Height(arcHeight + railHeight).
					Draw(func(painter *ui.Painter, bounds ui.Rect) {
						a.workflowTimelineRails(painter, bounds, timeline, railHeight, arcHeight)
					})
				ui.Row(c.Key("workflow-timeline-platform:" + view.RunID)).FillWidth().Gap(workflow.StationGap).
					AlignItems(ui.Start).Children(func() {
					for i, station := range timeline.Stations {
						a.workflowTimelineStation(c, view, station, expanded, i == len(timeline.Stations)-1 && timeline.TailStub != "")
					}
				})
			})
		})
		if state.MaxX > 1 {
			a.workflowTimelineScrollbar(c, view, state, position, viewport)
		}
	})
}

// workflowTimelineFold reports which stations have scrolled out of the stage
// line's viewport. Without a measured viewport nothing folds.
func (a *App) workflowTimelineFold(timeline workflow.Timeline, state *ui.ScrollState, viewport float32) workflow.TimelineFold {
	if viewport <= 0 || len(timeline.Stations) == 0 {
		return workflow.TimelineFold{}
	}
	lamps := make([]float32, len(timeline.Stations))
	for i, station := range timeline.Stations {
		lamps[i] = workflow.StationCenterX(station.Index)
	}
	return workflow.FoldStations(lamps, state.X, viewport, -1, 0)
}

// workflowTimelineLedge draws the folded stations of both sides: up to three
// lamps each, then a count, and choosing one scrolls that station back into
// view. It is the stage line's own band, so it never covers the rail.
func (a *App) workflowTimelineLedge(c *ui.Context, view workflow.RunView, timeline workflow.Timeline, fold workflow.TimelineFold, state *ui.ScrollState) {
	ui.Row(c.Key("workflow-timeline-ledge:" + view.RunID)).FillWidth().Height(workflow.LedgePitch).
		AlignItems(ui.Center).Children(func() {
		a.workflowTimelineLedgeSide(c, view, timeline, fold.Left, workflow.LedgeLeft, state)
		ui.Spacer(c).Grow(1)
		a.workflowTimelineLedgeSide(c, view, timeline, fold.Right, workflow.LedgeRight, state)
	})
}

// workflowTimelineLedgeSide draws one side's lamps and its count.
func (a *App) workflowTimelineLedgeSide(c *ui.Context, view workflow.RunView, timeline workflow.Timeline, indexes []int, side string, state *ui.ScrollState) {
	shown, more := workflow.LedgeLamps(indexes, side)
	if len(shown) == 0 && more == 0 {
		return
	}
	ui.Row(c.Key("workflow-timeline-ledge-" + side + ":" + view.RunID)).Gap(workflow.LedgePitch - workflow.LedgeLamp).
		Height(workflow.LedgePitch).AlignItems(ui.Center).Children(func() {
		for _, index := range shown {
			station := timeline.Stations[index]
			ui.ButtonBase(c.Key("workflow-timeline-ledge-lamp:"+view.RunID+":"+itoa(index))).
				Size(workflow.LedgeLamp, workflow.LedgeLamp).Shrink(0).Padding(0).Radius(workflow.LedgeLamp / 2).
				Background(a.workflowTimelineLedgeColor(station)).
				Label(tr(c, "Scroll to") + " " + station.Name).
				Tooltip(station.Name).OnClick(func() {
				// 把那一站带回视口：灯心落在视口靠这一侧的边距上。
				target := workflow.StationCenterX(station.Index) - workflow.LedgeFade
				if side == workflow.LedgeRight {
					target = workflow.StationCenterX(station.Index) - a.chatWidth + workflow.LedgeFade
				}
				state.ScrollTo(max(0, target), 0, ui.Smooth)
			})
		}
		if more > 0 {
			ui.Text(c, fmt.Sprintf("+%d", more)).FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.palette.Muted).FontFeatures("tnum").SingleLine()
		}
	})
}

// workflowTimelineLedgeColor is a folded station's lamp color: the same one it
// wears on the rail, so the ledge is not a second legend.
func (a *App) workflowTimelineLedgeColor(station workflow.TimelineStation) ui.Color {
	if station.Kind == workflow.StationKindHole && station.Status != contracts.WorkflowHoleFilled {
		return a.palette.Readable(a.palette.Warning)
	}
	if station.Status == contracts.WorkflowPhasePending {
		return a.palette.Muted
	}
	return a.workflowPhaseLampColor(station.Status)
}

// itoa keeps the lamp key stable without pulling strconv into this file.
func itoa(value int) string { return fmt.Sprintf("%d", value) }

// workflowTimelineScroll returns the scroll state of one run card's stage
// line, creating it on first use.
func (a *App) workflowTimelineScroll(runID string) (*ui.ScrollState, *float64) {
	if a.workflowUI.scroll == nil {
		a.workflowUI.scroll = map[string]*ui.ScrollState{}
		a.workflowUI.position = map[string]*float64{}
	}
	state := a.workflowUI.scroll[runID]
	if state == nil {
		state = &ui.ScrollState{}
		a.workflowUI.scroll[runID] = state
	}
	position := a.workflowUI.position[runID]
	if position == nil {
		position = new(float64)
		a.workflowUI.position[runID] = position
	}
	return state, position
}

// workflowTimelineScrollbar draws the shared thin scrollbar under a stage
// line that does not fit, so a long run stays reachable by drag and by the
// keyboard.
func (a *App) workflowTimelineScrollbar(c *ui.Context, view workflow.RunView, state *ui.ScrollState, position *float64, viewport float32) {
	state.X = clampTableScroll(state.X, state.MaxX)
	if viewport <= 0 {
		return
	}
	*position = float64(state.X)
	controlHorizontalScrollbar(c.Key("workflow-timeline-scrollbar:"+view.RunID), position, state.MaxX, viewport).
		FillWidth().Label(tr(c, "Scroll the workflow stages")).OnChange(func() {
		state.X = float32(*position)
	})
}

// maxArcLane is the highest arc lane of the timeline.
func maxArcLane(timeline workflow.Timeline) int {
	highest := 0
	for _, arc := range timeline.Arcs {
		if arc.Lane > highest {
			highest = arc.Lane
		}
	}
	return highest
}

// workflowTimelineRails paints the rail, the station lamps, and the arcs.
func (a *App) workflowTimelineRails(painter *ui.Painter, bounds ui.Rect, timeline workflow.Timeline, railHeight, arcHeight float32) {
	p := a.palette
	rail := p.Border
	baseline := bounds.Y + arcHeight + railHeight/2
	// 轨道按段画：触到开着或等着补全的留白那一段是虚线——那里还不是代码。
	for i := 0; i+1 < len(timeline.Stations); i++ {
		from := workflow.StationCenterX(timeline.Stations[i].Index)
		to := workflow.StationCenterX(timeline.Stations[i+1].Index)
		if to <= from {
			continue
		}
		path := (&ui.Path{}).MoveTo(bounds.X+from, baseline).LineTo(bounds.X+to, baseline)
		if timeline.Stations[i].HoleOpen() || timeline.Stations[i+1].HoleOpen() {
			path.Dash(0, 5, 4)
		}
		painter.StrokePath(path, 1, rail)
	}
	a.workflowTimelineTailStub(painter, bounds, timeline, baseline)
	for _, station := range timeline.Stations {
		center := bounds.X + workflow.StationCenterX(station.Index)
		a.workflowTimelineLamp(painter, center, baseline, station)
	}
	for _, arc := range timeline.Arcs {
		a.workflowTimelineArc(painter, bounds, timeline, arc, baseline)
	}
}

// workflowTimelineTailStub paints the fading stub after a tail hole: the
// script's ending is not written yet.
func (a *App) workflowTimelineTailStub(painter *ui.Painter, bounds ui.Rect, timeline workflow.Timeline, baseline float32) {
	if timeline.TailStub != workflow.TailStubOpen || len(timeline.Stations) == 0 {
		return
	}
	last := timeline.Stations[len(timeline.Stations)-1]
	start := bounds.X + workflow.StationCenterX(last.Index) + 10
	end := start + workflow.TailStubWidth
	path := (&ui.Path{}).MoveTo(start, baseline).LineTo(end, baseline).Dash(0, 5, 4)
	painter.StrokePathGradient(path, 1, ui.LinearGradient{
		Angle: 90,
		From:  a.palette.Border,
		To:    a.palette.Border.Alpha(0),
		Start: 0,
		End:   1,
	})
}

// workflowTimelineLamp paints one station's lamp. A hole that is not filled
// yet is an empty dashed ring: there is no code there to light up.
func (a *App) workflowTimelineLamp(painter *ui.Painter, center, baseline float32, station workflow.TimelineStation) {
	if station.Kind == workflow.StationKindHole {
		a.workflowHoleLamp(painter, center, baseline, station)
		return
	}
	lamp := a.workflowPhaseLampColor(station.Status)
	if station.Status == contracts.WorkflowPhasePending {
		painter.StrokePath((&ui.Path{}).Circle(center, baseline, 4), 1, a.palette.Border)
		return
	}
	painter.FillPath((&ui.Path{}).Circle(center, baseline, 4), lamp)
}

// workflowHoleLamp paints the lamp of a hole: a dashed empty ring while the
// hole waits, a steady one once it is filled.
func (a *App) workflowHoleLamp(painter *ui.Painter, center, baseline float32, station workflow.TimelineStation) {
	if station.Status == contracts.WorkflowHoleFilled {
		painter.FillPath((&ui.Path{}).Circle(center, baseline, 4), a.palette.Readable(a.palette.Success))
		return
	}
	// 等着的留白带一圈常亮的光晕：那里没有东西在跑，run 在等主代理写代码。
	painter.StrokePath((&ui.Path{}).Circle(center, baseline, 5.5), 3, a.palette.Readable(a.palette.Warning).Alpha(.2))
	painter.StrokePath((&ui.Path{}).Circle(center, baseline, 4).Dash(0, 3, 2), 1.5, a.palette.Readable(a.palette.Warning))
}

// workflowTimelineArc paints one arc above the rail: a loop comes back to an
// earlier station, a skip jumps over stations.
func (a *App) workflowTimelineArc(painter *ui.Painter, bounds ui.Rect, timeline workflow.Timeline, arc workflow.TimelineArc, baseline float32) {
	if arc.From < 0 || arc.To < 0 || arc.From >= len(timeline.Stations) || arc.To >= len(timeline.Stations) {
		return
	}
	from := bounds.X + workflow.StationCenterX(arc.From)
	to := bounds.X + workflow.StationCenterX(arc.To)
	lift := float32(arc.Lane+1) * workflow.ArcGap
	top := baseline - workflow.ArcBase - lift
	path := (&ui.Path{}).MoveTo(from, baseline).QuadTo((from+to)/2, top, to, baseline)
	painter.StrokePath(path, 1, a.palette.Border)
}

// workflowPhaseLampColor is the color of one station's lamp.
func (a *App) workflowPhaseLampColor(status string) ui.Color {
	switch status {
	case contracts.WorkflowPhaseRunning:
		return a.palette.Readable(a.palette.Warning)
	case contracts.WorkflowPhaseDone:
		return a.palette.Readable(a.palette.Success)
	case contracts.WorkflowPhaseFailed:
		return a.palette.Readable(a.palette.Danger)
	default:
		return a.palette.Muted
	}
}

// workflowTimelineStation draws one station: its lamp, its name, and the
// roster of instances that ran in it. The last station of a run with a tail
// hole is wider: the fading stub needs the room.
func (a *App) workflowTimelineStation(c *ui.Context, view workflow.RunView, station workflow.TimelineStation, expanded bool, tail bool) {
	width := workflow.StationWidth
	if tail {
		width += workflow.TailStubWidth
	}
	ui.Column(c.Key("workflow-station:" + view.RunID + ":" + workflowStationKey(station))).Width(width).Shrink(0).Gap(workflow.PillGap).Children(func() {
		ui.Row(c).Height(workflow.PlatformRowHeight).Gap(6).AlignItems(ui.Center).Children(func() {
			// 名字先让位，留白的提示词保持完整：被截断的是名字，不是状态。
			ui.Text(c, station.Name).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
			a.workflowStationHoleMeta(c, station)
		})
		if station.Kind == workflow.StationKindHole {
			return
		}
		if !expanded {
			return
		}
		for _, entry := range station.Roster.Entries {
			a.workflowAgentPill(c, view, station, entry)
		}
		if station.Roster.Overflow > 0 {
			ui.Text(c, tr(c, "and %d more", station.Roster.Overflow)).
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
		}
	})
}

// workflowStationHoleMeta draws what a hole says next to its name: the
// waiting word while it waits, the pen mark once it is filled.
func (a *App) workflowStationHoleMeta(c *ui.Context, station workflow.TimelineStation) {
	if station.Kind != workflow.StationKindHole {
		return
	}
	if station.Status == contracts.WorkflowHoleFilled {
		icon(c, "pen", design.Controls().IconSize)
		return
	}
	ui.Text(c, tr(c, "Waiting for fill")).
		FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Readable(a.palette.Warning)).SingleLine()
}

// workflowStationKey is the stable identity of one station: a phase station
// is its phase, a hole station is its hole.
func workflowStationKey(station workflow.TimelineStation) string {
	if station.Kind == workflow.StationKindHole {
		return "hole:" + station.HoleID
	}
	return station.PhaseID
}

// workflowAgentPill draws one instance of a station. A pill with a session
// opens that instance's transcript; a pill without one says so.
func (a *App) workflowAgentPill(c *ui.Context, view workflow.RunView, station workflow.TimelineStation, entry workflow.RosterEntry) {
	actor := workflowActorOf(view, station.PhaseID, entry.ActorID)
	label := entry.Name
	if label == "" {
		label = entry.ActorID
	}
	pill := ui.ButtonBase(c.Key("workflow-pill:"+view.RunID+":"+entry.ActorID)).
		Height(workflow.PillHeight).Padding(0, 10).Radius(999).Gap(6).AlignItems(ui.Center).
		Border(1, a.palette.Border).Background(a.palette.Background).
		Label(label).TextColor(a.palette.Text)
	dot := a.workflowActorDotColor(entry.Status)
	pill.Children(func() {
		ui.Box(c).Size(8, 8).Shrink(0).Draw(func(painter *ui.Painter, bounds ui.Rect) {
			painter.FillPath((&ui.Path{}).Circle(bounds.X+bounds.W/2, bounds.Y+bounds.H/2, bounds.W/2), dot)
		})
		ui.Text(c, label).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
	})
	if actor != nil && actor.SessionID != "" {
		pill.OnClick(func() { a.workspace.OpenSession(actor.SessionID) })
	} else {
		pill.Disabled(true)
	}
}

// workflowActorDotColor is the color of an instance's dot.
func (a *App) workflowActorDotColor(status string) ui.Color {
	switch status {
	case contracts.WorkflowActorRunning:
		return a.palette.Readable(a.palette.Warning)
	case contracts.WorkflowActorDone:
		return a.palette.Readable(a.palette.Success)
	case contracts.WorkflowActorFailed:
		return a.palette.Readable(a.palette.Danger)
	case contracts.WorkflowActorAsking:
		return a.palette.Readable(a.palette.Warning)
	default:
		return a.palette.Muted
	}
}

// workflowActorOf finds one instance inside a phase of the projection.
func workflowActorOf(view workflow.RunView, phaseID, actorID string) *workflow.RunActorView {
	for i := range view.Phases {
		phase := &view.Phases[i]
		if phase.ID != phaseID {
			continue
		}
		for j := range phase.Actors {
			if phase.Actors[j].ID == actorID {
				return &phase.Actors[j]
			}
		}
	}
	return nil
}
