package desktop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

type questionUIService struct {
	contracts.Service
	state    contracts.Snapshot
	commands []contracts.Command
	failNext bool
}

func (s *questionUIService) Snapshot(context.Context) (contracts.Snapshot, error) {
	return contracts.CloneSnapshot(s.state), nil
}

func (s *questionUIService) Execute(_ context.Context, cmd contracts.Command) error {
	if cmd.Kind != "request.answer" {
		return nil
	}
	s.commands = append(s.commands, cmd.Clone())
	if s.failNext {
		s.failNext = false
		return errors.New("answer delivery failed")
	}
	for sessionID, messages := range s.state.Messages {
		for _, message := range messages {
			for _, block := range message.Blocks {
				if block.Question != nil && block.Question.ID == cmd.ID {
					if err := contracts.ValidateQuestionAnswers(block.Question.Pages, cmd.QuestionAnswers); err != nil {
						return err
					}
					block.Question.Resolved = true
					block.Question.Answers = contracts.CloneQuestionAnswers(cmd.QuestionAnswers)
					for i := range s.state.Sessions {
						if s.state.Sessions[i].ID == sessionID {
							s.state.Sessions[i].Status = contracts.Working
						}
					}
				}
			}
		}
	}
	s.state.Revision++
	return nil
}

func questionComposerFixture(t *testing.T) (*App, *questionUIService, *contracts.Question) {
	t.Helper()
	app := testApp(t, "questions")
	app.selectSession("chat-0")
	app.draft = "Keep the ordinary message draft"
	app.attachments = []contracts.Attachment{{ID: "ordinary-attachment", Name: "draft.txt", Status: "ready"}}
	app.state.Settings.Appearance.ReduceMotion = true
	question := &contracts.Question{ID: "wizard", Pages: []contracts.QuestionPage{
		{Question: "Which approach?", Header: "Approach", Options: []contracts.QuestionOption{
			{Label: "Approach A", Description: "Use the first approach"},
			{Label: "Approach B", Description: "Use the second approach"},
		}},
		{Question: "Which features?", Header: "Features", MultiSelect: true, Options: []contracts.QuestionOption{
			{Label: "Skills", Description: "Reusable instructions"},
			{Label: "Hooks", Description: "Lifecycle callbacks"},
		}},
	}}
	app.state.Messages[app.sessionID] = []contracts.Message{{ID: "question-message", SessionID: app.sessionID, Role: "assistant",
		Blocks: []contracts.Block{{ID: question.ID, Kind: "question", Question: question}}}}
	app.state.Revision++
	service := &questionUIService{Service: app.service, state: contracts.CloneSnapshot(app.state)}
	app.service = service
	return app, service, question
}

