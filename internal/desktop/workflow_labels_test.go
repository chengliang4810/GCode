package desktop

import (
	"strings"
	"testing"

	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// 没写名字的阶段与实例用序号称呼：内部 id 不当名字。
func TestWorkflowLabelsFallBackToOrdinals(t *testing.T) {
	var phase, station, actor, named string
	ui.NewTester(func(c *ui.Context) {
		phase = workflowPhaseLabel(c, workflow.RunPhaseView{Name: "", Index: 1})
		station = workflowStationLabel(c, workflow.TimelineStation{Name: "", PhaseOrdinal: 2})
		actor = workflowActorLabel(c, 3, "")
		named = workflowActorLabel(c, 3, "Reviewer")
	}, 1, 1)
	if phase != "Phase 2" || station != "Phase 2" || actor != "Agent 3" {
		t.Fatalf("ordinal labels: phase=%q station=%q actor=%q", phase, station, actor)
	}
	if named != "Reviewer" {
		t.Fatalf("a written name must win over the ordinal: %q", named)
	}
}

// 运行详情面板：没有名字的运行说状态，阶段与实例说序号，面板里不出现 id。
func TestWorkflowRunPaneNamesUnnamedRunsByState(t *testing.T) {
	app := workflowCardApp(t)
	run := &app.state.WorkflowRuns[0]
	run.Name = ""
	run.Phases[0].Name, run.Phases[1].Name = "", ""
	run.Actors[1].Name, run.Actors[2].Name = "", ""
	run.Holes[0].Title = ""
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowRunView(c, run.ID)
	}, 900, 900)
	tt.Frame()
	for _, text := range []string{"Running", "Phase 1", "Phase 2", "Planner", "Agent 1", "Agent 2"} {
		if !tt.HasText(text) {
			t.Fatalf("panel is missing %q: %v", text, tt.Texts())
		}
	}
	for _, text := range tt.Texts() {
		for _, id := range []string{"workflow-run-1", "phase-1", "phase-2", "actor-2", "actor-3"} {
			if strings.Contains(text, id) {
				t.Fatalf("panel named something by its id: %q", text)
			}
		}
	}
}

// 运行目录：没有名字的运行用状态词作标题，编号不进列表。
func TestWorkflowRunDirectoryNamesUnnamedRunsByState(t *testing.T) {
	app := directoryApp(t)
	app.state.WorkflowRuns[0].Name = ""
	tt := directoryTester(app)
	tt.Frame()
	if tt.HasText("run-live") {
		t.Fatalf("directory named a run by its id: %v", tt.Texts())
	}
	if !tt.HasText("Running") {
		t.Fatalf("directory lost the run state: %v", tt.Texts())
	}
}
