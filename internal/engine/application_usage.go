package engine

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"time"

	"gcode/internal/appusage"
	"gcode/internal/contracts"
)

var _ contracts.ApplicationUsageService = (*Engine)(nil)

// refreshApplicationUsageLocked updates the compact usage ledger and its
// generated report while Engine.mu is held. Only loaded dirty sessions may
// project resident messages; historical sessions without an existing compact
// record are deliberately left untouched.
func (e *Engine) refreshApplicationUsageLocked(dirty []string) {
	usage := e.state.ApplicationUsage
	if usage == nil {
		if !e.hasLoadedDirtyUsageSessionLocked(dirty) {
			return
		}
		usage = &contracts.ApplicationUsage{}
		e.state.ApplicationUsage = usage
	}
	if usage.Records == nil {
		usage.Records = make(map[string][]contracts.UsageRecord)
	}
	ledgerChanged := false

	validSessions := make(map[string]struct{}, len(e.state.Sessions))
	for _, session := range e.state.Sessions {
		validSessions[session.ID] = struct{}{}
	}
	for sessionID := range usage.Records {
		if _, ok := validSessions[sessionID]; !ok {
			delete(usage.Records, sessionID)
			ledgerChanged = true
		}
	}
	priorRuns := len(usage.Runs)
	usage.Runs = slices.DeleteFunc(usage.Runs, func(run contracts.Run) bool {
		_, ok := validSessions[run.SessionID]
		return !ok
	})
	ledgerChanged = ledgerChanged || len(usage.Runs) != priorRuns

	dirtySet := make(map[string]struct{}, len(dirty))
	for _, sessionID := range dirty {
		if sessionID != "" {
			dirtySet[sessionID] = struct{}{}
		}
	}
	for sessionID := range dirtySet {
		if _, ok := validSessions[sessionID]; !ok {
			if _, exists := usage.Records[sessionID]; exists {
				delete(usage.Records, sessionID)
				ledgerChanged = true
			}
			priorRuns := len(usage.Runs)
			usage.Runs = slices.DeleteFunc(usage.Runs, func(run contracts.Run) bool {
				return run.SessionID == sessionID
			})
			ledgerChanged = ledgerChanged || len(usage.Runs) != priorRuns
			continue
		}
		if !e.loadedSessions[sessionID] {
			continue
		}
		records := contracts.UsageRecordsFromMessages(e.state.Messages[sessionID])
		if old, exists := usage.Records[sessionID]; !exists || !reflect.DeepEqual(old, records) {
			usage.Records[sessionID] = records
			ledgerChanged = true
		}
		var runs []contracts.Run
		for _, run := range e.state.Runs {
			if run.SessionID == sessionID {
				runs = append(runs, run)
			}
		}
		projectedRuns := contracts.UsageRuns(runs)
		currentRuns, runsChanged := replaceApplicationUsageRuns(usage.Runs, sessionID, projectedRuns)
		if runsChanged {
			usage.Runs = currentRuns
			ledgerChanged = true
		}
	}

	now := time.Now()
	if !ledgerChanged && !usage.GeneratedAt.IsZero() &&
		appusage.Date(usage.GeneratedAt) == appusage.Date(now) {
		return
	}
	report := appusage.ToContractReport(appusage.BuildRecords(usage.Records, usage.Runs, now))
	usage.Report = report
	usage.GeneratedAt = now
}

func replaceApplicationUsageRuns(current []contracts.Run, sessionID string, replacement []contracts.Run) ([]contracts.Run, bool) {
	oldCount := 0
	first := -1
	for index, run := range current {
		if run.SessionID == sessionID {
			oldCount++
			if first < 0 {
				first = index
			}
		}
	}
	if oldCount == len(replacement) {
		position := 0
		unchanged := true
		for _, run := range current {
			if run.SessionID != sessionID {
				continue
			}
			if !reflect.DeepEqual(run, replacement[position]) {
				unchanged = false
				break
			}
			position++
		}
		if unchanged {
			return current, false
		}
	}
	without := make([]contracts.Run, 0, len(current)-oldCount+len(replacement))
	for _, run := range current {
		if run.SessionID != sessionID {
			without = append(without, run)
		}
	}
	if first < 0 {
		return append(without, replacement...), true
	}
	if first > len(without) {
		first = len(without)
	}
	updated := make([]contracts.Run, 0, len(without)+len(replacement))
	updated = append(updated, without[:first]...)
	updated = append(updated, replacement...)
	updated = append(updated, without[first:]...)
	return updated, true
}

func (e *Engine) hasLoadedDirtyUsageSessionLocked(dirty []string) bool {
	for _, sessionID := range dirty {
		if sessionID != "" && e.sessionIndex(sessionID) >= 0 && e.loadedSessions[sessionID] {
			return true
		}
	}
	return false
}

// RestoreUnavailableUsage restores compact records for sessions whose storage
// write could not commit. The durable index must continue to describe the
// previous record for those sessions while retaining newly committed data from
// other sessions.
func RestoreUnavailableUsage(
	current, committed *contracts.ApplicationUsage,
	unavailable []string,
) *contracts.ApplicationUsage {
	if current == nil && committed == nil {
		return nil
	}
	base := committed
	if current != nil {
		copy := current.Clone()
		base = &copy
	} else {
		copy := committed.Clone()
		base = &copy
	}
	if base.Records == nil {
		base.Records = make(map[string][]contracts.UsageRecord)
	}
	ids := make(map[string]struct{}, len(unavailable))
	for _, sessionID := range unavailable {
		if sessionID != "" {
			ids[sessionID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return base
	}
	for sessionID := range ids {
		delete(base.Records, sessionID)
		if committed == nil {
			continue
		}
		if records, ok := committed.Records[sessionID]; ok {
			base.Records[sessionID] = slices.Clone(records)
		}
	}
	keptRuns := make([]contracts.Run, 0, len(base.Runs))
	for _, run := range base.Runs {
		if _, unavailable := ids[run.SessionID]; !unavailable {
			keptRuns = append(keptRuns, run)
		}
	}
	if committed != nil {
		for _, run := range committed.Runs {
			if _, unavailable := ids[run.SessionID]; unavailable {
				keptRuns = append(keptRuns, run)
			}
		}
	}
	base.Runs = keptRuns
	if base.GeneratedAt.IsZero() {
		base.GeneratedAt = time.Now()
	}
	base.Report = appusage.ToContractReport(appusage.BuildRecords(base.Records, base.Runs, base.GeneratedAt))
	return base
}

// ReadApplicationUsage returns the durable report without loading historical
// session records or exposing the compact ledger to the UI.
func (e *Engine) ReadApplicationUsage(ctx context.Context) (contracts.ApplicationUsage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return contracts.ApplicationUsage{}, err
	}
	if e.state.ApplicationUsage == nil {
		if len(e.state.Sessions) > 0 {
			return contracts.ApplicationUsage{}, errors.New("Saved usage statistics are unavailable for these historical records.")
		}
		return contracts.ApplicationUsage{GeneratedAt: time.Now()}, nil
	}
	if appusage.Date(e.state.ApplicationUsage.GeneratedAt) != appusage.Date(time.Now()) {
		e.refreshApplicationUsageLocked(nil)
		if err := e.persistLocked(); err != nil {
			return contracts.ApplicationUsage{}, err
		}
	}
	usage := e.state.ApplicationUsage
	return contracts.ApplicationUsage{
		Report:      usage.Report.Clone(),
		GeneratedAt: usage.GeneratedAt,
	}, nil
}
