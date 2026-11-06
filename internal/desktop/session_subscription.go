package desktop

import (
	"context"
	"maps"
	"slices"

	"gcode/internal/contracts"
)

type sessionScopeCapture struct {
	scope      []string
	generation uint64
}

type appJob struct {
	contracts.Command
	sessionScopeCapture
}

// sessionScopeIDs is computed on the UI thread from the current central
// conversation and the workspace surfaces that can render conversation data.
// The returned slice is owned by the caller and can be captured by a worker.
func (a *App) sessionScopeIDs() []string {
	capacity := 1
	if a.workspace != nil {
		capacity += len(a.workspace.VisibleSessionIDs())
	}
	ids := make([]string, 0, capacity)
	if a.sessionID != "" {
		ids = append(ids, a.sessionID)
	}
	if a.workspace != nil {
		ids = append(ids, a.workspace.VisibleSessionIDs()...)
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return id == "" })
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (a *App) sessionScopeContains(id string) bool {
	if id == "" {
		return false
	}
	if a.sessionSubscriptionGeneration == 0 {
		return true
	}
	return slices.Contains(a.sessionSubscriptionScope, id)
}

func (a *App) releaseUnrequestedSessionProjections(scope []string) {
	requested := make(map[string]bool, len(scope))
	for _, id := range scope {
		requested[id] = true
	}
	clonedMessages := false
	for id := range a.state.Messages {
		if !requested[id] {
			if !clonedMessages {
				a.state.Messages = maps.Clone(a.state.Messages)
				clonedMessages = true
			}
			delete(a.state.Messages, id)
		}
	}
	for id := range a.sideTranscripts {
		if !requested[id] {
			delete(a.sideTranscripts, id)
		}
	}
	for id := range a.streamText {
		if !requested[id] {
			delete(a.streamText, id)
		}
	}
	for id := range a.sessionDetailsLoading {
		if !requested[id] {
			delete(a.sessionDetailsLoading, id)
		}
	}
	for id, view := range a.trajectoryViews {
		if requested[id] || view == nil {
			continue
		}
		// Keep search and fold state, but drop the potentially large diagnostic
		// projection when its session is no longer visible.
		view.data = contracts.ModelTrajectory{}
		view.copies = nil
		view.searchText = nil
		view.toolText = nil
		view.loaded = false
		view.loading = false
		view.error = ""
	}
	if cachedSession := a.transcriptCache.session; cachedSession != "" && cachedSession != a.sessionID {
		a.transcriptCache = transcriptCache{}
	}
}

func (a *App) markSessionDetailsLoading(scope []string) {
	if a.sessionDetailsLoading == nil {
		a.sessionDetailsLoading = make(map[string]bool)
	}
	for _, id := range scope {
		if _, loaded := a.state.Messages[id]; !loaded {
			a.sessionDetailsLoading[id] = true
		}
	}
}

func (a *App) sessionDetailsAreLoading(id string) bool {
	return id != "" && a.sessionDetailsLoading[id]
}

func (a *App) clearSessionDetailsLoading(snapshot contracts.Snapshot) {
	if len(a.sessionDetailsLoading) == 0 {
		return
	}
	// The scoped snapshot is authoritative for every requested session. A
	// session may legitimately have no Messages entry because its history is
	// missing, deleted, or failed to load; those states are rendered from the
	// Session row and must still leave the loading state.
	if a.sessionSubscriptionGeneration > 0 {
		for id := range a.sessionDetailsLoading {
			if a.sessionScopeContains(id) {
				delete(a.sessionDetailsLoading, id)
			}
		}
		return
	}
	for id := range a.sessionDetailsLoading {
		if _, ok := snapshot.Messages[id]; ok || slices.ContainsFunc(snapshot.Sessions, func(session contracts.Session) bool { return session.ID == id }) {
			delete(a.sessionDetailsLoading, id)
		}
	}
}

func (a *App) snapshotAddsDetails(snapshot contracts.Snapshot) bool {
	for id := range snapshot.Messages {
		if a.sessionScopeContains(id) {
			if _, loaded := a.state.Messages[id]; !loaded {
				return true
			}
		}
	}
	// A scoped snapshot can settle a pending detail load without carrying a
	// Messages entry: storage errors, deleted sessions, and empty histories all
	// use the session summary as the authoritative result.
	for id := range a.sessionDetailsLoading {
		if a.sessionScopeContains(id) {
			return true
		}
	}
	return false
}

// refreshSessionSubscription replaces the detail scope after a UI selection or
// workspace-tab change. Native subscriptions are established in a worker so a
// remote HTTP/SSE connection cannot block the UI thread. Tester-only apps have
// no native window; they synchronously fetch the captured projection so the
// existing in-process rendering tests still see conversation bodies.
func (a *App) refreshSessionSubscription() {
	if a.closed {
		return
	}
	scope := a.sessionScopeIDs()
	active := a.sessionSubscriptionGeneration > 0
	if active && slices.Equal(scope, a.sessionSubscriptionScope) && ((a.win == nil && a.sessionSubscriptionCancel == nil) || (a.win != nil && a.sessionSubscriptionCancel != nil)) {
		return
	}
	if a.sessionSubscriptionCancel != nil {
		a.sessionSubscriptionCancel()
		a.sessionSubscriptionCancel = nil
	}
	a.sessionSubscriptionGeneration++
	generation := a.sessionSubscriptionGeneration
	a.sessionSubscriptionScope = slices.Clone(scope)
	a.releaseUnrequestedSessionProjections(scope)
	a.markSessionDetailsLoading(scope)

	if a.win == nil {
		snapshot, err := a.service.Snapshot(a.ctx, scope...)
		if err != nil {
			for _, id := range scope {
				delete(a.sessionDetailsLoading, id)
			}
			a.errorText = err.Error()
			return
		}
		a.apply(snapshot)
		return
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.sessionSubscriptionCancel = cancel
	window, service, captured := a.win, a.service, slices.Clone(scope)
	go func() {
		events, err := service.Subscribe(ctx, captured...)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			window.Update(func() {
				if a.closed || generation != a.sessionSubscriptionGeneration {
					return
				}
				a.sessionSubscriptionCancel = nil
				for _, id := range captured {
					delete(a.sessionDetailsLoading, id)
				}
				a.errorText = err.Error()
			})
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				window.Update(func() {
					if a.closed || generation != a.sessionSubscriptionGeneration {
						return
					}
					a.applyEvent(event)
				})
			}
		}
	}()
}

// captureSessionScope returns an immutable scope and the generation that owns
// it. Callers invoke it on the UI thread before starting backend work.
func (a *App) captureSessionScope() ([]string, uint64) {
	scope := a.sessionScopeIDs()
	if !slices.Equal(scope, a.sessionSubscriptionScope) || a.sessionSubscriptionGeneration == 0 {
		a.refreshSessionSubscription()
	}
	return slices.Clone(a.sessionSubscriptionScope), a.sessionSubscriptionGeneration
}

func (a *App) sessionGenerationCurrent(generation uint64, scope []string) bool {
	return !a.closed && generation == a.sessionSubscriptionGeneration && slices.Equal(scope, a.sessionSubscriptionScope)
}
