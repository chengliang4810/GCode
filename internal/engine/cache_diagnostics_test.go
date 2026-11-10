package engine

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"gcode/internal/model"
)

func TestCacheDiagnosticsSeparateFailuresCompactionAndUnexpectedChanges(t *testing.T) {
	core := &Engine{}
	complete := time.Now()
	first := model.Invocation{ID: "first", CompletedAt: &complete, ModelID: "m", Protocol: model.Chat,
		Request:  model.RequestDiagnostics{SystemHash: "system", ToolsHash: "tools", ParameterHash: "params", MessageHashes: []string{"one", "two"}},
		Response: &model.Result{Usage: model.Usage{CacheKnown: true, Input: 20000, Cached: 18000}}}
	core.annotateCacheChange("session", "main", &first)
	failed := first
	failed.ID, failed.Error, failed.Response = "failed", "timeout", nil
	core.annotateCacheChange("session", "main", &failed)
	if failed.CacheChange.ReadDrop || core.cacheBaselines["session"].id != "first" {
		t.Fatal("unknown failed usage became zero cache hits")
	}
	for _, reason := range []string{"", "compacted", "summary"} {
		t.Run(reason, func(t *testing.T) {
			current := first
			current.ID, current.ContextChange = "current", reason
			current.Request.MessageHashes = []string{"one", "changed", "three"}
			current.Response = &model.Result{Usage: model.Usage{CacheKnown: true, Input: 6000, Cached: 1000}}
			core.cacheBaselines["session"] = cacheBaseline{id: first.ID, modelID: first.ModelID, protocol: first.Protocol, request: first.Request, usage: first.Response.Usage}
			core.annotateCacheChange("session", "main", &current)
			if reason == "summary" {
				if !current.CacheChange.Expected || core.cacheBaselines["session"].id != "first" {
					t.Fatal("summary replaced the conversation baseline")
				}
				return
			}
			if !slices.Contains(current.CacheChange.Changes, "history_changed") || current.CacheChange.CommonPrefixMessages != 1 {
				t.Fatal("changed history prefix was not identified")
			}
			if current.CacheChange.ReadDrop != (reason == "") || current.CacheChange.Expected != (reason != "") {
				t.Fatalf("expected compaction confused with unexplained drop: %+v", current.CacheChange)
			}
		})
	}
}

func TestCacheDiagnosticsRetainsOnlyBoundedRecentConversations(t *testing.T) {
	core := &Engine{}
	now := time.Now()
	for i := range 40 {
		record := model.Invocation{ID: "request", CompletedAt: &now, Response: &model.Result{}}
		core.annotateCacheChange(fmt.Sprint(i), "main", &record)
	}
	if len(core.cacheBaselines) != 32 || len(core.cacheBaselineOrder) != 32 {
		t.Fatal("cache diagnostic baselines grew without bounds")
	}
	if _, exists := core.cacheBaselines["0"]; exists {
		t.Fatal("old baseline was not released")
	}
}
