package workflow

import "gcode/internal/contracts"

// The reference timeline measures: station width, gap and pitch; pill height
// and gap; track and platform rows; arc spacing and base; the lamp inset from
// a station's left edge. They describe layout, not shared control appearance:
// buttons and inputs still come from the design tokens.
const (
	StationWidth      float32 = 168
	StationGap        float32 = 24
	StationPitch      float32 = 192
	PillHeight        float32 = 32
	PillGap           float32 = 6
	TrackRowHeight    float32 = 24
	PlatformRowHeight float32 = 24
	ArcGap            float32 = 14
	ArcBase           float32 = 26
	LampInset         float32 = 17
	// TailStubWidth 是尾巴留白之后轨道多跑的那一截：脚本的结尾还没写下。
	TailStubWidth float32 = 40
)

// StationX is the left edge of a station at the given declared index. Phases
// keep their declared order, so the index never depends on status.
func StationX(index int) float32 {
	return float32(index) * StationPitch
}

// StationCenterX is where a station's phase marker sits.
func StationCenterX(index int) float32 {
	return StationX(index) + LampInset
}

// TrackWidth is the horizontal span of a run's stage line. Stations include
// the holes: they take a place on the same rail.
func TrackWidth(stations int) float32 {
	if stations <= 0 {
		return 0
	}
	return float32(stations-1)*StationPitch + StationWidth
}

// RosterInlineLimit is how many instances a card shows before it collapses the
// rest into "and N more". The threshold counts every instance of the phase,
// whether or not a pill would be drawn for it.
const RosterInlineLimit = 6

// RosterFixedSlots is how many named pills a crowded phase keeps.
const RosterFixedSlots = 5

// RosterEntry is one named pill on a card or in a phase head.
type RosterEntry struct {
	ActorID string
	Name    string
	Status  string
}

// Roster is the bounded projection of a phase's instances.
type Roster struct {
	Entries []RosterEntry
	// Overflow is how many instances the projection leaves out.
	Overflow int
	// Total counts every instance of the phase, including out-of-table ones.
	Total int
	// Pending counts instances that never settled. Out-of-table instances
	// count as pending rather than being guessed as done.
	Pending int
}

// BuildRoster projects a phase's instances onto at most RosterInlineLimit
// pills. Beyond the limit it keeps RosterFixedSlots fixed slots ordered by
// asking, running, failed, then participant order, and reports the rest.
func BuildRoster(actors []contracts.WorkflowActor) Roster {
	roster := Roster{Total: len(actors)}
	if len(actors) <= RosterInlineLimit {
		roster.Entries = make([]RosterEntry, 0, len(actors))
		for _, actor := range actors {
			state := ActorState(actor.Status)
			roster.Entries = append(roster.Entries, RosterEntry{ActorID: actor.ID, Name: actor.Name, Status: state})
			if state == contracts.WorkflowActorPending {
				roster.Pending++
			}
		}
		return roster
	}
	// Order a copy by slot priority, keeping participant order inside a group.
	ordered := make([]contracts.WorkflowActor, len(actors))
	copy(ordered, actors)
	stableSortByPriority(ordered)
	roster.Entries = make([]RosterEntry, 0, RosterFixedSlots)
	shown := make(map[string]bool, RosterFixedSlots)
	for i, actor := range ordered {
		if i >= RosterFixedSlots {
			break
		}
		state := ActorState(actor.Status)
		roster.Entries = append(roster.Entries, RosterEntry{ActorID: actor.ID, Name: actor.Name, Status: state})
		shown[actor.ID] = true
	}
	for _, actor := range actors {
		if shown[actor.ID] {
			continue
		}
		roster.Overflow++
		if ActorState(actor.Status) == contracts.WorkflowActorPending {
			roster.Pending++
		}
	}
	return roster
}

// stableSortByPriority orders actors by slot priority without disturbing the
// participant order inside one priority group.
func stableSortByPriority(actors []contracts.WorkflowActor) {
	for i := 1; i < len(actors); i++ {
		for j := i; j > 0; j-- {
			left, right := ActorPriority(ActorState(actors[j-1].Status)), ActorPriority(ActorState(actors[j].Status))
			if left <= right {
				break
			}
			actors[j-1], actors[j] = actors[j], actors[j-1]
		}
	}
}
