package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAgentIslandProjectionPriorityAndDismissal(t *testing.T) {
	now := time.Now()
	state := contracts.Snapshot{
		Sessions: []contracts.Session{
			{ID: "working", Title: "Working task", Status: contracts.Working, UpdatedAt: now},
			{ID: "question", Title: "Question", Status: contracts.AwaitingInput, UpdatedAt: now},
			{ID: "failed", Title: "Failed task", Status: contracts.Errored, Unread: true, UpdatedAt: now},
			{ID: "done", Title: "Done task", Status: contracts.Idle, Unread: true, UpdatedAt: now},
			{ID: "read", Status: contracts.Idle, UpdatedAt: now},
			{ID: "old", Status: contracts.Idle, Unread: true, UpdatedAt: now.Add(-25 * time.Hour)},
			{ID: "archived", Status: contracts.Working, Archived: true, UpdatedAt: now},
			{ID: "child", ParentSessionID: "working", SubagentType: "general", Status: contracts.AwaitingInput, UpdatedAt: now},
			{ID: "child-done", ParentSessionID: "working", SubagentType: "general", Status: contracts.Idle, Unread: true, UpdatedAt: now},
			{ID: "side", ParentSessionID: "working", SideChat: true, Status: contracts.Working, UpdatedAt: now},
		},
	}
	for _, s := range state.Sessions {
		status := "completed"
		if s.Status == contracts.Working || s.Status == contracts.AwaitingInput {
			status = "running"
		}
		if s.Status == contracts.Errored {
			status = "failed"
		}
		state.Runs = append(state.Runs, contracts.Run{ID: s.ID + "-run", SessionID: s.ID, Status: status, StartedAt: now.Add(-time.Minute)})
	}
	entries := islandEntries(state, now, nil)
	want := []string{"question", "failed", "working", "done"}
	if len(entries) != len(want) {
		t.Fatalf("entries=%+v", entries)
	}
	for i, id := range want {
		if entries[i].SessionID != id {
			t.Fatalf("priority=%+v", entries)
		}
	}
	dismissed := map[string]bool{"done-run": true}
	if len(islandEntries(state, now, dismissed)) != 3 {
		t.Fatal("dismissal did not hide result")
	}
	state.Runs = append(state.Runs, contracts.Run{ID: "done-next", SessionID: "done", Status: "completed", StartedAt: now})
	if len(islandEntries(state, now, dismissed)) != 4 {
		t.Fatal("dismissing one completion hid a later result")
	}
}

// A workflow run opens one session per instance. Those sessions belong to the
// run, not to the conversation list, so the island must count the conversation
// once rather than once per instance.
func TestAgentIslandCountsTheConversationNotTheWorkflowInstances(t *testing.T) {
	now := time.Now()
	state := contracts.Snapshot{
		Sessions: []contracts.Session{{ID: "talk", Title: "Review the change", Status: contracts.Working, UpdatedAt: now}},
		Runs:     []contracts.Run{{ID: "talk-run", SessionID: "talk", Status: "running", StartedAt: now.Add(-2 * time.Minute)}},
	}
	for _, name := range []string{"reader", "writer", "planner", "reviewer", "tester", "summarizer", "critic", "scribe", "runner"} {
		id := "actor-" + name
		state.Sessions = append(state.Sessions, contracts.Session{
			ID: id, Title: name, ParentSessionID: "talk", ParentRunID: "talk-run",
			SubagentType: contracts.SubagentWorkflow, Status: contracts.Working, UpdatedAt: now,
		})
		state.Runs = append(state.Runs, contracts.Run{ID: "run-" + name, SessionID: id, Status: "running", StartedAt: now.Add(-time.Minute)})
	}
	entries := islandEntries(state, now, nil)
	if len(entries) != 1 || entries[0].SessionID != "talk" {
		t.Fatalf("the island counted workflow instances as conversations: %+v", entries)
	}
}

