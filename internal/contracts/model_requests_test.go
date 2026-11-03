package contracts

import (
	"encoding/json"
	"testing"
	"time"

	"gcode/internal/model"
)

func TestModelRequestTotalsUsesPairedDecodeWeights(t *testing.T) {
	var totals ModelRequestTotals
	for _, request := range []ModelRequest{
		{Usage: model.Usage{Known: true, OutputKnown: true, Input: 100, Output: 100, Cached: 80}, DecodeDuration: time.Second,
			RequestDuration: 2 * time.Second, FirstTokenObserved: true, FirstTokenLatency: 200 * time.Millisecond},
		{Usage: model.Usage{Known: true, OutputKnown: true, Input: 200, Output: 90}, DecodeDuration: 9 * time.Second,
			RequestDuration: 10 * time.Second, FirstTokenObserved: true, FirstTokenLatency: 600 * time.Millisecond},
		{Usage: model.Usage{Known: true, OutputKnown: true, Input: 10, Output: 500}}, // no stream timing
		{Usage: model.Usage{Input: 5, Output: 40}, DecodeDuration: time.Second},      // output is a lower bound
	} {
		totals.Add(request)
	}
	speed, known := totals.TokensPerSecond()
	if !known || speed != 19 || totals.Input+totals.Output != 1045 || totals.Cached != 80 || totals.UnknownUsage != 1 {
		t.Fatalf("weighted totals=%+v, speed=%v, known=%v", totals, speed, known)
	}
	if totals.TimedRequests != 2 || totals.RequestDuration != 12*time.Second ||
		totals.FirstTokenRequests != 2 || totals.FirstTokenLatency/2 != 400*time.Millisecond {
		t.Fatalf("missing timing diluted averages: %+v", totals)
	}
	var zero ModelRequestTotals
	zero.Add(ModelRequest{Usage: model.Usage{Known: true, OutputKnown: true}, DecodeDuration: time.Second})
	if speed, known := zero.TokensPerSecond(); !known || speed != 0 || zero.UnknownUsage != 0 {
		t.Fatal("reported zero was treated as missing")
	}
}

func TestModelRequestsCloneAndWireRoundTrip(t *testing.T) {
	snapshot := Snapshot{ModelRequests: []ModelRequest{{
		ID: "request", SessionID: "session", TurnID: "turn", ProviderID: "custom",
		ProviderName: "User provider", ModelID: "family/model",
		Usage:           model.Usage{Known: true, OutputKnown: true, CacheKnown: true, Input: 20, Output: 8, Cached: 10},
		DecodeDuration:  250 * time.Millisecond,
		RequestDuration: time.Second, FirstTokenObserved: true, FirstTokenLatency: 750 * time.Millisecond,
	}}}
	cloned := CloneSnapshot(snapshot)
	cloned.ModelRequests[0].ProviderName = "changed"
	if snapshot.ModelRequests[0].ProviderName != "User provider" {
		t.Fatal("snapshot shares mutable request storage")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored Snapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ModelRequests[0] != snapshot.ModelRequests[0] {
		t.Fatal("remote wire lost accounting or provider identity")
	}
}

func TestModelRequestCacheHitExcludesUnreportedInput(t *testing.T) {
	var totals ModelRequestTotals
	for _, usage := range []model.Usage{
		{CacheKnown: true, Input: 100, Cached: 80},
		{CacheKnown: true, Input: 300, Cached: 30},
		{Known: true, Input: 10000},               // omitted cache: neither zero hit nor denominator
		{CacheKnown: true, Input: 100, Cached: 0}, // explicit zero hit participates
		{Input: 200, Cached: 50},                  // incomplete cache/input accounting
		{CacheKnown: true, Input: 10, Cached: 20}, // invalid report
	} {
		totals.Add(ModelRequest{Usage: usage})
	}
	fraction, known := totals.CacheHitFraction()
	if !known || fraction != .22 || totals.CacheRequests != 3 || totals.CacheInputTokens != 500 || totals.CacheReadTokens != 110 {
		t.Fatalf("cache ratio included missing/invalid input or averaged percentages: %+v %v %v", totals, fraction, known)
	}
	for _, tc := range []struct {
		name  string
		usage model.Usage
		known bool
	}{
		{"missing", model.Usage{Input: 100}, false},
		{"explicit zero", model.Usage{CacheKnown: true, Input: 100}, true},
		{"zero denominator", model.Usage{CacheKnown: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var totals ModelRequestTotals
			totals.Add(ModelRequest{Usage: tc.usage})
			if fraction, known := totals.CacheHitFraction(); known != tc.known || fraction != 0 {
				t.Fatalf("cache fraction=%v known=%v", fraction, known)
			}
		})
	}
}
