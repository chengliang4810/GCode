package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSettingsUsageThemesAndNarrowWindows(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("%d-dark=%v", width, dark), func(t *testing.T) {
				app := testApp(t, "default")
				app.state.Settings.General.Language = "zh-CN"
				app.state.Settings.Appearance.Mode = "Light"
				app.state.Settings.Appearance.ThemeID = "rcode-light"
				if dark {
					app.state.Settings.Appearance.Mode = "Dark"
					app.state.Settings.Appearance.ThemeID = "rcode-dark"
				}
				app.state.Messages = make(map[string][]contracts.Message)
				now := app.now()
				for i := range 40 {
					model := "custom/model-a"
					if i%3 == 0 {
						model = "custom/model-b"
					}
					app.state.Messages["history"] = append(app.state.Messages["history"], contracts.Message{
						ID: fmt.Sprint(i), TurnID: fmt.Sprint(i), Role: "assistant", ModelID: model,
						CreatedAt: now.AddDate(0, 0, -i),
						Usage:     contracts.Usage{InputTokens: (i%7 + 1) * 1200, OutputTokens: 300, CachedTokens: 200},
					})
				}
				app.service = applicationUsageTestService{Service: app.service, usage: applicationUsageFixture(contracts.ApplicationUsage{Runs: []contracts.Run{
					{ID: "long-session", SessionID: "history", Status: "completed", DurationMS: (51*60*60 + 4*60 + 5) * 1000},
				}}, app.state.Messages, now)}
				app.route = "settings"
				app.selectSettingsSection("Usage statistics")
				tt := ui.NewTester(app.View, width, 1400)
				for _, label := range []string{"Settings page viewport", "Settings page content", "Application usage", "Token activity", "Model usage", "2天 03小时 04分 05秒"} {
					bounds, exists := tt.Find(app.t(label))
					if !exists || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 {
						t.Fatalf("usage layout exceeds the viewport: %s %+v", label, bounds)
					}
					if label == "2天 03小时 04分 05秒" && bounds.H > 32 {
						t.Fatalf("longest session duration wraps within its units: %+v", bounds)
					}
				}
				if tt.HasText(app.t("Coding Plan")) {
					t.Fatal("application usage exposed a subscription surface")
				}
				if err := tt.Click(app.t("Last 30 days")); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click(app.t("Weekly")); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if directory := os.Getenv("RCODE_USAGE_EVIDENCE"); directory != "" {
					if err := os.MkdirAll(directory, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(directory, fmt.Sprintf("%d-dark-%v.png", width, dark)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("render usage evidence: %v %v", err, closeErr)
					}
				}
			})
		}
	}
}
