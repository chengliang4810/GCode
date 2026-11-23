package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

// completionRun is a run that finished and delivered something.
func completionRun(status string) contracts.WorkflowRun {
	started := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	finished := started.Add(95 * time.Second)
	return contracts.WorkflowRun{
		ID: "run-completed", Name: "Review the change", Status: status,
		SessionID: "chat-0", MessageID: "reply-1",
		StartedAt: started, FinishedAt: &finished, UpdatedAt: finished,
		Phases: []contracts.WorkflowPhase{
			{ID: "phase-1", Name: "Plan", Index: 0, Status: contracts.WorkflowPhaseDone, EnteredAt: &started},
			{ID: "phase-2", Name: "Build", Index: 1, Status: contracts.WorkflowPhasePending},
		},
		Actors: []contracts.WorkflowActor{
			{ID: "actor-1", PhaseID: "phase-1", Name: "Reviewer", Status: contracts.WorkflowActorDone, SessionID: "actor-1"},
		},
		Artifacts: []contracts.WorkflowArtifact{
			{ID: "art-1", RunID: "run-completed", Name: "report", Content: "the report body", Version: 1, Primary: true},
			{ID: "art-2", RunID: "run-completed", Name: "diff", Content: "the diff", Version: 1, Order: 1},
		},
	}
}

func completionApp(t *testing.T, run contracts.WorkflowRun) *App {
	t.Helper()
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-dark", true)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	app.state.Sessions = append(app.state.Sessions,
		contracts.Session{ID: "actor-1", ParentRunID: run.ID, Title: "Reviewer", ProjectID: ""})
	app.state.ModelRequests = []contracts.ModelRequest{{
		ID: "req-1", SessionID: "actor-1",
		Usage: model.Usage{Known: true, OutputKnown: true, Input: 1200, Output: 300},
	}}
	return app
}

func completionTester(app *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowCompletionCard(c, app.state.WorkflowRuns[0], 720)
		})
	}, 900, 900)
}

func TestWorkflowCompletionCardShowsPreviewIndexAndFacts(t *testing.T) {
	app := completionApp(t, completionRun(contracts.WorkflowStatusCompleted))
	tt := completionTester(app)
	tt.Frame()
	for _, text := range []string{
		"Workflow completed", "Review the change", "2 delivered",
		"report", "the report body", "Deliveries", "diff",
		"Duration", "1m 35s", "Tokens", "1500", "Instances", "1", "Phases entered", "1",
	} {
		if !tt.HasText(text) {
			t.Fatalf("completion card missing %q: %v", text, tt.Texts())
		}
	}
}

// 完成卡只服务成功的运行：失败和停止都没有卡，不伪造交付。
func TestWorkflowCompletionCardNeedsASuccessfulRun(t *testing.T) {
	for _, status := range []string{contracts.WorkflowStatusErrored, contracts.WorkflowStatusStopped, contracts.WorkflowStatusRunning} {
		app := completionApp(t, completionRun(status))
		tt := completionTester(app)
		tt.Frame()
		if tt.HasText("Workflow completed") {
			t.Fatalf("a %q run has no completion card: %v", status, tt.Texts())
		}
	}
}

// 没有交付的运行也没有卡：没有东西可交付时，没什么可展示的。
func TestWorkflowCompletionCardNeedsADelivery(t *testing.T) {
	run := completionRun(contracts.WorkflowStatusCompleted)
	run.Artifacts = nil
	app := completionApp(t, run)
	tt := completionTester(app)
	tt.Frame()
	if tt.HasText("Workflow completed") {
		t.Fatalf("a run without a delivery has no completion card: %v", tt.Texts())
	}
}

// 未知的统计显示 —，不拿 0 冒充。
func TestWorkflowCompletionCardKeepsUnknownFactsUnknown(t *testing.T) {
	run := completionRun(contracts.WorkflowStatusCompleted)
	run.FinishedAt = nil
	for i := range run.Phases {
		run.Phases[i].EnteredAt = nil
	}
	run.Actors = nil
	app := completionApp(t, run)
	app.state.ModelRequests = nil
	tt := completionTester(app)
	tt.Frame()
	if tt.HasText("0") {
		t.Fatalf("unknown facts must not read as zero: %v", tt.Texts())
	}
	dashes := 0
	for _, text := range tt.Texts() {
		if text == "—" {
			dashes++
		}
	}
	if dashes < 4 {
		t.Fatalf("every unknown fact shows a dash, got %d: %v", dashes, tt.Texts())
	}
}

// 多个交付、没有显式主产物时只画索引，不伪造一张预览。
func TestWorkflowCompletionCardWithoutPrimaryShowsTheIndexOnly(t *testing.T) {
	run := completionRun(contracts.WorkflowStatusCompleted)
	run.Artifacts = []contracts.WorkflowArtifact{
		{ID: "art-1", RunID: run.ID, Name: "report", Content: "the report body", Version: 1},
		{ID: "art-2", RunID: run.ID, Name: "diff", Content: "the diff", Version: 1, Order: 1},
	}
	app := completionApp(t, run)
	tt := completionTester(app)
	tt.Frame()
	if tt.HasText("the report body") {
		t.Fatalf("no primary means no preview: %v", tt.Texts())
	}
	for _, text := range []string{"Deliveries", "report", "diff"} {
		if !tt.HasText(text) {
			t.Fatalf("the index holds every delivery, missing %q: %v", text, tt.Texts())
		}
	}
}

// 完成卡排在运行卡前面：先看见交付，再看见产生它的那次运行。
func TestWorkflowCompletionCardComesBeforeTheRunCard(t *testing.T) {
	app := completionApp(t, completionRun(contracts.WorkflowStatusCompleted))
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowCompletionCards(c, "reply-1", 720)
			app.workflowRunCards(c, "reply-1", 720)
		})
	}, 900, 1200)
	tt.Frame()
	completion, ok := tt.Find("Workflow completed")
	if !ok {
		t.Fatalf("completion card missing: %v", tt.Texts())
	}
	run, ok := tt.Find("Completed")
	if !ok {
		t.Fatalf("run card missing: %v", tt.Texts())
	}
	if completion.Y > run.Y {
		t.Fatalf("the completion card sits above the run card: %g vs %g", completion.Y, run.Y)
	}
}

// 一条不带完成卡的消息不因此多出空位。
func TestWorkflowCompletionCardsDrawNothingWithoutRuns(t *testing.T) {
	app := completionApp(t, completionRun(contracts.WorkflowStatusErrored))
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.workflowCompletionCards(c, "reply-1", 720) })
	}, 900, 400)
	tt.Frame()
	for _, text := range tt.Texts() {
		if text == "Workflow completed" || text == "Deliveries" {
			t.Fatalf("a failed run leaves no completion card: %v", tt.Texts())
		}
	}
}
