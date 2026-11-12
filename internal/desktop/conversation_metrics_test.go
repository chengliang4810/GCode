package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
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
	t0 := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	at := func(seconds int) *time.Time {
		end := t0.Add(time.Duration(seconds) * time.Second)
		return &end
	}
	a := &App{state: contracts.Snapshot{
		Sessions: []contracts.Session{
			{ID: "main"}, {ID: "child", ParentSessionID: "main", ParentRunID: "first", SubagentType: "general"},
			{ID: "nested", ParentSessionID: "child", ParentRunID: "child-turn", SubagentType: "general"},
			{ID: "other-child", ParentSessionID: "main", ParentRunID: "second", SubagentType: "general"},
			{ID: "side", ParentSessionID: "main", SideChat: true}, {ID: "fork", ParentSessionID: "main"},
		},
		Messages: map[string][]contracts.Message{"main": {{TurnID: "first", Blocks: []contracts.Block{{Subagent: &contracts.Subagent{SessionID: "child"}}}}}},
		Runs: []contracts.Run{
			{ID: "first", SessionID: "main", StartedAt: t0, FinishedAt: at(10)},
			{ID: "child-turn", SessionID: "child", StartedAt: t0.Add(2 * time.Second), FinishedAt: at(5)},
			{ID: "nested-turn", SessionID: "nested", StartedAt: t0.Add(3 * time.Second), FinishedAt: at(4)},
			{ID: "other-turn", SessionID: "other-child", StartedAt: t0.Add(6 * time.Second), FinishedAt: at(8)},
			{ID: "second", SessionID: "main", StartedAt: t0.Add(20 * time.Second), FinishedAt: at(35)},
			{ID: "side-turn", SessionID: "side", StartedAt: t0, FinishedAt: at(60)},
			{ID: "fork-turn", SessionID: "fork", StartedAt: t0, FinishedAt: at(90)},
		},
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
	publishMetricsFixture(a)
	reply := a.metricsFor("main", "first")
	if reply.totals.Requests != 4 || reply.totals.Input+reply.totals.Output != 270 ||
		len(reply.models) != 3 || reply.generationDuration != 4*time.Second {
		t.Fatalf("reply scope or multi-model totals: %+v", reply)
	}
	if reply.wallDuration != 10*time.Second {
		t.Fatalf("reply wall time counted overlapping subagent runs: %v", reply.wallDuration)
	}
	total := a.metricsFor("main", "")
	if total.totals.Requests != 7 || total.totals.Input+total.totals.Output != 572 || total.generationDuration != 15*time.Second {
		t.Fatalf("conversation counted a side chat/fork or omitted maintenance: %+v", total)
	}
	if total.wallDuration != 25*time.Second {
		t.Fatalf("conversation wall time counted a side chat/fork or double counted parallel runs: %v", total.wallDuration)
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
		{"partial", contracts.ModelRequestTotals{Requests: 2, UnknownUsage: 1, Input: 20, Output: 10}, "30 Token"},
		{"unrecorded earlier reply", contracts.ModelRequestTotals{Requests: 1, Unrecorded: true, Input: 20, Output: 10}, "30 Token"},
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
	publishMetricsFixture(a)
	if stats := a.metricsFor("main", ""); !stats.totals.Unrecorded || stats.totals.Input+stats.totals.Output != 120 {
		t.Fatal("missing earlier accounting was presented as a complete total")
	}
	publishMetricsFixture(a)
	if stats := a.metricsFor("fork", ""); stats.totals.Unrecorded || stats.totals.Requests != 0 {
		t.Fatal("inherited text was charged to its fork")
	}
}

func TestConversationMetricsVisibilityFollowsUsefulData(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request contracts.ModelRequest
		run     time.Duration
		zeroRun bool
		want    string
	}{
		{name: "unknown failed request"},
		{name: "reported zero", request: contracts.ModelRequest{Usage: model.Usage{Known: true}}, want: "0 Token"},
		{name: "partial usage", request: contracts.ModelRequest{Usage: model.Usage{Input: 12}}, want: "12 Token"},
		{name: "generation time only", request: contracts.ModelRequest{DecodeDuration: time.Second}, want: "0 rounds 1 steps · Time taken —"},
		{name: "wall time", request: contracts.ModelRequest{Usage: model.Usage{Known: true}}, run: 2500 * time.Millisecond, want: "0 rounds 1 steps · Time taken 2s"},
		{name: "known zero wall time", request: contracts.ModelRequest{Usage: model.Usage{Known: true}}, zeroRun: true, want: "0 rounds 1 steps · Time taken 0s"},
		{name: "negative generation time", request: contracts.ModelRequest{DecodeDuration: -time.Second}},
		{name: "cache without usable denominator", request: contracts.ModelRequest{Usage: model.Usage{CacheKnown: true}}},
		{name: "cache", request: contracts.ModelRequest{Usage: model.Usage{CacheKnown: true, Input: 20, Cached: 10}}, want: "20 Token · Cache hit 50.0%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.ModelRequests = nil
			a.state.Runs = nil
			sessionID := "metrics"
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).FillWidth().Children(func() { publishMetricsFixture(a); a.conversationMetricsView(c, sessionID) })
			}, 720, 80)
			if len(tt.Texts()) != 0 {
				t.Fatalf("empty conversation exposed metrics: %v", tt.Texts())
			}
			tc.request.SessionID = sessionID
			a.state.ModelRequests = []contracts.ModelRequest{tc.request}
			if tc.run > 0 || tc.zeroRun {
				started := a.now().Add(-tc.run)
				finished := a.now()
				a.state.Runs = []contracts.Run{{ID: "run", SessionID: sessionID, StartedAt: started, FinishedAt: &finished, DurationMS: tc.run.Milliseconds()}}
			}
			a.state.Revision++ // The core publishes new accounting with a new revision.
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
			a.state.Revision++
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

func TestMetricDurationUsesWholeSeconds(t *testing.T) {
	for _, test := range []struct {
		name     string
		duration time.Duration
		known    bool
		want     string
	}{
		{"unknown", time.Second, false, "—"},
		{"zero", 0, true, "0s"},
		{"negative", -time.Second, true, "0s"},
		{"subsecond", 900 * time.Millisecond, true, "0s"},
		{"fraction discarded", 2500 * time.Millisecond, true, "2s"},
		{"last second", time.Minute - time.Nanosecond, true, "59s"},
		{"minute rollover", time.Minute, true, "1m 00s"},
		{"last minute", time.Hour - time.Nanosecond, true, "59m 59s"},
		{"hour rollover", time.Hour, true, "1h 00m 00s"},
		{"long running", 353*time.Minute + 48900*time.Millisecond, true, "5h 53m 48s"},
		{"last hour", 24*time.Hour - time.Nanosecond, true, "23h 59m 59s"},
		{"day rollover", 24 * time.Hour, true, "1d 00h 00m 00s"},
		{"multiple days", 51*time.Hour + 4*time.Minute + 5900*time.Millisecond, true, "2d 03h 04m 05s"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := metricDuration(test.duration, test.known); got != test.want {
				t.Fatalf("elapsed = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConversationMetricsElapsedSecondsAndStableDigits(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, dark := range []bool{false, true} {
			for _, reply := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/dark=%v/reply=%v", language, dark, reply), func(t *testing.T) {
					a := testApp(t, "default")
					a.state.Settings.General.Language = language
					a.palette = design.Palette("rcode-dark", dark)
					stats := conversationMetrics{turns: 2, wallKnown: true, totals: contracts.ModelRequestTotals{Requests: 47, Input: 100}}
					tt := ui.NewTester(func(c *ui.Context) {
						localize.Set(c, language)
						c.SetTheme(a.palette.UI())
						ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
							a.metricsGroup(c, "elapsed", stats, reply)
						})
					}, 900, 100)
					tt.SetLocale(i18n.Native(language))
					for _, format := range []struct {
						base   time.Duration
						prefix string
					}{
						{0, ""},
						{42 * time.Minute, "42m "},
						{5*time.Hour + 53*time.Minute, "5h 53m "},
						{51*time.Hour + 4*time.Minute, "2d 03h 04m "},
					} {
						var initial ui.Rect
						for i, seconds := range []int{11, 12, 18, 19, 20, 22, 28, 38, 48, 58, 59} {
							for _, fractional := range []time.Duration{100 * time.Millisecond, 900 * time.Millisecond} {
								stats.wallDuration = format.base + time.Duration(seconds)*time.Second + fractional
								tt.Frame()
								duration := fmt.Sprintf("%ds", seconds)
								if format.prefix != "" {
									duration = fmt.Sprintf("%s%02ds", format.prefix, seconds)
								}
								reading := a.t("Time taken") + " " + duration
								if !reply {
									reading = fmt.Sprintf(a.t("%d rounds %d steps"), 2, 47) + " · " + reading
								}
								bounds, ok := tt.Find(a.t("Time taken") + ": " + reading)
								if !ok {
									t.Fatalf("integer-second reading missing: %q; got %v", reading, tt.Texts())
								}
								if i == 0 && fractional == 100*time.Millisecond {
									initial = bounds
								}
								if bounds != initial {
									t.Fatalf("elapsed digits moved the footer: initial=%+v actual=%+v at %v", initial, bounds, stats.wallDuration)
								}
							}
						}
					}
				})
			}
		}
	}
}

