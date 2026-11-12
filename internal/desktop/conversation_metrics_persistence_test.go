package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestStoppedConversationMetricsDoNotRecalculateOnScroll(t *testing.T) {
	start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	finish := start.Add(time.Second)
	a := &App{state: contracts.Snapshot{
		Revision:      1,
		Sessions:      []contracts.Session{{ID: "session"}},
		Messages:      map[string][]contracts.Message{"session": {{ID: "message", TurnID: "turn", Blocks: []contracts.Block{{Tool: &contracts.ToolCall{ID: "tool", StartedAt: start, FinishedAt: &finish}}}}}},
		Runs:          []contracts.Run{{ID: "turn", SessionID: "session", StartedAt: start, FinishedAt: &finish, Status: "completed"}},
		ModelRequests: []contracts.ModelRequest{metricRequest("session", "turn", "provider", "request", 100, 20, time.Second)},
	}}
	publishMetricsFixture(a)
	a.metricsFor("session", "")
	a.metricsFor("session", "turn")
	if allocs := testing.AllocsPerRun(30, func() { a.metricsFor("session", ""); a.metricsFor("session", "turn") }); allocs != 0 {
		t.Fatalf("scroll rebuilt stopped metrics: %.0f allocations", allocs)
	}
	a.state.ModelRequests = append(a.state.ModelRequests, metricRequest("session", "turn", "provider", "request2", 50, 10, time.Second))
	a.state.Revision++
	publishMetricsFixture(a)
	if stats := a.metricsFor("session", ""); stats.totals.Requests != 2 {
		t.Fatal("new accounting did not invalidate metrics")
	}
}

func TestRunningMetricsOnlyAdvanceTheClock(t *testing.T) {
	start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	now := start.Add(2 * time.Second)
	a := &App{now: func() time.Time { return now }, state: contracts.Snapshot{
		Revision:      1,
		Sessions:      []contracts.Session{{ID: "session", Status: contracts.Working}},
		Runs:          []contracts.Run{{ID: "turn", SessionID: "session", StartedAt: start, Status: "running"}},
		ModelRequests: []contracts.ModelRequest{metricRequest("session", "turn", "provider", "request", 100, 20, time.Second)},
	}}
	publishMetricsFixture(a)
	first := a.metricsFor("session", "")
	now = now.Add(3 * time.Second)
	later := a.metricsFor("session", "")
	if later.wallDuration-first.wallDuration != 3*time.Second || later.totals != first.totals {
		t.Fatal("clock or accounting changed incorrectly")
	}
	a.state.Runs[0].Status = "completed"
	finish := now
	a.state.Runs[0].FinishedAt = &finish
	a.state.Revision++
	publishMetricsFixture(a)
	stopped := a.metricsFor("session", "")
	now = now.Add(time.Hour)
	if a.metricsFor("session", "").wallDuration != stopped.wallDuration {
		t.Fatal("stopped clock kept advancing")
	}
}

func TestSavedMetricsFollowSameRevisionDetails(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.sessionSubscriptionScope = []string{"chat-0"}
	next := contracts.CloneSnapshot(a.state)
	next.Messages["chat-0"] = []contracts.Message{{ID: "loaded", SessionID: "chat-0", TurnID: "loaded-turn"}}
	next.ModelRequests = append(next.ModelRequests, metricRequest("chat-0", "loaded-turn", "provider", "loaded-request", 100, 20, time.Second))
	prepared := &App{state: next}
	publishMetricsFixture(prepared)
	next = prepared.state
	delete(a.state.Messages, "chat-0")

	before := a.metricsFor("chat-0", "").totals.Requests
	a.apply(next)
	if after := a.metricsFor("chat-0", "").totals.Requests; after != before+1 {
		t.Fatalf("same revision details retained old metrics: %d -> %d", before, after)
	}
	stale := contracts.CloneSnapshot(a.state)
	if stale.Revision > 0 {
		stale.Revision--
	}
	stale.ModelRequests = nil
	stale.Metrics = nil
	a.apply(stale)
	if a.metricsFor("chat-0", "").totals.Requests != before+1 {
		t.Fatal("stale snapshot changed saved projection")
	}
}

// Restart/snapshot projections may contain summaries without historical bodies.
// Reading them must not need a client-side reconstruction or depend on revision.
func TestMetricsReadSavedSummaryWithoutExecutionRecords(t *testing.T) {
	a := &App{state: contracts.Snapshot{Metrics: map[string]contracts.ConversationMetrics{
		"session": {Total: contracts.ExecutionMetrics{Totals: contracts.ModelRequestTotals{Requests: 2, Input: 100}, WallKnown: true, WallDuration: 5 * time.Second}, Turns: map[string]contracts.ExecutionMetrics{
			"turn": {Totals: contracts.ModelRequestTotals{Requests: 1, Input: 50}, WallKnown: true, WallDuration: 2 * time.Second},
		}},
	}}}
	for range 20 {
		a.state.Revision++
		if stats := a.metricsFor("session", ""); stats.totals.Input != 100 || stats.wallDuration != 5*time.Second {
			t.Fatal("saved conversation summary needs execution records")
		}
		if stats := a.metricsFor("session", "turn"); stats.totals.Input != 50 || stats.wallDuration != 2*time.Second {
			t.Fatal("saved reply summary needs execution records")
		}
	}
}

func TestActiveSavedMetricsRefreshClockWithoutOtherUIWork(t *testing.T) {
	a := testApp(t, "default")
	now := a.now()
	a.now = func() time.Time { return now }
	a.state.Metrics = map[string]contracts.ConversationMetrics{"clock": {Total: contracts.ExecutionMetrics{
		Totals: contracts.ModelRequestTotals{Requests: 1}, WallKnown: true, RunningSince: now.Add(-2 * time.Second),
	}}}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(a.palette.UI()); a.conversationMetricsView(c, "clock") }, 700, 100)
	if !tt.HasText("0 rounds 1 steps · Time taken 2s") {
		t.Fatalf("missing initial saved clock: %v", tt.Texts())
	}
	now = now.Add(3 * time.Second)
	tt.Advance(time.Second)
	if !tt.HasText("0 rounds 1 steps · Time taken 5s") {
		t.Fatal("active saved clock depended on another UI event")
	}
}
