package workflow

import (
	"sort"

	"gcode/internal/contracts"
)

// 站类型。阶段站和留白站在同一条时间线上。
const (
	StationKindPhase = "phase"
	StationKindHole  = "hole"
)

// 尾巴留白残段的状态。open 表示脚本的结尾还没写下，filled 表示已经补上。
const (
	TailStubOpen   = "open"
	TailStubFilled = "filled"
)

// Timeline 是一条横向时间线的显示投影。它只描述布局：站、轨道、弧与宽度。
// 它不画任何东西，也不决定状态。
type Timeline struct {
	Stations []TimelineStation
	Arcs     []TimelineArc
	// Parallel 说明某一站是否因为与别站同时在跑而离开了主线。
	Parallel bool
	// TailStub 说明末站是尾巴留白：open 时轨道再跑一小段淡出，filled 时只留余地。
	TailStub string
}

// TimelineStation 是一站：一枚灯、一个名字和一列有界的实例药丸。
type TimelineStation struct {
	PhaseID string
	// Kind 是站的类型：phase 是阶段站，hole 是留白站。
	Kind string
	// HoleID 只在留白站上有值。
	HoleID string
	Name   string
	Index  int
	Status string
	// Track 是这一站所在的轨道号。0 是主线。
	Track    int
	Roster   Roster
	Observed int
	Settled  int
}

// HoleOpen 报告这一站是不是还没补上的留白。触到它的轨道段是虚线：
// 那里还不是代码。
func (s TimelineStation) HoleOpen() bool {
	return s.Kind == StationKindHole && s.Status != contracts.WorkflowHoleFilled
}

// TimelineArc 是一条画在轨道上方的弧。回边与跳段是两种，不混为一谈。
type TimelineArc struct {
	// From 与 To 是站的下标。
	From, To int
	// Kind 是 loop（回边）或 skip（跳段）。
	Kind string
	// Lane 是弧的分道，避免两条弧叠在一起。
	Lane int
}

// BuildTimeline 从一次运行投影出时间线。阶段按声明顺序，留白紧跟在它所属的
// 阶段之后：那里正是脚本停下的位置。轨道按实际重叠划分：两个阶段真的同时
// 在跑，它们就不在同一条线上。
func BuildTimeline(run contracts.WorkflowRun) Timeline {
	timeline := Timeline{}
	actorsByPhase := map[string][]contracts.WorkflowActor{}
	for _, actor := range run.Actors {
		actorsByPhase[actor.PhaseID] = append(actorsByPhase[actor.PhaseID], actor)
	}
	for _, phase := range run.Phases {
		actors := actorsByPhase[phase.ID]
		timeline.Stations = append(timeline.Stations, TimelineStation{
			PhaseID:  phase.ID,
			Kind:     StationKindPhase,
			Name:     phaseName(phase),
			Status:   phaseStatus(phase, actors),
			Roster:   BuildRoster(actors),
			Observed: phase.Observed,
			Settled:  phase.Settled,
		})
	}
	timeline.insertHoles(run)
	// 站的下标就是显示位置：插进留白之后统一重排，弧和灯都按它画。
	for i := range timeline.Stations {
		timeline.Stations[i].Index = i
	}
	timeline.assignTracks(run)
	timeline.assignArcs(run)
	for _, station := range timeline.Stations {
		if station.Track > 0 {
			timeline.Parallel = true
		}
	}
	timeline.TailStub = tailStubOf(timeline.Stations)
	return timeline
}

// insertHoles 把每一处留白插到它所属的阶段之后。找不到所属阶段的留白接在
// 最后：它仍然是这次运行的一部分，不能丢。
func (t *Timeline) insertHoles(run contracts.WorkflowRun) {
	if len(run.Holes) == 0 {
		return
	}
	known := make(map[string]bool, len(run.Phases))
	for _, phase := range run.Phases {
		known[phase.ID] = true
	}
	stations := make([]TimelineStation, 0, len(t.Stations)+len(run.Holes))
	for _, station := range t.Stations {
		stations = append(stations, station)
		if station.Kind != StationKindPhase {
			continue
		}
		for _, hole := range run.Holes {
			if hole.PhaseID != station.PhaseID {
				continue
			}
			stations = append(stations, holeStation(hole))
		}
	}
	for _, hole := range run.Holes {
		if hole.PhaseID != "" && known[hole.PhaseID] {
			continue
		}
		stations = append(stations, holeStation(hole))
	}
	t.Stations = stations
}

// holeStation 把一处留白折成一站。留白的类型签名不在界面上出现：它是编译
// 契约，不是给用户读的。
func holeStation(hole contracts.WorkflowHole) TimelineStation {
	name := hole.Title
	if name == "" {
		name = hole.ID
	}
	return TimelineStation{
		PhaseID: hole.PhaseID,
		Kind:    StationKindHole,
		HoleID:  hole.ID,
		Name:    name,
		Status:  hole.Status,
	}
}

// tailStubOf 报告末站是不是尾巴留白：open 表示脚本的结尾还没写下，
// filled 表示补上了、只留出余量。
func tailStubOf(stations []TimelineStation) string {
	if len(stations) == 0 {
		return ""
	}
	last := stations[len(stations)-1]
	if last.Kind != StationKindHole {
		return ""
	}
	if last.Status == contracts.WorkflowHoleFilled {
		return TailStubFilled
	}
	return TailStubOpen
}

// phaseName 给阶段一个能读的名字。
func phaseName(phase contracts.WorkflowPhase) string {
	if phase.Name != "" {
		return phase.Name
	}
	return phase.ID
}

