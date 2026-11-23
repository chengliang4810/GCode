package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// workflowCardRun is a run with a phase, two instances, a question, a hole and
// two artifacts. It is the shape the conversation card has to draw.
func workflowCardRun() contracts.WorkflowRun {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	return contracts.WorkflowRun{
		ID: "workflow-run-1", Name: "Review the change", Status: contracts.WorkflowStatusRunning,
		ModelID: "goal/model", Concurrency: 2, SessionID: "chat-0", MessageID: "reply-1",
		StartedAt: now, UpdatedAt: now,
		Phases: []contracts.WorkflowPhase{
			{ID: "phase-1", Name: "Plan", Index: 0, Status: contracts.WorkflowPhaseDone},
			{ID: "phase-2", Name: "Build", Index: 1, Status: contracts.WorkflowPhaseRunning},
		},
		Actors: []contracts.WorkflowActor{
			{ID: "actor-1", PhaseID: "phase-1", Name: "Planner", Status: contracts.WorkflowActorDone, SessionID: "actor-1"},
			{ID: "actor-2", PhaseID: "phase-2", Name: "Builder", Status: contracts.WorkflowActorRunning},
			{ID: "actor-3", PhaseID: "phase-2", Name: "Reviewer", Status: contracts.WorkflowActorAsking},
		},
		Questions: []contracts.WorkflowQuestion{{ID: "q-1", RunID: "workflow-run-1", Text: "Which database?", Status: "pending"}},
		Holes:     []contracts.WorkflowHole{{ID: "h-1", RunID: "workflow-run-1", Title: "Missing target", Status: contracts.WorkflowHoleWaiting}},
		Artifacts: []contracts.WorkflowArtifact{
			{ID: "artifact-1", RunID: "workflow-run-1", Name: "plan", Content: "the plan", Version: 1, Order: 0},
			{ID: "artifact-2", RunID: "workflow-run-1", Name: "diff", Content: "the diff", Version: 1, Order: 1},
		},
	}
}

// workflowCardApp builds an app whose state holds one workflow run.
func workflowCardApp(t *testing.T) *App {
	t.Helper()
	app := testApp(t, "default")
	run := workflowCardRun()
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	return app
}

func TestWorkflowRunCardShowsRunFacts(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("%t-%d", dark, width), func(t *testing.T) {
				app := workflowCardApp(t)
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(app.palette.UI())
					ui.Column(c).Fill().Gap(8).Children(func() {
						app.workflowRunCard(c, workflowCardRun(), 720)
					})
				}, width, 900)
				tt.Frame()
				for _, text := range []string{
					"Review the change", "Running", "2 phases", "2 agents working",
					"Plan", "Build", "Planner", "Builder", "Reviewer",
					"1 questions", "1 holes", "plan", "diff",
				} {
					if !tt.HasText(text) {
						t.Fatalf("run card missing %q: %v", text, tt.Texts())
					}
				}
				if err := tt.Click("Stop run"); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestWorkflowRunPaneShowsPhasesQuestionsAndArtifacts(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunView(c, "workflow-run-1")
	}, 900, 1000)
	tt.Frame()
	for _, text := range []string{
		"Review the change", "Plan", "Build", "Planner", "Builder", "Reviewer",
		"Which database?", "Missing target", "the plan", "the diff",
	} {
		if !tt.HasText(text) {
			t.Fatalf("run pane missing %q: %v", text, tt.Texts())
		}
	}
}

func TestWorkflowRunPaneReportsAMissingRun(t *testing.T) {
	app := workflowCardApp(t)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunView(c, "workflow-run-missing")
	}, 900, 600)
	tt.Frame()
	if !tt.HasText("This run is no longer available.") {
		t.Fatalf("a missing run says so: %v", tt.Texts())
	}
}

