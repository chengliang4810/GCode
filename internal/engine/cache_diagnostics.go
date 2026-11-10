package engine

import (
	"slices"

	"gcode/internal/model"
)

type cacheBaseline struct {
	id       string
	modelID  string
	protocol model.Protocol
	request  model.RequestDiagnostics
	usage    model.Usage
}

// annotateCacheChange runs under trajectoryMu. Only successful conversation
// requests advance the baseline; naming, summaries and failed retries cannot
// masquerade as a cache miss or replace the last known-good prefix.
func (e *Engine) annotateCacheChange(sessionID, source string, invocation *model.Invocation) {
	if source != "main" && source != "subagent" {
		return
	}
	change := &model.CacheChange{}
	invocation.CacheChange = change
	if invocation.ContextChange == "summary" {
		change.Expected = true
		change.Changes = []string{"summary"}
		return
	}
	if invocation.ContextChange == "compacted" {
		change.Expected = true
		change.Changes = append(change.Changes, "context_compacted")
	}
	previous, ok := e.cacheBaselines[sessionID]
	if ok {
		change.ComparedTo = previous.id
		if previous.modelID != invocation.ModelID || previous.protocol != invocation.Protocol {
			change.Changes = append(change.Changes, "model_changed")
		}
		if previous.request.TargetHash != invocation.Request.TargetHash {
			change.Changes = append(change.Changes, "target_changed")
		}
		if previous.request.SystemHash != invocation.Request.SystemHash {
			change.Changes = append(change.Changes, "system_changed")
		}
		if previous.request.ToolsHash != invocation.Request.ToolsHash {
			change.Changes = append(change.Changes, "tools_changed")
		}
		if previous.request.ParameterHash != invocation.Request.ParameterHash {
			change.Changes = append(change.Changes, "parameters_changed")
		}
		before, after := previous.request.MessageHashes, invocation.Request.MessageHashes
		for change.CommonPrefixMessages < min(len(before), len(after)) &&
			before[change.CommonPrefixMessages] == after[change.CommonPrefixMessages] {
			change.CommonPrefixMessages++
		}
		if change.CommonPrefixMessages < len(before) || invocation.Request.MessageCount < previous.request.MessageCount {
			change.Changes = append(change.Changes, "history_changed")
		}
		if len(before) < previous.request.MessageCount || len(after) < invocation.Request.MessageCount {
			change.Changes = append(change.Changes, "history_comparison_limited")
		}
		if invocation.CompletedAt != nil && invocation.Error == "" && invocation.Response != nil &&
			previous.usage.CacheKnown && invocation.Response.Usage.CacheKnown {
			current := invocation.Response.Usage
			change.ReadDrop = !change.Expected && previous.usage.Cached-current.Cached > 2000 &&
				float64(current.Cached) < float64(previous.usage.Cached)*.95
		}
	}
	if invocation.CompletedAt == nil || invocation.Error != "" || invocation.Response == nil {
		return
	}
	if e.cacheBaselines == nil {
		e.cacheBaselines = map[string]cacheBaseline{}
	}
	e.cacheBaselines[sessionID] = cacheBaseline{
		id: invocation.ID, modelID: invocation.ModelID, protocol: invocation.Protocol,
		request: invocation.Request, usage: invocation.Response.Usage,
	}
	e.cacheBaselineOrder = slices.DeleteFunc(e.cacheBaselineOrder, func(id string) bool { return id == sessionID })
	e.cacheBaselineOrder = append(e.cacheBaselineOrder, sessionID)
	if len(e.cacheBaselineOrder) > 32 {
		delete(e.cacheBaselines, e.cacheBaselineOrder[0])
		e.cacheBaselineOrder = e.cacheBaselineOrder[1:]
	}
}
