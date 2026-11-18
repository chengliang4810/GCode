package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestAgentsProjectSkillDiscoveredAndInvoked(t *testing.T) {
	const (
		description  = "Description of repository skill"
		instructions = "PROJECT SKILL INSTRUCTIONS"
	)
	var requests atomic.Int32
	core, cwd := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		messages, err := json.Marshal(body["messages"])
		if err != nil {
			t.Error(err)
		}
		text := string(messages)
		switch requests.Add(1) {
		case 1:
			if !strings.Contains(text, "repo-review: "+description) || strings.Contains(text, instructions) {
				t.Error("initial prompt should advertise the project skill without including its body")
			}
			subagentWireCall(w, model.Chat, "Skill", `{"skill":"repo-review","args":"current changes"}`)
		case 2:
			if !strings.Contains(text, instructions+" current changes") || strings.Contains(text, "skill not found") {
				t.Error("Skill call did not return project instructions with arguments")
			}
			subagentText(w, model.Chat, "Completed")
		default:
			t.Error("unexpected extra model request")
			subagentText(w, model.Chat, "Completed")
		}
	}))
	writePluginFile(t, cwd, ".agents/skills/example/SKILL.md",
		"---\nname: repo-review\ndescription: "+description+"\n---\n"+instructions+" $ARGUMENTS")
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: state.Projects[0].ID, ModelID: "goal/model", Text: "Use the repository skill.",
		},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	if requests.Load() != 2 || len(state.Runs) != 1 || state.Runs[0].Status != "completed" {
		t.Fatalf("unexpected run result: requests=%d runs=%+v", requests.Load(), state.Runs)
	}
}

func TestAgentsProjectSkillResolution(t *testing.T) {
	root, cwd := t.TempDir(), t.TempDir()
	catalog := userResourceCatalog{Commands: []contracts.UserCommand{{
		ID: "repo-review", Name: "repo-review", Content: "PERSONAL COMMAND", Enabled: true,
	}}}
	if err := storage.WriteJSON(filepath.Join(root, resourceCatalogFile), catalog); err != nil {
		t.Fatal(err)
	}
	writePluginFile(t, cwd, ".agents/skills/repo-review/SKILL.md", "AGENTS SKILL $ARGUMENTS")
	writePluginFile(t, cwd, ".agents/skills/shared/SKILL.md", "AGENTS SHARED")
	writePluginFile(t, cwd, ".claude/skills/shared/SKILL.md", "CLAUDE SHARED")
	writePluginFile(t, cwd, ".agents/skills/manual/SKILL.md", "---\ndisable-model-invocation: true\n---\nMANUAL ONLY")
	writePluginFile(t, cwd, ".agents/commands/not-a-command.md", "DO NOT REGISTER")
	resources, err := buildRuntimeResources(root, t.TempDir(), cwd, "test", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resources.skills["shared"].component.Content != "CLAUDE SHARED" {
		t.Fatal("existing .claude skill must win a duplicate project name")
	}
	if _, registered := resources.skills["not-a-command"]; registered {
		t.Fatal(".agents/commands is outside the project skill source")
	}
	if _, registered := resources.skills["manual"]; !registered || strings.Contains(resources.instructions(), "\n- manual:") {
		t.Fatal("manual-only skill must be registered but not advertised to the model")
	}
	const invocation = "/repo-review current changes"
	prepared, err := expandPrompt(root, cwd, invocation)
	if err != nil || prepared != invocation {
		t.Fatalf("project invocation was prematurely expanded: %q (%v)", prepared, err)
	}
	input, err := (&Engine{}).prepareRuntimeInput(
		context.Background(),
		model.Message{Role: "user", Text: prepared, SourceText: invocation},
		contracts.Session{ID: "test", CWD: cwd}, &activeRun{id: "test-run"}, resources, nil, nil,
	)
	if err != nil || !strings.Contains(input.Text, "AGENTS SKILL current changes") {
		t.Fatalf("slash invocation = %q (%v)", input.Text, err)
	}
}

func TestAgentsProjectSkillRejectsSymlinkedConfigDirectory(t *testing.T) {
	root, cwd, external := t.TempDir(), t.TempDir(), t.TempDir()
	writePluginFile(t, external, "skills/shared/SKILL.md", "EXTERNAL SKILL")
	if err := os.Symlink(external, filepath.Join(cwd, ".agents")); err != nil {
		t.Skipf("cannot create test symlink: %v", err)
	}
	resources, err := buildRuntimeResources(root, t.TempDir(), cwd, "test", nil, nil, nil)
	if _, found := resources.skills["shared"]; found || err == nil || !strings.Contains(err.Error(), "symbolic links") {
		t.Fatalf("symlinked .agents directory was not rejected: loaded=%t, error=%v", found, err)
	}
}

func TestClaudeUserSkillSlashInvocation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		content    string
		enabled    bool
		want       string
		wantReject bool
	}{
		{name: "arguments", content: "Review $ARGUMENTS", enabled: true, want: "Review current changes"},
		// A disabled or unregistered skill leaves the prompt untouched so the
		// message still reaches the model as ordinary text.
		{name: "disabled", content: "Disabled instructions", want: "/review current changes"},
		{
			name: "model only", enabled: true, wantReject: true,
			content: "---\nuser-invocable: false\n---\nModel-only instructions",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, cwd := t.TempDir(), t.TempDir()
			catalog := userResourceCatalog{Skills: []contracts.UserSkill{{
				ID: "review", Name: "Review", Content: tc.content, Enabled: tc.enabled,
			}}}
			if err := storage.WriteJSON(filepath.Join(root, resourceCatalogFile), catalog); err != nil {
				t.Fatal(err)
			}
			const invocation = "/review current changes"
			prepared, err := expandPrompt(root, cwd, invocation)
			if err == nil {
				var resources runtimeResources
				resources, err = buildRuntimeResources(root, t.TempDir(), cwd, "test", nil, nil, nil)
				if err == nil {
					var input model.Message
					input, err = (&Engine{}).prepareRuntimeInput(
						context.Background(),
						model.Message{Role: "user", Text: prepared, SourceText: invocation},
						contracts.Session{ID: "test", CWD: cwd}, &activeRun{id: "test-run"}, resources, nil, nil,
					)
					if err == nil && !strings.Contains(input.Text, tc.want) {
						t.Fatalf("invoked skill = %q, want %q", input.Text, tc.want)
					}
				}
			}
			if (err != nil) != tc.wantReject {
				t.Fatalf("invocation error = %v, want rejection = %v", err, tc.wantReject)
			}
		})
	}
}

