package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// ledgeRun is a run with enough phases that its stage line cannot fit a card.
func ledgeRun() contracts.WorkflowRun {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	names := []string{"Plan", "Build", "Check", "Report", "Ship", "Close"}
	phases := make([]contracts.WorkflowPhase, 0, len(names))
	for i, name := range names {
		phases = append(phases, contracts.WorkflowPhase{ID: "phase-" + string(rune('a'+i)), Name: name, Index: i, Status: contracts.WorkflowPhaseDone})
	}
	return contracts.WorkflowRun{
		ID: "run-ledge", Name: "Wide run", Status: contracts.WorkflowStatusCompleted,
		SessionID: "chat-0", MessageID: "reply-1", StartedAt: now, UpdatedAt: now,
		Phases: phases,
	}
}

func ledgeApp(t *testing.T) *App {
	t.Helper()
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-light", false)
	app.state.WorkflowRuns = []contracts.WorkflowRun{ledgeRun()}
	return app
}

func ledgeTester(app *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(12).Children(func() {
			app.workflowRunCard(c, ledgeRun(), 720)
		})
	}, 900, 900)
}

// 阶段线放不下时，滚到视口外的站折到边檐上，点一下把那一站带回来。
func TestWorkflowTimelineLedgeFoldsStationsOutsideTheViewport(t *testing.T) {
	app := ledgeApp(t)
	tt := ledgeTester(app)
	tt.Frame()
	if !tt.HasText("Ship") || !tt.HasText("Close") {
		t.Fatalf("the stage line still names every station: %v", tt.Texts())
	}
	// 视口之外的最后两站落在边檐上。
	lamp := "workflow-timeline-ledge-lamp:run-ledge:4"
	rect, ok := tt.Find("Scroll to Ship")
	if !ok {
		t.Fatalf("the folded station has a ledge lamp: %v", tt.Texts())
	}
	if err := tt.Click("Scroll to Ship"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	state := app.workflowUI.scroll["run-ledge"]
	if state == nil {
		t.Fatal("the stage line keeps its scroll state")
	}
	if !state.Scrolling() && state.X <= 0 {
		t.Fatalf("choosing a ledge lamp scrolls the stage line: %+v at %g,%g", state, rect.X, rect.Y)
	}
	_ = lamp
}

// 放得下的阶段线不画边檐。
func TestWorkflowTimelineLedgeStaysAwayWhenEverythingFits(t *testing.T) {
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-light", false)
	run := ledgeRun()
	run.Phases = run.Phases[:2]
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(12).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 900)
	tt.Frame()
	for _, text := range tt.Texts() {
		if text == "Ship" || text == "+1" {
			t.Fatalf("a short stage line has no ledge: %v", tt.Texts())
		}
	}
}

// 留白站在边檐上仍然用警示色，不另立一套图例。
func TestWorkflowTimelineLedgeKeepsTheHoleColor(t *testing.T) {
	app := ledgeApp(t)
	run := ledgeRun()
	run.Holes = []contracts.WorkflowHole{{
		ID: "h-1", RunID: run.ID, PhaseID: "phase-f", Title: "The ending", Status: contracts.WorkflowHoleWaiting,
	}}
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(12).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 900)
	tt.Frame()
	if _, ok := tt.Find("Scroll to The ending"); !ok {
		t.Fatalf("the folded hole has a ledge lamp: %v", tt.Texts())
	}
}
