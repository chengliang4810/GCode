package desktop

import (
	"maps"
	"strings"

	"gcode/internal/linkui"
	"github.com/egoist/mygo/ui/markdown"
)

// Session identities remain in scoped snapshots even when their bodies are
// unloaded. Only an accepted authoritative snapshot may remove UI state.
func (a *App) pruneSessionUIState() {
	exists := make(map[string]bool, len(a.state.Sessions)+1)
	exists[""] = true // The new-conversation canvas owns its own draft.
	for _, session := range a.state.Sessions {
		exists[session.ID] = true
	}
	pruneSessionMap(a.sessionStates, exists)
	pruneSessionMap(a.drafts, exists)
	pruneSessionMap(a.pendingSends, exists)
	pruneSessionMap(a.sideComposers, exists)
	pruneSessionMap(a.goalEditors, exists)
	pruneSessionMap(a.goalActions, exists)
	pruneSessionMap(a.trajectoryViews, exists)
	pruneSessionMap(a.sessionDetailsLoading, exists)
	pruneSessionMap(a.sideTranscripts, exists)
	pruneSessionMap(a.streamText, exists)
	absentScope := func(key string) bool {
		scope, id, found := strings.Cut(key, ":")
		return found && (scope == "main" || scope == "side") && !exists[id]
	}
	maps.DeleteFunc(a.questionForms, func(key string, _ *questionFormState) bool { return absentScope(key) })
	maps.DeleteFunc(a.todoPanels, func(key string, _ *todoPanelState) bool { return absentScope(key) })
	maps.DeleteFunc(a.transcriptSelections, func(key string, _ *linkui.Selection) bool { return absentScope(key) })
	maps.DeleteFunc(a.transcriptHover, func(key, _ string) bool { return absentScope(key) })
	maps.DeleteFunc(a.linkOptions, func(key string, _ *markdown.Options) bool {
		at := strings.LastIndexByte(key, ':')
		return at >= 0 && !exists[key[:at]]
	})
}

func pruneSessionMap[V any](items map[string]V, exists map[string]bool) {
	maps.DeleteFunc(items, func(id string, _ V) bool { return !exists[id] })
}
