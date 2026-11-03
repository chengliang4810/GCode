package desktop

import (
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
	if reply.totals.Requests != 4 || reply.totals.Input+reply.totals.Output != 270 ||
		len(reply.models) != 3 || reply.generationDuration != 4*time.Second {
		t.Fatalf("reply scope or multi-model totals: %+v", reply)
	}
	total := a.metricsFor("main", "")
	if total.totals.Requests != 7 || total.totals.Input+total.totals.Output != 572 || total.generationDuration != 15*time.Second {
		t.Fatalf("conversation counted a side chat/fork or omitted maintenance: %+v", total)
	}
}

func TestConversationMetricsUnknownAndZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		totals contracts.ModelRequestTotals
		want   string
	}{
		{"unrecorded", contracts.ModelRequestTotals{}, "—"},
		{"reported zero", contracts.ModelRequestTotals{Requests: 1}, "0 Token"},
		{"unknown", contracts.ModelRequestTotals{Requests: 1, UnknownUsage: 1}, "—"},
		{"partial", contracts.ModelRequestTotals{Requests: 2, UnknownUsage: 1, Input: 20, Output: 10}, "≥30 Token"},
		{"unrecorded earlier reply", contracts.ModelRequestTotals{Requests: 1, Unrecorded: true, Input: 20, Output: 10}, "≥30 Token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			a := &App{}
			ui.NewTester(func(c *ui.Context) {
				got = a.metricTokens(c, tc.totals.Input+tc.totals.Output, tc.totals.Requests == 0 || tc.totals.UnknownUsage > 0 || tc.totals.Unrecorded)
			}, 1, 1)
			if got != tc.want {
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
	if stats := a.metricsFor("main", ""); !stats.totals.Unrecorded || stats.totals.Input+stats.totals.Output != 120 {
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
		{name: "reported zero", request: contracts.ModelRequest{Usage: model.Usage{Known: true}}, want: "0 Token"},
		{name: "partial usage", request: contracts.ModelRequest{Usage: model.Usage{Input: 12}}, want: "≥12 Token"},
		{name: "generation time only", request: contracts.ModelRequest{DecodeDuration: time.Second}, want: "0 rounds 1 steps · Time taken 1.00s"},
		{name: "negative generation time", request: contracts.ModelRequest{DecodeDuration: -time.Second}},
		{name: "cache without usable denominator", request: contracts.ModelRequest{Usage: model.Usage{CacheKnown: true}}},
		{name: "cache", request: contracts.ModelRequest{Usage: model.Usage{CacheKnown: true, Input: 20, Cached: 10}}, want: "≥20 Token · Cache hit 50.0%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.ModelRequests = nil
			a.state.Runs = nil
			sessionID := "metrics"
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).FillWidth().Children(func() { a.conversationMetricsView(c, sessionID) })
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
			var dateLabel string
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				dateLabel = date(c, message.CreatedAt.Local(), "Jan 2, 3:04 PM")
				ui.Column(c).Fill().Children(func() {
					a.transcriptFooter(c, message, true)
					a.conversationMetricsView(c, "metrics")
				})
			}, width, 700)
			tt.Advance(time.Second)
			timing, timingOK := tt.Find("Time taken: Time taken 1.00s")
			usage, usageOK := tt.Find("Turn usage: Usage 120 Token")
			copyButton, copyOK := tt.Find("Copy message")
			dateText, dateOK := tt.Find(dateLabel)
			if !timingOK || !usageOK || !copyOK || !dateOK ||
				copyButton.X+copyButton.W > usage.X || usage.X+usage.W > timing.X || timing.X+timing.W > dateText.X {
				t.Fatalf("reply footer must show copy, usage, time, date: copy=%+v usage=%+v time=%+v date=%+v",
					copyButton, usage, timing, dateText)
			}
			conversationTime, timeOK := tt.Find("Time taken: 1 rounds 1 steps · Time taken 1.00s")
			conversationUsage, usageOK := tt.Find("Conversation usage: 120 Token · Cache hit 50.0%")
			if !timeOK || !usageOK || conversationTime.X+conversationTime.W > conversationUsage.X {
				t.Fatalf("composer footer must show time before usage: time=%+v usage=%+v", conversationTime, conversationUsage)
			}
			if err := tt.Click("Copy message"); err != nil || tt.Clipboard() != "copy only this reply" {
				t.Fatalf("copy action copied accounting or broke: %q %v", tt.Clipboard(), err)
			}
			tt.Move(usage.X+usage.W/2, usage.Y+usage.H/2)
			tt.Advance(time.Second)
			if !tt.HasText("Custom provider · family/model") || !tt.HasText("50.0%") {
				t.Fatalf("full reply panel missing fields: %v", tt.Texts())
			}
			if tt.HasText("Cache count coverage") || tt.HasText("1/1 requests") {
				t.Fatalf("usage panel still shows count coverage: %v", tt.Texts())
			}
			if panel, ok := tt.Find("Turn usage"); !ok || panel.X < 0 || panel.X+panel.W > float32(width+1) {
				t.Fatalf("panel outside narrow pane: %+v", panel)
			} else {
				tt.Move(panel.X+2, panel.Y+2)
				tt.Advance(time.Second)
				if !tt.HasText("Custom provider · family/model") {
					t.Fatal("moving into the usage panel closed it")
				}
			}
			tt.Key(0, ui.KeyEscape)
			if _, ok := tt.Find("Turn usage"); ok {
				t.Fatalf("Escape left usage panel open: %v", tt.Texts())
			}
			tt.Move(float32(width-1), 699)
			tt.Advance(time.Second)
			conversationTime, ok := tt.Find("Time taken: 1 rounds 1 steps · Time taken 1.00s")
			if !ok {
				t.Fatal("conversation time trigger missing")
			}
			tt.Move(conversationTime.X+conversationTime.W/2, conversationTime.Y+conversationTime.H/2)
			tt.Advance(time.Second)
			if !tt.HasText("Time taken") || !tt.HasText("Output speed") || !tt.HasText("1.00s") {
				t.Fatalf("time panel did not open at %+v (width %d): %v", conversationTime, width, tt.Texts())
			}
			for _, removed := range []string{"Speed count coverage", "First-token count coverage", "Speed excludes first-token wait and tool execution."} {
				if tt.HasText(removed) {
					t.Fatalf("time panel still shows %q", removed)
				}
			}
			tt.Move(float32(width-1), 699)
			tt.Advance(time.Second)
			if tt.HasText("Output speed") {
				t.Fatal("leaving the trigger and panel did not close the time panel")
			}
		}
	}
}

