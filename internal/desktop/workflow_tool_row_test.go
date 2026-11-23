package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
)

// 工作流工具行说明这一次工具动作：标题按工具区分，详情不复制整段脚本，
// 运行图由运行卡与详情承担。
func TestWorkflowToolRowsSummarizeTheActionNotTheRun(t *testing.T) {
	script := `{"name":"Review","phases":[{"id":"a","instances":[{"id":"reader"}]}]}`
	cases := []struct{ name, label string }{
		{"CreateWorkflow", "Start workflow"},
		{"AmendWorkflow", "Amend workflow"},
		{"SaveWorkflow", "Save workflow"},
		{"ListSavedWorkflows", "List saved workflows"},
		{"EvalWorkflowSnippet", "Check workflow script"},
		{"ListWorkflowRuns", "List workflow runs"},
		{"GetWorkflowRun", "Read workflow run"},
		{"ResumeWorkflowRun", "Resume workflow run"},
		{"ResolveWorkflowQuestion", "Answer workflow question"},
		{"FillWorkflowHole", "Fill workflow hole"},
	}
	a := testApp(t, "default")
	a.palette = design.Palette("rcode-light", false)
	for _, test := range cases {
		block := contracts.Block{Kind: "tool", Tool: &contracts.ToolCall{
			Kind: contracts.ToolWorkflow, Name: test.name, Summary: "Review", Args: `{"script":` + script + `}`,
		}}
		label, detail, glyph := activityTitle(block)
		if label != test.label || detail != "Review" || glyph != "git-branch" {
			t.Fatalf("%s: label=%q detail=%q glyph=%q", test.name, label, detail, glyph)
		}
		if i18n.In("zh-CN", label) == label {
			t.Fatalf("%s has no Chinese label", label)
		}
		source := readableDetailSource(nil, block, false)
		if strings.Contains(source.args, "phases") || strings.Contains(source.command, "phases") {
			t.Fatalf("%s: the workflow script leaked into the tool row: %+v", test.name, source)
		}
	}
}

// 分类只在保存的工具缺 Kind 时才用到：老记录也要认得出工作流工具。
func TestWorkflowToolNamesMapToTheirKind(t *testing.T) {
	for _, name := range []string{
		"CreateWorkflow", "AmendWorkflow", "SaveWorkflow", "ListSavedWorkflows", "EvalWorkflowSnippet",
		"ListWorkflowRuns", "GetWorkflowRun", "ResumeWorkflowRun", "ResolveWorkflowQuestion", "FillWorkflowHole",
	} {
		if got := (&contracts.ToolCall{Name: name}).Category(); got != contracts.ToolWorkflow {
			t.Fatalf("%s maps to %q", name, got)
		}
	}
	if got := (&contracts.ToolCall{Name: "Workflow"}).Category(); got == contracts.ToolWorkflow {
		t.Fatal("the legacy Workflow tool is not this tool family")
	}
}
