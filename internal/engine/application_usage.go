package engine

import (
	"context"
	"slices"

	"gcode/internal/contracts"
)

var _ contracts.ApplicationUsageService = (*Engine)(nil)

type sessionUsage struct {
	record  string
	records []contracts.UsageRecord
	runs    []contracts.Run
}

// ReadApplicationUsage walks committed records only when statistics are
// requested. It retains metadata, never all conversation bodies, and does not
// install historical records into the running engine.
func (e *Engine) ReadApplicationUsage(ctx context.Context) (contracts.ApplicationUsage, error) {
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return contracts.ApplicationUsage{}, err
	}
	ids := make([]string, len(e.state.Sessions))
	for i, session := range e.state.Sessions {
		ids[i] = session.ID
	}
	for id := range e.usageCache {
		if !slices.Contains(ids, id) {
			delete(e.usageCache, id)
		}
	}
	e.mu.Unlock()
	result := contracts.ApplicationUsage{Records: make(map[string][]contracts.UsageRecord, len(ids))}
	for _, id := range ids {
		records, runs, err := e.readSessionUsage(ctx, id)
		if err != nil {
			return contracts.ApplicationUsage{}, err
		}
		result.Records[id] = records
		result.Runs = append(result.Runs, runs...)
	}
	return result, nil
}

func (e *Engine) readSessionUsage(ctx context.Context, id string) ([]contracts.UsageRecord, []contracts.Run, error) {
	for {
		e.mu.Lock()
		if err := e.checkLocked(ctx); err != nil {
			e.mu.Unlock()
			return nil, nil, err
		}
		index := e.sessionIndex(id)
		if index < 0 {
			e.mu.Unlock()
			return nil, nil, nil
		}
		if e.loadedSessions[id] {
			records := contracts.UsageRecordsFromMessages(e.state.Messages[id])
			runs := make([]contracts.Run, 0)
			for _, run := range e.state.Runs {
				if run.SessionID == id {
					runs = append(runs, run)
				}
			}
			runs = contracts.UsageRuns(runs)
			e.mu.Unlock()
			return records, runs, nil
		}
		session, record := e.state.Sessions[index], e.sessionRecords[id]
		if cached, ok := e.usageCache[id]; ok && cached.record == record {
			e.mu.Unlock()
			return slices.Clone(cached.records), slices.Clone(cached.runs), nil
		}
		e.mu.Unlock()
		disk, err := e.readSessionDisk(session, record)
		e.mu.Lock()
		stale := e.sessionIndex(id) < 0 || e.sessionRecords[id] != record || e.loadedSessions[id]
		e.mu.Unlock()
		if stale {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		records := contracts.UsageRecordsFromMessages(disk.Messages)
		runs := contracts.UsageRuns(disk.Runs)
		e.mu.Lock()
		if e.usageCache == nil {
			e.usageCache = make(map[string]sessionUsage)
		}
		e.usageCache[id] = sessionUsage{record: record, records: records, runs: runs}
		e.mu.Unlock()
		return slices.Clone(records), slices.Clone(runs), nil
	}
}
