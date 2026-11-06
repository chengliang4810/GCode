package desktop

import (
	"context"
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/mock"
	"github.com/egoist/mygo/ui"
	"math"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T, scenario string) *App {
	t.Helper()
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }
	backend, err := mock.New(mock.Options{Scenario: scenario, Instant: true, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	// Reference geometry and existing interaction assertions intentionally use English.
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Settings.General.Language = "en"
	if err := backend.UpdateSettings(context.Background(), snapshot.Settings); err != nil {
		t.Fatal(err)
	}
	app, err := NewWithOptions(backend, Options{Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	// Establish the backend-owned initial refs before callers customize this
	// fixture snapshot. A first-render resource load publishes a full snapshot.
	app.ensureComposerFooterRefs(false)
	ui.NewTester(func(c *ui.Context) {
		app.trackFooterUsage(c, &app.footerUsage, contracts.Session{ID: "fixture-cache-prime", DeviceID: "local"}, app.selectedModel().ProviderID, false)
	}, 1, 1)
	return app
}
func TestHomeAndNewSession(t *testing.T) {
	a := testApp(t, "default")
	initialSessions := len(a.state.Sessions)
	tt := ui.NewTester(a.View, 1320, 880)
	if _, ok := tt.Find("Message"); !ok {
		t.Fatal("composer missing")
	}
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Compose("你好", 2)
	tt.Key(0, ui.KeyEnter)
	if len(a.state.Sessions) != initialSessions {
		t.Fatal("IME composition changed session")
	}
	tt.Compose("", 0)
	tt.Type("Build a settings page")
	tt.Key(0, ui.KeyEnter)
	if a.sessionID == "" {
		t.Fatalf("send did not select new session: %s", a.errorText)
	}
	messages := a.state.Messages[a.sessionID]
	found := false
	for _, m := range messages {
		if m.Role == "user" {
			for _, b := range m.Blocks {
				if strings.Contains(b.Text, "Build a settings page") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("typed message did not reach backend")
	}
}
func TestNavigationPreservesDraftAndStaleSnapshots(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "first draft"
	a.selectSession("chat-1")
	a.draft = "second draft"
	a.selectSession("chat-0")
	if a.draft != "first draft" {
		t.Fatal("draft lost")
	}
	old := a.state
	a.dispatch(contracts.Command{Kind: "session.pin", ID: "chat-0", Enabled: false})
	revision := a.state.Revision
	a.apply(old)
	if a.state.Revision != revision {
		t.Fatal("stale snapshot replaced current state")
	}
	a.navigate("settings/Appearance")
	if a.route != "settings" || a.section != "Appearance" {
		t.Fatal("route failed")
	}
	a.back(-1)
	if a.route != "chat" {
		t.Fatal("back failed")
	}
}
func TestQuestionAndPermissionAnswers(t *testing.T) {
	for _, scenario := range []string{"questions", "permissions"} {
		t.Run(scenario, func(t *testing.T) {
			a := testApp(t, scenario)
			a.selectSession("chat-0")
			tt := ui.NewTester(a.View, 1320, 880)
			if scenario == "questions" {
				for _, m := range a.state.Messages[a.sessionID] {
					for _, b := range m.Blocks {
						if b.Question != nil && len(b.Question.Pages) > 0 {
							option := b.Question.Pages[0].Options[0].Label
							if err := tt.Click(option); err != nil {
								t.Fatal(err)
							}
							tt.Advance(questionAutoAdvance)
							state, _ := a.service.Snapshot(context.Background(), a.sessionID)
							answer := ""
							for _, message := range state.Messages[a.sessionID] {
								for _, block := range message.Blocks {
									if block.Question != nil {
										if len(block.Question.Answers) > 0 {
											answer = block.Question.Answers[0].Options[0]
										}
									}
								}
							}
							if answer != option {
								t.Fatalf("answer %q, want %q", answer, option)
							}
							return
						}
					}
				}
				t.Fatal("question scene missing")
			} else {
				if err := tt.Click("Allow once"); err != nil {
					t.Fatal(err)
				}
				state, _ := a.service.Snapshot(context.Background(), a.sessionID)
				for _, message := range state.Messages[a.sessionID] {
					for _, block := range message.Blocks {
						if block.Permission != nil && block.Permission.Answer == "Allow once" {
							return
						}
					}
				}
				t.Fatal("permission not saved")
			}
		})
	}
}
func TestComposerAndPanelAtSizes(t *testing.T) {
	for _, size := range [][2]int{{900, 600}, {1320, 880}, {1600, 1000}} {
		a := testApp(t, "default")
		a.selectSession("chat-0")
		a.right = true
		a.workspace.OpenSurface("changes")
		tt := ui.NewTester(a.View, size[0], size[1])
		box, ok := tt.Find("Message")
		if !ok {
			t.Fatal("composer missing")
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > float32(size[0])+1 || box.Y+box.H > float32(size[1])+1 {
			t.Fatalf("composer outside %v: %+v", size, box)
		}
	}
}

// Historical source/geometry assertions intentionally target the English option.
func useEnglishSettings(t *testing.T, backend *mock.Server) {
	t.Helper()
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Settings.General.Language = "en"
	if err := backend.UpdateSettings(context.Background(), snapshot.Settings); err != nil {
		t.Fatal(err)
	}
}

// nearRect allows subpixel floating point rounding while catching pixel shifts.
func nearRect(a, b ui.Rect) bool {
	return math.Abs(float64(a.X-b.X)) <= .001 && math.Abs(float64(a.Y-b.Y)) <= .001 &&
		math.Abs(float64(a.W-b.W)) <= .001 && math.Abs(float64(a.H-b.H)) <= .001
}
