// Package appusage builds a read-only presentation of recorded application usage.
package appusage

import (
	"maps"
	"sort"
	"time"

	"gcode/internal/contracts"
)

type Model struct {
	ID     string
	Tokens int64
}

type Day struct {
	Date   time.Time
	Tokens int64
	Turns  int
	Tools  int
	Models map[string]int64
}

type Summary struct {
	Total, Input, Output, Cached int64
	Peak                         int64
	LongestSessionMS             int64
	CurrentStreak, LongestStreak int
	UnknownModelTokens           int64
}

type Report struct {
	Summary Summary
	Days    map[string]Day
}

// ToContractReport converts the calculation projection into the durable
// snapshot representation. It copies nested model maps so the report can be
// retained by the core without sharing mutable calculation state.
func ToContractReport(report Report) contracts.ApplicationUsageReport {
	durable := contracts.ApplicationUsageReport{
		Summary: contracts.ApplicationUsageSummary{
			Total:              report.Summary.Total,
			Input:              report.Summary.Input,
			Output:             report.Summary.Output,
			Cached:             report.Summary.Cached,
			Peak:               report.Summary.Peak,
			LongestSessionMS:   report.Summary.LongestSessionMS,
			CurrentStreak:      report.Summary.CurrentStreak,
			LongestStreak:      report.Summary.LongestStreak,
			UnknownModelTokens: report.Summary.UnknownModelTokens,
		},
	}
	if report.Days != nil {
		durable.Days = make(map[string]contracts.ApplicationUsageDay, len(report.Days))
		for key, day := range report.Days {
			durable.Days[key] = contracts.ApplicationUsageDay{
				Date:   day.Date,
				Tokens: day.Tokens,
				Turns:  day.Turns,
				Tools:  day.Tools,
				Models: maps.Clone(day.Models),
			}
		}
	}
	return durable
}

// FromContractReport converts a persisted report into the local calculation
// projection used by the usage charts and calendar helpers.
func FromContractReport(report contracts.ApplicationUsageReport) Report {
	converted := Report{
		Summary: Summary{
			Total:              report.Summary.Total,
			Input:              report.Summary.Input,
			Output:             report.Summary.Output,
			Cached:             report.Summary.Cached,
			Peak:               report.Summary.Peak,
			LongestSessionMS:   report.Summary.LongestSessionMS,
			CurrentStreak:      report.Summary.CurrentStreak,
			LongestStreak:      report.Summary.LongestStreak,
			UnknownModelTokens: report.Summary.UnknownModelTokens,
		},
	}
	if report.Days != nil {
		converted.Days = make(map[string]Day, len(report.Days))
		for key, day := range report.Days {
			converted.Days[key] = Day{
				Date:   day.Date,
				Tokens: day.Tokens,
				Turns:  day.Turns,
				Tools:  day.Tools,
				Models: maps.Clone(day.Models),
			}
		}
	}
	return converted
}

func Date(t time.Time) string { return t.Format("2006-01-02") }

func Midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// Build projects resident messages to usage records before building the
// report. It never attributes old requests to a session's currently selected
// model.
func Build(messages map[string][]contracts.Message, runs []contracts.Run, now time.Time) Report {
	return BuildWithRecords(nil, messages, runs, now)
}

