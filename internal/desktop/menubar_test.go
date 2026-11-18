package desktop

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/appusage"
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestMenuBarTaskProjection(t *testing.T) {
	now := time.Now()
	state := contracts.Snapshot{Sessions: []contracts.Session{
		{ID: "working", Status: contracts.Working, Unread: true, UpdatedAt: now},
		{ID: "waiting", Status: contracts.AwaitingInput, UpdatedAt: now.Add(time.Second)},
		{ID: "result", Status: contracts.Idle, Unread: true},
		{ID: "read", Status: contracts.Idle},
		{ID: "archived", Status: contracts.Idle, Unread: true, Archived: true},
		{ID: "child", Status: contracts.Working, SubagentType: "custom"},
	}}
	active := menuPanelTasks(state, true)
	unread := menuPanelTasks(state, false)
	if len(active) != 2 || active[0].ID != "waiting" || len(unread) != 1 || unread[0].ID != "result" {
		t.Fatalf("active=%+v unread=%+v", active, unread)
	}
}

func TestMenuBarViewsAndActions(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-%t", language, dark), func(t *testing.T) {
				app, _ := settingsTestApp(t)
				app.state.Settings.General.Language = language
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				sessionID := app.state.Sessions[0].ID
				now := time.Now()
				app.state.Sessions = []contracts.Session{{ID: sessionID, Title: "Review complete", Status: contracts.Idle, Unread: true}, {ID: "waiting", Title: "Approval needed", Status: contracts.AwaitingInput}}
				app.appUsage.generated = now
				app.appUsage.report = appusage.Report{Days: map[string]appusage.Day{now.Format("2006-01-02"): {Tokens: 1800, Turns: 2, Tools: 3, Models: map[string]int64{"local/model": 1800}}}}
				tt := ui.NewTester(app.menuPanelView, 360, 520)
				if !tt.HasText("Review complete") || !tt.HasText(app.t("Your work, at a glance")) {
					t.Fatalf("missing overview: %v", tt.Texts())
				}
				for _, tab := range []string{"Unread", "In progress", "Usage today"} {
					if err := tt.Click(app.t(tab)); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if tab == "In progress" && !tt.HasText("Approval needed") {
						t.Fatal("active task absent")
					}
					if tab == "Usage today" && !tt.HasText(app.t("Token today")) {
						t.Fatal("usage absent")
					}
					if dir := os.Getenv("GCODE_FEATURE_EVIDENCE"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						f, err := os.Create(filepath.Join(dir, fmt.Sprintf("menubar-%s-%t-%s.png", language, dark, app.menuBar.Tab)))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(f, tt.Image())
						closeErr := f.Close()
						if err != nil || closeErr != nil {
							t.Fatalf("image: %v %v", err, closeErr)
						}
					}
				}
				app.route = "chat"
				app.sessionID = sessionID
				app.draft = "keep draft"
				if err := tt.Click(app.t("Open GCode")); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if app.sessionID != sessionID || app.draft != "keep draft" {
					t.Fatal("open application discarded current task")
				}
				if err := tt.Click(app.t("Open usage statistics")); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if app.route != "settings" || app.section != "Usage statistics" {
					t.Fatal("summary did not open usage page")
				}
			})
		}
	}
}

func TestMenuBarSettingsAndIcon(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Scroll(c).Fill().Children(func() { app.appUsageSettings(c) })
	}, 1200, 1600)
	if err := tt.Click("Toggle Show tasks and usage in menu bar"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	state, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Settings.MenuBar {
		t.Fatal("menu bar toggle did not persist")
	}
	data, err := assets.MenuBarIcon()
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || image.Width != 32 || image.Height != 32 {
		t.Fatalf("template icon: %+v %v", image, err)
	}
}

func TestMenuBarStatusShowsTodayTokens(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.state.Settings.General.Language = "en"
	now := time.Date(2026, 11, 18, 14, 0, 0, 0, time.Local)
	if title := app.menuBarUsageTitle(now); title != "—" {
		t.Fatalf("unknown reading=%q", title)
	}
	app.appUsage.generated = now
	app.appUsage.report = appusage.Report{Summary: appusage.Summary{Total: 900000}, Days: map[string]appusage.Day{now.Format("2006-01-02"): {Tokens: 1800}}}
	if title := app.menuBarUsageTitle(now); title != "1.8K Tokens" {
		t.Fatalf("not today's usage: %q", title)
	}
	if title := app.menuBarUsageTitle(now.AddDate(0, 0, 1)); title != "0 Tokens" {
		t.Fatalf("rollover=%q", title)
	}
	app.appUsage.errorText = "offline"
	if title := app.menuBarUsageTitle(now); title != "—" {
		t.Fatalf("failed reading=%q", title)
	}
}

func TestMenuBarUsageOmitsZeroModelGroups(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		t.Run(language, func(t *testing.T) {
			app, _ := settingsTestApp(t)
			app.state.Settings.General.Language = language
			now := time.Now()
			app.appUsage.generated = now
			app.menuBar.Tab = "today"
			app.appUsage.report = appusage.Report{Days: map[string]appusage.Day{now.Format("2006-01-02"): {Tokens: 120, Turns: 1, Tools: 2, Models: map[string]int64{"": 0, "unused/model": 0, "provider/model": 120}}}}
			tt := ui.NewTester(app.menuPanelView, 360, 520)
			if tt.HasText(app.t("Unknown model")) || tt.HasText("unused/model") {
				t.Fatalf("zero group displayed: %v", tt.Texts())
			}
			if !tt.HasText("provider/model") || !tt.HasText("120") {
				t.Fatalf("recorded model usage lost: %v", tt.Texts())
			}
		})
	}
}
