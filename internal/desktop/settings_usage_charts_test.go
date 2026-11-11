package desktop

import (
	"fmt"
	"strings"
	"testing"

	"gcode/internal/appusage"
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestUsageChartModelName(t *testing.T) {
	app := testApp(t, "default")
	app.state.Settings.General.Language = "zh-CN"
	app.state.Providers = []contracts.Provider{
		{ID: "provider-one", Name: "自定义供应商"},
		{ID: "provider-two", Name: "另一供应商"},
	}
	app.state.Models = []contracts.Model{
		{ID: "provider-one/family/model", ProviderID: "provider-one", Label: "family/model"},
		{ID: "provider-two/family/model", ProviderID: "provider-two", Label: "family/model"},
	}
	for _, test := range []struct{ name, id, want string }{
		{"configured model", "provider-one/family/model", "自定义供应商/family/model"},
		{"same model from another provider", "provider-two/family/model", "另一供应商/family/model"},
		{"removed model", "provider-one/removed/model", "自定义供应商/removed/model"},
		{"removed provider", "removed-provider/family/model", "removed-provider/family/model"},
		{"unqualified model", "model", "model"},
		{"unknown model", "", app.t("Unknown model")},
		{"other models", "\x00other", app.t("Other models")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := app.usageChartModelName(test.id); got != test.want {
				t.Fatalf("model name = %q, want %q", got, test.want)
			}
		})
	}
	app.state.Providers[0].Name = "重命名供应商"
	if got := app.usageChartModelName("provider-one/family/model"); got != "重命名供应商/family/model" {
		t.Fatalf("provider rename was not reflected: %q", got)
	}
}

func TestUsageChartsDisplayProviderAndModel(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("%d-dark=%v", width, dark), func(t *testing.T) {
				app := testApp(t, "default")
				app.state.Settings.Appearance.Mode = "Light"
				app.state.Settings.Appearance.ThemeID = "rcode-light"
				if dark {
					app.state.Settings.Appearance.Mode = "Dark"
					app.state.Settings.Appearance.ThemeID = "rcode-dark"
				}
				app.state.Providers = []contracts.Provider{
					{ID: "provider-one", Name: "第一供应商"},
					{ID: "provider-two", Name: "第二供应商"},
				}
				app.state.Models = []contracts.Model{
					{ID: "provider-one/family/model", ProviderID: "provider-one", Label: "family/model"},
					{ID: "provider-two/family/model", ProviderID: "provider-two", Label: "family/model"},
				}
				now := app.now()
				app.appUsage.report = appusage.Report{Days: map[string]appusage.Day{
					appusage.Date(now): {
						Date: now, Tokens: 300,
						Models: map[string]int64{"provider-one/family/model": 200, "provider-two/family/model": 100},
					},
				}}
				app.appUsage.generated = now
				app.route, app.section = "settings", "Usage statistics"
				tt := ui.NewTester(app.View, width, 1400)
				for _, name := range []string{"第一供应商/family/model", "第二供应商/family/model"} {
					if !tt.HasText(name) {
						t.Fatalf("chart legend or model list is missing %q", name)
					}
				}
				for _, label := range []string{
					"第一供应商/family/model · 200 Tokens · 66.7%",
					"第二供应商/family/model · 100 Tokens · 33.3%",
				} {
					if _, ok := tt.Find(label); !ok {
						t.Fatalf("model share identity or accounting is wrong: %q", label)
					}
				}
				hint := fmt.Sprintf("%s · 300 Tokens\n第一供应商/family/model · 200\n第二供应商/family/model · 100", appusage.Date(now))
				if _, ok := tt.Find(hint); !ok {
					t.Fatalf("trend tooltip is missing resolved model names: %q", hint)
				}
				text := strings.Join(tt.Texts(), "\n")
				if strings.Contains(text, "provider-one/") || strings.Contains(text, "provider-two/") {
					t.Fatal("charts exposed internal provider identifiers")
				}
			})
		}
	}
}