// BuildRecords counts each execution step once from compact usage records.
// Forks mint new message IDs but retain execution IDs and the sequence of
// steps within them.
func BuildRecords(records map[string][]contracts.UsageRecord, runs []contracts.Run, now time.Time) Report {
	report := Report{Days: make(map[string]Day)}
	type stepKey struct {
		execution, message, role string
		step                     int
	}
	unique := make(map[stepKey]contracts.UsageRecord)
	for _, list := range records {
		steps := make(map[string]int)
		for _, record := range list {
			key := stepKey{message: record.ID}
			if record.TurnID != "" {
				key = stepKey{execution: record.TurnID, role: record.Role, step: steps[record.TurnID]}
				steps[record.TurnID]++
			}
			if old, exists := unique[key]; !exists || recordTokens(record) > recordTokens(old) ||
				(recordTokens(record) == recordTokens(old) && record.BlockCount > old.BlockCount) {
				unique[key] = record
			}
		}
	}
	ordered := make([]contracts.UsageRecord, 0, len(unique))
	for _, record := range unique {
		ordered = append(ordered, record)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})
	turns := make(map[string]bool)
	for _, record := range ordered {
		if record.CreatedAt.After(now) {
			continue
		}
		input, output := int64(max(0, record.Usage.InputTokens)), int64(max(0, record.Usage.OutputTokens))
		tokens := input + output
		report.Summary.Input += input
		report.Summary.Output += output
		report.Summary.Cached += int64(max(0, record.Usage.CachedTokens))
		report.Summary.Total += tokens
		if record.ModelID == "" {
			report.Summary.UnknownModelTokens += tokens
		}
		// Undated records remain in the lifetime total, without inventing a day.
		if record.CreatedAt.IsZero() {
			continue
		}
		date := Midnight(record.CreatedAt.In(now.Location()))
		key := Date(date)
		day := report.Days[key]
		if day.Models == nil {
			day = Day{Date: date, Models: make(map[string]int64)}
		}
		day.Tokens += tokens
		day.Models[record.ModelID] += tokens
		if record.Role == "assistant" && record.TurnID != "" && !turns[record.TurnID] {
			day.Turns++
			turns[record.TurnID] = true
		}
		day.Tools += max(0, record.ToolCount)
		report.Days[key] = day
	}
	// Completed execution time, summed per original session, excludes idle
	// time and does not inflate the statistic when history is forked.
	seenRuns, durations := make(map[string]bool), make(map[string]int64)
	for _, run := range runs {
		if !seenRuns[run.ID] && run.Status == "completed" {
			seenRuns[run.ID] = true
			durations[run.SessionID] += max(0, run.DurationMS)
			report.Summary.LongestSessionMS = max(report.Summary.LongestSessionMS, durations[run.SessionID])
		}
	}
	active := make([]Day, 0, len(report.Days))
	for _, day := range report.Days {
		report.Summary.Peak = max(report.Summary.Peak, day.Tokens)
		if day.Tokens > 0 {
			active = append(active, day)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Date.Before(active[j].Date) })
	streak := 0
	for i, day := range active {
		if i == 0 || Date(active[i-1].Date.AddDate(0, 0, 1)) != Date(day.Date) {
			streak = 0
		}
		streak++
		report.Summary.LongestStreak = max(report.Summary.LongestStreak, streak)
		if Date(day.Date) == Date(now) {
			report.Summary.CurrentStreak = streak
		}
	}
	return report
}

// BuildWithRecords uses compact records as the history source and replaces a
// session's records with resident messages when that session is present. This
// lets streaming detail update usage immediately without loading old bodies.
func BuildWithRecords(
	records map[string][]contracts.UsageRecord,
	messages map[string][]contracts.Message,
	runs []contracts.Run,
	now time.Time,
) Report {
	if len(messages) == 0 {
		return BuildRecords(records, runs, now)
	}
	merged := make(map[string][]contracts.UsageRecord, len(records)+len(messages))
	for sessionID, list := range records {
		merged[sessionID] = list
	}
	for sessionID, list := range messages {
		merged[sessionID] = contracts.UsageRecordsFromMessages(list)
	}
	return BuildRecords(merged, runs, now)
}

func recordTokens(record contracts.UsageRecord) int64 {
	return int64(max(0, record.Usage.InputTokens)) + int64(max(0, record.Usage.OutputTokens))
}

// Window fills calendar days rather than elapsed 24-hour intervals (DST safe).
func (r Report) Window(now time.Time, count int) []Day {
	days := make([]Day, 0, count)
	for i := count - 1; i >= 0; i-- {
		date := Midnight(now).AddDate(0, 0, -i)
		day, exists := r.Days[Date(date)]
		if !exists {
			day = Day{Date: date}
		}
		days = append(days, day)
	}
	return days
}

// Heatmap uses 52 natural weeks, Sunday first, including the current week.
func (r Report) Heatmap(now time.Time) []Day {
	saturday := Midnight(now).AddDate(0, 0, 6-int(now.Weekday()))
	return r.Window(saturday, 52*7)
}

func Models(days []Day) []Model {
	totals := make(map[string]int64)
	for _, day := range days {
		for id, tokens := range day.Models {
			totals[id] += tokens
		}
	}
	models := make([]Model, 0, len(totals))
	for id, tokens := range totals {
		if tokens > 0 {
			models = append(models, Model{ID: id, Tokens: tokens})
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Tokens == models[j].Tokens {
			return models[i].ID < models[j].ID
		}
		return models[i].Tokens > models[j].Tokens
	})
	return models
}
