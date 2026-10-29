package contracts

import (
	"encoding/json"
	"testing"
)

func contextCount(value uint64) *uint64 { return &value }

func TestContextUsageUnknownZeroAndOverCapacity(t *testing.T) {
	for _, test := range []struct {
		name   string
		usage  ContextUsage
		window bool
		known  bool
		value  float64
	}{
		{"unknown", ContextUsage{}, false, false, 0},
		{"tokens-only", ContextUsage{Tokens: contextCount(42)}, false, false, 0},
		{"zero-capacity", ContextUsage{Tokens: contextCount(42), Window: contextCount(0)}, false, false, 0},
		{"capacity-only", ContextUsage{Window: contextCount(200000)}, true, false, 0},
		{"zero-tokens", ContextUsage{Tokens: contextCount(0), Window: contextCount(200000)}, true, true, 0},
		{"over-capacity", ContextUsage{Tokens: contextCount(240000), Window: contextCount(200000)}, true, true, 1.2},
	} {
		t.Run(test.name, func(t *testing.T) {
			fraction, known := test.usage.Fraction()
			if test.usage.HasWindow() != test.window || known != test.known || fraction != test.value {
				t.Fatalf("window=%v fraction=%v known=%v", test.usage.HasWindow(), fraction, known)
			}
		})
	}
}

func TestContextUsageMergeOwnsPointersAndRetainsUnknownFields(t *testing.T) {
	before := ContextUsage{Tokens: contextCount(42000), Window: contextCount(200000)}
	report := ContextUsage{Tokens: contextCount(0), Window: contextCount(0)}
	next := before.Merged(report)
	if *next.Tokens != 0 || *next.Window != 200000 {
		t.Fatalf("zero report: %+v", next)
	}
	*next.Tokens, *next.Window = 17, 18
	if *before.Tokens != 42000 || *before.Window != 200000 || *report.Tokens != 0 || *report.Window != 0 {
		t.Fatal("merged counters share caller ownership")
	}
	next = before.Merged(ContextUsage{Tokens: contextCount(6000)})
	if *next.Tokens != 6000 || *next.Window != 200000 {
		t.Fatal("compaction was not an absolute occupancy report")
	}
	clone := next.Merged(ContextUsage{})
	*clone.Tokens = 7
	if *next.Tokens != 6000 {
		t.Fatal("an empty report reused mutable pointers")
	}
}

func TestContextUsageJSONDistinguishesMissingAndRealZero(t *testing.T) {
	for _, test := range []struct {
		usage ContextUsage
		json  string
	}{
		{ContextUsage{}, `{}`},
		{ContextUsage{Tokens: contextCount(0)}, `{"tokens":0}`},
		{ContextUsage{Window: contextCount(200000)}, `{"window":200000}`},
	} {
		data, err := json.Marshal(test.usage)
		if err != nil || string(data) != test.json {
			t.Fatalf("json=%s err=%v", data, err)
		}
		var restored ContextUsage
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if (restored.Tokens == nil) != (test.usage.Tokens == nil) || (restored.Window == nil) != (test.usage.Window == nil) {
			t.Fatal("JSON lost optional counter presence")
		}
	}
}
