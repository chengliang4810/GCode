package desktop

import (
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

type goalUIService struct {
	contracts.Service
	commands []contracts.Command
}

func (s *goalUIService) Execute(ctx context.Context, command contracts.Command) error {
	if strings.HasPrefix(command.Kind, "goal.") || command.Kind == "message.send" {
		s.commands = append(s.commands, command.Clone())
	}
	if strings.HasPrefix(command.Kind, "goal.") {
		return nil
	}
	return s.Service.Execute(ctx, command)
}

func setGoalFixture(a *App, sessionID string, goal *contracts.SessionGoal) {
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == sessionID {
			a.state.Sessions[i].Goal = goal
		}
	}
}

func TestGoalCommandPickerUsesExistingSendFlow(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new-session"
		if existing {
			name = "existing-session"
		}
		t.Run(name, func(t *testing.T) {
			a := testApp(t, "default")
			if existing {
				a.selectSession("chat-0")
			}
			service := &goalUIService{Service: a.service}
			a.service = service
			tt := ui.NewTester(a.View, 1320, 880)
			if _, ok := tt.Find("Start a goal"); ok {
				t.Fatal("composer exposed a standalone goal shortcut")
			}
			if err := tt.Click("Message"); err != nil {
				t.Fatal(err)
			}
			tt.Type("/go")
			tt.Key(0, ui.KeyEnter)
			tt.Type("Finish the current task")
			if contracts.PlainPrompt(a.draft) != "/goal Finish the current task" || !tt.Focused("Message") {
				t.Fatalf("goal command did not retain the objective and focus: %q", a.draft)
			}
			tt.Key(0, ui.KeyEnter)
			if len(service.commands) != 1 {
				t.Fatalf("send commands = %d", len(service.commands))
			}
			command := service.commands[0]
			if command.Kind != "message.send" || command.Send == nil || contracts.PlainPrompt(command.Send.Text) != "/goal Finish the current task" || command.Send.RequestID == "" {
				t.Fatalf("goal entry bypassed the typed send contract: %+v", command)
			}
			if existing && command.Send.SessionID != "chat-0" || !existing && command.Send.SessionID != "" {
				t.Fatalf("goal entry used the wrong creation path: %+v", command.Send)
			}
			if a.pendingNew || a.sessionID == "" || a.draft != "" {
				t.Fatal("goal entry broke send acknowledgement or new-session takeover")
			}
		})
	}
}

func TestGoalBannerUsesAuthoritativeStateAndRoutesCommands(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "Steer without replacing the goal"
	service := &goalUIService{Service: a.service}
	a.service = service
	budget := int64(5000)
	goal := &contracts.SessionGoal{
		ID: "goal-1", Objective: "Complete the typed backend", Status: contracts.GoalActive,
		Iteration: 3, TokenBudget: &budget, TokensUsed: 1200, TimeUsedMS: 90000,
		Verifications: []contracts.GoalVerification{{Iteration: 2, Reason: "The integration test still fails", NextAction: "Fix event ordering"}},
	}
	setGoalFixture(a, a.sessionID, goal)
	tt := ui.NewTester(a.View, 1320, 880)
	if !tt.HasText(goal.Objective) || !tt.HasText("Active goal") {
		t.Fatal("banner omitted authoritative goal state")
	}
	if _, ok := tt.Find("Start a goal"); ok {
		t.Fatal("an existing goal exposed a new-goal shortcut")
	}
	if err := tt.Click("Edit goal"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	for _, text := range []string{"1,200 / 5,000 tokens", "The integration test still fails", "Next: Fix event ordering", "Verification · Round 2 · Needs more work"} {
		if !tt.HasText(text) {
			t.Fatalf("goal details omitted %q", text)
		}
	}
	if err := tt.Click("Pause goal"); err != nil {
		t.Fatal(err)
	}
	if a.currentSession().Goal.Status != contracts.GoalActive {
		t.Fatal("pause optimistically changed authoritative goal state")
	}
	goal.Status = contracts.GoalPaused
	tt.Frame()
	if err := tt.Click("Resume goal"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Clear goal"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 3 {
		t.Fatalf("goal commands = %d", len(service.commands))
	}
	for index, kind := range []string{"goal.pause", "goal.resume", "goal.clear"} {
		command := service.commands[index]
		if command.Kind != kind || command.SessionID != "chat-0" || command.ID != goal.ID {
			t.Fatalf("goal action crossed ownership: %+v", command)
		}
	}
	if a.draft != "Steer without replacing the goal" {
		t.Fatal("goal actions replaced the message draft")
	}
	setGoalFixture(a, a.sessionID, nil)
	tt.Frame()
	setGoalFixture(a, a.sessionID, goal)
	tt.Frame()
	if err := tt.Click("Edit goal"); err != nil {
		t.Fatal("goal did not survive removal and remount:", err)
	}
}

func TestGoalBudgetAndIncompleteUsage(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	budget := int64(3000)
	goal := &contracts.SessionGoal{
		ID: "limited", Objective: "Finish within the supplied budget", Status: contracts.GoalBudgetLimited,
		TokenBudget: &budget, UnknownUsageCount: 1, Reason: "The token budget was reached",
	}
	setGoalFixture(a, a.sessionID, goal)
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Edit goal"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("At least 0 / 3,000 tokens · Usage incomplete") || !tt.HasText("Budget reached") || !tt.HasText(goal.Reason) {
		t.Fatal("budget state hid missing usage or its stopping reason")
	}
	if _, ok := tt.Find("Resume goal"); ok {
		t.Fatal("budget-limited goal offered resume")
	}
	goal.Status = contracts.GoalPaused
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	service := &goalUIService{Service: a.service}
	a.service = service
	tt.Frame()
	_ = tt.Click("Resume goal")
	if len(service.commands) != 0 {
		t.Fatal("resume was allowed while an ordinary turn was running")
	}
}

