package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/plugins"
)

func TestForkSkillHookProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "--" || i+1 >= len(os.Args) {
			continue
		}
		var input struct {
			Event   string `json:"hook_event_name"`
			Session string `json:"session_id"`
			Tool    string `json:"tool_name"`
			Agent   string `json:"agent_id"`
		}
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			os.Exit(3)
		}
		file, err := os.OpenFile(os.Args[i+1], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(4)
		}
		err = json.NewEncoder(file).Encode(input)
		_ = file.Close()
		if err != nil {
			os.Exit(5)
		}
		os.Exit(0)
	}
}

func TestForkSkillScopesHooksAndToolDenialsToChild(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "hooks.log")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	handler := plugins.Hook{
		Type: "command", Command: executable,
		Args: []string{"-test.run=TestForkSkillHookProcess", "--", trace},
	}
	hooks, err := json.Marshal(map[string][]plugins.HookMatcher{
		"PreToolUse": {{Matcher: "^(Bash|Read)$", Hooks: []plugins.Hook{handler}}},
		"Stop":       {{Hooks: []plugins.Hook{handler}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var requestMu sync.Mutex
	parentRequests := 0
	childRequests := 0
	core, cwd := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		encoded, _ := json.Marshal(body)
		text := string(encoded)
		isChild := strings.Contains(text, "Subagent task instructions:")
		requestMu.Lock()
		defer requestMu.Unlock()
		if isChild {
			// Each fork has a fresh history; the returned messages identify
			// its own model step without sharing parent conversation history.
			step := 1
			if strings.Contains(text, "fixture evidence") {
				step = 2
			}
			for _, raw := range body["tools"].([]any) {
				tool := raw.(map[string]any)["function"].(map[string]any)
				if tool["name"] == "Write" {
					t.Error("fork advertised its disallowed Write tool")
				}
			}
			if strings.Contains(text, "disabled by the active skill") {
				step = 3
			}
			childRequests++
			switch step {
			case 1:
				subagentWireCall(w, model.Chat, "Read", `{"file_path":"evidence.txt"}`)
			case 2:
				subagentWireCall(w, model.Chat, "Write", `{"file_path":"child-forbidden.txt","content":"forbidden"}`)
			default:
				subagentText(w, model.Chat, "Fork completed")
			}
			return
		}
		parentRequests++
		switch parentRequests {
		case 1, 3:
			subagentWireCall(w, model.Chat, "Skill", `{"skill":"fork-review"}`)
		case 2:
			subagentWireCall(w, model.Chat, "Read", `{"file_path":"evidence.txt"}`)
		case 4:
			foundWrite := false
			for _, raw := range body["tools"].([]any) {
				tool := raw.(map[string]any)["function"].(map[string]any)
				foundWrite = foundWrite || tool["name"] == "Write"
			}
			if !foundWrite {
				t.Error("fork restriction survived in the parent")
			}
			subagentWireCall(w, model.Chat, "Write", `{"file_path":"parent-allowed.txt","content":"allowed"}`)
		default:
			subagentText(w, model.Chat, "Parent completed")
		}
	}))
	writePluginFile(t, cwd, "evidence.txt", "fixture evidence")
	skill := contracts.UserSkill{
		ID: "fork-review", Name: "Fork review", Enabled: true,
		Content: "---\ncontext: fork\nagent: general-purpose\ndisallowed-tools: [Write]\nhooks: " +
			string(hooks) + "\n---\nInspect evidence. !`printf DYNAMIC`\n",
	}
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &skill})
	state, _ := core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: state.Projects[0].ID, ModelID: "goal/model", Text: "Invoke the fork twice",
		},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	for _, run := range state.Runs {
		if run.Status != "completed" {
			t.Fatalf("run failed: %+v", run)
		}
	}
	if len(state.Runs) != 3 || parentRequests != 5 || childRequests != 6 {
		t.Fatalf("fork did not repeat successfully: runs=%d parent requests=%d child requests=%d",
			len(state.Runs), parentRequests, childRequests)
	}
	if _, err := os.Stat(filepath.Join(cwd, "child-forbidden.txt")); !os.IsNotExist(err) {
		t.Fatalf("child bypassed disallowed-tools: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(cwd, "parent-allowed.txt")); err != nil || string(content) != "allowed" {
		t.Fatalf("parent restriction was not removed: %q %v", content, err)
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	var dynamic, childReads, childStops int
	parentID := ""
	for _, session := range state.Sessions {
		if session.ParentSessionID == "" {
			parentID = session.ID
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var observed struct {
			Event   string `json:"hook_event_name"`
			Session string `json:"session_id"`
			Tool    string `json:"tool_name"`
			Agent   string `json:"agent_id"`
		}
		if err := json.Unmarshal([]byte(line), &observed); err != nil {
			t.Fatalf("invalid hook trace: %q: %v", line, err)
		}
		switch {
		case observed.Event == "PreToolUse" && observed.Session == parentID && observed.Tool == "Bash":
			dynamic++
		case observed.Event == "PreToolUse" && observed.Session != parentID && observed.Tool == "Read":
			childReads++
		case observed.Event == "SubagentStop" && observed.Session == parentID && observed.Agent != "" && observed.Agent != parentID:
			childStops++
		default:
			t.Errorf("fork hook escaped its scope: %q", line)
		}
	}
	if dynamic != 2 || childReads != 2 || childStops != 2 {
		t.Fatalf("fork hooks were not active throughout expansion and child: dynamic=%d reads=%d stops=%d\n%s",
			dynamic, childReads, childStops, data)
	}
}

func TestInlineSkillRemainsActiveForRunAndRepeats(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "hooks.log")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := json.Marshal(map[string][]plugins.HookMatcher{
		"PreToolUse": {{Matcher: "^Read$", Hooks: []plugins.Hook{{
			Type: "command", Command: executable,
			Args: []string{"-test.run=TestForkSkillHookProcess", "--", trace},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var requestMu sync.Mutex
	requests := 0
	core, cwd := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		requestMu.Lock()
		defer requestMu.Unlock()
		requests++
		if requests >= 3 && requests <= 5 {
			for _, raw := range body["tools"].([]any) {
				tool := raw.(map[string]any)["function"].(map[string]any)
				if tool["name"] == "Write" {
					t.Error("inline skill denial ended before the run finished")
				}
			}
		}
		switch requests {
		case 1, 2:
			subagentWireCall(w, model.Chat, "Skill", `{"skill":"inline-review"}`)
		case 3, 7:
			subagentWireCall(w, model.Chat, "Read", `{"file_path":"evidence.txt"}`)
		case 4:
			subagentWireCall(w, model.Chat, "Write", `{"file_path":"inline-forbidden.txt","content":"forbidden"}`)
		case 6:
			subagentWireCall(w, model.Chat, "Write", `{"file_path":"next-run-allowed.txt","content":"allowed"}`)
		default:
			subagentText(w, model.Chat, "Completed")
		}
	}))
	writePluginFile(t, cwd, "evidence.txt", "fixture evidence")
	skill := contracts.UserSkill{
		ID: "inline-review", Name: "Inline review", Enabled: true,
		Content: "---\ndisallowed-tools: [Write]\nhooks: " + string(hooks) + "\n---\nInspect evidence.\n",
	}
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &skill})
	state, _ := core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: state.Projects[0].ID, ModelID: "goal/model", Text: "Invoke the inline skill twice",
		},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	if _, err := os.Stat(filepath.Join(cwd, "inline-forbidden.txt")); !os.IsNotExist(err) {
		t.Fatalf("inline skill denial was not enforced: %v", err)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			SessionID: state.Sessions[0].ID, Text: "Proceed without invoking a skill",
		},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	for _, run := range state.Runs {
		if run.Status != "completed" {
			t.Fatalf("run failed: %+v", run)
		}
	}
	if requests != 8 {
		t.Fatalf("repeated inline skill failed: requests=%d", requests)
	}
	if content, err := os.ReadFile(filepath.Join(cwd, "next-run-allowed.txt")); err != nil || string(content) != "allowed" {
		t.Fatalf("inline skill denial survived its run: %q %v", content, err)
	}
	data, err := os.ReadFile(trace)
	if err != nil || strings.Count(string(data), "PreToolUse") != 1 {
		t.Fatalf("inline skill hook did not remain active exactly for its run: %s %v", data, err)
	}
}

func TestSkillDynamicContextHonorsBashDenial(t *testing.T) {
	cwd := t.TempDir()
	skill := runtimeSkill{component: plugins.MarkdownComponent{
		Content: "!`printf forbidden`", Path: filepath.Join(cwd, "SKILL.md"),
		Metadata: plugins.SkillMetadata{DisallowedTools: []string{"Bash"}},
	}}
	invoked := false
	_, err := (runtimeResources{}).expand(
		context.Background(), skill, "", cwd, "fixture",
		func(context.Context, agent.HookInput) (agent.HookResult, error) {
			invoked = true
			return agent.HookResult{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "Bash is disabled") || invoked {
		t.Fatalf("dynamic context bypassed its tool denial: invoked=%v error=%v", invoked, err)
	}
}

func TestDynamicSkillRespectsEffectiveRunPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		inline      bool
		fork        bool
		profile     contracts.SubagentProfile
		wantBlocked string
	}{
		{name: "earlier inline skill", inline: true, wantBlocked: "active execution policy"},
		{name: "inherited fork denial", fork: true, wantBlocked: "active execution policy"},
		{
			name: "profile allowlist",
			profile: contracts.SubagentProfile{
				ID: "worker", Name: "Worker", Description: "Read and invoke skills", Tools: []string{"Read", "Skill"},
			},
			wantBlocked: "active execution policy",
		},
		{
			name: "profile denylist",
			profile: contracts.SubagentProfile{
				ID: "worker", Name: "Worker", Description: "Work without shell", DisallowedTools: []string{"Bash"},
			},
			wantBlocked: "active execution policy",
		},
		{
			name: "read only built in", profile: contracts.SubagentProfile{ID: "Explore"},
			wantBlocked: `unknown tool`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requestMu sync.Mutex
			parentRequests, childRequests := 0, 0
			core, cwd := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				body := subagentRequest(t, request)
				encoded, _ := json.Marshal(body)
				isChild := strings.Contains(string(encoded), "Subagent task instructions:")
				requestMu.Lock()
				defer requestMu.Unlock()
				if isChild {
					childRequests++
					if childRequests == 1 {
						subagentWireCall(w, model.Chat, "Skill", `{"skill":"dynamic-shell"}`)
					} else {
						subagentText(w, model.Chat, "Child completed")
					}
					return
				}
				parentRequests++
				switch {
				case tc.inline && parentRequests == 1:
					subagentWireCall(w, model.Chat, "Skill", `{"skill":"deny-shell"}`)
				case tc.inline && parentRequests == 2:
					subagentWireCall(w, model.Chat, "Skill", `{"skill":"dynamic-shell"}`)
				case tc.fork && parentRequests == 1:
					subagentWireCall(w, model.Chat, "Skill", `{"skill":"deny-shell"}`)
				case !tc.inline && !tc.fork && parentRequests == 1:
					input, _ := json.Marshal(map[string]any{
						"description": "Check dynamic skill", "prompt": "Invoke dynamic-shell", "subagent_type": tc.profile.ID,
					})
					subagentCall(w, string(input))
				default:
					subagentText(w, model.Chat, "Parent completed")
				}
			}))
			saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &contracts.UserSkill{
				ID: "dynamic-shell", Name: "Dynamic shell", Enabled: true,
				Content: "!`printf BYPASS > bypass.txt`",
			}})
			metadata := "---\ndisallowed-tools: [Bash]\n"
			if tc.fork {
				metadata += "context: fork\n"
			}
			saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &contracts.UserSkill{
				ID: "deny-shell", Name: "Deny shell", Enabled: true, Content: metadata + "---\nInvoke dynamic-shell.\n",
			}})
			state, _ := core.Snapshot(context.Background())
			if tc.profile.ID == "worker" {
				settings := state.Settings
				settings.Subagents = append(settings.Subagents, tc.profile)
				if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
					t.Fatal(err)
				}
			}
			if err := core.Execute(context.Background(), contracts.Command{
				Kind: "message.send", Send: &contracts.SendRequest{
					ProjectID: state.Projects[0].ID, ModelID: "goal/model", Text: "Check the execution policy",
				},
			}); err != nil {
				t.Fatal(err)
			}
			state = waitSubagentState(t, core, runsFinished)
			for _, run := range state.Runs {
				if run.Status != "completed" {
					t.Fatalf("run failed: %+v", run)
				}
			}
			if _, err := os.Stat(filepath.Join(cwd, "bypass.txt")); !os.IsNotExist(err) {
				t.Fatalf("dynamic context bypassed the effective run policy: %v", err)
			}
			sessionIDs := make([]string, 0, len(state.Sessions))
			for _, session := range state.Sessions {
				sessionIDs = append(sessionIDs, session.ID)
			}
			state, err := core.Snapshot(context.Background(), sessionIDs...)
			if err != nil {
				t.Fatal(err)
			}
			messages, _ := json.Marshal(state.Messages)
			if !strings.Contains(string(messages), tc.wantBlocked) {
				t.Fatalf("execution was not explicitly blocked: %s", messages)
			}
		})
	}
}