func TestQuestionComposerReplacesMessageAndRestoresDraft(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, size := range []struct{ width, height int }{{900, 600}, {1320, 880}} {
			t.Run(fmt.Sprintf("dark=%t/%dx%d", dark, size.width, size.height), func(t *testing.T) {
				app, service, _ := questionComposerFixture(t)
				app.state.Settings.Appearance.Mode = "light"
				if dark {
					app.state.Settings.Appearance.Mode = "dark"
				}
				tt := ui.NewTester(app.View, size.width, size.height)
				if _, ok := tt.Find("Message"); ok {
					t.Fatal("ordinary message input is still mounted under the question")
				}
				panel, ok := tt.Find("Question panel")
				if !ok || panel.Y <= 100 || panel.Y+panel.H > float32(size.height) {
					t.Fatalf("question panel is not in the bottom composer area: %+v", panel)
				}
				count := 0
				for _, text := range tt.Texts() {
					if text == "Which approach?" {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("interactive question duplicated into transcript: %d", count)
				}
				if err := tt.Click("Approach A"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(questionAutoAdvance)
				if !tt.HasText("2/2") || !tt.HasText("Which features?") || len(service.commands) != 0 {
					t.Fatal("first page submitted before the whole request was answered")
				}
				if err := tt.Click("Skills"); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click("Previous question"); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click("Approach B"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(questionAutoAdvance)
				if err := tt.Click("Hooks"); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click("Submit"); err != nil {
					t.Fatal(err)
				}
				if len(service.commands) != 1 || len(service.commands[0].QuestionAnswers) != 2 ||
					!slices.Equal(service.commands[0].QuestionAnswers[0].Options, []string{"Approach B"}) ||
					!slices.Equal(service.commands[0].QuestionAnswers[1].Options, []string{"Skills", "Hooks"}) {
					t.Fatalf("paged answers were lost: %+v", service.commands)
				}
				if _, ok := tt.Find("Message"); !ok || app.draft != "Keep the ordinary message draft" {
					t.Fatal("ordinary composer or its draft was not restored")
				}
				if len(app.attachments) != 1 || app.attachments[0].ID != "ordinary-attachment" {
					t.Fatal("question discarded the ordinary message attachment")
				}
				if tt.HasText("Which features?") || tt.HasText("Reusable instructions") {
					t.Fatal("resolved question kept its interactive content in the transcript")
				}
				if !tt.HasText("Approach") {
					t.Fatalf("resolved question lost its passive history record: %v resolved=%v revision=%d", tt.Texts(), app.state.Messages[app.sessionID][0].Blocks[0].Question.Resolved, app.state.Revision)
				}
			})
		}
	}
}

func TestQuestionComposerTextOverrideFailureAndSessionIsolation(t *testing.T) {
	app, service, _ := questionComposerFixture(t)
	service.failNext = true
	tt := ui.NewTester(app.View, 900, 700)
	if err := tt.Click("Approach A"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(questionAutoAdvance)
	if err := tt.Click("Skills"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Your answer"); err != nil {
		t.Fatal(err)
	}
	tt.Type("42 custom features")
	// Number input must stay text instead of selecting an option.
	tt.TypeKey(0, ui.Key1, "1")
	scope := "main:" + app.sessionID
	state := app.questionForms[scope]
	if state.Typed[1] != "42 custom features1" || state.Picked[1][1] {
		t.Fatal("typing a number changed a question option")
	}
	originalSession := app.sessionID
	app.selectSession("chat-1")
	tt.Frame()
	if _, ok := tt.Find("Your answer"); ok {
		t.Fatal("question from another session took over the composer")
	}
	app.selectSession(originalSession)
	tt.Frame()
	if !tt.HasText("2/2") || state.Typed[1] != "42 custom features1" {
		t.Fatal("navigation discarded the current question page")
	}
	if err := tt.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	state = app.questionForms[scope]
	if state == nil || state.Pending || state.Typed[1] != "42 custom features1" || !tt.HasText("answer delivery failed") {
		t.Fatal("failed answer lost its form or left it unanswerable")
	}
	if err := tt.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 2 || service.commands[1].QuestionAnswers[1].Text != "42 custom features1" ||
		len(service.commands[1].QuestionAnswers[1].Options) != 0 {
		t.Fatalf("text did not override selected options: %+v", service.commands)
	}
	if app.draft != "Keep the ordinary message draft" {
		t.Fatal("question text overwrote the ordinary draft")
	}
}

func TestQuestionComposerPassiveHistoryAndStoppedRun(t *testing.T) {
	app, _, question := questionComposerFixture(t)
	app.state.Sessions[0].Status = contracts.Idle
	question.Resolved = true
	app.state.Settings.General.Language = i18n.Chinese
	app.palette = design.Palette("rcode", true)
	tt := ui.NewTester(app.View, 900, 600)
	if _, ok := tt.Find("Message"); !ok {
		// The localized composer label is checked through the actual UI below.
		if _, translated := tt.Find("消息"); !translated {
			t.Fatal("a stopped question prevented further messages")
		}
	}
	if tt.HasText("Which approach?") || tt.HasText("Approach A") {
		t.Fatal("passive history still includes the complete question")
	}
}

func TestQuestionComposerNumberShortcutSelectsOnlyVisiblePage(t *testing.T) {
	app, service, _ := questionComposerFixture(t)
	tt := ui.NewTester(app.View, 900, 700)
	tt.Key(0, ui.Key2)
	tt.Advance(questionAutoAdvance)
	if !tt.HasText("2/2") || len(service.commands) != 0 {
		t.Fatal("number shortcut did not advance the first question")
	}
	tt.Key(0, ui.Key1)
	tt.Key(0, ui.Key2)
	tt.Key(ui.Cmd, ui.KeyEnter)
	if len(service.commands) != 1 || !slices.Equal(service.commands[0].QuestionAnswers[0].Options, []string{"Approach B"}) ||
		!slices.Equal(service.commands[0].QuestionAnswers[1].Options, []string{"Skills", "Hooks"}) {
		t.Fatalf("question keyboard submission lost choices: %+v", service.commands)
	}
}

func TestQuestionComposerEnterWaitsForIMEAndShiftEnterMakesNewline(t *testing.T) {
	app, service, _ := questionComposerFixture(t)
	tt := ui.NewTester(app.View, 900, 700)
	if err := tt.Click("Your answer"); err != nil {
		t.Fatal(err)
	}
	tt.Compose("自定义", 3)
	tt.Key(0, ui.KeyEnter)
	if !tt.HasText("1/2") || len(service.commands) != 0 {
		t.Fatal("IME composition submitted the question")
	}
	tt.Compose("", 0)
	tt.Type("custom first answer")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("continued")
	tt.Key(0, ui.KeyEnter)
	if !tt.HasText("2/2") {
		t.Fatal("Enter did not advance the custom answer")
	}
	if err := tt.Click("Your answer"); err != nil {
		t.Fatal(err)
	}
	tt.Type("custom second answer")
	tt.Key(0, ui.KeyEnter)
	if len(service.commands) != 1 || service.commands[0].QuestionAnswers[0].Text != "custom first answer\ncontinued" ||
		service.commands[0].QuestionAnswers[1].Text != "custom second answer" {
		t.Fatalf("Enter submission lost custom answers: %+v", service.commands)
	}
}

func TestQuestionComposerSidePaneKeepsBothMessageDrafts(t *testing.T) {
	app, service, _ := questionComposerFixture(t)
	app.fit.Right, app.rightExpanded = true, true
	session := app.currentSession()
	side := app.sideComposer(session.ID)
	side.Draft = "Keep the side message draft"
	side.modelID = app.modelID
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.sideComposerView(c, session, side)
	}, 600, 800)
	if _, ok := tt.Find("Question panel"); !ok {
		t.Fatal("side question did not replace its message composer")
	}
	if err := tt.Click("Approach A"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(questionAutoAdvance)
	if err := tt.Click("Skills"); err != nil {
		t.Fatalf("%v: texts=%v form=%+v", err, tt.Texts(), app.questionForms["side:"+session.ID])
	}
	if err := tt.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 1 || service.commands[0].SessionID != session.ID ||
		side.Draft != "Keep the side message draft" || app.draft != "Keep the ordinary message draft" {
		t.Fatal("side question used the wrong owner or changed a message draft")
	}
	if _, ok := tt.Find("Message to " + session.Title); !ok {
		t.Fatal("side message editor was not restored")
	}
}
