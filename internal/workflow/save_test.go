package workflow

import (
	"testing"

	"gcode/internal/contracts"
)

func TestValidSavedNameFollowsTheFileNameRule(t *testing.T) {
	for _, name := range []string{"review", "review.v2", "a_b-c", "A1"} {
		if !ValidSavedName(name) {
			t.Fatalf("%q should be a valid name", name)
		}
	}
	for _, name := range []string{"", "has space", "has/slash", "..", ".", "名了字", string(make([]byte, 65))} {
		if ValidSavedName(name) {
			t.Fatalf("%q should be rejected", name)
		}
	}
}

func TestSavedPathPreviewNamesThePlace(t *testing.T) {
	if got := SavedPathPreview(SavedScopeProject, "review"); got != ".gcode/workflows/review.json" {
		t.Fatalf("project preview: %q", got)
	}
	if got := SavedPathPreview(SavedScopeGlobal, "review"); got != "~/.gcode/workflows/review.json" {
		t.Fatalf("global preview: %q", got)
	}
}

func TestInitialSaveDraftPrefillsOnlyAUsableName(t *testing.T) {
	draft := InitialSaveDraft("review", SavedScopeProject)
	if draft.Name != "review" || draft.Scope != SavedScopeProject || draft.Description != "review" {
		t.Fatalf("a usable run name fills both fields: %+v", draft)
	}
	// 模型起的中文名不是文件名：它只进说明，不进名字框。
	draft = InitialSaveDraft("PR 分层评审", SavedScopeProject)
	if draft.Name != "" {
		t.Fatalf("an unusable name is not prefilled: %+v", draft)
	}
	if draft.Description != "PR 分层评审" {
		t.Fatalf("the run name still says what the workflow does: %+v", draft)
	}
}

func TestSavedDefinitionKeepsTheExecutedScript(t *testing.T) {
	run := contracts.WorkflowRun{
		ID: "run-1", Name: "Review", ProjectID: "project-1", Script: `{"phases":[]}`,
		Phases: []contracts.WorkflowPhase{{ID: "phase-1", Name: "Plan", Index: 0}},
		Actors: []contracts.WorkflowActor{{ID: "actor-1", PhaseID: "phase-1", Name: "Planner"}},
		Edges:  []contracts.WorkflowEdge{{From: "phase-1", To: "phase-1", Kind: "loop"}},
	}
	definition := SavedDefinition(run, SaveDraft{Name: "review", Scope: SavedScopeProject, Description: "the review"})
	if definition.Script != run.Script {
		t.Fatalf("the saved definition carries the script that ran: %q", definition.Script)
	}
	if definition.ProjectID != "project-1" || definition.Scope != SavedScopeProject {
		t.Fatalf("a project definition keeps its project: %+v", definition)
	}
	if len(definition.Program.Phases) != 1 || definition.Program.Phases[0].ID != "phase-1" {
		t.Fatalf("the program follows the run's phases: %+v", definition.Program)
	}
	if len(definition.Program.Phases[0].Instances) != 1 || definition.Program.Phases[0].Instances[0].ID != "actor-1" {
		t.Fatalf("the program keeps its instances: %+v", definition.Program.Phases[0])
	}
}

func TestSavedByScriptLooksForTheSameScript(t *testing.T) {
	run := contracts.WorkflowRun{ID: "run-1", Script: "same"}
	definitions := []contracts.WorkflowDefinition{
		{ID: "global-1", Name: "global-copy", Scope: SavedScopeGlobal, Script: "same"},
		{ID: "project-1", Name: "project-copy", Scope: SavedScopeProject, Script: "same"},
	}
	definition, ok := SavedByScript(run, definitions, 50)
	if !ok || definition.ID != "project-1" {
		t.Fatalf("the project definition wins: %+v %v", definition, ok)
	}
	// 没有脚本的运行不能被认成任何一份定义。
	empty := contracts.WorkflowRun{ID: "run-2"}
	if _, ok := SavedByScript(empty, definitions, 50); ok {
		t.Fatal("a run without a script matches nothing")
	}
}

func TestLaunchRequestCarriesTheRunsArgs(t *testing.T) {
	run := contracts.WorkflowRun{
		ID: "run-1", Script: "same",
		Args: []contracts.WorkflowArgValue{{Name: "target", Kind: "string", Text: "src"}},
	}
	definition := contracts.WorkflowDefinition{ID: "definition-1", Name: "review", ProjectID: "project-1"}
	request := LaunchRequest(run, definition, "")
	if request.DefinitionID != "definition-1" {
		t.Fatalf("the launch names the saved definition: %+v", request)
	}
	if len(request.Args) != 1 || request.Args[0].Text != "src" {
		t.Fatalf("the launch carries this run's args: %+v", request.Args)
	}
	if request.Origin != contracts.WorkflowOriginRerun {
		t.Fatalf("a rerun says where it came from: %q", request.Origin)
	}
	if request.Program.Phases != nil {
		t.Fatal("the launch lets the core take the program from the definition")
	}
}
