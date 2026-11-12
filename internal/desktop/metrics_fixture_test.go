package desktop

import (
	"gcode/internal/accounting"
	"gcode/internal/contracts"
)

// Tests author raw execution fixtures; this is their core publication boundary.
// Production views never reconstruct missing saved summaries from raw records.
func publishMetricsFixture(a *App) {
	ids := make(map[string]bool)
	for _, session := range a.state.Sessions {
		ids[session.ID] = true
	}
	for id := range a.state.Messages {
		ids[id] = true
	}
	for _, run := range a.state.Runs {
		ids[run.SessionID] = true
	}
	for _, request := range a.state.ModelRequests {
		ids[request.SessionID] = true
	}
	for id := range ids {
		if !a.hasSession(id) {
			a.state.Sessions = append(a.state.Sessions, contracts.Session{ID: id})
		}
	}
	if a.state.Messages == nil {
		a.state.Messages = make(map[string][]contracts.Message)
	}
	for id := range ids {
		if _, exists := a.state.Messages[id]; !exists {
			a.state.Messages[id] = nil
		}
	}
	dirty := make([]string, 0, len(ids))
	for id := range ids {
		dirty = append(dirty, id)
	}
	accounting.Refresh(&a.state, dirty)
}
