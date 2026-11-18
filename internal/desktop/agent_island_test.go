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
	}{{false, 0, 210, 28}, {false, 1, 298, 32}, {true, 0, 340, 152}, {true, 1, 440, 168}, {true, 8, 440, 400}} {
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