func TestWorkflowRunCardSitsAtTheTurnThatLaunchedIt(t *testing.T) {
	app := testApp(t, "default")
	reply := contracts.Message{ID: "reply-1", SessionID: "chat-0", Role: "assistant", CreatedAt: time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC), Blocks: []contracts.Block{
		{ID: "text-1", Kind: "text", Text: "The review is running."},
	}}
	run := workflowCardRun()
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.transcriptMessageForTest(c, reply)
	}, 900, 900)
	tt.Frame()
	if !tt.HasText("Review the change") {
		t.Fatalf("the run card belongs to the turn that launched it: %v", tt.Texts())
	}
	other := contracts.Message{ID: "reply-2", SessionID: "chat-0", Role: "assistant", CreatedAt: reply.CreatedAt.Add(time.Minute), Blocks: []contracts.Block{
		{ID: "text-2", Kind: "text", Text: "Another turn."},
	}}
	tt = ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.transcriptMessageForTest(c, other)
	}, 900, 900)
	tt.Frame()
	if tt.HasText("Review the change") {
		t.Fatalf("another turn does not carry the card: %v", tt.Texts())
	}
}

func TestWorkflowSettingsSheetOnlySendsChangedFields(t *testing.T) {
	app := workflowCardApp(t)
	run := workflowCardRun()
	state := &workflowSettingsState{modelID: run.ModelID, hasModel: true, concurrency: run.Concurrency, hasConc: true}
	if app.workflowSettingsChanged(run, state) {
		t.Fatal("an unchanged sheet cannot be applied")
	}
	state.concurrency = 4
	if !app.workflowSettingsChanged(run, state) {
		t.Fatal("a changed concurrency is a change")
	}
	state.concurrency = run.Concurrency
	state.modelID = "other/model"
	if !app.workflowSettingsChanged(run, state) {
		t.Fatal("a changed model is a change")
	}
	state.modelID = run.ModelID
	state.modelID = ""
	if !app.workflowSettingsChanged(run, state) {
		t.Fatal("the session model differs from the run's model")
	}
}

// 留白是一座独立的站：它插在所属阶段之后，名字就是留白的标题。
func TestWorkflowRunCardShowsAHoleStation(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := workflowCardRun()
	run.Holes = []contracts.WorkflowHole{{
		ID: "h-1", RunID: run.ID, PhaseID: "phase-1", Title: "Missing target", Status: contracts.WorkflowHoleWaiting,
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 900)
	tt.Frame()
	if !tt.HasText("Missing target") {
		t.Fatalf("the hole keeps its title on the timeline: %v", tt.Texts())
	}
	if !tt.HasText("Waiting for fill") {
		t.Fatalf("a waiting hole says so: %v", tt.Texts())
	}
}

// 留白芯片点开的是这次运行：补全由主代理写代码，用户在这里看不到输入框。
func TestWorkflowHolesChipOpensTheRun(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowRunCard(c, workflowCardRun(), 720)
		})
	}, 900, 900)
	tt.Frame()
	if err := tt.Click("1 holes"); err != nil {
		t.Fatal(err)
	}
	if app.workspace.ActiveKind() != "workflow" {
		t.Fatalf("the holes chip opens the run detail: %+v", app.workspace.ActiveKind())
	}
}

// 详情页分得清等着补全的留白和已经补上的留白。
func TestWorkflowRunPaneSeparatesFilledHoles(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := workflowCardRun()
	run.Holes = []contracts.WorkflowHole{
		{ID: "h-1", RunID: run.ID, PhaseID: "phase-1", Title: "Missing target", Status: contracts.WorkflowHoleWaiting},
		{ID: "h-2", RunID: run.ID, PhaseID: "phase-1", Title: "Filled target", Text: "the target", Status: contracts.WorkflowHoleFilled},
	}
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunView(c, run.ID)
	}, 900, 1000)
	tt.Frame()
	for _, text := range []string{"Missing target", "Filled target", "the target", "Filled holes"} {
		if !tt.HasText(text) {
			t.Fatalf("run pane missing %q: %v", text, tt.Texts())
		}
	}
}
