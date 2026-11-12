package contracts

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestApplicationUsageCloneOwnsLedgerAndReport(t *testing.T) {
	date := time.Date(2026, 11, 13, 0, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	usage := ApplicationUsage{
		Records: map[string][]UsageRecord{
			"session": {{ID: "message", Usage: Usage{InputTokens: 3}}},
		},
		Runs: []Run{{ID: "run", SessionID: "session", Status: "completed", DurationMS: 1200}},
		Report: ApplicationUsageReport{
			Summary: ApplicationUsageSummary{Total: 3, CurrentStreak: 1},
			Days: map[string]ApplicationUsageDay{
				"2026-11-13": {Date: date, Tokens: 3, Models: map[string]int64{"provider/model": 3}},
			},
		},
		GeneratedAt: date.Add(time.Hour),
	}
	clone := usage.Clone()
	clone.Records["session"][0].Usage.InputTokens = 99
	clone.Runs[0].DurationMS = 99
	clone.Report.Days["2026-11-13"].Models["provider/model"] = 99
	if usage.Records["session"][0].Usage.InputTokens != 3 || usage.Runs[0].DurationMS != 1200 ||
		usage.Report.Days["2026-11-13"].Models["provider/model"] != 3 {
		t.Fatal("application usage clone shares mutable nested data")
	}
	if !reflect.DeepEqual(usage.GeneratedAt, clone.GeneratedAt) {
		t.Fatal("clone changed generated timestamp")
	}
}

func TestApplicationUsageReportJSONUsesTypedFields(t *testing.T) {
	data, err := json.Marshal(ApplicationUsage{Report: ApplicationUsageReport{
		Summary: ApplicationUsageSummary{Total: 7},
		Days:    map[string]ApplicationUsageDay{"2026-11-13": {Tokens: 7}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || !bytes.Contains(data, []byte(`"report"`)) || !bytes.Contains(data, []byte(`"summary"`)) {
		t.Fatalf("typed report was not serialized: %s", data)
	}
}
