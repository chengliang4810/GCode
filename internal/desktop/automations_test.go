package desktop

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestAutomationSettingsRealCoreCRUD(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "schedule", Name: "Test", Protocol: model.Chat, BaseURL: "http://127.0.0.1:1", Models: []model.Model{{ID: "model"}}}}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(t.Context())
	app.apply(snapshot)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() { app.automationPage(c) })
	}, 900, 1400)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if app.automationUI.Error != "" {
			t.Fatal(app.automationUI.Error)
		}
	}
	click("New automation")
	app.automationUI.Draft.Name = "Daily review"
	app.automationUI.Draft.Prompt = "Review changes"
	app.automationUI.Draft.Mode = "manual"
	app.automationUI.Draft.ModelID = "schedule/model"
	tt.Frame()
	click("Save")
	state, _ := core.Snapshot(context.Background())
	if len(state.Automations) != 1 || state.Automations[0].Name != "Daily review" {
		t.Fatalf("save failed: %+v", state.Automations)
	}
	click("Pause")
	state, _ = core.Snapshot(t.Context())
	if state.Automations[0].Enabled {
		t.Fatal("pause did not persist")
	}
	click("Edit")
	app.automationUI.Draft.Prompt = "Review and test"
	tt.Frame()
	click("Save")
	state, _ = core.Snapshot(t.Context())
	if state.Automations[0].Prompt != "Review and test" {
		t.Fatal("edit did not persist")
	}
	click("Delete")
	click("Confirm delete")
	state, _ = core.Snapshot(t.Context())
	if len(state.Automations) != 0 {
		t.Fatal("delete failed")
	}
}

func TestAutomationSettingsInvalidDraftAndRender(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("%t-%d", dark, width), func(t *testing.T) {
				app, _ := runtimeSettingsTestApp(t)
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				app.editAutomation(nil)
				tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.automationPage(c) }, width, 1000)
				if err := tt.Click("Save"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !app.automationUI.Open || app.automationUI.Error == "" {
					t.Fatal("invalid draft was discarded")
				}
				if dir := os.Getenv("GCODE_FEATURE_EVIDENCE"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("automations-%t-%d.png", dark, width)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					closeErr := f.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("render: %v %v", err, closeErr)
					}
				}
			})
		}
	}
}

func TestAutomationsSidebarEntryAndIndependentPage(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{900, 1320} {
				t.Run(fmt.Sprintf("%s-%t-%d", language, dark, width), func(t *testing.T) {
					app := testApp(t, "default")
					app.state.Settings.General.Language = language
					app.state.Settings.Appearance.Mode = "light"
					if dark {
						app.state.Settings.Appearance.Mode = "dark"
					}
					app.navigate("chat/chat-0")
					app.draft = "保留原对话的草稿"
					tt := ui.NewTester(app.View, width, 880)
					create, ok := tt.Find(app.t("New conversation"))
					if !ok {
						t.Fatal("new conversation entry missing")
					}
					entry, ok := tt.Find(app.t("Automations"))
					if !ok {
						t.Fatal("automation entry missing")
					}
					if entry.Y < create.Y+create.H || entry.X != create.X {
						t.Fatalf("automation must follow new conversation: create=%+v entry=%+v", create, entry)
					}
					for _, section := range app.sections() {
						if section == "Automations" {
							t.Fatal("automation still appears in settings")
						}
					}
					if err := tt.Click(app.t("Automations")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if app.route != "automations" || !tt.HasText(app.t("New automation")) {
						t.Fatalf("independent page missing: route=%s", app.route)
					}
					if app.sessionID != "chat-0" || app.draft != "保留原对话的草稿" {
						t.Fatal("opening automations changed conversation or draft")
					}
					if tt.HasText(app.t("Toggle right pane")) {
						t.Fatal("conversation workspace controls leaked into automation page")
					}
					app.back(-1)
					tt.Frame()
					if app.route != "chat" || app.sessionID != "chat-0" || app.draft != "保留原对话的草稿" {
						t.Fatal("back lost conversation draft")
					}
					app.back(1)
					tt.Frame()
					if app.route != "automations" {
						t.Fatal("forward failed to restore automation page")
					}
					if err := tt.Click(app.t("New conversation")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if app.route != "chat" || app.sessionID != "" {
						t.Fatal("new conversation did not leave automation page")
					}
				})
			}
		}
	}
}

func TestAutomationMasterDetailLayout(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{620, 1100} {
			t.Run(fmt.Sprintf("%t-%d", dark, width), func(t *testing.T) {
				app, _ := settingsTestApp(t)
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				now := time.Now()
				app.state.Automations = []contracts.Automation{
					{ID: "daily", Name: "Daily review", Prompt: "Review the project changes and identify follow-up work.", Mode: "cron", Cron: "0 9 * * 1-5", Timezone: "Asia/Shanghai", Enabled: true, ModelID: "configured/model", NextAt: now.Add(time.Hour)},
					{ID: "weekly", Name: "Weekly summary", Prompt: "Summarize the week's work.", Mode: "manual", Enabled: false, ModelID: "configured/model"},
				}
				app.state.AutomationRuns = []contracts.AutomationRun{{ID: "latest", AutomationID: "daily", Status: "completed", StartedAt: now.Add(-time.Hour), FinishedAt: now.Add(-time.Hour + 65*time.Second)}}
				tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.automationPage(c) }, width, 780)
				if !tt.HasText("Recent runs") || !tt.HasText("1m 05s") {
					t.Fatalf("missing detail: %v", tt.Texts())
				}
				if err := tt.Click("Weekly summary"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !tt.HasText("No runs yet") || app.automationUI.SelectedID != "weekly" {
					t.Fatal("selection did not change detail")
				}
				if err := tt.Click("Edit"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !app.automationUI.Open || !tt.HasText("Weekly summary") {
					t.Fatal("editing lost list context")
				}
				app.automationUI.Open = false
				app.automationUI.SelectedID = "daily"
				tt.Frame()
				if dir := os.Getenv("GCODE_FEATURE_EVIDENCE"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("automation-workspace-%t-%d.png", dark, width)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					closeErr := f.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("image: %v %v", err, closeErr)
					}
				}
			})
		}
	}
}
