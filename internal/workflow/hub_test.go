package workflow

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

func hubDefinition(id, name, scope, projectID string, updated time.Time) contracts.WorkflowDefinition {
	return contracts.WorkflowDefinition{ID: id, Name: name, Scope: scope, ProjectID: projectID, UpdatedAt: updated}
}

func TestGroupDefinitionsSeparatesScopesAndKeepsTheCurrentProjectFirst(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	definitions := []contracts.WorkflowDefinition{
		hubDefinition("d-global", "Nightly", SavedScopeGlobal, "", now),
		hubDefinition("d-b", "Older", SavedScopeProject, "proj-b", now.Add(-time.Hour)),
		hubDefinition("d-a2", "Newer", SavedScopeProject, "proj-a", now),
		hubDefinition("d-a1", "Earlier", SavedScopeProject, "proj-a", now.Add(-2*time.Hour)),
		hubDefinition("d-gone", "Orphan", SavedScopeProject, "proj-gone", now),
	}
	projects := []ProjectRef{
		{ID: "proj-a", Name: "Alpha"},
		{ID: "proj-b", Name: "Beta"},
	}
	groups := GroupDefinitions(definitions, projects, "proj-b")
	if groups.Count() != 5 {
		t.Fatalf("every definition is in exactly one group, got %d", groups.Count())
	}
	if len(groups.Projects) != 3 {
		t.Fatalf("expected three project groups, got %+v", groups.Projects)
	}
	if groups.Projects[0].ProjectID != "proj-b" || !groups.Projects[0].Current {
		t.Fatalf("the current project comes first: %+v", groups.Projects[0])
	}
	if groups.Projects[1].ProjectID != "proj-a" || groups.Projects[1].Current {
		t.Fatalf("the other project follows without claiming to be current: %+v", groups.Projects[1])
	}
	alpha := groups.Projects[1]
	if alpha.Definitions[0].Name != "Newer" || alpha.Definitions[1].Name != "Earlier" {
		t.Fatalf("definitions are newest first: %+v", alpha.Definitions)
	}
	orphan := groups.Projects[2]
	if !orphan.Gone || orphan.Name != "proj-gone" {
		t.Fatalf("a definition whose project is gone is still listed: %+v", orphan)
	}
	if len(groups.Global) != 1 || groups.Global[0].Name != "Nightly" {
		t.Fatalf("global definitions have their own group: %+v", groups.Global)
	}
}

func TestGroupDefinitionsKeepsTheRemoteIdentity(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	groups := GroupDefinitions(
		[]contracts.WorkflowDefinition{hubDefinition("d-1", "Remote run", SavedScopeProject, "proj-cloud", now)},
		[]ProjectRef{{ID: "proj-cloud", Name: "Cloud box", Remote: true}},
		"proj-cloud",
	)
	if len(groups.Projects) != 1 || !groups.Projects[0].Remote {
		t.Fatalf("a remote project keeps its host identity: %+v", groups.Projects)
	}
}

func TestDefinitionRunsAreNewestFirstAndBounded(t *testing.T) {
	base := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	runs := []contracts.WorkflowRun{
		{ID: "run-old", DefinitionID: "d-1", StartedAt: base},
		{ID: "run-other", DefinitionID: "d-2", StartedAt: base.Add(time.Hour)},
		{ID: "run-new", DefinitionID: "d-1", StartedAt: base.Add(2 * time.Hour)},
	}
	history := DefinitionRuns("d-1", runs, 0)
	if len(history) != 2 || history[0].ID != "run-new" || history[1].ID != "run-old" {
		t.Fatalf("history is this definition's runs, newest first: %+v", history)
	}
	if bounded := DefinitionRuns("d-1", runs, 1); len(bounded) != 1 || bounded[0].ID != "run-new" {
		t.Fatalf("history is bounded: %+v", bounded)
	}
	if last, ok := DefinitionLastRun("d-1", runs); !ok || last.ID != "run-new" {
		t.Fatalf("the last run is the newest one: %+v", last)
	}
	if _, ok := DefinitionLastRun("d-missing", runs); ok {
		t.Fatal("a definition that never ran has no last run")
	}
}

func TestDefinitionArtifactsComeFromTheLastRunThatDelivered(t *testing.T) {
	base := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	runs := []contracts.WorkflowRun{
		{
			ID: "run-failed", DefinitionID: "d-1", StartedAt: base.Add(3 * time.Hour),
			Status: contracts.WorkflowStatusErrored,
		},
		{
			ID: "run-delivered", DefinitionID: "d-1", StartedAt: base,
			Status: contracts.WorkflowStatusCompleted,
			Artifacts: []contracts.WorkflowArtifact{
				{ID: "a-1", Name: "report", Content: "the report", Version: 1, Order: 0},
			},
		},
	}
	slice := DefinitionArtifacts("d-1", runs)
	if len(slice.Shown) != 1 || slice.Shown[0].Name != "report" {
		t.Fatalf("a newer failure does not hide what an earlier run delivered: %+v", slice)
	}
	if empty := DefinitionArtifacts("d-1", []contracts.WorkflowRun{{ID: "r", DefinitionID: "d-1", Status: contracts.WorkflowStatusCompleted}}); len(empty.Shown) != 0 {
		t.Fatalf("a run without artifacts delivers nothing: %+v", empty)
	}
}

