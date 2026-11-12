package accounting

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestRefreshUsesSavedChildAndUnloadedAncestorSummaries(t *testing.T) {
	snapshot := contracts.Snapshot{
		Sessions: []contracts.Session{{ID: "parent"}, {ID: "child", ParentSessionID: "parent", ParentRunID: "turn", SubagentType: "Explore"}},
		Messages: map[string][]contracts.Message{"parent": nil, "child": nil},
		ModelRequests: []contracts.ModelRequest{
			{ID: "parent-request", SessionID: "parent", TurnID: "turn", Usage: model.Usage{Known: true, Input: 100}},
			{ID: "child-request", SessionID: "child", TurnID: "child-turn", Usage: model.Usage{Known: true, Input: 20}},
		},
	}
	Refresh(&snapshot, []string{"child", "parent"})
	if snapshot.Metrics["parent"].Total.Totals.Input != 120 {
		t.Fatal("initial parent composition failed")
	}
	delete(snapshot.Messages, "parent")
	snapshot.ModelRequests = append(snapshot.ModelRequests, contracts.ModelRequest{ID: "child-request2", SessionID: "child", TurnID: "child-turn", Usage: model.Usage{Known: true, Input: 5}})
	Refresh(&snapshot, []string{"child"})
	if snapshot.Metrics["parent"].Total.Totals.Input != 125 || snapshot.Metrics["parent"].Turns["turn"].Totals.Input != 125 {
		t.Fatal("child update lost parent own summary")
	}
	if snapshot.Metrics["parent"].Own.Totals.Input != 100 {
		t.Fatal("unloaded parent summary was recomputed from absent raw records")
	}
	delete(snapshot.Messages, "child")
	snapshot.ModelRequests = nil
	Refresh(&snapshot, []string{"parent"})
	if snapshot.Metrics["parent"].Total.Totals.Input != 125 {
		t.Fatal("saved child summary was not sufficient")
	}
}

func TestRefreshKeepsParallelChildWallTimeUnionAndOwnAgentDuration(t *testing.T) {
	start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Second)
	childEnd := start.Add(8 * time.Second)
	snapshot := contracts.Snapshot{
		Sessions: []contracts.Session{{ID: "parent"}, {ID: "child", ParentSessionID: "parent", ParentRunID: "turn", SubagentType: "Explore"}},
		Messages: map[string][]contracts.Message{"parent": nil, "child": nil},
		Runs: []contracts.Run{
			{ID: "turn", SessionID: "parent", StartedAt: start, FinishedAt: &end, Status: "completed"},
			{ID: "child-turn", SessionID: "child", StartedAt: start.Add(2 * time.Second), FinishedAt: &childEnd, Status: "completed"},
		},
	}
	Refresh(&snapshot, []string{"parent", "child"})
	if duration, _ := snapshot.Metrics["parent"].Total.Elapsed(end); duration != 10*time.Second {
		t.Fatalf("parallel wall time duplicated: %v", duration)
	}
	if duration, _ := snapshot.Metrics["child"].Own.Elapsed(end); duration != 6*time.Second {
		t.Fatalf("agent own execution time changed: %v", duration)
	}
}
