package workflow

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

// advanceRun 造一次已经走过第一站的运行。
func advanceRun() contracts.WorkflowRun {
	entered := time.Now().Add(-time.Minute)
	settled := time.Now().Add(-30 * time.Second)
	return contracts.WorkflowRun{
		Phases: []contracts.WorkflowPhase{
			{ID: "a", Name: "A", Index: 0, Status: contracts.WorkflowPhaseDone, EnteredAt: &entered, SettledAt: &settled, Rounds: 1},
			{ID: "b", Name: "B", Index: 1, Status: contracts.WorkflowPhasePending},
		},
	}
}

func TestAdvanceEntersTheFirstPhaseAndThenTheNext(t *testing.T) {
	empty := contracts.WorkflowRun{Phases: []contracts.WorkflowPhase{{ID: "a", Name: "A", Index: 0}}}
	decision := Advance(empty)
	if decision.Kind != AdvanceEnter || decision.PhaseID != "a" {
		t.Fatalf("decision = %+v, want the first phase", decision)
	}
	decision = Advance(advanceRun())
	if decision.Kind != AdvanceEnter || decision.PhaseID != "b" {
		t.Fatalf("decision = %+v, want the next phase", decision)
	}
}

func TestAdvanceSkipsAPhaseWhoseEdgeDoesNotHold(t *testing.T) {
	run := advanceRun()
	run.Edges = []contracts.WorkflowEdge{{From: "a", To: "b", Kind: "next", When: `phase.a.status == "failed"`}}
	decision := Advance(run)
	if decision.Kind != AdvanceSkip || decision.PhaseID != "b" {
		t.Fatalf("decision = %+v, want the phase skipped", decision)
	}
	if decision.Reason == "" {
		t.Fatal("a skipped phase says why")
	}
	run.Edges[0].When = `phase.a.status == "done"`
	decision = Advance(run)
	if decision.Kind != AdvanceEnter || decision.PhaseID != "b" {
		t.Fatalf("decision = %+v, want the phase entered", decision)
	}
}

func TestAdvanceLoopsBackToAnEarlierPhase(t *testing.T) {
	run := advanceRun()
	run.Phases[1].Status = contracts.WorkflowPhaseDone
	later := time.Now()
	run.Phases[1].EnteredAt = &later
	run.Edges = []contracts.WorkflowEdge{{From: "b", To: "a", Kind: "loop"}}
	decision := Advance(run)
	if decision.Kind != AdvanceEnter || decision.PhaseID != "a" {
		t.Fatalf("decision = %+v, want the loop target", decision)
	}
	if decision.ResetFrom != "a" {
		t.Fatalf("a loop resets from its target, got %q", decision.ResetFrom)
	}
}

func TestAdvanceStopsAtTheRoundLimit(t *testing.T) {
	run := advanceRun()
	run.Phases[0].Rounds = PhaseRoundLimit
	run.Phases[1].Status = contracts.WorkflowPhaseDone
	later := time.Now()
	run.Phases[1].EnteredAt = &later
	run.Edges = []contracts.WorkflowEdge{{From: "b", To: "a", Kind: "loop"}}
	if decision := Advance(run); decision.Kind != AdvanceFinish {
		t.Fatalf("decision = %+v, want the run to finish", decision)
	}
}

func TestAdvanceFinishesWhenNoPhaseIsLeft(t *testing.T) {
	run := advanceRun()
	run.Phases[1].Status = contracts.WorkflowPhaseDone
	later := time.Now()
	run.Phases[1].EnteredAt = &later
	if decision := Advance(run); decision.Kind != AdvanceFinish {
		t.Fatalf("decision = %+v, want the run to finish", decision)
	}
}

func TestFactsExposeTheRunToConditions(t *testing.T) {
	entered := time.Now()
	run := contracts.WorkflowRun{
		Status: contracts.WorkflowStatusRunning,
		Phases: []contracts.WorkflowPhase{{ID: "a", Name: "A", Index: 0, Status: contracts.WorkflowPhaseDone, EnteredAt: &entered, Rounds: 2}},
		Actors: []contracts.WorkflowActor{{ID: "reader", PhaseID: "a", Status: contracts.WorkflowActorDone, Result: "found it"}},
		Holes:  []contracts.WorkflowHole{{ID: "input", Status: contracts.WorkflowHoleFilled, Text: "the target"}},
		Questions: []contracts.WorkflowQuestion{
			{ID: "q1", Status: "resolved", Answer: "yes"},
		},
		Args: []contracts.WorkflowArgValue{{Name: "target", Kind: "string", Text: "docs"}},
	}
	facts := Facts(run)
	for key, want := range map[string]string{
		"run.status":          contracts.WorkflowStatusRunning,
		"phase.a.status":      contracts.WorkflowPhaseDone,
		"phase.a.rounds":      "2",
		"actor.reader.result": "found it",
		"hole.input":          "the target",
		"question.q1":         "yes",
		"arg.target":          "docs",
	} {
		if facts[key] != want {
			t.Errorf("fact %q = %q, want %q", key, facts[key], want)
		}
	}
}

func TestModeRulesKeepTheFeatureOutOfTheWay(t *testing.T) {
	if Available(contracts.WorkflowModeDisabled, true) {
		t.Fatal("the disabled mode never offers workflow tools")
	}
	if Available(contracts.WorkflowModeOnDemand, false) {
		t.Fatal("the on-demand mode waits for the user to turn it on")
	}
	if !Available(contracts.WorkflowModeOnDemand, true) {
		t.Fatal("an activated session keeps the workflow tools")
	}
	if !Available(contracts.WorkflowModeAlwaysOn, false) {
		t.Fatal("the always-on mode offers workflow tools right away")
	}
	if NormalizeMode("nonsense") != contracts.WorkflowModeOnDemand {
		t.Fatal("an unknown mode falls back to on-demand")
	}
	if NormalizeMode("") != contracts.WorkflowModeOnDemand {
		t.Fatal("an empty mode falls back to on-demand")
	}
}
