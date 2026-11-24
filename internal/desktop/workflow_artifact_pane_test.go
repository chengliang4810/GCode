package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestWorkflowArtifactPaneRendersMarkdown(t *testing.T) {
	a := workflowCardApp(t)
	run := &a.state.WorkflowRuns[0]
	run.Artifacts = []contracts.WorkflowArtifact{{
		ID: "report", Name: "Report.md",
		Content: "# Overview\n\n| Topic | Status |\n| --- | --- |\n| Review | Ready |",
	}}
	key := run.ID + "/" + run.Artifacts[0].Name
	tt := ui.NewTester(func(c *ui.Context) { a.workflowArtifactView(c, key) }, 640, 480)
	tt.Frame()
	for _, label := range []string{"Report.md", "Overview", "Topic", "Ready"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing rendered %q: %v", label, tt.Texts())
		}
	}
	if tt.HasText("# Overview") {
		t.Fatal("Markdown was shown as source")
	}
}
