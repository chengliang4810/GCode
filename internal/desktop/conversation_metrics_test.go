package desktop

import (
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func metricRequest(session, turn, provider, id string, input, output int, duration time.Duration) contracts.ModelRequest {
	return contracts.ModelRequest{ID: id, SessionID: session, TurnID: turn,
		ProviderID: provider, ProviderName: provider, ModelID: "family/model",
		Usage:          model.Usage{Known: true, OutputKnown: true, CacheKnown: true, Input: input, Output: output, Cached: input / 2},
		DecodeDuration: duration}
}

func TestConversationMetricsIncludeOnlyOwnedWork(t *testing.T) {
	a := &App{state: contracts.Snapshot{
		Sessions: []contracts.Session{
			{ID: "main"}, {ID: "child", ParentSessionID: "main", SubagentType: "general"},
			{ID: "nested", ParentSessionID: "child", SubagentType: "general"},
			{ID: "other-child", ParentSessionID: "main", SubagentType: "general"},
			{ID: "side", ParentSessionID: "main", SideChat: true}, {ID: "fork", ParentSessionID: "main"},
		},
		Messages: map[string][]contracts.Message{"main": {{TurnID: "first", Blocks: []contracts.Block{{Subagent: &contracts.Subagent{SessionID: "child"}}}}}},
		ModelRequests: []contracts.ModelRequest{
			metricRequest("main", "first", "A", "a", 100, 100, time.Second),
			metricRequest("main", "first", "A", "retry", 20, 10, time.Second),
			metricRequest("main", "second", "B", "b", 200, 90, 9*time.Second),
			metricRequest("main", "", "Naming", "name", 1, 1, time.Second),
			metricRequest("child", "child-turn", "C", "c", 10, 10, time.Second),
			metricRequest("nested", "nested-turn", "D", "d", 10, 10, time.Second),
			metricRequest("other-child", "other-turn", "C", "e", 5, 5, time.Second),
			metricRequest("side", "side-turn", "S", "s", 999, 999, time.Second),
			metricRequest("fork", "fork-turn", "F", "f", 999, 999, time.Second),
		},
	}}
	reply := a.metricsFor("main", "first")
	if reply.totals.Requests != 4 || reply.totals.Input+reply.totals.Output != 270 || len(reply.models) != 3 {
		t.Fatalf("reply scope or multi-model totals: %+v", reply)
	}
	total := a.metricsFor("main", "")
	if total.totals.Requests != 7 || total.totals.Input+total.totals.Output != 572 {
		t.Fatalf("conversation counted a side chat/fork or omitted maintenance: %+v", total)
	}
}

func TestConversationMetricsUnknownAndZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		totals contracts.ModelRequestTotals
		want   string
	}{
		{"unrecorded", contracts.ModelRequestTotals{}, "— tokens · — tokens/s"},
		{"reported zero", contracts.ModelRequestTotals{Requests: 1}, "0 tokens · — tokens/s"},
		{"unknown", contracts.ModelRequestTotals{Requests: 1, UnknownUsage: 1}, "— tokens · — tokens/s"},
		{"partial", contracts.ModelRequestTotals{Requests: 2, UnknownUsage: 1, Input: 20, Output: 10}, "≥30 tokens · — tokens/s"},
		{"unrecorded earlier reply", contracts.ModelRequestTotals{Requests: 1, Unrecorded: true, Input: 20, Output: 10}, "≥30 tokens · — tokens/s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := metricsReading(tc.totals); got != tc.want {
				t.Fatalf("reading=%q want %q", got, tc.want)
			}
		})
	}
}

func TestConversationMetricsMarkUnrecordedRunsWithoutChargingForks(t *testing.T) {
	a := &App{state: contracts.Snapshot{
		Sessions: []contracts.Session{{ID: "main"}, {ID: "fork", ParentSessionID: "main"}},
		Runs:     []contracts.Run{{ID: "old", SessionID: "main"}},
		Messages: map[string][]contracts.Message{
			"main": {{Role: "assistant", ModelID: "P/M", TurnID: "old"}},
			"fork": {{Role: "assistant", ModelID: "P/M", TurnID: "old"}},
		},
		ModelRequests: []contracts.ModelRequest{metricRequest("main", "new", "P", "r", 100, 20, time.Second)},
	}}
	if stats := a.metricsFor("main", ""); !stats.totals.Unrecorded || !strings.HasPrefix(metricsReading(stats.totals), "≥120") {
		t.Fatal("missing earlier accounting was presented as a complete total")
	}
	if stats := a.metricsFor("fork", ""); stats.totals.Unrecorded || stats.totals.Requests != 0 {
		t.Fatal("inherited text was charged to its fork")
	}
}

