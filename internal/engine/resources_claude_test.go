package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestClaudeUserSkillSlashInvocation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		content    string
		enabled    bool
		want       string
		wantReject bool
	}{
		{name: "arguments", content: "Review $ARGUMENTS", enabled: true, want: "Review current changes"},
		{name: "disabled", content: "Disabled instructions", wantReject: true},
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