func TestGoalSideChatIsolationAndSubagentReadOnly(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.draft = "Keep the parent draft"
	child := sideSession(t, a, "showcase-agent")
	child.Goal = &contracts.SessionGoal{ID: "side-goal", Objective: "Check the side conversation", Status: contracts.GoalActive}
	setGoalFixture(a, child.ID, child.Goal)
	service := &goalUIService{Service: a.service}
	a.service = service
	tt := ui.NewTester(func(c *ui.Context) {
		localize.Set(c, a.state.Settings.General.Language)
		c.SetTheme(a.palette.UI())
		ui.Column(c).Fill().Children(func() { a.sideTranscript(c, child.ID) })
	}, 600, 760)
	if err := tt.Click("Pause goal"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 1 || service.commands[0].SessionID != child.ID || service.commands[0].ID != "side-goal" || a.draft != "Keep the parent draft" || a.sessionID != "chat-0" {
		t.Fatal("side goal action changed the parent conversation")
	}
	setGoalFixture(a, child.ID, nil)
	sideDraft := a.sideComposer(child.ID)
	sideDraft.Draft = ""
	tt.Frame()
	if _, ok := tt.Find("Start a goal"); ok {
		t.Fatal("side composer exposed a standalone goal shortcut")
	}
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("/go")
	tt.Key(0, ui.KeyEnter)
	tt.Type("Check this independently")
	if contracts.PlainPrompt(sideDraft.Draft) != "/goal Check this independently" || a.draft != "Keep the parent draft" || !tt.Focused("Message to "+child.Title) {
		t.Fatal("side goal entry crossed draft or focus ownership")
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].SubagentType = "Explore"
		}
	}
	tt.Frame()
	for _, label := range []string{"Pause goal", "Resume goal", "Edit goal", "Start a goal"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("read-only child exposed %q", label)
		}
	}
}

func TestGoalElapsedOnlyAccruesWhileRunning(t *testing.T) {
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		status contracts.GoalStatus
		want   int64
	}{
		{"active", contracts.GoalActive, 15000},
		{"verifying", contracts.GoalVerifying, 15000},
		{"paused", contracts.GoalPaused, 5000},
		{"complete", contracts.GoalComplete, 5000},
		{"budget-limited", contracts.GoalBudgetLimited, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			goal := &contracts.SessionGoal{Status: tc.status, ActiveSince: &start, TimeUsedMS: 5000}
			if got := goalElapsed(goal, start.Add(10*time.Second)); got != tc.want {
				t.Fatalf("elapsed = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGoalHeaderDurationPreservesWholeSeconds(t *testing.T) {
	for _, test := range []struct {
		seconds int64
		want    string
	}{{0, "0s"}, {59, "59s"}, {60, "1m 00s"}, {3599, "59m 59s"}, {3600, "1h 00m 00s"}, {86399, "23h 59m 59s"}, {86400, "1d 00h 00m 00s"}} {
		t.Run(test.want, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.General.Language = "en"
			session := contracts.Session{ID: "duration", Goal: &contracts.SessionGoal{
				ID: "goal", Objective: "Timing fixture", Status: contracts.GoalPaused, TimeUsedMS: test.seconds*1000 + 999,
			}}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				a.goalBanner(c, session)
			}, 900, 200)
			if !tt.HasText("· " + test.want) {
				t.Fatalf("goal header omitted whole-second duration: %v", tt.Texts())
			}
		})
	}
}

