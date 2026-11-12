package accounting

import "gcode/internal/contracts"

// Refresh runs only at an owning core data change. Raw records of unchanged
// sessions are never needed: ancestors compose their saved own/child summaries.
func Refresh(state *contracts.Snapshot, dirty []string) {
	if state.Metrics == nil {
		state.Metrics = make(map[string]contracts.ConversationMetrics)
	}
	sessions := make(map[string]contracts.Session, len(state.Sessions))
	children := make(map[string][]string)
	for _, session := range state.Sessions {
		sessions[session.ID] = session
		if session.SubagentType != "" {
			children[session.ParentSessionID] = append(children[session.ParentSessionID], session.ID)
		}
	}
	for id := range state.Metrics {
		if _, exists := sessions[id]; !exists {
			delete(state.Metrics, id)
		}
	}
	affected := make(map[string]bool)
	for _, id := range dirty {
		session, exists := sessions[id]
		if !exists {
			continue
		}
		if messages, loaded := state.Messages[id]; loaded {
			own := Own(id, "", messages, state.ModelRequests, state.Runs)
			turns := make(map[string]contracts.ExecutionMetrics)
			ids := make(map[string]bool)
			for _, run := range state.Runs {
				if run.SessionID == id {
					ids[run.ID] = true
				}
			}
			for _, request := range state.ModelRequests {
				if request.SessionID == id && request.TurnID != "" {
					ids[request.TurnID] = true
				}
			}
			for turn := range ids {
				turns[turn] = Own(id, turn, messages, state.ModelRequests, state.Runs)
			}
			state.Metrics[id] = contracts.ConversationMetrics{Own: own, OwnTurns: turns}
		}
		for {
			if affected[session.ID] {
				break
			}
			affected[session.ID] = true
			if session.SubagentType == "" {
				break
			}
			parent, exists := sessions[session.ParentSessionID]
			if !exists {
				break
			}
			session = parent
		}
	}
	visited := make(map[string]bool)
	var compose func(string)
	compose = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		metrics, exists := state.Metrics[id]
		if !exists {
			return
		}
		metrics.Total = metrics.Own.Clone()
		metrics.Turns = make(map[string]contracts.ExecutionMetrics, len(metrics.OwnTurns))
		for turn, own := range metrics.OwnTurns {
			metrics.Turns[turn] = own.Clone()
		}
		for _, childID := range children[id] {
			if affected[childID] {
				compose(childID)
			}
			child, exists := state.Metrics[childID]
			if !exists {
				continue
			}
			metrics.Total = Merge(metrics.Total, child.Total)
			turn := sessions[childID].ParentRunID
			if turn != "" {
				metrics.Turns[turn] = Merge(metrics.Turns[turn], child.Total)
			}
		}
		state.Metrics[id] = metrics
	}
	for id := range affected {
		compose(id)
	}
}
