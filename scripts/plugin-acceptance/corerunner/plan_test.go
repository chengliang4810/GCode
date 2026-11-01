package corerunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/scripts/plugin-acceptance/acceptance"
)

func TestMissingPlanNeverStartsRuntime(t *testing.T) {
	request := acceptance.RunRequest{RunID: "fixture", PluginID: "demo@market", SourceCommit: strings.Repeat("a", 40), Components: []acceptance.Component{{ID: "skill:demo:run", Kind: "skill", Digest: "digest"}}}
	evidence, err := (InProcessRunner{}).Run(context.Background(), request)
	if err != nil || len(evidence.Probes) != 1 || evidence.Probes[0].Outcome != "blocked" {
		t.Fatalf("missing explicit probe plan: %+v %v", evidence, err)
	}
}

func TestPlanRequiresEveryComponentBusinessAssertionsAndHookApproval(t *testing.T) {
	request := acceptance.RunRequest{PluginID: "demo@market", SourceCommit: strings.Repeat("a", 40), ProbeDir: t.TempDir(), Components: []acceptance.Component{{ID: "skill:demo:run", Kind: "skill"}, {ID: "hook:Stop/0/0", Kind: "hook"}}}
	for _, tc := range []struct {
		name string
		plan Plan
	}{
		{"missing component", Plan{PluginID: request.PluginID, SourceCommit: request.SourceCommit, Probes: []ProbePlan{{ID: "skill", ComponentID: request.Components[0].ID, Prompt: "task", Assertions: []Assertion{{Kind: "text", Contains: "business output"}}}}}},
		{"no assertion", Plan{PluginID: request.PluginID, SourceCommit: request.SourceCommit, Probes: []ProbePlan{{ID: "skill", ComponentID: request.Components[0].ID, Prompt: "say pass"}}}},
		{"no hook approval", Plan{PluginID: request.PluginID, SourceCommit: request.SourceCommit, Probes: []ProbePlan{{ID: "skill", ComponentID: request.Components[0].ID, Prompt: "task", Assertions: []Assertion{{Kind: "text", Contains: "business output"}}}, {ID: "hook", ComponentID: request.Components[1].ID, Prompt: "task", Assertions: []Assertion{{Kind: "text", Contains: "hook result"}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := saveJSON(filepath.Join(request.ProbeDir, request.PluginID+".json"), tc.plan); err != nil {
				t.Fatal(err)
			}
			if _, err := readPlan(request); err == nil {
				t.Fatal("incomplete or unapproved plan accepted")
			}
		})
	}
}

func TestProbePathsStayInsidePrivateWorkspace(t *testing.T) {
	for _, path := range []string{"../outside", "/outside", "C:/outside", `..\outside`} {
		if _, err := workspacePath(t.TempDir(), path); err == nil {
			t.Fatalf("unsafe fixture/assertion path %q accepted", path)
		}
	}
}

func TestProbePlanRejectsTrailingDocuments(t *testing.T) {
	request := acceptance.RunRequest{PluginID: "demo@market", SourceCommit: strings.Repeat("a", 40), ProbeDir: t.TempDir()}
	path := filepath.Join(request.ProbeDir, request.PluginID+".json")
	data := `{"pluginId":"demo@market","sourceCommit":"` + request.SourceCommit + `","probes":[]} {}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPlan(request); err == nil {
		t.Fatal("multiple JSON documents accepted")
	}
}

func TestPrivateEnvironmentRestoresAndHidesAmbientCredentials(t *testing.T) {
	t.Setenv("ACCEPTANCE_AMBIENT_SECRET", "must-not-inherit")
	before, _ := os.UserHomeDir()
	home := t.TempDir()
	restore := privateEnvironment(home)
	if os.Getenv("ACCEPTANCE_AMBIENT_SECRET") != "" {
		restore()
		t.Fatal("ambient personal credential reached probe environment")
	}
	current, _ := os.UserHomeDir()
	restore()
	if current != home || os.Getenv("ACCEPTANCE_AMBIENT_SECRET") != "must-not-inherit" {
		t.Fatal("probe home isolation or environment restore failed")
	}
	after, _ := os.UserHomeDir()
	if before != after {
		t.Fatal("user home was changed after probe")
	}
}
