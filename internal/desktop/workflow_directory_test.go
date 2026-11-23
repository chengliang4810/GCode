package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// directoryRun is one run of a conversation, for the directory page.
func directoryRun(id, status string, updated time.Time) contracts.WorkflowRun {
	return contracts.WorkflowRun{
		ID: id, Name: id, SessionID: "chat-0", Status: status, MessageID: "reply-1",
		StartedAt: updated.Add(-30 * time.Second), UpdatedAt: updated, Concurrency: 1,
		Phases: []contracts.WorkflowPhase{{ID: "plan", Name: "Plan", Index: 0}},
	}
}

func directoryApp(t *testing.T) *App {
	t.Helper()
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-dark", true)
	base := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	// 用场景里已有的那条会话，只换标题：目录按会话标题命名。
	found := false
	for i := range app.state.Sessions {
		if app.state.Sessions[i].ID == "chat-0" {
			app.state.Sessions[i].Title = "Review the change"
			found = true
		}
	}
	if !found {
		app.state.Sessions = append(app.state.Sessions, contracts.Session{ID: "chat-0", Title: "Review the change"})
	}
	app.state.WorkflowRuns = []contracts.WorkflowRun{
		directoryRun("run-live", contracts.WorkflowStatusRunning, base.Add(2*time.Minute)),
		directoryRun("run-done", contracts.WorkflowStatusCompleted, base),
		directoryRun("run-failed", contracts.WorkflowStatusErrored, base.Add(time.Minute)),
	}
	return app
}

func directoryTester(app *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunDirectoryView(c, "chat-0")
	}, 900, 700)
}

// 运行目录按「还在跑 / 已结束」两桶列出这条对话的运行，最新在前。
func TestWorkflowRunDirectoryListsTheConversationsRuns(t *testing.T) {
	app := directoryApp(t)
	tt := directoryTester(app)
	tt.Frame()
	for _, text := range []string{
		"Runs of Review the change", "3 runs", "Running", "Ended",
		"run-live", "run-done", "run-failed", "Completed", "Failed",
	} {
		if !tt.HasText(text) {
			t.Fatalf("run directory missing %q: %v", text, tt.Texts())
		}
	}
	running, ok := tt.Find("Running")
	if !ok {
		t.Fatalf("the running group has a heading: %v", tt.Texts())
	}
	ended, ok := tt.Find("Ended")
	if !ok {
		t.Fatalf("the ended group has a heading: %v", tt.Texts())
	}
	if running.Y > ended.Y {
		t.Fatalf("running comes before ended: %g vs %g", running.Y, ended.Y)
	}
}

// 点一行进那一次运行的详情。
func TestWorkflowRunDirectoryOpensOneRun(t *testing.T) {
	app := directoryApp(t)
	tt := directoryTester(app)
	tt.Frame()
	// 行按钮的无障碍名是「Open run details」；这里按名字点第一行即可。
	if err := tt.Click("Open run details"); err != nil {
		t.Fatal(err)
	}
	if app.workspace.ActiveKind() != "workflow" {
		t.Fatalf("a row opens the run detail: %q", app.workspace.ActiveKind())
	}
}

// 从运行详情能回到这条对话的运行目录。
func TestWorkflowRunPaneOpensTheDirectory(t *testing.T) {
	app := directoryApp(t)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunView(c, "run-done")
	}, 900, 900)
	tt.Frame()
	if err := tt.Click("All runs in this conversation"); err != nil {
		t.Fatal(err)
	}
	if app.workspace.ActiveKind() != "workflow-runs" {
		t.Fatalf("the run detail opens the directory: %q", app.workspace.ActiveKind())
	}
}

// 没跑过工作流的对话说清楚，不画一张空表。
func TestWorkflowRunDirectorySaysWhenNothingRan(t *testing.T) {
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-dark", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunDirectoryView(c, "chat-0")
	}, 900, 500)
	tt.Frame()
	if !tt.HasText("This conversation has not run a workflow") {
		t.Fatalf("an empty directory says so: %v", tt.Texts())
	}
	for _, text := range tt.Texts() {
		if text == "Running" || text == "Ended" {
			t.Fatalf("an empty directory has no groups: %v", tt.Texts())
		}
	}
}
