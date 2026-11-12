package appusage

import (
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestBuildRecordedUsageAcrossModelsAndForks(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	first := contracts.Message{ID: "first", TurnID: "run-one", SessionID: "parent", Role: "assistant",
		ModelID: "one/a", CreatedAt: now.AddDate(0, 0, -1), Usage: contracts.Usage{InputTokens: 100, OutputTokens: 30, CachedTokens: 60},
		Blocks: []contracts.Block{{Kind: "tool", Tool: &contracts.ToolCall{ID: "read"}}}}
	second := contracts.Message{ID: "second", TurnID: "run-one", SessionID: "parent", Role: "assistant",
		ModelID: "one/a", CreatedAt: now.AddDate(0, 0, -1), Usage: contracts.Usage{InputTokens: 50, OutputTokens: 20, CachedTokens: 10}}
	third := contracts.Message{ID: "third", TurnID: "run-two", SessionID: "fork", Role: "assistant",
		ModelID: "two/b", CreatedAt: now, Usage: contracts.Usage{InputTokens: 10, OutputTokens: 5}}
	forkFirst, forkSecond := first, second
	forkFirst.ID, forkFirst.SessionID = "fork-first", "fork"
	forkSecond.ID, forkSecond.SessionID = "fork-second", "fork"
	report := Build(map[string][]contracts.Message{
		"parent": {first, second}, "fork": {forkFirst, forkSecond, third},
	}, []contracts.Run{
		{ID: "run-one", SessionID: "parent", Status: "completed", DurationMS: 90000},
		{ID: "run-two", SessionID: "fork", Status: "completed", DurationMS: 60000},
		{ID: "run-three", SessionID: "parent", Status: "completed", DurationMS: 30000},
		{ID: "run-one", SessionID: "parent", Status: "completed", DurationMS: 90000},
		{ID: "failed", SessionID: "parent", Status: "failed", DurationMS: 9000000},
	}, now)
	s := report.Summary
	if s.Total != 215 || s.Input != 160 || s.Output != 55 || s.Cached != 70 {
		t.Fatalf("fork duplicated consumption or cached input counted twice: %+v", s)
	}
	if s.Peak != 200 || s.LongestSessionMS != 120000 || s.CurrentStreak != 2 || s.LongestStreak != 2 {
		t.Fatalf("incorrect lifetime summary: %+v", s)
	}
	days := report.Window(now, 7)
	if len(days) != 7 || days[5].Turns != 1 || days[5].Tools != 1 || days[6].Turns != 1 || days[0].Tokens != 0 {
		t.Fatalf("calendar gap filling or per-execution counting failed: %+v", days)
	}
	heatmap := report.Heatmap(now)
	if len(heatmap) != 364 || heatmap[0].Date.Weekday() != time.Sunday || heatmap[363].Date.Weekday() != time.Saturday {
		t.Fatal("heatmap is not aligned to 52 natural weeks")
	}
	models := Models(days)
	if len(models) != 2 || models[0].ID != "one/a" || models[0].Tokens != 200 || models[1].Tokens != 15 {
		t.Fatalf("actual request models were lost: %+v", models)
	}
}

func TestBuildRecordsMatchesResidentMessageUsage(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	messages := map[string][]contracts.Message{
		"history": {
			{ID: "first", TurnID: "turn", Role: "assistant", ModelID: "provider/model", CreatedAt: now,
				Usage:  contracts.Usage{InputTokens: 20, OutputTokens: 5},
				Blocks: []contracts.Block{{Kind: "tool", Tool: &contracts.ToolCall{ID: "tool"}}}},
		},
	}
	fromMessages := Build(messages, nil, now)
	fromRecords := BuildRecords(map[string][]contracts.UsageRecord{
		"history": contracts.UsageRecordsFromMessages(messages["history"]),
	}, nil, now)
	if !reflect.DeepEqual(fromMessages, fromRecords) {
		t.Fatalf("records changed usage report:\nmessages=%+v\nrecords=%+v", fromMessages, fromRecords)
	}
}

func TestBuildWithRecordsResidentMessagesReplaceSessionSummary(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	resident := contracts.Message{
		ID: "message", TurnID: "turn", Role: "assistant", ModelID: "provider/model", CreatedAt: now,
		Usage: contracts.Usage{InputTokens: 40, OutputTokens: 10},
	}
	report := BuildWithRecords(map[string][]contracts.UsageRecord{
		"history": {{
			ID: "message", TurnID: "turn", Role: "assistant", ModelID: "provider/model",
			CreatedAt: now, Usage: contracts.Usage{InputTokens: 1, OutputTokens: 1},
		}},
	}, map[string][]contracts.Message{"history": {resident}}, nil, now)
	want := Build(map[string][]contracts.Message{"history": {resident}}, nil, now)
	if !reflect.DeepEqual(report, want) {
		t.Fatalf("resident messages did not replace stale summary:\nmerged=%+v\nresident=%+v", report, want)
	}
}

func TestBuildRecordsDeduplicatesForks(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	record := contracts.UsageRecord{
		ID: "parent-message", TurnID: "turn", Role: "assistant", ModelID: "provider/model", CreatedAt: now,
		Usage: contracts.Usage{InputTokens: 20, OutputTokens: 5}, BlockCount: 1,
	}
	fork := record
	fork.ID = "fork-message"
	if got := BuildRecords(map[string][]contracts.UsageRecord{
		"parent": {record},
		"fork":   {fork},
	}, nil, now).Summary.Total; got != 25 {
		t.Fatalf("fork records were counted twice: total=%d", got)
	}
}

func TestBuildCalendarBoundariesAndUnknownMetadata(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 30, 0, 0, time.FixedZone("Shanghai", 8*3600))
	report := Build(map[string][]contracts.Message{"s": {
		{ID: "prior", CreatedAt: now.Add(-time.Hour).UTC(), Usage: contracts.Usage{InputTokens: 20}},
		{ID: "undated", Usage: contracts.Usage{InputTokens: 5}},
		{ID: "negative", CreatedAt: now, Usage: contracts.Usage{InputTokens: -8, OutputTokens: -2, CachedTokens: -1}},
		{ID: "future", CreatedAt: now.Add(time.Hour), Usage: contracts.Usage{InputTokens: 1000}},
	}}, nil, now)
	if s := report.Summary; s.Total != 25 || s.UnknownModelTokens != 25 || s.CurrentStreak != 0 || s.LongestStreak != 1 {
		t.Fatalf("unknown, future or negative records corrupted totals: %+v", s)
	}
	days := report.Window(now, 7)
	if days[5].Tokens != 20 || days[6].Tokens != 0 {
		t.Fatal("request was attributed to the wrong local date")
	}
	models := Models(days)
	if len(models) != 1 || models[0].ID != "" || models[0].Tokens != 20 {
		t.Fatalf("missing model was guessed instead of kept unknown: %+v", models)
	}
}

func TestWindowUsesCalendarDaysAcrossDST(t *testing.T) {
	// The Go toolchain ships the IANA zone data used on Windows.
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, location)
	days := (Report{}).Window(now, 7)
	for i := 1; i < len(days); i++ {
		if Date(days[i-1].Date.AddDate(0, 0, 1)) != Date(days[i].Date) || days[i].Date.Hour() != 0 {
			t.Fatalf("calendar skipped a date around DST: %+v", days)
		}
	}
}

func TestContractReportRoundTripPreservesChartData(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	want := Report{
		Summary: Summary{Total: 42, Input: 30, Output: 12, Cached: 8, Peak: 20, LongestSessionMS: 9_000, CurrentStreak: 2, LongestStreak: 3, UnknownModelTokens: 4},
		Days: map[string]Day{
			Date(now): {Date: Midnight(now), Tokens: 42, Turns: 2, Tools: 1, Models: map[string]int64{"provider/model": 42}},
		},
	}
	got := FromContractReport(ToContractReport(want))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contract report round trip changed chart data:\n got=%+v\nwant=%+v", got, want)
	}
}