func TestAgentIslandInterruptedIsNotACompletion(t *testing.T) {
	now := time.Now()
	state := contracts.Snapshot{
		Sessions: []contracts.Session{
			{ID: "stopped", Title: "Stopped task", Status: contracts.Idle, Unread: true, UpdatedAt: now},
			{ID: "done", Title: "Done task", Status: contracts.Idle, Unread: true, UpdatedAt: now},
		},
		Runs: []contracts.Run{
			{ID: "stopped-run", SessionID: "stopped", Status: "interrupted", StartedAt: now.Add(-time.Minute)},
			{ID: "done-run", SessionID: "done", Status: "completed", StartedAt: now.Add(-time.Minute)},
		},
	}
	entries := islandEntries(state, now, nil)
	if len(entries) != 2 {
		t.Fatalf("entries=%+v", entries)
	}
	phases := map[string]islandEntry{}
	for _, entry := range entries {
		phases[entry.SessionID] = entry
	}
	if got := phases["stopped"].Phase; got != "interrupted" {
		t.Fatalf("interrupted phase=%q", got)
	}
	if got := phases["stopped"].Detail; got != "Interrupted" {
		t.Fatalf("interrupted detail=%q", got)
	}
	if got := phases["done"].Phase; got != "completed" {
		t.Fatalf("completed phase=%q", got)
	}
	if islandRank("interrupted") <= islandRank("running") || islandRank("interrupted") >= islandRank("completed") {
		t.Fatal("interrupted must rank between running and completed")
	}
	a, _ := settingsTestApp(t)
	sessionID := a.state.Sessions[0].ID
	a.state.Sessions = []contracts.Session{{ID: sessionID, Title: "Stopped task", Status: contracts.Idle, Unread: true, UpdatedAt: now}}
	a.state.Runs = []contracts.Run{{ID: "stopped-run", SessionID: sessionID, Status: "interrupted", StartedAt: now.Add(-65 * time.Second)}}
	a.island.Expanded = true
	tt := ui.NewTester(a.agentIslandView, 360, 400)
	if !tt.HasText("Interrupted") {
		t.Fatalf("interrupted detail missing: %v", tt.Texts())
	}
	if tt.HasText("Completed") {
		t.Fatal("an interrupted run was reported as completed")
	}
	if _, ok := tt.Find("Dismiss"); !ok {
		t.Fatal("interrupted entry must offer dismissal, not a stop action")
	}
}

func TestAgentIslandControlsAndRender(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			app, _ := settingsTestApp(t)
			theme := "rcode-light"
			if dark {
				theme = "rcode-dark"
			}
			app.palette = design.Palette(theme, dark)
			sessionID := app.state.Sessions[0].ID
			now := time.Now()
			app.state.Sessions = []contracts.Session{{ID: sessionID, Title: "Automation review", Status: contracts.Idle, Unread: true, UpdatedAt: now}}
			app.state.Runs = []contracts.Run{{ID: "result", SessionID: sessionID, Status: "completed", StartedAt: now.Add(-65 * time.Second)}}
			app.island.Expanded = true
			tt := ui.NewTester(app.agentIslandView, 360, 400)
			if !tt.HasText("Automation review") || !tt.HasText("Completed") {
				t.Fatalf("missing task: %v", tt.Texts())
			}
			if err := tt.Click("Dismiss"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if tt.HasText("Automation review") {
				t.Fatal("dismissed card still visible")
			}
			if err := tt.Click("No active tasks"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if app.island.Expanded {
				t.Fatal("island did not collapse")
			}
			// Render representative working and waiting states as durable evidence.
			app.island.Expanded = true
			app.state.Sessions = []contracts.Session{{ID: sessionID, Title: "Automation review", Status: contracts.AwaitingInput, UpdatedAt: now}}
			app.state.Runs = []contracts.Run{{ID: "next-run", SessionID: sessionID, Status: "running", StartedAt: now.Add(-65 * time.Second)}}
			tt.Frame()
			if !tt.HasText("Waiting for input") {
				t.Fatal("waiting state missing")
			}
			if err := tt.Click("Automation review"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if app.sessionID != sessionID {
				t.Fatal("open action did not select conversation")
			}
			if dir := os.Getenv("GCODE_FEATURE_EVIDENCE"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("island-dark-%t.png", dark)))
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

func TestAgentIslandGeometryAndDisplaySelection(t *testing.T) {
	displays := []mygo.Display{
		{ID: 11, Primary: true, Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1080}},
		{ID: 22, Bounds: mygo.Rectangle{X: 1920, Y: -200, Width: 1600, Height: 900}},
	}
	for _, test := range []struct {
		target string
		count  int
		first  int64
	}{{"", 1, 11}, {"all", 2, 11}, {"22", 1, 22}, {"missing", 1, 11}} {
		selected := islandDisplays(displays, test.target)
		if len(selected) != test.count || selected[0].ID != test.first {
			t.Fatalf("target %s: %+v", test.target, selected)
		}
	}
	for _, test := range []struct {
		expanded    bool
		count, w, h int
	}{{false, 0, 210, 28}, {false, 1, 298, 32}, {true, 0, 340, 152}, {true, 1, 440, 104}, {true, 8, 440, 400}} {
		w, h := design.IslandGeometry(test.expanded, test.count)
		if w != test.w || h != test.h {
			t.Fatalf("geometry=%dx%d", w, h)
		}
	}
}

func TestAgentIslandSettingsPersistenceAndSoundDecode(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.state.Settings.Island = contracts.AgentIslandSettings{DisplayTarget: "all", Mascot: "pululu"}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.islandSettings(c) })
	}, 1000, 1200)
	if err := tt.Click("Toggle Subtle reminders"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Toggle Event sounds"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	snapshot, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	prefs := snapshot.Settings.Island
	if !prefs.SubtleReminders || !prefs.SoundEnabled || prefs.CompleteSound != "gem-collect" || prefs.DisplayTarget != "all" {
		t.Fatalf("preferences not persisted: %+v", prefs)
	}
	for _, option := range islandSounds {
		if option.value == "" || option.value == "none" {
			continue
		}
		sound, err := islandNotificationSound(notificationCompleted, option.value)
		if err != nil || sound == nil {
			t.Fatalf("sound %s: %v", option.value, err)
		}
	}
	for _, option := range islandMascots {
		if assets.IslandMascot(option.value) == nil {
			t.Fatalf("mascot %s could not decode", option.value)
		}
	}
	if dir := os.Getenv("GCODE_FEATURE_EVIDENCE"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "island-settings.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("image %v %v", err, closeErr)
		}
	}
}

