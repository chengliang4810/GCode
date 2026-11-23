package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// workflowConfirmationFixture 是一次等待决定的运行确认。
func workflowConfirmationFixture() contracts.WorkflowConfirmation {
	return contracts.WorkflowConfirmation{
		ID: "confirm-1", SessionID: "chat-0", Name: "Review the change",
		Question: "Run this workflow?", Status: contracts.WorkflowConfirmationPending,
		Concurrency: 2, Script: `{"name":"Review"}`,
		Program: contracts.WorkflowProgram{Phases: []contracts.WorkflowPhaseSpec{
			{ID: "phase-1", Name: "Plan", Instances: []contracts.WorkflowActorSpec{{ID: "planner", Name: "Planner"}}},
			{ID: "phase-2", Name: "Build", Instances: []contracts.WorkflowActorSpec{{ID: "builder", Name: "Builder"}}},
		}},
		Args: []contracts.WorkflowArgValue{{Name: "target", Kind: "string", Text: "the diff"}},
		Diagnostics: []contracts.WorkflowDiagnostic{
			{Severity: contracts.WorkflowDiagnosticWarning, Code: "unknown_field", Message: `Unknown field "budget" is ignored.`},
		},
	}
}

// workflowConfirmationApp builds an app that waits for one confirmation.
func workflowConfirmationApp(t *testing.T) *App {
	t.Helper()
	app := testApp(t, "default")
	app.state.WorkflowConfirmations = []contracts.WorkflowConfirmation{workflowConfirmationFixture()}
	return app
}

func TestWorkflowConfirmationDialogShowsTheFactsBeforeTheRun(t *testing.T) {
	app := workflowConfirmationApp(t)
	app.palette = design.Palette("rcode-dark", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowConfirmationDialog(c)
	}, 1100, 900)
	tt.Frame()
	for _, text := range []string{
		"Run workflow", "Review the change", "Run this workflow?",
		"2 phases, 2 instances", "Arguments", "target (string): the diff",
		"Diagnostics", `Unknown field "budget" is ignored.`,
		"Show script", "Allow once", "Allow for this session", "Reject",
	} {
		if !tt.HasText(text) {
			t.Fatalf("the confirmation is missing %q: %v", text, tt.Texts())
		}
	}
	// The script stays folded until the reader asks for it.
	if tt.HasText(`{"name":"Review"}`) {
		t.Fatalf("the script is folded by default: %v", tt.Texts())
	}
	if err := tt.Click("Show script"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(`{"name":"Review"}`) {
		t.Fatalf("the script opens on request: %v", tt.Texts())
	}
}

func TestWorkflowConfirmationDialogIgnoresSettledRequests(t *testing.T) {
	app := testApp(t, "default")
	confirmation := workflowConfirmationFixture()
	confirmation.Status = contracts.WorkflowConfirmationApproved
	app.state.WorkflowConfirmations = []contracts.WorkflowConfirmation{confirmation}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowConfirmationDialog(c)
	}, 1100, 900)
	tt.Frame()
	if tt.HasText("Allow once") {
		t.Fatalf("a settled confirmation asks nothing: %v", tt.Texts())
	}
}
