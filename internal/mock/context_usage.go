package mock

import (
	"encoding/json"
	"math"

	"gcode/internal/contracts"
)

// This is host state, separate from message billing counters and the UI's
// currently selected model. A provider session can be resumed after switching
// to another provider and back.
type contextSession struct {
	Scope       string                      `json:"scope"`
	RunID       string                      `json:"runId,omitempty"`
	Generation  uint64                      `json:"generation"`
	Progress    uint8                       `json:"progress"`
	StartTokens uint64                      `json:"startTokens"`
	Window      uint64                      `json:"window"`
	Scopes      map[string]contextHostScope `json:"scopes"`
}

type contextHostScope struct {
	Usage contracts.ContextUsage `json:"usage"`
	Turns uint64                 `json:"turns"`
}

func contextCount(v uint64) *uint64 { return &v }

func (s *Server) withContextSessionsLocked(p persisted) persisted {
	p.ContextSessions = cloneContextSessions(s.contextSessions)
	return p
}

func cloneContextSessions(src map[string]contextSession) map[string]contextSession {
	if src == nil {
		return nil
	}
	out := make(map[string]contextSession, len(src))
	for id, session := range src {
		session.Scopes = cloneContextScopes(session.Scopes)
		out[id] = session
	}
	return out
}

func cloneContextScopes(src map[string]contextHostScope) map[string]contextHostScope {
	out := make(map[string]contextHostScope, len(src))
	for key, scope := range src {
		scope.Usage = scope.Usage.Clone()
		out[key] = scope
	}
	return out
}

func (s *Server) contextScopeLocked(session contracts.Session) string {
	provider := session.ModelID
	for _, model := range s.state.Models {
		if model.ID == session.ModelID {
			provider = model.ProviderID
			break
		}
	}
	cwd, target := session.CWD, session.DeviceID
	for _, project := range s.state.Projects {
		if project.ID != session.ProjectID {
			continue
		}
		if target == "" {
			target = project.DeviceID
		}
		if cwd == "" {
			cwd = project.Path
			for _, worktree := range project.Worktrees {
				if session.WorktreeID != "" && worktree.ID == session.WorktreeID {
					cwd = worktree.Path
				}
			}
		}
		break
	}
	key, _ := json.Marshal([3]string{provider, target, cwd})
	return string(key)
}

// These capacities and absolute reports are the deterministic mock host's
// protocol, not a fallback from catalog ContextLimit or historical Usage.
func mockHostContextWindow(modelID string) uint64 {
	switch modelID {
	case "gpt-5.4":
		return 1000000
	case "gpt-5.3-codex":
		return 400000
	default:
		return 200000
	}
}

func seededHostContext(session contracts.Session) contracts.ContextUsage {
	window := mockHostContextWindow(session.ModelID)
	if session.SideChat {
		return contracts.ContextUsage{Tokens: contextCount(8400), Window: contextCount(window)}
	}
	switch session.ID {
	case "chat-1":
		return contracts.ContextUsage{}
	case "chat-2":
		return contracts.ContextUsage{Window: contextCount(window)}
	case "chat-3":
		return contracts.ContextUsage{Tokens: contextCount(0), Window: contextCount(window)}
	case "chat-4":
		return contracts.ContextUsage{Tokens: contextCount(window * 6 / 5), Window: contextCount(window)}
	case "chat-5":
		return contracts.ContextUsage{Tokens: contextCount(window * 3 / 4), Window: contextCount(window)}
	case "chat-6":
		return contracts.ContextUsage{Tokens: contextCount(window * 9 / 10), Window: contextCount(window)}
	default:
		return contracts.ContextUsage{Tokens: contextCount(42000), Window: contextCount(window)}
	}
}

func (s *Server) ensureContextUsageLocked(restored bool) {
	if s.state.ContextUsage == nil {
		s.state.ContextUsage = make(map[string]contracts.ContextUsage)
	}
	if s.contextSessions == nil {
		s.contextSessions = make(map[string]contextSession)
	}
	for id := range s.state.ContextUsage {
		if s.sessionIndexLocked(id) < 0 {
			delete(s.state.ContextUsage, id)
		}
	}
	for id := range s.contextSessions {
		if s.sessionIndexLocked(id) < 0 {
			delete(s.contextSessions, id)
		}
	}
	for _, session := range s.state.Sessions {
		if !restored {
			s.state.ContextUsage[session.ID] = seededHostContext(session)
		}
		if _, ok := s.contextSessions[session.ID]; !ok {
			key := s.contextScopeLocked(session)
			s.contextSessions[session.ID] = contextSession{Scope: key, Scopes: map[string]contextHostScope{
				key: {Usage: s.state.ContextUsage[session.ID].Clone()},
			}}
		}
	}
}