func TestConversationMetricsFooterThemesAndNarrowWidth(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := testApp(t, "default")
		a.palette = design.Palette("rcode-dark", dark)
		a.state.ModelRequests = []contracts.ModelRequest{metricRequest("metrics", "turn", "Custom provider", "r", 100, 20, time.Second)}
		turnStarted, turnFinished := a.now().Add(-time.Second), a.now()
		a.state.Runs = []contracts.Run{{ID: "turn", SessionID: "metrics", StartedAt: turnStarted, FinishedAt: &turnFinished, DurationMS: 1000}}
		message := contracts.Message{ID: "reply", SessionID: "metrics", TurnID: "turn", CreatedAt: time.Now(),
			Blocks: []contracts.Block{{Kind: "text", Text: "copy only this reply"}}}
		publishMetricsFixture(a)
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
			timing, timingOK := tt.Find("Time taken: Time taken 1s")
			usage, usageOK := tt.Find("Turn usage: Usage 120 Token")
			copyButton, copyOK := tt.Find("Copy message")
			dateText, dateOK := tt.Find(dateLabel)
			if !timingOK || !usageOK || !copyOK || !dateOK ||
				copyButton.X+copyButton.W > usage.X || usage.X+usage.W > timing.X || timing.X+timing.W > dateText.X {
				t.Fatalf("reply footer must show copy, usage, time, date: copy=%+v usage=%+v time=%+v date=%+v",
					copyButton, usage, timing, dateText)
			}
			conversationTime, timeOK := tt.Find("Time taken: 1 rounds 1 steps · Time taken 1s")
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
			conversationTime, ok := tt.Find("Time taken: 1 rounds 1 steps · Time taken 1s")
			if !ok {
				t.Fatal("conversation time trigger missing")
			}
			tt.Move(conversationTime.X+conversationTime.W/2, conversationTime.Y+conversationTime.H/2)
			tt.Advance(time.Second)
			if !tt.HasText("Time taken") || !tt.HasText("Output speed") || !tt.HasText("1s") {
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
	turnStarted, turnFinished := a.now().Add(-4*time.Second), a.now()
	a.state.Runs = []contracts.Run{{ID: "turn", SessionID: "metrics", StartedAt: turnStarted, FinishedAt: &turnFinished, DurationMS: 4000}}
	message := contracts.Message{ID: "reply", SessionID: "metrics", TurnID: "turn"}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Fill().Children(func() { publishMetricsFixture(a); a.replyMetricsView(c, message) })
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
	a.state.Revision++
	tt.Frame()
	if !tt.HasText("480 Token") || tt.HasText("Cache count coverage") || tt.HasText("2/3 requests") {
		t.Fatalf("open panel did not update with new accounting: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("Time taken: Time taken 4s"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("0/3 requests") || !tt.HasText("—") || !tt.HasText("20 Token/s") || !tt.HasText("4s") {
		t.Fatalf("historical missing timing was inferred: %v", tt.Texts())
	}
}
