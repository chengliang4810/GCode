package engine

import (
	"gcode/internal/accounting"
	"gcode/internal/contracts"
	"time"
)

// refreshStatisticsLocked is the single publication preparation for changed
// execution records, including parents changed while a child is finishing.
func (e *Engine) refreshStatisticsLocked() {
	dirty := make([]string, 0, len(e.sessionDirty))
	for id := range e.sessionDirty {
		dirty = append(dirty, id)
	}
	e.refreshMetricsLocked()
	e.refreshApplicationUsageLocked(dirty)
}

// Summaries and their source records commit through the same index transaction.
// Merely taking a snapshot or opening a history does not rebuild summaries.
func (e *Engine) refreshMetricsLocked() {
	dirty := make([]string, 0, len(e.sessionDirty))
	for id := range e.sessionDirty {
		dirty = append(dirty, id)
	}
	accounting.Refresh(&e.state, dirty)
	for i := range e.state.Runs {
		run := &e.state.Runs[i]
		metrics, exists := e.state.Metrics[run.SessionID]
		if !exists {
			continue
		}
		if turn, exists := metrics.OwnTurns[run.ID]; exists {
			run.Summary = turn.Activity
		}
	}
	// Message time is execution-owned metadata, not a render-time scan of Runs.
	for _, id := range dirty {
		messages, loaded := e.state.Messages[id]
		if !loaded {
			continue
		}
		times := make(map[string]int64)
		for _, run := range e.state.Runs {
			if run.SessionID == id && run.Status != "running" {
				times[run.ID] = run.DurationMS
			}
		}
		for i := range messages {
			if milliseconds, exists := times[messages[i].TurnID]; exists {
				messages[i].DurationMS = milliseconds
			}
		}
		e.state.Messages[id] = messages
	}
}

// Freeze open intervals using the last committed execution duration on host
// restart. An ended host cannot leave a saved summary clock ticking forever.
func freezeRecoveredMetrics(state *contracts.Snapshot) {
	changed := false
	for id, metrics := range state.Metrics {
		freeze := func(value contracts.ExecutionMetrics, turn string) contracts.ExecutionMetrics {
			if value.RunningSince.IsZero() {
				return value
			}
			changed = true
			end := value.RunningSince
			for _, run := range state.Runs {
				if run.SessionID == id && (turn == "" || run.ID == turn) && !run.StartedAt.IsZero() {
					candidate := run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond)
					if run.FinishedAt != nil {
						candidate = *run.FinishedAt
					}
					if candidate.After(end) {
						end = candidate
					}
				}
			}
			value.WallDuration += max(0, end.Sub(value.RunningSince))
			value.RunningSince = time.Time{}
			for i := range value.Spans {
				if value.Spans[i].End.IsZero() {
					value.Spans[i].End = end
				}
			}
			return value
		}
		metrics.Own = freeze(metrics.Own, "")
		for turn, value := range metrics.OwnTurns {
			metrics.OwnTurns[turn] = freeze(value, turn)
		}
		state.Metrics[id] = metrics
	}
	if !changed {
		return
	}
	ids := make([]string, 0, len(state.Sessions))
	for _, session := range state.Sessions {
		ids = append(ids, session.ID)
	}
	accounting.Refresh(state, ids)
}
