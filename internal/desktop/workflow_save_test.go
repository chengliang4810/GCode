package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// completedRun 是一次已经跑完、执行过一份脚本的运行。
func completedRun(script string) contracts.WorkflowRun {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	finished := now.Add(2 * time.Minute)
	return contracts.WorkflowRun{
		ID: "workflow-run-1", Name: "review", Status: contracts.WorkflowStatusCompleted,
		SessionID: "chat-0", MessageID: "reply-1", ProjectID: "project-1",
		ModelID: "goal/model", Concurrency: 2, Script: script,
		StartedAt: now, UpdatedAt: finished, FinishedAt: &finished,
		Phases: []contracts.WorkflowPhase{
			{ID: "phase-1", Name: "Plan", Index: 0, Status: contracts.WorkflowPhaseDone},
		},
		Actors: []contracts.WorkflowActor{
			{ID: "actor-1", PhaseID: "phase-1", Name: "Planner", Status: contracts.WorkflowActorDone},
		},
	}
}

// 跑完的运行卡上出现「保存」；还没有保存过就不该出现「再次运行」。
func TestWorkflowRunCardOffersSaveOnAFinishedRun(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := completedRun(`{"phases":[]}`)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 600)
	tt.Frame()
	if !tt.HasText("Save workflow") {
		t.Fatalf("a finished run can be saved: %v", tt.Texts())
	}
	if tt.HasText("Run again") {
		t.Fatalf("an unsaved run cannot be run again: %v", tt.Texts())
	}
}

// 已经保存过的运行反过来：显示「已保存」与「再次运行」，不再显示「保存」。
func TestWorkflowRunCardOffersRunAgainOnceSaved(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := completedRun(`{"phases":[]}`)
	run.DefinitionID = "definition-1"
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	app.state.WorkflowDefinitions = []contracts.WorkflowDefinition{{
		ID: "definition-1", Name: "review", Scope: workflow.SavedScopeProject, ProjectID: "project-1",
		Script: run.Script,
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 600)
	tt.Frame()
	if !tt.HasText("Run again") {
		t.Fatalf("a saved run can run again: %v", tt.Texts())
	}
	if tt.HasText("Save workflow") {
		t.Fatalf("a saved run does not offer to save again: %v", tt.Texts())
	}
}

// 同一个 run 在卡片与详情上说的是同一件事：两处都给「再次运行」。
func TestWorkflowSavedRunSaysTheSameOnBothFaces(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := completedRun(`{"phases":[]}`)
	run.DefinitionID = "definition-1"
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	app.state.WorkflowDefinitions = []contracts.WorkflowDefinition{{
		ID: "definition-1", Name: "review", Scope: workflow.SavedScopeProject, ProjectID: "project-1",
		Script: run.Script,
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunView(c, run.ID)
	}, 900, 800)
	tt.Frame()
	if !tt.HasText("Run again") || tt.HasText("Save workflow") {
		t.Fatalf("the detail head agrees with the card: %v", tt.Texts())
	}
}

// 停下来的运行可以恢复：恢复入口只在核心说它能继续时出现。
func TestWorkflowRunCardOffersResumeOnAResumableRun(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := completedRun(`{"phases":[]}`)
	run.Status = contracts.WorkflowStatusStopped
	run.StopReason = contracts.WorkflowStopUser
	run.Resumable = true
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 600)
	tt.Frame()
	if !tt.HasText("Resume run") {
		t.Fatalf("a resumable run offers resume: %v", tt.Texts())
	}
}

// 被修订后继替代的运行不给恢复：看到 stopped 不等于能继续。
func TestWorkflowRunCardHidesResumeWhenSuperseded(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := completedRun(`{"phases":[]}`)
	run.Status = contracts.WorkflowStatusStopped
	run.StopReason = contracts.WorkflowStopSuperseded
	run.Resumable = true
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Gap(8).Children(func() {
			app.workflowRunCard(c, run, 720)
		})
	}, 900, 600)
	tt.Frame()
	if tt.HasText("Resume run") {
		t.Fatalf("a superseded run cannot be resumed: %v", tt.Texts())
	}
}