// assignTracks 按执行窗口的重叠给站分轨道。重叠是真的事实，不是猜的。
// 留白不另开轨道：它跟着自己所属的阶段走。
func (t *Timeline) assignTracks(run contracts.WorkflowRun) {
	// 每条轨道记住自己放过哪些阶段；新站要落在第一条与它不重叠的轨道上。
	tracks := [][]string{}
	for i := range t.Stations {
		station := &t.Stations[i]
		if station.Kind == StationKindHole {
			continue
		}
		placed := false
		for track := range tracks {
			conflict := false
			for _, other := range tracks[track] {
				if phasesOverlap(run, station.PhaseID, other) {
					conflict = true
					break
				}
			}
			if conflict {
				continue
			}
			tracks[track] = append(tracks[track], station.PhaseID)
			station.Track = track
			placed = true
			break
		}
		if !placed {
			tracks = append(tracks, []string{station.PhaseID})
			station.Track = len(tracks) - 1
		}
	}
	// 留白继承所属阶段的轨道：它在那个阶段里发生。
	for i := range t.Stations {
		station := &t.Stations[i]
		if station.Kind != StationKindHole || station.PhaseID == "" {
			continue
		}
		for j := range t.Stations {
			owner := &t.Stations[j]
			if owner.Kind == StationKindPhase && owner.PhaseID == station.PhaseID {
				station.Track = owner.Track
				break
			}
		}
	}
}

// phasesOverlap 报告两个阶段是否真的同时在跑。只有两边都进了时间窗、
// 且至少一边的结束晚于另一边的开始，才算重叠。
func phasesOverlap(run contracts.WorkflowRun, phaseID string, otherPhaseID string) bool {
	var left, right contracts.WorkflowPhase
	for _, phase := range run.Phases {
		switch phase.ID {
		case phaseID:
			left = phase
		case otherPhaseID:
			right = phase
		}
	}
	if left.EnteredAt == nil || right.EnteredAt == nil {
		return false
	}
	leftEnd := left.EnteredAt
	if left.SettledAt != nil {
		leftEnd = left.SettledAt
	}
	rightEnd := right.EnteredAt
	if right.SettledAt != nil {
		rightEnd = right.SettledAt
	}
	return leftEnd.After(*right.EnteredAt) || rightEnd.After(*left.EnteredAt)
}

// assignArcs 把声明的阶段边折成画在轨道上方的弧。相邻的走向是平轨，
// 不画弧；回边与跳段才画。弧只连阶段站：插进留白不改变阶段之间的邻接。
func (t *Timeline) assignArcs(run contracts.WorkflowRun) {
	index := map[string]int{}
	order := map[string]int{}
	for i, station := range t.Stations {
		if station.Kind != StationKindPhase {
			continue
		}
		index[station.PhaseID] = i
	}
	for i, phase := range run.Phases {
		order[phase.ID] = i
	}
	type arc struct {
		from, to int
		kind     string
		track    int
	}
	var arcs []arc
	for _, edge := range run.Edges {
		from, fromOK := index[edge.From]
		to, toOK := index[edge.To]
		if !fromOK || !toOK || from == to {
			continue
		}
		kind := "skip"
		switch edge.Kind {
		case "loop":
			kind = "loop"
		case "next":
			// 邻接按阶段顺序算：中间插了留白也不改变这两个阶段是挨着的。
			left, leftOK := order[edge.From]
			right, rightOK := order[edge.To]
			if leftOK && rightOK && abs(right-left) == 1 {
				continue
			}
		}
		fromTrack, _ := t.trackOf(from)
		toTrack, _ := t.trackOf(to)
		track := -1
		if fromTrack == toTrack {
			track = fromTrack
		}
		arcs = append(arcs, arc{from: from, to: to, kind: kind, track: track})
	}
	t.Arcs = nil
	for _, a := range arcs {
		t.Arcs = append(t.Arcs, TimelineArc{From: a.from, To: a.to, Kind: a.kind, Lane: a.track})
	}
	t.Arcs = assignArcLanes(t.Arcs)
}

// trackOf 找站所在的轨道。
func (t *Timeline) trackOf(index int) (int, bool) {
	if index < 0 || index >= len(t.Stations) {
		return 0, false
	}
	return t.Stations[index].Track, true
}

// assignArcLanes 按区间图贪心着色：互不相交的弧同一高度，嵌套的弧里矮外高，
// 只有真正交叉的弧才被推上去。
func assignArcLanes(arcs []TimelineArc) []TimelineArc {
	type placed struct {
		lo, hi, lane int
	}
	var done []placed
	ordered := append([]TimelineArc(nil), arcs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftLo, leftHi := arcSpan(ordered[i])
		rightLo, rightHi := arcSpan(ordered[j])
		if leftHi-leftLo != rightHi-rightLo {
			return leftHi-leftLo < rightHi-rightLo
		}
		return ordered[i].From < ordered[j].From
	})
	out := make([]TimelineArc, 0, len(ordered))
	for _, arc := range ordered {
		lo, hi := arcSpan(arc)
		taken := map[int]bool{}
		for _, other := range done {
			if max(lo, other.lo) <= min(hi, other.hi) {
				taken[other.lane] = true
			}
		}
		lane := 0
		for taken[lane] {
			lane++
		}
		done = append(done, placed{lo: lo, hi: hi, lane: lane})
		arc.Lane = lane
		out = append(out, arc)
	}
	return out
}

// arcSpan 是弧覆盖的区间跨度。
func arcSpan(arc TimelineArc) (lo, hi int) {
	lo, hi = arc.From, arc.To
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
