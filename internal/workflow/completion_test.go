package workflow

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

func completionRun(status string, artifacts []contracts.WorkflowArtifact) contracts.WorkflowRun {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	finished := now.Add(90 * time.Second)
	return contracts.WorkflowRun{
		ID: "run-1", Name: "Delivery", Status: status,
		StartedAt: now, FinishedAt: &finished,
		Phases: []contracts.WorkflowPhase{
			{ID: "phase-1", Name: "Plan", EnteredAt: &now},
			{ID: "phase-2", Name: "Build", EnteredAt: &now},
			{ID: "phase-3", Name: "Check"},
		},
		Actors:    []contracts.WorkflowActor{{ID: "a-1"}, {ID: "a-2"}},
		Artifacts: artifacts,
	}
}

func TestCompletionNeedsASuccessfulRunWithDeliveries(t *testing.T) {
	artifact := []contracts.WorkflowArtifact{{ID: "art-1", Name: "report", Content: "the report", Version: 1}}
	if _, ok := Completion(completionRun(contracts.WorkflowStatusCompleted, artifact), TokenTotals{}); !ok {
		t.Fatal("a completed run with a delivery has a completion card")
	}
	for _, status := range []string{
		contracts.WorkflowStatusErrored, contracts.WorkflowStatusStopped,
		contracts.WorkflowStatusRunning, contracts.WorkflowStatusPending,
	} {
		if _, ok := Completion(completionRun(status, artifact), TokenTotals{}); ok {
			t.Fatalf("a %q run has no completion card", status)
		}
	}
	if _, ok := Completion(completionRun(contracts.WorkflowStatusCompleted, nil), TokenTotals{}); ok {
		t.Fatal("a run without a delivery has no completion card")
	}
}

func TestCompletionShowsThePrimaryDeliveryAndIndexesTheRest(t *testing.T) {
	artifacts := []contracts.WorkflowArtifact{
		{ID: "art-1", Name: "report", Content: "the report", Version: 1, Primary: true},
		{ID: "art-2", Name: "diff", Content: "the diff", Version: 1, Order: 1},
	}
	view, ok := Completion(completionRun(contracts.WorkflowStatusCompleted, artifacts), TokenTotals{})
	if !ok {
		t.Fatal("the run has a completion card")
	}
	if !view.HasPrimary || view.Primary.Name != "report" {
		t.Fatalf("the explicit primary is the preview: %+v", view.Primary)
	}
	if len(view.Index.Shown) != 1 || view.Index.Shown[0].Name != "diff" {
		t.Fatalf("the rest is indexed: %+v", view.Index.Shown)
	}
	if view.Delivered != 2 {
		t.Fatalf("the card counts every delivery: %d", view.Delivered)
	}
}

func TestCompletionWithoutAPrimaryShowsOnlyTheIndex(t *testing.T) {
	artifacts := []contracts.WorkflowArtifact{
		{ID: "art-1", Name: "report", Version: 1},
		{ID: "art-2", Name: "diff", Version: 1, Order: 1},
	}
	view, ok := Completion(completionRun(contracts.WorkflowStatusCompleted, artifacts), TokenTotals{})
	if !ok {
		t.Fatal("the run has a completion card")
	}
	if view.HasPrimary {
		t.Fatalf("two deliveries have no single preview: %+v", view.Primary)
	}
	if len(view.Index.Shown) != 2 {
		t.Fatalf("the index holds both deliveries: %+v", view.Index.Shown)
	}
}

func TestCompletionIndexIsBounded(t *testing.T) {
	artifacts := make([]contracts.WorkflowArtifact, 0, 8)
	for i := range 8 {
		// 名字各不相同：同名只留最新版，那不是索引要的。
		artifacts = append(artifacts, contracts.WorkflowArtifact{ID: "art-" + string(rune('a'+i)), Name: "file-" + string(rune('a'+i)), Version: 1, Order: i})
	}
	view, ok := Completion(completionRun(contracts.WorkflowStatusCompleted, artifacts), TokenTotals{})
	if !ok {
		t.Fatal("the run has a completion card")
	}
	if len(view.Index.Shown) != CompletionOverflowRows || view.Index.Overflow != 3 {
		t.Fatalf("the index keeps five rows and counts the rest: %+v", view.Index)
	}
}

func TestCompletionCountsWhatTheRunReallyDid(t *testing.T) {
	view, ok := Completion(completionRun(contracts.WorkflowStatusCompleted, []contracts.WorkflowArtifact{{ID: "art-1", Name: "report", Version: 1}}),
		TokenTotals{Input: 120, Output: 30, Known: true})
	if !ok {
		t.Fatal("the run has a completion card")
	}
	if view.Phases != 2 {
		t.Fatalf("only the phases the run entered count: %d", view.Phases)
	}
	if view.Agents != 2 {
		t.Fatalf("the instances the run created count: %d", view.Agents)
	}
	if view.Duration != 90*time.Second {
		t.Fatalf("the run's own duration: %s", view.Duration)
	}
	if view.Tokens.Total() != 150 {
		t.Fatalf("the token total: %d", view.Tokens.Total())
	}
}

func TestCompletionKeepsUnknownFactsUnknown(t *testing.T) {
	run := completionRun(contracts.WorkflowStatusCompleted, []contracts.WorkflowArtifact{{ID: "art-1", Name: "report", Version: 1}})
	run.FinishedAt = nil
	view, ok := Completion(run, TokenTotals{})
	if !ok {
		t.Fatal("the run has a completion card")
	}
	if view.Duration != 0 {
		t.Fatalf("an unfinished run has no measured duration: %s", view.Duration)
	}
	if view.Tokens.Known {
		t.Fatal("unknown tokens stay unknown")
	}
}
