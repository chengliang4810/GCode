package contracts

import (
	"reflect"
	"testing"
	"time"
)

func TestModelRequestTotalsMergeAddsEveryField(t *testing.T) {
	left := ModelRequestTotals{
		Requests: 1, UnknownUsage: 2, DecodeRequests: 3,
		Input: 4, Output: 5, Cached: 6, DecodeTokens: 7,
		DecodeDuration: 8 * time.Second, Unrecorded: false,
		CacheRequests: 9, CacheInputTokens: 10, CacheReadTokens: 11,
		TimedRequests: 12, FirstTokenRequests: 13,
		RequestDuration: 14 * time.Second, FirstTokenLatency: 15 * time.Second,
	}
	right := ModelRequestTotals{
		Requests: 16, UnknownUsage: 17, DecodeRequests: 18,
		Input: 19, Output: 20, Cached: 21, DecodeTokens: 22,
		DecodeDuration: 23 * time.Second, Unrecorded: true,
		CacheRequests: 24, CacheInputTokens: 25, CacheReadTokens: 26,
		TimedRequests: 27, FirstTokenRequests: 28,
		RequestDuration: 29 * time.Second, FirstTokenLatency: 30 * time.Second,
	}
	left.Merge(right)
	want := ModelRequestTotals{
		Requests: 17, UnknownUsage: 19, DecodeRequests: 21,
		Input: 23, Output: 25, Cached: 27, DecodeTokens: 29,
		DecodeDuration: 31 * time.Second, Unrecorded: true,
		CacheRequests: 33, CacheInputTokens: 35, CacheReadTokens: 37,
		TimedRequests: 39, FirstTokenRequests: 41,
		RequestDuration: 43 * time.Second, FirstTokenLatency: 45 * time.Second,
	}
	if left != want {
		t.Fatalf("merged totals = %+v, want %+v", left, want)
	}
}

func TestExecutionMetricsCloneAndElapsed(t *testing.T) {
	start := time.Date(2026, 11, 12, 23, 59, 59, 0, time.UTC)
	original := ExecutionMetrics{
		Models:       []ModelMetrics{{ProviderID: "p", Provider: "Provider", ModelID: "m", Totals: ModelRequestTotals{Requests: 1}}},
		WallKnown:    true,
		WallDuration: time.Second,
		RunningSince: start.Add(2 * time.Second),
		Spans:        []ExecutionSpan{{Start: start, End: start.Add(time.Second)}},
	}
	clone := original.Clone()
	clone.Models[0].Totals.Requests = 9
	clone.Spans[0].Start = start.Add(time.Hour)
	if original.Models[0].Totals.Requests != 1 || !original.Spans[0].Start.Equal(start) {
		t.Fatal("clone aliases source slices")
	}
	if elapsed, known := original.Elapsed(start.Add(5 * time.Second)); !known || elapsed != 4*time.Second {
		t.Fatalf("elapsed = %v, known=%v, want 4s,true", elapsed, known)
	}
	unknown := ExecutionMetrics{}
	if elapsed, known := unknown.Elapsed(start); known || elapsed != 0 {
		t.Fatalf("unknown elapsed = %v, known=%v", elapsed, known)
	}
}

func TestConversationMetricsCloneDoesNotAliasMaps(t *testing.T) {
	value := ConversationMetrics{
		Total: ExecutionMetrics{Models: []ModelMetrics{{ProviderID: "p"}}},
		Turns: map[string]ExecutionMetrics{
			"turn": {Spans: []ExecutionSpan{{Start: time.Unix(1, 0)}}},
		},
		OwnTurns: map[string]ExecutionMetrics{
			"turn": {Models: []ModelMetrics{{ModelID: "m"}}},
		},
	}
	clone := value.Clone()
	clone.Total.Models[0].ProviderID = "changed"
	clone.Turns["turn"] = ExecutionMetrics{}
	clone.OwnTurns["turn"].Models[0].ModelID = "changed"
	if value.Total.Models[0].ProviderID != "p" || len(value.Turns["turn"].Spans) != 1 || value.OwnTurns["turn"].Models[0].ModelID != "m" {
		t.Fatal("conversation clone aliases nested values")
	}
	if !reflect.DeepEqual(value.Turns["turn"].Spans[0].Start, time.Unix(1, 0)) {
		t.Fatal("conversation clone changed source span")
	}
}
