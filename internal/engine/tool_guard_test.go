package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/plugins"
	"gcode/internal/storage"
)

func TestToolGuardChecksHookRewrittenInputBeforeSideEffects(t *testing.T) {
	cwd := t.TempDir()
	calls := 0
	core := &Engine{toolGuard: func(_ context.Context, actualCWD, name string, raw json.RawMessage) error {
		calls++
		if actualCWD != cwd || name != "Write" || !strings.Contains(string(raw), "rewritten.txt") {
			t.Fatalf("guard saw unrewritten execution input: cwd=%q name=%q raw=%s", actualCWD, name, raw)
		}
		// The callback receives a copy; it cannot modify the actual invocation.
		for i := range raw {
			raw[i] = ' '
		}
		return errors.New("test guard denied rewritten write")
	}}
	var write agent.Tool
	for _, tool := range agent.CoreTools() {
		if tool.Definition().Name == "Write" {
			write = core.guardTool(core.guardTool(tool))
		}
	}
	if write == nil {
		t.Fatal("Write tool is missing")
	}
	_, err := agent.ExecuteHookedTool(
		context.Background(), write, cwd,
		json.RawMessage(`{"file_path":"original.txt","content":"forbidden"}`),
		func(_ context.Context, input agent.HookInput) (agent.HookResult, error) {
			if input.Event == contracts.HookPreToolUse {
				return agent.HookResult{
					UpdatedInput: json.RawMessage(`{"file_path":"rewritten.txt","content":"forbidden"}`),
				}, nil
			}
			return agent.HookResult{}, nil
		},
		"guarded-write",
	)
	if err == nil || !strings.Contains(err.Error(), "test guard denied") || calls != 1 {
		t.Fatalf("guard was skipped or duplicated: calls=%d err=%v", calls, err)
	}
	for _, name := range []string{"original.txt", "rewritten.txt"} {
		if _, err := os.Stat(filepath.Join(cwd, name)); !os.IsNotExist(err) {
			t.Fatalf("guard allowed a file side effect %s: %v", name, err)
		}
	}
}

