package workflow

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

func projectionRun() contracts.WorkflowRun {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	return contracts.WorkflowRun{
		ID: "run-1", Name: "Review", Status: contracts.WorkflowStatusRunning,
		ModelID: "goal/model", Concurrency: 2, SessionID: "session-1", MessageID: "message-1",
		StartedAt: now, UpdatedAt: now,
		Phases: []contracts.WorkflowPhase{
			{ID: "phase-1", Name: "Plan", Index: 0, Status: contracts.WorkflowPhaseDone},
			{ID: "phase-2", Name: "Build", Index: 1, Status: contracts.WorkflowPhaseRunning},
		},
		Actors: []contracts.WorkflowActor{
			{ID: "actor-1", PhaseID: "phase-1", Name: "Planner", Status: contracts.WorkflowActorDone, SessionID: "actor-session-1"},
			{ID: "actor-2", PhaseID: "phase-2", Name: "Builder", Status: contracts.WorkflowActorRunning},
			{ID: "actor-3", PhaseID: "phase-2", Name: "Reviewer", Status: contracts.WorkflowActorAsking},
		},
		Questions: []contracts.WorkflowQuestion{
			{ID: "q-1", RunID: "run-1", Text: "Which database?", Status: "pending"},
			{ID: "q-2", RunID: "run-1", Text: "Done?", Status: "resolved", Answer: "Yes"},
		},
		Holes: []contracts.WorkflowHole{
			{ID: "h-1", RunID: "run-1", Title: "Missing target", Status: contracts.WorkflowHoleWaiting},
		},
		Artifacts: []contracts.WorkflowArtifact{
			{ID: "artifact-1", RunID: "run-1", Name: "plan", Content: "the plan", Version: 1, Order: 0},
			{ID: "artifact-2", RunID: "run-1", Name: "diff", Content: "the diff", Version: 1, Order: 1},
		},
	}
}

func TestProjectRunReportsPhasesAgentsAndPendingWork(t *testing.T) {
	view := ProjectRun(projectionRun())
	if len(view.Phases) != 2 {
		t.Fatalf("expected two phases, got %d", len(view.Phases))
	}
	if view.Phases[0].Name != "Plan" || view.Phases[1].Name != "Build" {
		t.Fatalf("phases keep their declared order: %+v", view.Phases)
	}
	if view.AgentTotal() != 3 {
		t.Fatalf("expected three instances, got %d", view.AgentTotal())
	}
	if view.AgentWorking() != 2 {
		t.Fatalf("a waiting instance is still working, got %d", view.AgentWorking())
	}
	if view.PendingQuestions() != 1 {
		t.Fatalf("expected one pending question, got %d", view.PendingQuestions())
	}
	if view.WaitingHoles() != 1 {
		t.Fatalf("expected one waiting hole, got %d", view.WaitingHoles())
	}
	if view.MessageID != "message-1" || view.SessionID != "session-1" {
		t.Fatalf("the run keeps the turn that launched it: %+v", view)
	}
	if view.Concurrency != 2 {
		t.Fatalf("expected a concurrency of two, got %d", view.Concurrency)
	}
}

func TestProjectRunKeepsArtifactsBounded(t *testing.T) {
	run := projectionRun()
	for i := 0; i < 5; i++ {
		run.Artifacts = append(run.Artifacts, contracts.WorkflowArtifact{
			ID: "artifact-extra-" + string(rune('a'+i)), RunID: run.ID, Name: "extra-" + string(rune('a'+i)), Version: 1, Order: 10 + i,
		})
	}
	view := ProjectRun(run)
	if len(view.Artifacts.Shown) != CardArtifactLimit {
		t.Fatalf("a card shows at most %d artifacts, got %d", CardArtifactLimit, len(view.Artifacts.Shown))
	}
	if view.Artifacts.Overflow != 4 {
		t.Fatalf("expected four hidden artifacts, got %d", view.Artifacts.Overflow)
	}
	if view.Artifacts.Shown[0].Name != "plan" {
		t.Fatalf("artifacts keep their publication order: %+v", view.Artifacts.Shown)
	}
}

func TestProjectRunReportsTruncationOnlyWhenTheRosterIsCut(t *testing.T) {
	run := projectionRun()
	if ProjectRun(run).Truncated {
		t.Fatal("a small roster is not truncated")
	}
	for i := 0; i < 6; i++ {
		run.Actors = append(run.Actors, contracts.WorkflowActor{
			ID: "actor-extra-" + string(rune('a'+i)), PhaseID: "phase-2", Name: "Extra", Status: contracts.WorkflowActorPending,
		})
	}
	view := ProjectRun(run)
	if !view.Truncated {
		t.Fatal("a cut roster says so")
	}
	if view.AgentTotal() != 9 {
		t.Fatalf("instances out of the table still count, got %d", view.AgentTotal())
	}
	phase := view.Phases[1]
	if len(phase.Roster.Entries) != RosterFixedSlots {
		t.Fatalf("a crowded phase keeps %d named pills, got %d", RosterFixedSlots, len(phase.Roster.Entries))
	}
	if phase.Roster.Overflow != 3 {
		t.Fatalf("expected three hidden instances, got %d", phase.Roster.Overflow)
	}
}

func TestBuildTimelineKeepsDeclaredOrderAndFindsOverlaps(t *testing.T) {
	run := projectionRun()
	// 这一条只看阶段：留白站由 timeline_test.go 负责。
	run.Holes = nil
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	settled := now.Add(time.Minute)
	run.Phases[0].EnteredAt = &now
	run.Phases[0].SettledAt = &settled
	run.Phases[1].EnteredAt = &now
	timeline := BuildTimeline(run)
	if len(timeline.Stations) != 2 {
		t.Fatalf("expected two stations, got %d", len(timeline.Stations))
	}
	if timeline.Stations[0].Name != "Plan" || timeline.Stations[1].Name != "Build" {
		t.Fatalf("stations keep their declared order: %+v", timeline.Stations)
	}
	if timeline.Stations[0].Track != 0 || timeline.Stations[1].Track != 1 {
		t.Fatalf("overlapping phases leave the main line: %+v", timeline.Stations)
	}
	if !timeline.Parallel {
		t.Fatal("a timeline with a second track says so")
	}
	if timeline.Stations[1].Status != contracts.WorkflowPhaseRunning {
		t.Fatalf("a station reports its own state, got %q", timeline.Stations[1].Status)
	}
}

func TestBuildTimelineDrawsArcsOnlyForRealEdges(t *testing.T) {
	run := projectionRun()
	run.Edges = []contracts.WorkflowEdge{
		{From: "phase-1", To: "phase-2", Kind: "next"},
		{From: "phase-2", To: "phase-1", Kind: "loop"},
		{From: "phase-1", To: "phase-3", Kind: "branch"},
	}
	timeline := BuildTimeline(run)
	if len(timeline.Arcs) != 1 {
		t.Fatalf("only a real edge becomes an arc, got %+v", timeline.Arcs)
	}
	if timeline.Arcs[0].Kind != "loop" || timeline.Arcs[0].From != 1 || timeline.Arcs[0].To != 0 {
		t.Fatalf("the loop arc keeps its ends: %+v", timeline.Arcs[0])
	}
}
