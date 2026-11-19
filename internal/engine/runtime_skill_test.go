package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// A prompt may open with a slash command and still reference a selected skill.
// Prompt preparation expands the command first and appends the skill text after
// it, so the runtime command expansion must replace only its own segment and
// never drop the skill instructions the user selected.
func TestRuntimeSkillKeepsExpandedPromptReferences(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		payload, err := json.Marshal(body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		bodies = append(bodies, string(payload))
		mu.Unlock()
		goalResponse(w, model.Chat, "Done.", nil, true)
	}))
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{
		Skill: &contracts.UserSkill{ID: "docs", Name: "文档", Content: "SKILL INSTRUCTIONS E2E", Enabled: true},
	})
	saveResource(t, core, "resource.command.save", &contracts.ResourceInput{
		Command: &contracts.UserCommand{ID: "review", Name: "review", Content: "REVIEW $ARGUMENTS", Enabled: true},
	})
	command := contracts.PromptReference{Kind: "command", ID: "review", Label: "review"}
	skill := contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}
	prompt := command.Markdown() + " 请检查 " + skill.Markdown()
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send",
		Send: &contracts.SendRequest{Text: prompt, ModelID: "goal/model", ProjectID: state.Projects[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	waitSubagentState(t, core, runsFinished)
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) == 0 {
		t.Fatal("provider received no request")
	}
	body := bodies[len(bodies)-1]
	commandAt, skillAt := strings.Index(body, "REVIEW"), strings.Index(body, "SKILL INSTRUCTIONS E2E")
	if commandAt < 0 || skillAt < 0 || commandAt > skillAt {
		t.Fatalf("provider request lost the expanded command or the selected skill: %s", body)
	}
	if strings.Contains(body, "gcode://") {
		t.Fatalf("provider request carried an internal reference: %s", body)
	}
}

// The runtime expansion replaces the command segment of an already prepared
// prompt, so the references expanded during preparation stay in place. A
// prompt that no longer starts with that segment keeps the previous whole-text
// replacement instead of duplicating the command.
func TestPromptReferenceTailKeepsPreparedReferences(t *testing.T) {
	root, cwd := t.TempDir(), t.TempDir()
	catalog := userResourceCatalog{
		Skills:   []contracts.UserSkill{{ID: "docs", Name: "文档", Content: "SKILL INSTRUCTIONS E2E", Enabled: true}},
		Commands: []contracts.UserCommand{{ID: "review", Name: "review", Content: "REVIEW $ARGUMENTS", Enabled: true}},
	}
	if err := storage.WriteJSON(filepath.Join(root, resourceCatalogFile), catalog); err != nil {
		t.Fatal(err)
	}
	command := contracts.PromptReference{Kind: "command", ID: "review", Label: "review"}
	skill := contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}
	prompt := command.Markdown() + " 请检查 " + skill.Markdown()
	prepared, err := expandPrompt(root, cwd, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prepared, "REVIEW") || !strings.Contains(prepared, catalog.Skills[0].Content) {
		t.Fatalf("prompt preparation did not expand both references: %q", prepared)
	}
	resources, err := buildRuntimeResources(root, t.TempDir(), cwd, "test", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := func(text string) (model.Message, error) {
		return (&Engine{}).prepareRuntimeInput(
			context.Background(),
			model.Message{Role: "user", Text: text, SourceText: prompt},
			contracts.Session{ID: "test", CWD: cwd}, &activeRun{id: "test-run"}, resources, nil, nil,
		)
	}
	input, err := run(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(input.Text, "REVIEW") || !strings.Contains(input.Text, catalog.Skills[0].Content) {
		t.Fatalf("runtime expansion dropped a prepared reference: %q", input.Text)
	}
	if strings.Count(input.Text, "REVIEW") != 1 || strings.Contains(input.Text, "gcode://") {
		t.Fatalf("runtime expansion duplicated the command or kept an internal reference: %q", input.Text)
	}
	if !strings.Contains(input.Text, "\n\nUser-selected skill docs:") {
		t.Fatalf("runtime expansion did not keep the separator of the prepared prompt: %q", input.Text)
	}
	// A prompt whose prepared text is no longer recognizable still expands the
	// command and keeps the selected skill instead of dropping it.
	unprepared, err := run("REVIEW something else")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(unprepared.Text, "REVIEW") != 1 || !strings.Contains(unprepared.Text, "SKILL INSTRUCTIONS E2E") {
		t.Fatalf("unrecognized prepared prompt lost the selected skill: %q", unprepared.Text)
	}
}