func TestClaudeProjectResourcesOverridePersonalResources(t *testing.T) {
	root, cwd := t.TempDir(), t.TempDir()
	catalog := userResourceCatalog{
		Skills: []contracts.UserSkill{{
			ID: "review-skill", Name: "Review skill", Content: "PERSONAL SKILL $ARGUMENTS", Enabled: true,
		}},
		Commands: []contracts.UserCommand{{
			ID: "review-command", Name: "review-command", Content: "PERSONAL COMMAND $ARGUMENTS", Enabled: true,
		}},
	}
	if err := storage.WriteJSON(filepath.Join(root, resourceCatalogFile), catalog); err != nil {
		t.Fatal(err)
	}
	writePluginFile(t, cwd, ".claude/skills/review-skill/SKILL.md", "PROJECT SKILL $ARGUMENTS")
	writePluginFile(t, cwd, ".claude/commands/review-command.md", "PROJECT COMMAND $ARGUMENTS")
	resources, err := buildRuntimeResources(root, t.TempDir(), cwd, "test", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "review-skill", want: "PROJECT SKILL current changes"},
		{name: "review-command", want: "PROJECT COMMAND current changes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocation := "/" + tc.name + " current changes"
			prepared, err := expandPrompt(root, cwd, invocation)
			if err != nil {
				t.Fatal(err)
			}
			if prepared != invocation {
				t.Fatalf("project invocation expanded personal content before admission: %q", prepared)
			}
			input, err := (&Engine{}).prepareRuntimeInput(
				context.Background(),
				model.Message{Role: "user", Text: prepared, SourceText: invocation},
				contracts.Session{ID: "test", CWD: cwd}, &activeRun{id: "test-run"}, resources, nil, nil,
			)
			if err != nil || !strings.Contains(input.Text, tc.want) || strings.Contains(input.Text, "PERSONAL") {
				t.Fatalf("project invocation = %q, error = %v", input.Text, err)
			}
		})
	}
}
