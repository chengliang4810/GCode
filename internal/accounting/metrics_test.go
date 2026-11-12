package accounting

import (
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func metricsRequest(session, turn, providerID, provider, modelID string, usage model.Usage, decode time.Duration) contracts.ModelRequest {
	return contracts.ModelRequest{
		SessionID:      session,
		TurnID:         turn,
		ProviderID:     providerID,
		ProviderName:   provider,
		ModelID:        modelID,
		Usage:          usage,
		DecodeDuration: decode,
	}
}

func finishedAt(start time.Time, duration time.Duration) *time.Time {
	end := start.Add(duration)
	return &end
}

func TestOwnScopesTurnsMaintenanceAndModels(t *testing.T) {
	start := time.Date(2026, 11, 12, 23, 59, 59, 0, time.UTC)
	messages := []contracts.Message{
		{
			SessionID: "main", TurnID: "turn-1", Role: "assistant", ModelID: "p/m",
			Blocks: []contracts.Block{
				{ID: "call", Kind: "toolCall", Tool: &contracts.ToolCall{ID: "tool", StartedAt: start}},
				{ID: "result", Kind: "toolResult", Tool: &contracts.ToolCall{ID: "tool", FinishedAt: finishedAt(start, 2*time.Second)}},
			},
		},
		{SessionID: "main", TurnID: "turn-2", Role: "assistant", ModelID: "p/m"},
		// This copied transcript belongs to another session and cannot affect
		// the main session's own tool totals.
		{SessionID: "child", TurnID: "turn-1", Role: "assistant", ModelID: "p/m", Blocks: []contracts.Block{{Tool: &contracts.ToolCall{ID: "copied", StartedAt: start, FinishedAt: finishedAt(start, time.Hour)}}}},
	}
	requests := []contracts.ModelRequest{
		metricsRequest("main", "turn-1", "p", "Provider", "m", model.Usage{Known: true, OutputKnown: true, Input: 10, Output: 4}, time.Second),
		metricsRequest("main", "turn-1", "p", "Provider", "m", model.Usage{OutputKnown: true, Output: 2}, 2*time.Second),
		metricsRequest("main", "", "naming", "Naming", "m", model.Usage{Known: true}, time.Second),
		metricsRequest("main", "turn-2", "q", "Other", "m2", model.Usage{Known: true, OutputKnown: true, Input: 3, Output: 1}, time.Second),
		metricsRequest("other", "turn-1", "x", "Other", "m", model.Usage{Known: true, Input: 999}, time.Second),
	}
	runs := []contracts.Run{
		{ID: "turn-1", SessionID: "main", Status: "completed", StartedAt: start, FinishedAt: finishedAt(start, 5*time.Second)},
		{ID: "turn-2", SessionID: "main", Status: "completed", StartedAt: start.Add(10 * time.Second), FinishedAt: finishedAt(start, 12*time.Second)},
		{ID: "turn-1", SessionID: "other", Status: "completed", StartedAt: start, FinishedAt: finishedAt(start, time.Hour)},
	}

	turn := Own("main", "turn-1", messages, requests, runs)
	if turn.Totals.Requests != 2 || turn.Totals.UnknownUsage != 1 || turn.Totals.Input != 10 || turn.Totals.Output != 6 {
		t.Fatalf("turn totals = %+v", turn.Totals)
	}
	if turn.Turns != 1 || turn.GenerationDuration != 3*time.Second || turn.ToolDuration != 2*time.Second {
		t.Fatalf("turn detail = %+v", turn)
	}
	if len(turn.Models) != 1 || turn.Models[0].Totals.Requests != 2 || turn.WallDuration != 5*time.Second {
		t.Fatalf("turn models/wall = %+v", turn)
	}

	total := Own("main", "", messages, requests, runs)
	if total.Totals.Requests != 4 || total.Turns != 2 || total.GenerationDuration != 5*time.Second {
		t.Fatalf("session totals = %+v", total)
	}
	if total.ToolDuration != 2*time.Second || total.WallDuration != 7*time.Second || !total.WallKnown {
		t.Fatalf("session tool/wall = %+v", total)
	}
	if len(total.Models) != 3 || total.Models[0].ProviderID != "p" || total.Models[1].ProviderID != "naming" || total.Models[2].ProviderID != "q" {
		t.Fatalf("session model order = %+v", total.Models)
	}
}

func TestOwnUnrecordedRequiresOwnedRunAndAssistantModel(t *testing.T) {
	start := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	runs := []contracts.Run{{ID: "missing", SessionID: "main", Status: "completed", StartedAt: start, FinishedAt: finishedAt(start, time.Second)}}
	messages := []contracts.Message{
		{SessionID: "main", TurnID: "missing", Role: "assistant", ModelID: "p/m"},
		{SessionID: "fork", TurnID: "missing", Role: "assistant", ModelID: "p/m"},
	}
	if got := Own("main", "", messages, nil, runs); !got.Totals.Unrecorded {
		t.Fatal("owned run with assistant model text was not marked unrecorded")
	}
	if got := Own("fork", "", messages, nil, runs); got.Totals.Unrecorded || got.Totals.Requests != 0 {
		t.Fatalf("copied text without owned run affected accounting: %+v", got)
	}
	if got := Own("fork", "missing", messages, nil, runs); got.ToolDuration != 0 || got.Activity != (contracts.ActivitySummary{}) {
		t.Fatalf("turn-scoped copied text affected accounting: %+v", got)
	}
	withoutModel := []contracts.Message{{SessionID: "main", TurnID: "missing", Role: "assistant"}}
	if got := Own("main", "", withoutModel, nil, runs); got.Totals.Unrecorded {
		t.Fatal("assistant message without model identity was treated as missing accounting")
	}
}

func TestOwnKnownZeroUnknownAndActiveElapsed(t *testing.T) {
	start := time.Date(2026, 11, 12, 23, 59, 59, 0, time.UTC)
	run := contracts.Run{ID: "turn", SessionID: "main", Status: "running", StartedAt: start}
	requests := []contracts.ModelRequest{
		metricsRequest("main", "turn", "p", "Provider", "m", model.Usage{Known: true}, 0),
		metricsRequest("main", "turn", "p", "Provider", "m", model.Usage{}, 0),
	}
	got := Own("main", "turn", nil, requests, []contracts.Run{run})
	if !got.WallKnown || got.WallDuration != 0 {
		t.Fatalf("active wall state = %+v", got)
	}
	if got.Totals.Requests != 2 || got.Totals.UnknownUsage != 1 || got.Totals.Input != 0 || got.Totals.Output != 0 {
		t.Fatalf("known zero/unknown totals = %+v", got.Totals)
	}
	if elapsed, known := got.Elapsed(start.Add(2 * time.Second)); !known || elapsed != 2*time.Second {
		t.Fatalf("active elapsed = %v, known=%v", elapsed, known)
	}
}

func TestMergeUnionsParallelIntervalsAndKeepsBaseTurnsActivity(t *testing.T) {
	start := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	base := Own("main", "", []contracts.Message{{
		SessionID: "main", TurnID: "base-turn", Role: "assistant",
		Blocks: []contracts.Block{{ID: "base-call", Kind: "tool", Tool: &contracts.ToolCall{ID: "base-call", Kind: contracts.ToolExec}}},
	}},
		[]contracts.ModelRequest{metricsRequest("main", "base-turn", "p", "Provider", "m", model.Usage{Known: true, OutputKnown: true, Output: 2}, time.Second)},
		[]contracts.Run{{ID: "base-turn", SessionID: "main", Status: "completed", StartedAt: start, FinishedAt: finishedAt(start, 10*time.Second)}})
	child := Own("child", "", []contracts.Message{{
		SessionID: "child", TurnID: "child-turn", Role: "assistant",
		Blocks: []contracts.Block{{ID: "child-call", Kind: "tool", Tool: &contracts.ToolCall{ID: "child-call", Kind: contracts.ToolExec}}},
	}},
		[]contracts.ModelRequest{metricsRequest("child", "child-turn", "p", "Provider", "m", model.Usage{Known: true, OutputKnown: true, Output: 3}, 2*time.Second)},
		[]contracts.Run{{ID: "child-turn", SessionID: "child", Status: "completed", StartedAt: start.Add(5 * time.Second), FinishedAt: finishedAt(start.Add(5*time.Second), 15*time.Second)}})
	active := Own("active", "", nil,
		[]contracts.ModelRequest{metricsRequest("active", "active-turn", "q", "Other", "m2", model.Usage{Known: true, OutputKnown: true, Output: 4}, 3*time.Second)},
		[]contracts.Run{{ID: "active-turn", SessionID: "active", Status: "running", StartedAt: start.Add(15 * time.Second)}})

	baseBefore, childBefore, activeBefore := base.Clone(), child.Clone(), active.Clone()
	merged := Merge(base, child, active)
	if merged.Totals.Requests != 3 || merged.GenerationDuration != 6*time.Second || merged.Turns != 1 {
		t.Fatalf("merged scalar totals = %+v", merged)
	}
	if len(merged.Models) != 2 || merged.Models[0].Totals.Requests != 2 || merged.Models[1].ProviderID != "q" {
		t.Fatalf("merged models = %+v", merged.Models)
	}
	if merged.Activity.Commands != 1 {
		t.Fatalf("delegated activity leaked into base view: %+v", merged.Activity)
	}
	if len(merged.Spans) != 1 || !merged.Spans[0].Start.Equal(start) || !merged.Spans[0].End.IsZero() {
		t.Fatalf("parallel spans = %+v", merged.Spans)
	}
	if elapsed, known := merged.Elapsed(start.Add(25 * time.Second)); !known || elapsed != 25*time.Second {
		t.Fatalf("parallel elapsed = %v, known=%v", elapsed, known)
	}
	if !reflect.DeepEqual(base, baseBefore) || !reflect.DeepEqual(child, childBefore) || !reflect.DeepEqual(active, activeBefore) {
		t.Fatal("Merge modified an input metrics value")
	}
	merged.Models[0].Totals.Output = 99
	merged.Spans[0].Start = start.Add(time.Hour)
	if base.Models[0].Totals.Output == 99 || base.Spans[0].Start.Equal(start.Add(time.Hour)) {
		t.Fatal("merged slices alias input slices")
	}
}

func TestMergeSequentialActiveWallUnion(t *testing.T) {
	start := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	base := contracts.ExecutionMetrics{WallKnown: true, WallDuration: 10 * time.Second, Spans: []contracts.ExecutionSpan{{Start: start, End: start.Add(10 * time.Second)}}}
	child := contracts.ExecutionMetrics{WallKnown: true, RunningSince: start.Add(12 * time.Second), Spans: []contracts.ExecutionSpan{{Start: start.Add(12 * time.Second)}}}
	merged := Merge(base, child)
	if len(merged.Spans) != 2 || merged.WallDuration != 10*time.Second || !merged.RunningSince.Equal(start.Add(12*time.Second)) {
		t.Fatalf("sequential union = %+v", merged)
	}
	if elapsed, known := merged.Elapsed(start.Add(20 * time.Second)); !known || elapsed != 18*time.Second {
		t.Fatalf("sequential elapsed = %v, known=%v", elapsed, known)
	}
}