func TestGoalEditorOwnsDraftByConversationAndGoal(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	setGoalFixture(a, "chat-0", &contracts.SessionGoal{ID: "first", Objective: "Original objective", Status: contracts.GoalPaused})
	state := a.goalEditor("chat-0")
	state.draft = "Keep this unsaved edit"
	setGoalFixture(a, "chat-0", &contracts.SessionGoal{ID: "second", Objective: "Replacement goal", Status: contracts.GoalPaused})
	a.goalEditor("chat-0")
	if state.goalID != "first" || state.draft != "Keep this unsaved edit" || state.original != "Original objective" {
		t.Fatal("a replacement goal overwrote an unsaved draft")
	}
	service := &goalUIService{Service: a.service}
	a.service = service
	a.saveGoalEditor("chat-0", state)
	if len(service.commands) != 1 || service.commands[0].Kind != "goal.edit" ||
		service.commands[0].ID != "first" ||
		service.commands[0].GoalEdit.ExpectedRevision != 0 ||
		service.commands[0].GoalEdit.Objective != "Keep this unsaved edit" {
		t.Fatal("editor save lost optimistic ownership:", service.commands)
	}
}

func TestGoalEditorFocusAndSaveShortcut(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "Keep the message draft"
	setGoalFixture(a, "chat-0", &contracts.SessionGoal{ID: "goal", Objective: "Original objective", Status: contracts.GoalPaused})
	service := &goalUIService{Service: a.service}
	a.service = service
	state := a.goalEditor("chat-0")
	state.focusPending = true
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(a.palette.UI()); a.goalEditorView(c, "chat-0") }, 640, 600)
	tt.Frame()
	if !tt.Focused("Goal objective") {
		t.Fatal("opening the editor did not focus its objective")
	}
	tt.Type("Edited ")
	submitted := strings.TrimSpace(state.draft)
	tt.Key(ui.Cmd, ui.KeyS)
	if len(service.commands) != 1 || service.commands[0].Kind != "goal.edit" ||
		service.commands[0].GoalEdit.Objective != submitted ||
		service.commands[0].GoalEdit.ExpectedRevision != 0 ||
		a.draft != "Keep the message draft" {
		t.Fatal("save shortcut crossed ownership or did not save the draft")
	}
}

func TestGoalBannerRenderEvidence(t *testing.T) {
	for _, language := range []string{i18n.English, i18n.Chinese} {
		t.Run(language, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.General.Language = language
			session := a.state.Sessions[0]
			budget := int64(4000)
			session.Goal = &contracts.SessionGoal{
				ID: "render-goal", Objective: "完成目标模式并验证远程事件续接", Status: contracts.GoalPaused,
				Iteration: 4, TokenBudget: &budget, TokensUsed: 1800, UnknownUsageCount: 1, TimeUsedMS: 72000,
				Reason: "等待修复断线重连测试", NextAction: "验证事件游标与任务状态",
				Verifications: []contracts.GoalVerification{{Iteration: 4, Reason: "远程事件续接尚未通过", NextAction: "补充断线场景"}},
			}
			tt := ui.NewTester(func(c *ui.Context) {
				localize.Set(c, language)
				a.palette = design.Palette("rcode-light", false)
				c.SetTheme(a.palette.UI())
				c.Root().Font("Geist").FontSize(13).Background(a.palette.Background).TextColor(a.palette.Text)
				ui.Column(c).Fill().Children(func() { a.goalBanner(c, session) })
			}, 460, 400)
			tt.Frame()
			tt.Frame()
			for _, label := range []string{a.t("Edit goal"), a.t("Resume goal"), a.t("Clear goal")} {
				bounds, ok := tt.Find(label)
				if !ok || bounds.X < 0 || bounds.X+bounds.W > 460 || bounds.H <= 0 {
					t.Fatalf("narrow goal control is missing or clipped: %s %+v", label, bounds)
				}
			}
			writeGoalRenderEvidence(t, tt, language)
		})
	}
}

func writeGoalRenderEvidence(t *testing.T, tt *ui.Tester, language string) {
	t.Helper()
	directory := os.Getenv("RCODE_GOAL_EVIDENCE")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "goal-details-"+language)
	file, err := os.Create(path + ".png")
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, tt.Image())
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	evidence, err := json.MarshalIndent(map[string]any{
		"renderer": "MyGo ui.NewTester implementation render; not native-window or visual-parity acceptance",
		"language": language, "width": tt.Image().Bounds().Dx(), "height": tt.Image().Bounds().Dy(),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", append(evidence, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
