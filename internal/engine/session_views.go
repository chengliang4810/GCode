package engine

import (
	"context"
	"slices"

	"gcode/internal/contracts"
)

type sessionRead struct{ done chan struct{} }

func uniqueSessionIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result
}

// A view pins only the conversations it displays. Disk I/O happens outside the
// engine lock; concurrent viewers share the same read and validate its revision.
func (e *Engine) prepareSessionView(ctx context.Context, ids []string) error {
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return err
	}
	if e.sessionReads == nil {
		e.sessionReads = make(map[string]*sessionRead)
	}
	e.retainSessionDataLocked(ids)
	e.mu.Unlock()
	for _, id := range ids {
		if err := e.prepareViewedSession(ctx, id); err != nil {
			e.mu.Lock()
			e.releaseSessionDataLocked(ids)
			e.mu.Unlock()
			return err
		}
	}
	return nil
}

func (e *Engine) prepareViewedSession(ctx context.Context, id string) error {
	for {
		e.mu.Lock()
		if err := e.checkLocked(ctx); err != nil {
			e.mu.Unlock()
			return err
		}
		index := e.sessionIndex(id)
		if index < 0 || e.loadedSessions[id] {
			e.mu.Unlock()
			return nil
		}
		if pending := e.sessionReads[id]; pending != nil {
			e.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pending.done:
				continue
			}
		}
		session, record := e.state.Sessions[index], e.sessionRecords[id]
		pending := &sessionRead{done: make(chan struct{})}
		e.sessionReads[id] = pending
		e.mu.Unlock()
		disk, err := e.readSessionDisk(session, record)
		e.mu.Lock()
		delete(e.sessionReads, id)
		close(pending.done)
		if viewErr := e.checkLocked(ctx); viewErr != nil {
			e.mu.Unlock()
			return viewErr
		}
		index = e.sessionIndex(id)
		current := index >= 0 && e.sessionRecords[id] == record
		if current && !e.loadedSessions[id] {
			if err != nil {
				e.state.Sessions[index].StorageError = err.Error()
			} else {
				e.installSessionDiskLocked(disk, record)
				if _, dirty := e.sessionDirty[id]; dirty {
					if saveErr := e.persistLocked(id); saveErr != nil {
						e.state.Sessions[index].StorageError = saveErr.Error()
					}
				}
			}
		}
		e.mu.Unlock()
		if !current {
			continue
		}
		return nil
	}
}

func (e *Engine) retainSessionDataLocked(ids []string) {
	if e.sessionRefs == nil {
		e.sessionRefs = make(map[string]int)
	}
	for _, id := range ids {
		e.sessionRefs[id]++
	}
}

func (e *Engine) releaseSessionDataLocked(ids []string) {
	for _, id := range ids {
		if e.sessionRefs[id] > 1 {
			e.sessionRefs[id]--
		} else {
			delete(e.sessionRefs, id)
		}
	}
	e.evictIdleSessionsLocked()
}

func (e *Engine) evictIdleSessionsLocked() {
	for id := range e.loadedSessions {
		if _, dirty := e.sessionDirty[id]; dirty {
			continue
		}
		if e.sessionRefs[id] > 0 || e.sessionReads[id] != nil || e.runs[id] != nil || e.naming[id] != nil {
			continue
		}
		index := e.sessionIndex(id)
		if index < 0 || e.state.Sessions[index].StorageError != "" || e.sessionRecords[id] == "" ||
			e.state.Sessions[index].RunningSubagents > 0 {
			continue
		}
		if slices.ContainsFunc(e.state.Queue, func(row contracts.QueuedMessage) bool { return row.SessionID == id }) {
			continue
		}
		if slices.ContainsFunc(e.state.Terminals, func(row contracts.Terminal) bool {
			return row.SessionID == id && e.terminals[row.ID] != nil
		}) {
			continue
		}
		goal := e.state.Sessions[index].Goal
		if goal != nil && (goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying) {
			continue
		}
		e.evictSessionLocked(id, index)
	}
}

func (e *Engine) evictSessionLocked(id string, index int) {
	delete(e.loadedSessions, id)
	delete(e.history, id)
	delete(e.context, id)
	delete(e.state.Messages, id)
	e.state.Sessions[index].Goal = nil
	for i := range e.state.Runs {
		if e.state.Runs[i].SessionID == id {
			e.state.Runs[i].Error = ""
		}
	}
	e.state.ModelRequests = slices.DeleteFunc(e.state.ModelRequests, func(row contracts.ModelRequest) bool { return row.SessionID == id })
	e.state.Changes = slices.DeleteFunc(e.state.Changes, func(row contracts.Change) bool { return row.SessionID == id })
	e.state.Terminals = slices.DeleteFunc(e.state.Terminals, func(row contracts.Terminal) bool { return row.SessionID == id })
}