func (s *Server) deleteContextUsageLocked(sessionID string) {
	delete(s.state.ContextUsage, sessionID)
	delete(s.contextSessions, sessionID)
}

func (s *Server) beginHostContextLocked(sessionID, runID string) {
	si := s.sessionIndexLocked(sessionID)
	if si < 0 {
		return
	}
	session := s.state.Sessions[si]
	source := s.contextSessions[sessionID]
	key := s.contextScopeLocked(session)
	if source.RunID == runID {
		return // A permission/question continuation can reuse its turn ID.
	}
	if source.Scopes == nil {
		source.Scopes = make(map[string]contextHostScope)
	}
	scope, resumed := source.Scopes[key]
	if !resumed {
		// Equivalent to dispatching without a provider-session resume token.
		s.state.ContextUsage[sessionID] = contracts.ContextUsage{}
		scope.Usage = contracts.ContextUsage{Window: contextCount(mockHostContextWindow(session.ModelID))}
		s.state.ContextUsage[sessionID] = scope.Usage.Clone()
	}
	scope.Turns++
	source.Scopes[key] = scope
	source.Scope, source.RunID = key, runID
	source.Generation++
	source.Progress = 0
	source.StartTokens = 4096 // Explicit host bootstrap occupancy for an unknown session.
	if scope.Usage.Tokens != nil {
		source.StartTokens = *scope.Usage.Tokens
	}
	source.Window = mockHostContextWindow(session.ModelID)
	s.contextSessions[sessionID] = source
}

// A completed run may deliver its final context report. Cancellation, deletion,
// or the next run taking ownership rejects an old report without reopening UI.
func (s *Server) mergeHostContextLocked(sessionID, runID string, generation uint64, report contracts.ContextUsage) bool {
	source, ok := s.contextSessions[sessionID]
	if s.closed || !ok || source.RunID != runID || source.Generation != generation || s.sessionIndexLocked(sessionID) < 0 {
		return false
	}
	valid := false
	for _, run := range s.state.Runs {
		if run.ID == runID && run.SessionID == sessionID {
			valid = run.Status == "running" || run.Status == "awaitingInput" || run.Status == "completed" || run.Status == "failed"
			break
		}
	}
	if !valid {
		return false
	}
	s.state.ContextUsage[sessionID] = s.state.ContextUsage[sessionID].Merged(report)
	scope := source.Scopes[source.Scope]
	scope.Usage = scope.Usage.Merged(report)
	source.Scopes[source.Scope] = scope
	s.contextSessions[sessionID] = source
	return true
}

func (s *Server) observeHostContextLocked(kind, sessionID, runID string) {
	if kind == "run.started" {
		s.beginHostContextLocked(sessionID, runID)
		return
	}
	var stage uint8
	switch kind {
	case "reasoning.updated":
		stage = 1
	case "activity.appended":
		stage = 2
	case "tool.started":
		stage = 4
	case "tool.completed", "tool.failed":
		stage = 8
	case "message.delta":
		stage = 16
	default:
		return
	}
	source, ok := s.contextSessions[sessionID]
	if !ok || source.RunID != runID || source.Progress&stage != 0 {
		return
	}
	source.Progress |= stage
	s.contextSessions[sessionID] = source
	tokens := addHostContextTokens(source.StartTokens, hostContextProgress(source.Progress))
	s.mergeHostContextLocked(sessionID, runID, source.Generation, contracts.ContextUsage{Tokens: &tokens, Window: &source.Window})
}

func hostContextProgress(stages uint8) uint64 {
	var tokens uint64
	for i, increment := range [...]uint64{320, 512, 768, 1024, 640} {
		if stages&(1<<i) != 0 {
			tokens += increment
		}
	}
	return tokens
}

func addHostContextTokens(base, increment uint64) uint64 {
	if base > math.MaxUint64-increment {
		return math.MaxUint64
	}
	return base + increment
}

func (s *Server) finishHostContextLocked(sessionID, runID string) {
	source, ok := s.contextSessions[sessionID]
	if !ok || source.RunID != runID {
		return
	}
	tokens := addHostContextTokens(source.StartTokens, hostContextProgress(source.Progress)+1536)
	// A deterministic host compaction emits a lower absolute occupancy. It does
	// not subtract from token accounting or introduce another logical turn.
	if tokens >= source.Window*9/10 {
		tokens = source.Window * 3 / 10
	}
	if s.mergeHostContextLocked(sessionID, runID, source.Generation, contracts.ContextUsage{Tokens: &tokens, Window: &source.Window}) {
		s.publishLocked("context.updated", sessionID, "", "", "")
	}
}
