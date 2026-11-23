package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
)

// 工作流命令入口。关闭模式连名字都不出现；内置名字是保留字，同名用户
// 命令既不列出也不展开。
func TestComposerOffersTheWorkflowCommandUnlessWorkflowsAreOff(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.WorkflowMode = contracts.WorkflowModeOnDemand
	a.state.UserResources.Commands = []contracts.UserCommand{
		{Name: "workflow", Description: "A user command with the same name", Enabled: true},
		{Name: "review", Description: "Review the change", Enabled: true},
	}
	choices := a.promptChoices("/", "")
	found := 0
	for _, choice := range choices {
		if choice.ref.Kind != "command" {
			continue
		}
		if choice.ref.ID == "workflow" {
			found++
			if choice.ref.Label != "Workflow" {
				t.Fatalf("the workflow command keeps its label: %q", choice.ref.Label)
			}
			if choice.description == "" {
				t.Fatal("the workflow command explains itself")
			}
		}
		if choice.ref.ID == "review" && choice.description != "Review the change" {
			t.Fatalf("user commands keep their description: %q", choice.description)
		}
	}
	if found != 1 {
		t.Fatalf("the built-in command is listed once: %+v", choices)
	}
	if promptGlyph(contracts.PromptReference{Kind: "command", ID: "workflow"}) != "git-branch" {
		t.Fatal("the workflow command has its own glyph")
	}
	a.state.Settings.WorkflowMode = contracts.WorkflowModeDisabled
	for _, choice := range a.promptChoices("/", "") {
		if choice.ref.Kind == "command" && choice.ref.ID == "workflow" {
			t.Fatal("the disabled mode hides the workflow command")
		}
	}
	if len(a.promptChoices("/", "workflow")) != 0 {
		t.Fatal("a user command cannot take the reserved name")
	}
}

// 按需模式也要给出命令：命令是用户打开能力的入口，不是能力本身。
func TestComposerKeepsTheWorkflowCommandInOnDemandMode(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.WorkflowMode = contracts.WorkflowModeOnDemand
	choices := a.promptChoices("/", "workflow")
	if len(choices) != 1 || choices[0].ref.ID != "workflow" {
		t.Fatalf("the on-demand mode still offers the command: %+v", choices)
	}
	if !strings.Contains(choices[0].description, "workflow") {
		t.Fatalf("the command explains itself: %q", choices[0].description)
	}
}