func TestArgumentPreviewIsBounded(t *testing.T) {
	arguments := []contracts.WorkflowArgument{
		{Name: "target"}, {Name: "reason"}, {Name: "depth"}, {Name: "scope"}, {Name: "extra"},
	}
	shown, overflow := ArgumentPreview(arguments, 3)
	if len(shown) != 3 || shown[0] != "target" || overflow != 2 {
		t.Fatalf("a card shows three names and counts the rest: %+v %d", shown, overflow)
	}
	if shown, overflow := ArgumentPreview(arguments[:2], 3); len(shown) != 2 || overflow != 0 {
		t.Fatalf("a short declaration has no overflow: %+v %d", shown, overflow)
	}
}

func TestLaunchPrefillKeepsOnlyWhatIsStillDeclared(t *testing.T) {
	definition := contracts.WorkflowDefinition{
		Arguments: []contracts.WorkflowArgument{
			{Name: "target", Type: "string"},
			{Name: "depth", Type: "number", Default: "2", HasDefault: true},
			{Name: "fresh", Type: "boolean"},
		},
	}
	run := &contracts.WorkflowRun{Args: []contracts.WorkflowArgValue{
		{Name: "target", Kind: "string", Text: "src"},
		{Name: "depth", Kind: "number", Text: "5"},
		{Name: "removed", Kind: "string", Text: "gone"},
	}}
	values := LaunchPrefill(definition, run)
	if values["target"] != "src" || values["depth"] != "5" {
		t.Fatalf("the run's own values come back: %+v", values)
	}
	if _, ok := values["removed"]; ok {
		t.Fatalf("a key the definition dropped is not carried over: %+v", values)
	}
	if _, ok := values["fresh"]; !ok {
		t.Fatalf("every declared argument has a starting value: %+v", values)
	}
	defaults := LaunchPrefill(definition, nil)
	if defaults["depth"] != "2" || defaults["target"] != "" {
		t.Fatalf("without a run the declaration's defaults stand in: %+v", defaults)
	}
}

func TestLaunchArgsOmitsEmptyValuesAndKeepsBooleansExplicit(t *testing.T) {
	definition := contracts.WorkflowDefinition{
		Arguments: []contracts.WorkflowArgument{
			{Name: "target", Type: "string"},
			{Name: "note", Type: "string"},
			{Name: "deep", Type: "boolean"},
			{Name: "extra", Type: "json"},
		},
	}
	args, issue := LaunchArgs(definition, map[string]string{"target": "src", "note": "  ", "deep": "false"})
	if issue != nil {
		t.Fatalf("a valid form launches: %+v", issue)
	}
	if len(args) != 2 {
		t.Fatalf("empty values are not sent: %+v", args)
	}
	if args[0].Name != "target" || args[0].Text != "src" {
		t.Fatalf("declaration order holds: %+v", args)
	}
	if args[1].Name != "deep" || args[1].Text != "false" {
		t.Fatalf("a boolean is sent as an explicit value: %+v", args)
	}
}

func TestLaunchArgsRefusesWhatTheCoreWouldRefuse(t *testing.T) {
	definition := contracts.WorkflowDefinition{
		Arguments: []contracts.WorkflowArgument{
			{Name: "target", Type: "string", Required: true},
			{Name: "depth", Type: "number"},
			{Name: "meta", Type: "json"},
			{Name: "deep", Type: "boolean"},
		},
	}
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"a required value is missing", map[string]string{}, "target"},
		{"a number must be finite", map[string]string{"target": "src", "depth": "1e999"}, "depth"},
		{"a number must parse", map[string]string{"target": "src", "depth": "two"}, "depth"},
		{"JSON must parse", map[string]string{"target": "src", "meta": "{"}, "meta"},
		{"an unknown argument is refused", map[string]string{"target": "src", "bogus": "x"}, "bogus"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, issue := LaunchArgs(definition, test.values)
			if issue == nil || issue.Name != test.want {
				t.Fatalf("expected the issue on %q, got %+v", test.want, issue)
			}
		})
	}
	if _, issue := LaunchArgs(definition, map[string]string{"target": "src", "meta": `{"a":1}`}); issue != nil {
		t.Fatalf("valid JSON is accepted: %+v", issue)
	}
}

func TestLaunchArgsUsesDeclaredDefaults(t *testing.T) {
	definition := contracts.WorkflowDefinition{
		Arguments: []contracts.WorkflowArgument{
			{Name: "depth", Type: "number", Default: "3", HasDefault: true},
			{Name: "deep", Type: "boolean", Default: "true", HasDefault: true},
		},
	}
	args, issue := LaunchArgs(definition, map[string]string{})
	if issue != nil {
		t.Fatalf("defaults satisfy the declaration: %+v", issue)
	}
	if len(args) != 2 || args[0].Text != "3" || args[1].Text != "true" {
		t.Fatalf("a declaration's default stands in for an empty value: %+v", args)
	}
}