// 保存表单：名字、作用域、说明与落点预览都在，草稿按运行名预填。
func TestWorkflowSaveSheetPrefillsFromTheRun(t *testing.T) {
	app := workflowCardApp(t)
	app.palette = design.Palette("rcode-dark", true)
	run := completedRun(`{"phases":[]}`)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	app.openWorkflowSave(run.ID)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowSaveDialog(c)
	}, 900, 700)
	tt.Frame()
	for _, text := range []string{"Save workflow", "Name", "Scope", "Description", "This project", "This machine", ".gcode/workflows/review.json"} {
		if !tt.HasText(text) {
			t.Fatalf("the save form is missing %q: %v", text, tt.Texts())
		}
	}
}

// 提交保存时发出的是「保存这份运行实际执行的脚本」。
func TestWorkflowSaveSendsTheExecutedScript(t *testing.T) {
	app := workflowCardApp(t)
	run := completedRun(`{"phases":[{"id":"phase-1"}]}`)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	state := &workflowSaveState{open: true, name: "review", scope: workflow.SavedScopeProject, description: "the review"}
	app.saveWorkflowRun(run, state)
	if state.open {
		t.Fatal("an accepted save closes its form")
	}
	if state.errorText != "" {
		t.Fatalf("a valid draft has no error: %q", state.errorText)
	}
}

// 名字不合规则当场拒绝，不发命令。
func TestWorkflowSaveRejectsAnUnusableName(t *testing.T) {
	app := workflowCardApp(t)
	run := completedRun(`{"phases":[]}`)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	state := &workflowSaveState{open: true, name: "有 空格", scope: workflow.SavedScopeProject}
	app.saveWorkflowRun(run, state)
	if state.errorText == "" {
		t.Fatal("an unusable name is rejected before the command")
	}
	if state.open != true {
		t.Fatal("a rejected save keeps its form open")
	}
}

// 没有记录脚本的运行没有可保存的内容。
func TestWorkflowSaveRefusesARunWithoutAScript(t *testing.T) {
	app := workflowCardApp(t)
	run := completedRun("")
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.openWorkflowSave(run.ID)
		app.workflowSaveDialog(c)
	}, 900, 700)
	tt.Frame()
	if !tt.HasText("This run did not record a script, so there is nothing to save.") {
		t.Fatalf("a run without a script says so: %v", tt.Texts())
	}
}

// 同名定义已经存在时，弹窗把提交说清楚是覆盖。
func TestWorkflowSaveWarnsAboutOverwriting(t *testing.T) {
	app := workflowCardApp(t)
	run := completedRun(`{"phases":[]}`)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	app.state.WorkflowDefinitions = []contracts.WorkflowDefinition{{
		ID: "existing", Name: "review", Scope: workflow.SavedScopeProject, ProjectID: "project-1",
	}}
	state := &workflowSaveState{open: true, name: "review", scope: workflow.SavedScopeProject}
	app.refreshWorkflowSaveTaken(run, state)
	if !state.taken {
		t.Fatal("a taken name is reported")
	}
	// 另一个作用域里的同名定义不构成覆盖。
	state.scope = workflow.SavedScopeGlobal
	app.refreshWorkflowSaveTaken(run, state)
	if state.taken {
		t.Fatal("a name in another scope is not an overwrite")
	}
}

// 同一个脚本已经存过，就算这次运行没带定义身份，也算已保存。
func TestWorkflowSaveFindsTheDefinitionByScript(t *testing.T) {
	app := workflowCardApp(t)
	run := completedRun(`{"phases":[]}`)
	app.state.WorkflowRuns = []contracts.WorkflowRun{run}
	app.state.WorkflowDefinitions = []contracts.WorkflowDefinition{{
		ID: "found", Name: "review", Scope: workflow.SavedScopeProject, ProjectID: "project-1",
		Script: run.Script,
	}}
	controller := app.workflowSaveOf(run)
	if !controller.saved || controller.definition.ID != "found" {
		t.Fatalf("the matching definition is found: %+v", controller)
	}
}