func TestConversationMetricsVisibilityFollowsUsefulData(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request contracts.ModelRequest
		want    string
	}{
		{name: "unknown failed request"},
		{name: "reported zero", request: contracts.ModelRequest{Usage: model.Usage{Known: true}}, want: "0 tokens · — tokens/s · — cache"},
		{name: "partial usage", request: contracts.ModelRequest{Usage: model.Usage{Input: 12}}, want: "≥12 tokens · — tokens/s · — cache"},
		{name: "speed only", request: contracts.ModelRequest{Usage: model.Usage{OutputKnown: true}, DecodeDuration: time.Second}, want: "— tokens · 0.0 tokens/s · — cache"},
		{name: "cache without usable denominator", request: contracts.ModelRequest{Usage: model.Usage{CacheKnown: true}}},
		{name: "cache", request: contracts.ModelRequest{Usage: model.Usage{CacheKnown: true, Input: 20, Cached: 10}}, want: "≥20 tokens · — tokens/s · 50.0% cache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.ModelRequests = nil
			a.state.Runs = nil
			sessionID := "metrics"
			tt := ui.NewTester(func(c *ui.Context) {
				a.conversationMetricsView(c, sessionID)
			}, 720, 80)
			if len(tt.Texts()) != 0 {
				t.Fatalf("empty conversation exposed metrics: %v", tt.Texts())
			}
			tc.request.SessionID = sessionID
			a.state.ModelRequests = []contracts.ModelRequest{tc.request}
			tt.Frame()
			if tc.want == "" {
				if len(tt.Texts()) != 0 {
					t.Fatalf("unknown data exposed placeholder metrics: %v", tt.Texts())
				}
				return
			}
			if !tt.HasText(tc.want) {
				t.Fatalf("first useful data did not reveal metrics: %v", tt.Texts())
			}
			a.state.ModelRequests = append(a.state.ModelRequests, contracts.ModelRequest{SessionID: sessionID})
			tt.Frame()
			if len(tt.Texts()) == 0 {
				t.Fatal("subsequent unknown request hid existing metrics")
			}
			sessionID = "empty"
			tt.Frame()
			if len(tt.Texts()) != 0 {
				t.Fatalf("switching sessions retained another conversation's metrics: %v", tt.Texts())
			}
			sessionID = "metrics"
			tt.Frame()
			if len(tt.Texts()) == 0 {
				t.Fatal("returning to the conversation lost its metrics")
			}
		})
	}
}

func TestConversationMetricsFooterThemesAndNarrowWidth(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := testApp(t, "default")
		a.palette = design.Palette("rcode-dark", dark)
		a.state.ModelRequests = []contracts.ModelRequest{metricRequest("metrics", "turn", "Custom provider", "r", 100, 20, time.Second)}
		message := contracts.Message{ID: "reply", SessionID: "metrics", TurnID: "turn", CreatedAt: time.Now(),
			Blocks: []contracts.Block{{Kind: "text", Text: "copy only this reply"}}}
		for _, width := range []int{320, 720} {
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Children(func() {
					a.transcriptFooter(c, message, true)
					a.conversationMetricsView(c, "metrics")
				})
			}, width, 180)
			if !tt.HasText("120 tokens · 20.0 tokens/s · 50.0% cache") || !tt.HasText("120 tokens · 20.0 tokens/s · 50.0% cache · Custom provider / family/model") {
				t.Fatal("metrics or frozen provider/model identity missing")
			}
			if err := tt.Click("Copy message"); err != nil || tt.Clipboard() != "copy only this reply" {
				t.Fatalf("copy action copied accounting or broke: %q %v", tt.Clipboard(), err)
			}
			if text, ok := tt.Find("120 tokens · 20.0 tokens/s · 50.0% cache\nConversation total\nInput: 100 · Output: 20 · Cached: 50\nSpeed = reported output tokens / decode time; excludes first-token wait and tools.\nSpeed available for 1/1 requests.\nCache hit = cached input / input from requests reporting cache usage.\nCache data available for 1/1 requests."); !ok || text.X < 0 || text.X+text.W > float32(width+1) {
				t.Fatalf("footer outside narrow pane: %+v", text)
			}
			if strings.Contains(metricsReading(a.metricsFor("metrics", "").totals), "provider") {
				t.Fatal("conversation reading unnecessarily names a provider")
			}
		}
	}
}
