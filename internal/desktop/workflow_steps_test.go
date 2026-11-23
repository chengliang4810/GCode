package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// journalStub answers the journal capability on top of whatever else the app's
// service does, and counts how often it was asked.
func journalStub(t *testing.T, app *App, journal contracts.WorkflowJournal, err error) *journalRecorder {
	t.Helper()
	recorder := &journalRecorder{Service: app.service, journal: journal, err: err}
	app.service = recorder
	return recorder
}

// plainService是只有 Service 的宿主：它不提供日志能力。
type plainService struct{ contracts.Service }

type journalRecorder struct {
	contracts.Service
	journal contracts.WorkflowJournal
	err     error
	calls   int
}

func (r *journalRecorder) WorkflowJournal(ctx context.Context, runID string) (contracts.WorkflowJournal, error) {
	r.calls++
	if err := ctx.Err(); err != nil {
		return contracts.WorkflowJournal{}, err
	}
	return r.journal, r.err
}

func journalPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// stepsJournal is one run's log: the run starts, a phase is entered, an
// instance starts, a question is asked, and the run settles.
func stepsJournal(t *testing.T) contracts.WorkflowJournal {
	t.Helper()
	base := time.Date(2026, 11, 23, 13, 0, 0, 0, time.UTC)
	return contracts.WorkflowJournal{Records: []contracts.WorkflowJournalRecord{
		{Sequence: 1, Kind: workflow.JournalRunStarted, At: base, Data: journalPayload(t, workflow.RunStartedPayload{Name: "Review"})},
		{Sequence: 2, Kind: workflow.JournalPhaseEntered, At: base.Add(time.Second), Data: journalPayload(t, workflow.PhasePayload{PhaseID: "plan", Name: "Plan"})},
		{Sequence: 3, Kind: workflow.JournalActorStarted, At: base.Add(2 * time.Second), Data: journalPayload(t, workflow.ActorPayload{ActorID: "a-1", PhaseID: "plan", Name: "Reviewer"})},
		{Sequence: 4, Kind: workflow.JournalQuestionAsked, At: base.Add(3 * time.Second), Data: journalPayload(t, workflow.QuestionPayload{QuestionID: "q-1", ActorID: "a-1", Text: "Which directory?"})},
		{Sequence: 5, Kind: workflow.JournalRunSettled, At: base.Add(4 * time.Second), Data: journalPayload(t, workflow.SettlePayload{Status: "completed"})},
	}}
}

func stepsApp(t *testing.T, journal contracts.WorkflowJournal) (*App, *journalRecorder) {
	t.Helper()
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	recorder := journalStub(t, app, journal, nil)
	return app, recorder
}

func stepsTester(app *App, runID string) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.workflowStepsSection(c, runID) })
	}, 900, 1200)
}

// 步骤面板按阶段分组，组里按事件序号排出发生了什么。
func TestWorkflowStepsPanelGroupsByPhase(t *testing.T) {
	app, _ := stepsApp(t, stepsJournal(t))
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	for _, text := range []string{
		"Steps", "The run", "Run started", "plan",
		"Phase entered", "Plan", "Instance started", "Reviewer",
		"Question asked", "Which directory?", "Run settled", "completed",
	} {
		if !tt.HasText(text) {
			t.Fatalf("steps panel missing %q: %v", text, tt.Texts())
		}
	}
}

// 日志只读一次：换帧不重拉，按需才刷新。
func TestWorkflowStepsPanelReadsOnce(t *testing.T) {
	app, recorder := stepsApp(t, stepsJournal(t))
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if recorder.calls != 1 {
		t.Fatalf("the log is read once, got %d reads", recorder.calls)
	}
	if err := tt.Click("Read the steps again"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if recorder.calls != 2 {
		t.Fatalf("asking again reads again, got %d reads", recorder.calls)
	}
}

// 组可以独立折叠：收起只藏这一组的步骤，别的组照旧。
func TestWorkflowStepsPanelFoldsGroupsIndependently(t *testing.T) {
	app, _ := stepsApp(t, stepsJournal(t))
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	if !tt.HasText("Instance started") {
		t.Fatalf("the group starts open: %v", tt.Texts())
	}
	if err := tt.Click("plan"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Instance started") {
		t.Fatalf("folding the phase hides its steps: %v", tt.Texts())
	}
	// 另一组照旧：运行本身那几条不是阶段的事。
	if !tt.HasText("Run started") {
		t.Fatalf("another group stays open: %v", tt.Texts())
	}
}

// 宿主不提供日志能力时说清楚，不假装这次运行没有步骤。
func TestWorkflowStepsPanelSaysWhenTheHostCannotReadThem(t *testing.T) {
	app, _ := stepsApp(t, contracts.WorkflowJournal{})
	app.service = plainService{Service: app.service}
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	if !tt.HasText("This host does not provide run steps.") {
		t.Fatalf("the panel explains the missing capability: %v", tt.Texts())
	}
}

// 读取失败保留原因，不把它当成空步骤。
func TestWorkflowStepsPanelShowsTheReadError(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	journalStub(t, app, contracts.WorkflowJournal{}, errors.New("the journal is unreadable"))
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	if !tt.HasText("the journal is unreadable") {
		t.Fatalf("the read error is shown: %v", tt.Texts())
	}
	if tt.HasText("This run has no steps yet") {
		t.Fatalf("a failed read is not an empty log: %v", tt.Texts())
	}
}

// 读不出来的行如实计数，不静默丢掉。
func TestWorkflowStepsPanelCountsDamagedLines(t *testing.T) {
	journal := stepsJournal(t)
	journal.Damaged = 2
	app, _ := stepsApp(t, journal)
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	if !tt.HasText("2 lines could not be read.") {
		t.Fatalf("damaged lines are counted: %v", tt.Texts())
	}
}

// 运行又走了一步（workflow.changed）：面板重读一次，而不是每帧重拉。
func TestWorkflowStepsPanelRereadsOnANewEvent(t *testing.T) {
	app, recorder := stepsApp(t, stepsJournal(t))
	tt := stepsTester(app, "workflow-run-1")
	tt.Frame()
	if recorder.calls != 1 {
		t.Fatalf("the first frame reads once, got %d", recorder.calls)
	}
	app.applyEvent(contracts.Event{Kind: "workflow.changed", RunID: "workflow-run-1"})
	tt.Frame()
	tt.Frame()
	if recorder.calls != 2 {
		t.Fatalf("one event means one more read, got %d", recorder.calls)
	}
	// 与这次运行无关的事件不触发重读。
	app.applyEvent(contracts.Event{Kind: "workflow.changed", RunID: "workflow-run-2"})
	tt.Frame()
	if recorder.calls != 2 {
		t.Fatalf("another run's event reads nothing, got %d", recorder.calls)
	}
}