func TestToolGuardChecksRewrittenDynamicShell(t *testing.T) {
	cwd := t.TempDir()
	calls := 0
	resources := runtimeResources{guard: func(_ context.Context, actualCWD, name string, raw json.RawMessage) error {
		calls++
		if actualCWD != cwd || name != "Bash" || !strings.Contains(string(raw), "rewritten.txt") {
			t.Fatalf("guard saw unrewritten dynamic shell: cwd=%q name=%q raw=%s", actualCWD, name, raw)
		}
		return errors.New("test guard denied dynamic shell")
	}}
	skill := runtimeSkill{component: plugins.MarkdownComponent{
		Path: filepath.Join(cwd, "SKILL.md"), Content: "!`printf forbidden > original.txt`",
	}}
	_, err := resources.expand(
		context.Background(), skill, "", cwd, "fixture",
		func(_ context.Context, input agent.HookInput) (agent.HookResult, error) {
			if input.Event == contracts.HookPreToolUse {
				return agent.HookResult{
					UpdatedInput: json.RawMessage(`{"command":"printf forbidden > rewritten.txt"}`),
				}, nil
			}
			return agent.HookResult{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "test guard denied dynamic") || calls != 1 {
		t.Fatalf("dynamic shell guard did not block execution: calls=%d err=%v", calls, err)
	}
	for _, name := range []string{"original.txt", "rewritten.txt"} {
		if _, err := os.Stat(filepath.Join(cwd, name)); !os.IsNotExist(err) {
			t.Fatalf("guard allowed a dynamic shell side effect %s: %v", name, err)
		}
	}
}

func TestToolGuardOptionsCoverRootChildHookAgentAndDynamicSkill(t *testing.T) {
	for _, mode := range []string{"root", "child", "hook agent", "dynamic skill"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			var requestMu, guardMu sync.Mutex
			parentRequests, helperRequests := 0, 0
			guardCalls := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				body := subagentRequest(t, request)
				encoded, _ := json.Marshal(body)
				text := string(encoded)
				helper := strings.Contains(text, "Subagent task instructions:") || strings.Contains(text, "You are a verifier.")
				requestMu.Lock()
				defer requestMu.Unlock()
				if helper {
					helperRequests++
					if helperRequests == 1 {
						subagentWireCall(w, model.Chat, "Write", `{"file_path":"forbidden.txt","content":"forbidden"}`)
					} else if mode == "hook agent" {
						subagentText(w, model.Chat, `{"ok":true}`)
					} else {
						subagentText(w, model.Chat, "Child completed")
					}
					return
				}
				parentRequests++
				if parentRequests != 1 {
					subagentText(w, model.Chat, "Parent completed")
					return
				}
				switch mode {
				case "root":
					subagentWireCall(w, model.Chat, "Write", `{"file_path":"forbidden.txt","content":"forbidden"}`)
				case "child":
					subagentCall(w, `{"description":"Guarded child","prompt":"Write forbidden.txt","subagent_type":"general-purpose"}`)
				case "dynamic skill":
					subagentWireCall(w, model.Chat, "Skill", `{"skill":"dynamic-shell"}`)
				default:
					subagentText(w, model.Chat, "Parent completed")
				}
			}))
			defer server.Close()
			store, err := storage.Open(storage.Options{Home: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			core, err := openExecutionFixture(t, Options{Storage: store, ToolGuard: func(_ context.Context, cwd, name string, _ json.RawMessage) error {
				guardMu.Lock()
				guardCalls = append(guardCalls, name)
				guardMu.Unlock()
				if cwd != workspace {
					return errors.New("test guard denied wrong workspace")
				}
				if name == "Write" || name == "Bash" {
					return errors.New("test guard denied side effect")
				}
				return nil
			}})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			if err := core.Execute(ctx, contracts.Command{
				Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "fixture", Name: "Fixture", Protocol: model.Chat, BaseURL: server.URL,
					Models: []model.Model{{ID: "model"}},
				}},
			}); err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{
				Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace},
			}); err != nil {
				t.Fatal(err)
			}
			if mode == "hook agent" {
				state, _ := core.Snapshot(ctx)
				settings := state.Settings
				settings.Hooks = []contracts.HookRule{{
					ID: "verify", Name: "Verify", Event: contracts.HookUserPromptSubmit,
					Type: "agent", Prompt: "Attempt the test write, then return a decision.", Enabled: true,
				}}
				if err := core.Execute(ctx, contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "dynamic skill" {
				saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &contracts.UserSkill{
					ID: "dynamic-shell", Name: "Dynamic shell", Enabled: true,
					Content: "!`printf forbidden > forbidden.txt`",
				}})
			}
			state, _ := core.Snapshot(ctx)
			if err := core.Execute(ctx, contracts.Command{
				Kind: "message.send", Send: &contracts.SendRequest{
					ProjectID: state.Projects[0].ID, ModelID: "fixture/model", Text: "Exercise the guarded execution boundary",
				},
			}); err != nil {
				t.Fatal(err)
			}
			state = waitSubagentState(t, core, runsFinished)
			for _, run := range state.Runs {
				if run.Status != "completed" {
					t.Fatalf("guarded run did not settle: %+v", run)
				}
			}
			if _, err := os.Stat(filepath.Join(workspace, "forbidden.txt")); !os.IsNotExist(err) {
				t.Fatalf("guard did not prevent the file side effect: %v", err)
			}
			want := "Write"
			if mode == "dynamic skill" {
				want = "Bash"
			}
			guardMu.Lock()
			found := false
			for _, name := range guardCalls {
				found = found || name == want
			}
			guardMu.Unlock()
			if !found {
				t.Fatalf("Options.ToolGuard did not observe %s; calls=%v", want, guardCalls)
			}
		})
	}
}
