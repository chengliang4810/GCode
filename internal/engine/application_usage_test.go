package engine

import (
	"context"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestRefreshApplicationUsageLockedUpdatesLoadedDirtySessions(t *testing.T) {
	now := time.Now()
	engine := &Engine{
		state: contracts.Snapshot{
			Sessions: []contracts.Session{{ID: "history"}, {ID: "resident"}},
			Messages: map[string][]contracts.Message{
				"resident": {{
					ID: "message", TurnID: "turn", Role: "assistant", ModelID: "provider/model", CreatedAt: now,
					Usage: contracts.Usage{InputTokens: 40, OutputTokens: 10},
				}},
			},
			Runs: []contracts.Run{{ID: "run", SessionID: "resident", Status: "completed", DurationMS: 1200}},
			ApplicationUsage: &contracts.ApplicationUsage{
				Records: map[string][]contracts.UsageRecord{
					"history": {{ID: "old", Usage: contracts.Usage{InputTokens: 20}}},
					"deleted": {{ID: "stale", Usage: contracts.Usage{InputTokens: 10}}},
				},
				Runs: []contracts.Run{{ID: "old-run", SessionID: "deleted", Status: "completed", DurationMS: 99}},
			},
		},
		loadedSessions: map[string]bool{"resident": true},
	}

	engine.refreshApplicationUsageLocked([]string{"resident", "deleted"})
	usage := engine.state.ApplicationUsage
	if usage == nil {
		t.Fatal("refresh did not create application usage state")
	}
	if _, ok := usage.Records["deleted"]; ok {
		t.Fatal("deleted session retained compact usage records")
	}
	if len(usage.Records["resident"]) != 1 || usage.Records["resident"][0].Usage.InputTokens != 40 {
		t.Fatalf("resident records were not refreshed: %+v", usage.Records["resident"])
	}
	if len(usage.Records["history"]) != 1 || usage.Records["history"][0].ID != "old" {
		t.Fatalf("unloaded history was migrated or rewritten: %+v", usage.Records["history"])
	}
	if usage.Report.Summary.Total != 70 || usage.Report.Summary.LongestSessionMS != 1200 {
		t.Fatalf("report used the wrong compact source: %+v", usage.Report.Summary)
	}
}

func TestRefreshApplicationUsageLockedDoesNotImportUnloadedMessages(t *testing.T) {
	now := time.Now()
	engine := &Engine{
		state: contracts.Snapshot{
			Sessions: []contracts.Session{{ID: "history"}},
			Messages: map[string][]contracts.Message{
				"history": {{ID: "body", CreatedAt: now, Usage: contracts.Usage{InputTokens: 90}}},
			},
		},
		loadedSessions: map[string]bool{"history": false},
	}

	engine.refreshApplicationUsageLocked([]string{"history"})
	if engine.state.ApplicationUsage != nil {
		t.Fatal("unloaded session created a compact ledger from resident messages")
	}
}

func TestReadApplicationUsageReturnsOwnedReportWithoutLedger(t *testing.T) {
	date := time.Now()
	engine := &Engine{state: contracts.Snapshot{ApplicationUsage: &contracts.ApplicationUsage{
		Records: map[string][]contracts.UsageRecord{"session": {{ID: "message"}}},
		Runs:    []contracts.Run{{ID: "run", SessionID: "session"}},
		Report: contracts.ApplicationUsageReport{
			Summary: contracts.ApplicationUsageSummary{Total: 42},
			Days: map[string]contracts.ApplicationUsageDay{
				contractsDate(date): {Date: date, Tokens: 42, Models: map[string]int64{"provider/model": 42}},
			},
		},
		GeneratedAt: date,
	}}}

	got, err := engine.ReadApplicationUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Records != nil || got.Runs != nil || got.Report.Summary.Total != 42 {
		t.Fatalf("read returned compact ledger or lost report: %+v", got)
	}
	got.Report.Days[contractsDate(date)].Models["provider/model"] = 0
	if engine.state.ApplicationUsage.Report.Days[contractsDate(date)].Models["provider/model"] != 42 {
		t.Fatal("read report shares nested model map with engine state")
	}
}

func contractsDate(value time.Time) string {
	return value.Format("2006-01-02")
}

func TestRestoreUnavailableUsageKeepsCommittedSessionLedger(t *testing.T) {
	current := &contracts.ApplicationUsage{
		Records: map[string][]contracts.UsageRecord{
			"available": {{ID: "new", Usage: contracts.Usage{InputTokens: 30}}},
			"offline":   {{ID: "new-offline", Usage: contracts.Usage{InputTokens: 90}}},
		},
		Runs:        []contracts.Run{{ID: "new-run", SessionID: "offline", Status: "completed", DurationMS: 900}},
		GeneratedAt: time.Now(),
	}
	committed := &contracts.ApplicationUsage{
		Records: map[string][]contracts.UsageRecord{
			"offline": {{ID: "old-offline", Usage: contracts.Usage{InputTokens: 10}}},
		},
		Runs:        []contracts.Run{{ID: "old-run", SessionID: "offline", Status: "completed", DurationMS: 100}},
		GeneratedAt: current.GeneratedAt,
	}

	restored := RestoreUnavailableUsage(current, committed, []string{"offline"})
	if got := restored.Records["offline"][0].ID; got != "old-offline" {
		t.Fatalf("unavailable record = %q, want committed record", got)
	}
	if len(restored.Runs) != 1 || restored.Runs[0].ID != "old-run" {
		t.Fatalf("unavailable runs = %+v, want committed run", restored.Runs)
	}
	if restored.Report.Summary.Total != 40 {
		t.Fatalf("restored report total = %d, want 40", restored.Report.Summary.Total)
	}
	if current.Records["offline"][0].ID != "new-offline" {
		t.Fatal("restore mutated current application usage")
	}
}

func TestRefreshApplicationUsageLockedPreservesRunOrderWhenUnchanged(t *testing.T) {
	now := time.Now()
	engine := &Engine{
		state: contracts.Snapshot{
			Sessions: []contracts.Session{{ID: "first"}, {ID: "second"}},
			Messages: map[string][]contracts.Message{
				"first":  {{ID: "first-message", CreatedAt: now, Usage: contracts.Usage{InputTokens: 1}}},
				"second": {{ID: "second-message", CreatedAt: now, Usage: contracts.Usage{InputTokens: 2}}},
			},
			Runs: []contracts.Run{
				{ID: "first-run", SessionID: "first", Status: "completed", DurationMS: 10},
				{ID: "second-run", SessionID: "second", Status: "completed", DurationMS: 20},
			},
			ApplicationUsage: &contracts.ApplicationUsage{
				Records: map[string][]contracts.UsageRecord{
					"first":  contracts.UsageRecordsFromMessages([]contracts.Message{{ID: "first-message", CreatedAt: now, Usage: contracts.Usage{InputTokens: 1}}}),
					"second": contracts.UsageRecordsFromMessages([]contracts.Message{{ID: "second-message", CreatedAt: now, Usage: contracts.Usage{InputTokens: 2}}}),
				},
				Runs: []contracts.Run{
					{ID: "first-run", SessionID: "first", Status: "completed", DurationMS: 10},
					{ID: "second-run", SessionID: "second", Status: "completed", DurationMS: 20},
				},
				GeneratedAt: now,
			},
		},
		loadedSessions: map[string]bool{"first": true, "second": true},
	}
	before := engine.state.ApplicationUsage.GeneratedAt
	engine.refreshApplicationUsageLocked([]string{"first", "second"})
	if !engine.state.ApplicationUsage.GeneratedAt.Equal(before) {
		t.Fatal("unchanged dirty sessions rebuilt the durable usage report")
	}
	if got := engine.state.ApplicationUsage.Runs[0].ID; got != "first-run" {
		t.Fatalf("run order changed during unchanged refresh: %+v", engine.state.ApplicationUsage.Runs)
	}
}

func TestRefreshApplicationUsageLockedRebuildsCurrentStreakAfterDateChange(t *testing.T) {
	now := time.Now()
	engine := &Engine{
		state: contracts.Snapshot{
			Sessions: []contracts.Session{{ID: "session"}},
			ApplicationUsage: &contracts.ApplicationUsage{
				Records: map[string][]contracts.UsageRecord{
					"session": {{ID: "message", CreatedAt: now, Usage: contracts.Usage{InputTokens: 5}}},
				},
				GeneratedAt: now.AddDate(0, 0, -1),
			},
		},
	}
	engine.refreshApplicationUsageLocked(nil)
	if appusageDate(engine.state.ApplicationUsage.GeneratedAt) != appusageDate(now) {
		t.Fatalf("generated date = %s, want %s", appusageDate(engine.state.ApplicationUsage.GeneratedAt), appusageDate(now))
	}
	if engine.state.ApplicationUsage.Report.Summary.CurrentStreak != 1 {
		t.Fatalf("current streak = %d, want 1", engine.state.ApplicationUsage.Report.Summary.CurrentStreak)
	}
}

func appusageDate(value time.Time) string {
	return value.Format("2006-01-02")
}
