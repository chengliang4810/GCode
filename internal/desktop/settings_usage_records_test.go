package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/appusage"
	"gcode/internal/contracts"
)

type applicationUsageTestService struct {
	contracts.Service
	usage contracts.ApplicationUsage
}

func (s applicationUsageTestService) ReadApplicationUsage(context.Context) (contracts.ApplicationUsage, error) {
	return s.usage, nil
}

type serviceWithoutApplicationUsage struct {
	contracts.Service
}

func TestRefreshAppUsageReportsUnsupportedCapability(t *testing.T) {
	app := testApp(t, "default")
	app.service = serviceWithoutApplicationUsage{Service: app.service}
	app.refreshAppUsage()
	if app.appUsage.errorText == "" {
		t.Fatal("missing application usage capability was not reported")
	}
}

func TestRefreshAppUsageUsesCoreReportWithoutReadingResidentMessages(t *testing.T) {
	app := testApp(t, "default")
	now := app.now()
	app.state.Messages = map[string][]contracts.Message{
		"resident": {{
			ID: "resident-message", TurnID: "resident-turn", Role: "assistant", ModelID: "provider/model",
			CreatedAt: now, Usage: contracts.Usage{InputTokens: 40, OutputTokens: 10},
		}},
	}
	app.service = applicationUsageTestService{
		Service: app.service,
		usage: contracts.ApplicationUsage{
			Records: map[string][]contracts.UsageRecord{
				"history": {{
					ID: "history-message", TurnID: "history-turn", Role: "assistant", ModelID: "provider/model",
					CreatedAt: now, Usage: contracts.Usage{InputTokens: 20, OutputTokens: 5},
				}},
				"resident": {{
					ID: "resident-message", TurnID: "resident-turn", Role: "assistant", ModelID: "provider/model",
					CreatedAt: now, Usage: contracts.Usage{InputTokens: 1, OutputTokens: 1},
				}},
			},
		},
	}
	prepared := app.service.(applicationUsageTestService)
	prepared.usage = applicationUsageFixture(prepared.usage, app.state.Messages, now)
	app.service = prepared
	app.state.Messages = nil // The UI must not need resident records to display the report.
	app.refreshAppUsage()
	if app.appUsage.errorText != "" {
		t.Fatal(app.appUsage.errorText)
	}
	if app.appUsage.report.Summary.Total != 75 {
		t.Fatalf("resident detail or unloaded history was lost: %+v", app.appUsage.report.Summary)
	}
	if !app.appUsage.generated.Equal(now) {
		t.Fatalf("generated = %v, want %v", app.appUsage.generated, now)
	}
}

func TestRefreshAppUsagePreservesCalendarTimeFromRead(t *testing.T) {
	app := testApp(t, "default")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
	app.now = func() time.Time { return now }
	app.service = applicationUsageTestService{Service: app.service, usage: contracts.ApplicationUsage{Records: map[string][]contracts.UsageRecord{
		"history": {{ID: "message", CreatedAt: now, Usage: contracts.Usage{InputTokens: 1}}},
	}}}
	prepared := app.service.(applicationUsageTestService)
	prepared.usage = applicationUsageFixture(prepared.usage, nil, now)
	app.service = prepared
	app.refreshAppUsage()
	if app.appUsage.generated != now || app.appUsage.report.Days[appusage.Date(now)].Tokens != 1 {
		t.Fatalf("usage report used the wrong calendar anchor: generated=%v report=%+v", app.appUsage.generated, app.appUsage.report)
	}
}

func applicationUsageFixture(ledger contracts.ApplicationUsage, messages map[string][]contracts.Message, now time.Time) contracts.ApplicationUsage {
	report := appusage.BuildWithRecords(ledger.Records, messages, ledger.Runs, now)
	return contracts.ApplicationUsage{Report: appusage.ToContractReport(report), GeneratedAt: now}
}