func TestAgentIslandPointerExitClearsClickAndHoverExpansion(t *testing.T) {
	for _, clicked := range []bool{false, true} {
		for _, hovered := range []bool{false, true} {
			t.Run(fmt.Sprintf("clicked=%t/hovered=%t", clicked, hovered), func(t *testing.T) {
				now := time.Now()
				s := agentIslandState{Expanded: clicked, HoverExpanded: hovered, CollapseAt: now.Add(time.Minute)}
				s.settlePointer(false, now)
				if s.Expanded || s.HoverExpanded || !s.CollapseAt.IsZero() || !s.HoverCooldown.After(now) {
					t.Fatalf("pointer exit retained expansion: %+v", s)
				}
			})
		}
	}
}

func TestAgentIslandCompactTaskLayout(t *testing.T) {
	for _, width := range []int{360, 440} {
		for _, language := range []string{"en", "zh-CN"} {
			t.Run(fmt.Sprintf("%d/%s", width, language), func(t *testing.T) {
				app, _ := settingsTestApp(t)
				app.state.Settings.General.Language = language
				now := time.Now()
				app.state.Sessions = []contracts.Session{
					{ID: "first", Title: "项目情况总结整理", Status: contracts.Working, UpdatedAt: now},
					{ID: "second", Title: "拉取 ZCode 最新代码并分析工作流复刻方案", Status: contracts.Working, UpdatedAt: now},
				}
				app.state.Runs = []contracts.Run{
					{ID: "run-first", SessionID: "first", Status: "running", StartedAt: now.Add(-8 * time.Minute)},
					{ID: "run-second", SessionID: "second", Status: "running", StartedAt: now.Add(-33 * time.Minute)},
				}
				app.island.Expanded = true
				_, height := design.IslandGeometry(true, 2)
				tt := ui.NewTester(app.agentIslandView, width, height)
				for _, entry := range app.state.Sessions {
					card, found := tt.Find(entry.Title + " — " + i18n.In(language, "Working"))
					action, hasAction := tt.Find(i18n.In(language, "Stop"))
					if entry.ID != "first" {
						if !found {
							t.Fatal("second task is not visible")
						}
						continue
					}
					if !found || !hasAction || action.X < card.X+card.W || action.Y < card.Y || action.Y+action.H > card.Y+card.H {
						t.Fatalf("task action is not beside card: card=%+v action=%+v", card, action)
					}
					red, _, _, alpha := tt.Image().At(int(action.X+action.W/2), int(action.Y+action.H/2)).RGBA()
					if red > 0x4000 || alpha == 0 {
						t.Fatal("stop glyph is not visibly dark inside the round button")
					}
				}
				if dir := os.Getenv("GCODE_FEATURE_EVIDENCE"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(dir, fmt.Sprintf("compact-%d-%s.png", width, language)))
					if err != nil {
						t.Fatal(err)
					}
					defer file.Close()
					if err := png.Encode(file, tt.Image()); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestAgentIslandClickExpansionSchedulesCollapseOnExit(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.island.Expanded = true
	app.island.Hovered = map[int64]bool{1: true}
	t.Cleanup(app.island.cancelHoverTimer)
	app.observeIslandPointer(1, false)
	if app.island.HoverTimer == nil {
		t.Fatal("click-expanded panel did not schedule collapse after pointer exit")
	}
	app.observeIslandPointer(1, true)
	if app.island.HoverTimer == nil || !app.island.Expanded {
		t.Fatal("pointer return did not preserve expansion and replace the exit timer")
	}
	app.island.cancelHoverTimer()
}
