package workflow

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

// holeRun is a run with two phases and one hole raised in the first one.
func holeRun() contracts.WorkflowRun {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	return contracts.WorkflowRun{
		ID: "run-holes", Name: "Holes", Status: contracts.WorkflowStatusRunning,
		StartedAt: now, UpdatedAt: now,
		Phases: []contracts.WorkflowPhase{
			{ID: "phase-1", Name: "Plan", Index: 0, Status: contracts.WorkflowPhaseDone},
			{ID: "phase-2", Name: "Build", Index: 1, Status: contracts.WorkflowPhaseRunning},
		},
		Holes: []contracts.WorkflowHole{
			{ID: "hole-1", RunID: "run-holes", PhaseID: "phase-1", Title: "Missing target", Status: contracts.WorkflowHoleWaiting},
		},
	}
}

func TestBuildTimelinePutsAHoleAfterItsPhase(t *testing.T) {
	timeline := BuildTimeline(holeRun())
	if len(timeline.Stations) != 3 {
		t.Fatalf("expected three stations, got %d", len(timeline.Stations))
	}
	if timeline.Stations[0].Kind != StationKindPhase || timeline.Stations[0].Name != "Plan" {
		t.Fatalf("the phase keeps its place: %+v", timeline.Stations[0])
	}
	hole := timeline.Stations[1]
	if hole.Kind != StationKindHole || hole.HoleID != "hole-1" {
		t.Fatalf("the hole sits right after its phase: %+v", hole)
	}
	if hole.Name != "Missing target" {
		t.Fatalf("the hole keeps its title: %+v", hole)
	}
	if timeline.Stations[2].Name != "Build" {
		t.Fatalf("the next phase follows the hole: %+v", timeline.Stations[2])
	}
	for i, station := range timeline.Stations {
		if station.Index != i {
			t.Fatalf("station %d reports its display place: %+v", i, station)
		}
	}
}

func TestBuildTimelineKeepsAHoleWithoutAPhase(t *testing.T) {
	run := holeRun()
	run.Holes = append(run.Holes, contracts.WorkflowHole{
		ID: "hole-2", RunID: run.ID, Title: "Orphan", Status: contracts.WorkflowHoleWaiting,
	})
	timeline := BuildTimeline(run)
	last := timeline.Stations[len(timeline.Stations)-1]
	if last.Kind != StationKindHole || last.HoleID != "hole-2" {
		t.Fatalf("a hole without a phase is still part of the run: %+v", timeline.Stations)
	}
}

func TestBuildTimelineKeepsArcsOutOfTheHole(t *testing.T) {
	run := holeRun()
	run.Edges = []contracts.WorkflowEdge{{From: "phase-1", To: "phase-2", Kind: "next"}}
	timeline := BuildTimeline(run)
	if len(timeline.Arcs) != 0 {
		t.Fatalf("adjacent phases stay on a flat rail: %+v", timeline.Arcs)
	}
}

func TestBuildTimelineHoleSharesItsPhaseTrack(t *testing.T) {
	run := holeRun()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	settled := now.Add(time.Minute)
	run.Phases[0].EnteredAt = &now
	run.Phases[0].SettledAt = &settled
	run.Phases[1].EnteredAt = &now
	timeline := BuildTimeline(run)
	if timeline.Stations[0].Track != 0 || timeline.Stations[2].Track != 1 {
		t.Fatalf("overlapping phases leave the main line: %+v", timeline.Stations)
	}
	if timeline.Stations[1].Track != timeline.Stations[0].Track {
		t.Fatalf("a hole rides the track of its phase: %+v", timeline.Stations)
	}
}

func TestBuildTimelineReportsTheTailHole(t *testing.T) {
	run := holeRun()
	run.Holes = append(run.Holes, contracts.WorkflowHole{
		ID: "hole-tail", RunID: run.ID, PhaseID: "phase-2", Title: "The ending", Status: contracts.WorkflowHoleWaiting,
	})
	timeline := BuildTimeline(run)
	if timeline.TailStub != TailStubOpen {
		t.Fatalf("a waiting tail hole says the ending is unwritten: %q", timeline.TailStub)
	}
	run.Holes[len(run.Holes)-1].Status = contracts.WorkflowHoleFilled
	timeline = BuildTimeline(run)
	if timeline.TailStub != TailStubFilled {
		t.Fatalf("a filled tail hole leaves room only: %q", timeline.TailStub)
	}
}

func TestBuildTimelineHoleOpenOnlyWhileItWaits(t *testing.T) {
	station := TimelineStation{Kind: StationKindHole, Status: contracts.WorkflowHoleWaiting}
	if !station.HoleOpen() {
		t.Fatal("a waiting hole is open: there is no code there yet")
	}
	station.Status = contracts.WorkflowHoleFilled
	if station.HoleOpen() {
		t.Fatal("a filled hole is not open")
	}
	station.Kind = StationKindPhase
	station.Status = contracts.WorkflowHoleWaiting
	if station.HoleOpen() {
		t.Fatal("a phase station is never a hole")
	}
}