func TestConversationMetricsMultiModelPanelUpdatesAndMissingTiming(t *testing.T) {
	a := testApp(t, "default")
	first := metricRequest("metrics", "turn", "Provider A", "first", 100, 20, time.Second)
	second := metricRequest("metrics", "turn", "Provider B", "second", 200, 40, 2*time.Second)
	second.Usage.CacheKnown = false
	a.state.ModelRequests = []contracts.ModelRequest{first, second}
	message := contracts.Message{ID: "reply", SessionID: "metrics", TurnID: "turn"}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Fill().Children(func() { a.replyMetricsView(c, message) })
	}, 700, 700)
	if err := tt.Click("Turn usage: Usage 360 Token"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Provider A · family/model", "Provider B · family/model", "50.0%", "—"} {
		if !tt.HasText(text) {
			t.Fatalf("multi-model or missing-cache detail omitted %q: %v", text, tt.Texts())
		}
	}
	a.state.ModelRequests = append(a.state.ModelRequests, metricRequest("metrics", "turn", "Provider A", "third", 100, 20, time.Second))
	tt.Frame()
	if !tt.HasText("480 Token") || tt.HasText("Cache count coverage") || tt.HasText("2/3 requests") {
		t.Fatalf("open panel did not update with new accounting: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("Time taken: Time taken 4.00s"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("0/3 requests") || !tt.HasText("—") || !tt.HasText("20 Token/s") || !tt.HasText("4.00s") {
		t.Fatalf("historical missing timing was inferred: %v", tt.Texts())
	}
}
