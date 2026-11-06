package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarUsageOpensApplicationStatisticsSettings(t *testing.T) {
	a := testApp(t, "default")
	now := a.now()
	messages := map[string][]contracts.Message{
		"one": {{ID: "one", ModelID: "provider/model-a", CreatedAt: now, Usage: contracts.Usage{InputTokens: 100, OutputTokens: 30, CachedTokens: 60}}},
		"two": {{ID: "two", ModelID: "provider/model-b", CreatedAt: now.AddDate(0, 0, -8), Usage: contracts.Usage{InputTokens: 50, OutputTokens: 20, CachedTokens: 10}}},
	}
	a.state.Messages = messages
	a.state.Runs = nil
	usage := contracts.ApplicationUsage{Records: make(map[string][]contracts.UsageRecord, len(messages))}
	for sessionID, sessionMessages := range messages {
		usage.Records[sessionID] = contracts.UsageRecordsFromMessages(sessionMessages)
	}
	a.service = applicationUsageTestService{Service: a.service, usage: usage}
	tt := ui.NewTester(a.View, 1320, 1200)
	if err := tt.Click(a.t("Usage statistics")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != "settings" || a.section != "Usage statistics" {
		t.Fatalf("usage entry did not open settings: %s/%s", a.route, a.section)
	}
	for _, value := range []string{"200", a.t("Token activity"), a.t("Daily model usage"), a.t("Model usage"), "provider/model-a"} {
		if !tt.HasText(value) {
			t.Fatalf("usage page is missing %q", value)
		}
	}
	if tt.HasText("provider/model-b") {
		t.Fatal("default 7-day chart included an older request")
	}
	if err := tt.Click(a.t("Last 30 days")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("provider/model-b") || !tt.HasText("200") {
		t.Fatal("30-day filter lost the older model or changed lifetime totals")
	}
	for _, label := range []string{"Weekly", "Cumulative", "Daily"} {
		if err := tt.Click(a.t(label)); err != nil {
			t.Fatal(err)
		}
	}
	a.now = func() time.Time { return now.AddDate(0, 0, 1) }
	a.refreshAppUsage()
	if !a.appUsage.generated.Equal(a.now()) {
		t.Fatal("refresh did not rebuild calendar bounds")
	}
	if err := tt.Click(a.t("Back to conversations")); err != nil {
		t.Fatal(err)
	}
	if a.route != "chat" {
		t.Fatal("cannot return to conversations from usage settings")
	}
}
