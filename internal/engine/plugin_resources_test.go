package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/plugins"
	"gcode/internal/storage"
)

func writePluginFile(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProjectMarkdownResourcesSharePluginNamingRules(t *testing.T) {
	configRoot, runtimeRoot, cwd := t.TempDir(), t.TempDir(), t.TempDir()
	writePluginFile(t, cwd, ".claude/commands/db/migrate.md", "---\nname: wrong:command:identity\n---\nMigrate fixture.")
	writePluginFile(t, cwd, ".claude/skills/directory-name/SKILL.md", "---\nname: explicit-skill\n---\nRun fixture.")
	writePluginFile(t, cwd, ".claude/agents/db/review.md", "---\nname: reviewer\n---\nReview fixture.")
	resources, err := buildRuntimeResources(configRoot, runtimeRoot, cwd, "naming-fixture", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resources.skills["db:migrate"]; !ok {
		t.Fatal("project command ignored its relative path identity")
	}
	if _, ok := resources.skills["explicit-skill"]; !ok {
		t.Fatal("project skill ignored its frontmatter name")
	}
	for _, profile := range resources.profiles {
		if profile.ID == "db:reviewer" {
			return
		}
	}
	t.Fatal("project agent did not retain its directory namespace")
}

func installRuntimeFixture(t *testing.T, core *Engine, market string) {
	t.Helper()
	ctx := context.Background()
	if err := core.plugins.PutSource(ctx, plugins.MarketplaceSource{ID: "fixture", Name: "fixture", Enabled: true, Source: plugins.Source{Source: "directory", Path: market}}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.plugins.Refresh(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := core.plugins.Install(ctx, "fixture", "sample"); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	core.state.Plugins = core.pluginProjection()
	core.mu.Unlock()
}

func TestPluginRuntimeResourcesStayWithConversationRoot(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	firstRoot := filepath.Join(home, "first-runtime")
	if err := core.storage.RequestDirectory(firstRoot); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	old := core.state.Sessions[0]
	configRoot := core.storage.ConfigDirectory()
	if err := storage.WriteJSON(filepath.Join(configRoot, resourceCatalogFile), userResourceCatalog{
		Skills: []contracts.UserSkill{{ID: "personal", Name: "Personal", Enabled: true, Content: "FIXED USER CONFIGURATION"}},
	}); err != nil {
		t.Fatal(err)
	}
	secondRoot := filepath.Join(home, "second-runtime")
	if err := core.storage.RequestDirectory(secondRoot); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	fresh := core.state.Sessions[0]
	bundle := plugins.Bundle{
		ID: "fixture", Root: filepath.Join(configRoot, "plugins", "installed", "fixture"),
		Manifest: plugins.Manifest{Name: "fixture"},
		Skills:   []plugins.MarkdownComponent{{Name: "fixture:inspect", Content: "${CLAUDE_PLUGIN_DATA}", Path: "SKILL.md"}},
		Agents:   []plugins.MarkdownComponent{{Name: "fixture:worker", Content: "${CLAUDE_PLUGIN_DATA}", Path: "worker.md"}},
		Hooks:    map[string][]plugins.HookMatcher{"PreToolUse": {{Hooks: []plugins.Hook{{Type: "command", Command: "test"}}}}},
		MCPServers: map[string]plugins.MCPServer{
			"test": {Command: "test", Env: map[string]string{"PLUGIN_DATA": "${CLAUDE_PLUGIN_DATA}", "PLUGIN_ROOT": "${CLAUDE_PLUGIN_ROOT}"}},
		},
	}
	for _, tc := range []struct {
		name    string
		session contracts.Session
		root    string
	}{
		{name: "existing conversation", session: old, root: firstRoot},
		{name: "new conversation", session: fresh, root: secondRoot},
		{name: "operation without conversation", session: contracts.Session{CWD: t.TempDir()}, root: secondRoot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resources, err := core.prepareRuntimeResources(configRoot, tc.session, []plugins.Bundle{bundle}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			data := resources.skills["fixture:inspect"].data
			relative, err := filepath.Rel(filepath.Join(tc.root, "plugins", "data"), data)
			if err != nil || relative == "." || !filepath.IsLocal(relative) {
				t.Fatalf("plugin persistent data escaped the bound runtime root: %q", data)
			}
			if info, err := os.Stat(data); err != nil || !info.IsDir() {
				t.Fatalf("plugin data directory was not created: %v", err)
			}
			if filepath.Dir(resources.envFile) != filepath.Join(tc.root, "plugins", "sessions") {
				t.Fatalf("plugin session environment used another root: %q", resources.envFile)
			}
			if resources.skills["personal"].component.Content != "FIXED USER CONFIGURATION" {
				t.Fatal("user skill definitions followed the runtime root")
			}
			if len(resources.hooks) != 1 || resources.hooks[0].PluginData != data ||
				len(resources.mcp) != 1 || resources.mcp[0].Env["PLUGIN_DATA"] != data ||
				resources.mcp[0].Env["PLUGIN_ROOT"] != bundle.Root {
				t.Fatal("Hook/MCP paths did not use runtime data and fixed plugin installations")
			}
			foundAgent := false
			for _, profile := range resources.profiles {
				if profile.ID == "fixture:worker" {
					foundAgent = profile.Prompt == data
				}
			}
			if !foundAgent {
				t.Fatal("plugin subagent prompt did not receive its bound runtime path")
			}
		})
	}
	for _, name := range []string{"data", "sessions"} {
		if _, err := os.Stat(filepath.Join(configRoot, "plugins", name)); !os.IsNotExist(err) {
			t.Fatalf("plugin runtime data grew in fixed configuration: %s, %v", name, err)
		}
	}
	marker := filepath.Join(firstRoot, ".gcode-root.json")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := core.prepareRuntimeResources(configRoot, old, []plugins.Bundle{bundle}, nil, nil); err == nil {
		t.Fatal("plugin runtime silently accepted unavailable historical storage")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("plugin runtime recreated a missing historical root identity")
	}
}

func TestPluginProcessHelper(t *testing.T) {
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	switch mode {
	case "rewrite":
		var input map[string]any
		if json.NewDecoder(os.Stdin).Decode(&input) != nil {
			os.Exit(3)
		}
		toolInput, _ := input["tool_input"].(map[string]any)
		if input["hook_event_name"] != "PreToolUse" || input["tool_name"] != "Write" {
			os.Exit(4)
		}
		toolInput["file_path"] = "actual.txt"
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "permissionDecision": "allow", "updatedInput": toolInput}})
		os.Exit(0)
	case "mcp":
		if os.Getenv("MCP_API_KEY") != "fixture-private-key" {
			os.Exit(5)
		}
		data := os.Getenv("MCP_PLUGIN_DATA")
		relative, err := filepath.Rel(filepath.Join(os.Getenv("MCP_DATA_ROOT"), "plugins", "data"), data)
		if err != nil || relative == "." || !filepath.IsLocal(relative) {
			os.Exit(7)
		}
		if info, err := os.Stat(data); err != nil || !info.IsDir() {
			os.Exit(8)
		}
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params map[string]any  `json:"params"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				os.Exit(6)
			}
			if len(request.ID) == 0 {
				continue
			}
			var result any
			switch request.Method {
			case "initialize":
				result = map[string]any{"protocolVersion": request.Params["protocolVersion"], "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "fixture", "version": "1"}}
			case "tools/list":
				result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo test text", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}}}}
			case "tools/call":
				arguments, _ := request.Params["arguments"].(map[string]any)
				result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "REAL MCP " + fmt.Sprint(arguments["text"])}}}
			default:
				result = map[string]any{}
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		}
		os.Exit(0)
	default:
		return
	}
}

func TestInstalledPluginExecutesSkillHookMCPAndConfiguredSubagent(t *testing.T) {
	core, root := resourceCore(t)
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := core.storage.RequestDirectory(runtimeRoot); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, filepath.Dir(root))
	defer core.Close()
	market, workspace := t.TempDir(), t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hook := map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{"matcher": "^Write$", "hooks": []any{map[string]any{"type": "command", "command": executable, "args": []string{"-test.run=TestPluginProcessHelper", "--", "rewrite"}}}}}}}
	hookJSON, _ := json.Marshal(hook)
	servers, _ := json.Marshal(map[string]any{"echo": map[string]any{"command": executable, "args": []string{"-test.run=TestPluginProcessHelper", "--", "mcp"}, "env": map[string]string{"MCP_API_KEY": "${user_config.key}", "MCP_PLUGIN_DATA": "${CLAUDE_PLUGIN_DATA}", "MCP_DATA_ROOT": runtimeRoot}}})
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"sample","version":"1.0.0","userConfig":{"key":{"type":"string","title":"Key","sensitive":true,"required":true}}}`)
	writePluginFile(t, market, "sample/hooks/hooks.json", string(hookJSON))
	writePluginFile(t, market, "sample/.mcp.json", string(servers))
	writePluginFile(t, market, "sample/skills/inspect/SKILL.md", "---\ndescription: Inspect the runtime fixture\nmodel: haiku\n---\nARG: $0 / $ARGUMENTS[1]\nPLUGIN: ${CLAUDE_PLUGIN_ROOT}\nDATA: ${CLAUDE_PLUGIN_DATA}\n")
	installRuntimeFixture(t, core, market)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "plugins.configure", ID: "sample@fixture", Plugin: &contracts.PluginInput{Configuration: map[string]string{"key": "fixture-private-key"}}}); err != nil {
		t.Fatal(err)
	}
	var parentCalls, childCalls atomic.Int32
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		if body["model"] != "configured-child" {
			t.Errorf("child model = %v", body["model"])
		}
		childCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		subagentText(w, model.Chat, "CHILD WITH CONFIGURED MODEL")
	}))
	defer child.Close()
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body := subagentRequest(t, request)
		data, _ := json.Marshal(body)
		if strings.Contains(string(data), "fixture-private-key") {
			t.Error("plugin configuration reached model")
		}
		if body["model"] != "main-model" {
			t.Errorf("parent model = %v", body["model"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch parentCalls.Add(1) {
		case 1:
			if !strings.Contains(string(data), "sample:inspect") {
				t.Error("installed skill was not discovered")
			}
			subagentWireCall(w, model.Chat, "Skill", `{"skill":"sample:inspect","args":"one \"two words\""}`)
		case 2:
			if !strings.Contains(string(data), "ARG: one / two words") {
				t.Errorf("skill arguments were not expanded; messages=%v", body["messages"])
			}
			if !strings.Contains(fmt.Sprint(body["messages"]), filepath.Join(runtimeRoot, "plugins", "data")) {
				t.Error("plugin skill did not receive the selected runtime root")
			}
			subagentWireCall(w, model.Chat, "Write", `{"file_path":"requested.txt","content":"WRITTEN AFTER HOOK"}`)
		case 3:
			subagentWireCall(w, model.Chat, "mcp__plugin_sample_echo__echo", `{"text":"EXECUTED"}`)
		case 4:
			if !strings.Contains(string(data), "REAL MCP EXECUTED") {
				t.Error("real stdio MCP result was absent")
			}
			subagentCall(w, `{"description":"Use configured model","prompt":"Verify","subagent_type":"worker","model":"haiku"}`)
		default:
			if !strings.Contains(string(data), "CHILD WITH CONFIGURED MODEL") {
				t.Error("subagent result was not returned")
			}
			subagentText(w, model.Chat, "ALL COMPONENTS EXECUTED")
		}
	}))
	defer parent.Close()
	for _, provider := range []model.Provider{
		{ID: "main", Name: "Main", Protocol: model.Chat, BaseURL: parent.URL, Models: []model.Model{{ID: "main-model"}}},
		{ID: "child", Name: "Child", Protocol: model.Chat, BaseURL: child.URL, Models: []model.Model{{ID: "configured-child"}}},
	} {
		if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.Subagents = append(settings.Subagents, contracts.SubagentProfile{ID: "worker", Name: "Worker", Description: "Configured worker", Model: &contracts.ModelSelection{ModelID: "child/configured-child"}})
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Fixture"}}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{ProjectID: state.Projects[0].ID, ModelID: "main/main-model", Text: "Exercise the installed plugin"}}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	if parentCalls.Load() != 5 || childCalls.Load() != 1 {
		t.Fatalf("actual calls parent=%d child=%d runs=%+v", parentCalls.Load(), childCalls.Load(), state.Runs)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "actual.txt"))
	if err != nil || string(data) != "WRITTEN AFTER HOOK" {
		t.Fatalf("hook parameter rewrite did not execute: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "requested.txt")); !os.IsNotExist(err) {
		t.Fatal("original tool parameters executed despite hook rewrite")
	}
	if len(state.Changes) == 0 || state.Changes[0].Path != "actual.txt" {
		t.Fatalf("diff did not follow rewritten parameters: %+v", state.Changes)
	}
	public, _ := json.Marshal(state)
	if strings.Contains(string(public), "fixture-private-key") {
		t.Fatal("plugin secret leaked into public state")
	}
	if _, err := os.Stat(filepath.Join(root, "plugin-configuration.json")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"data", "sessions"} {
		if _, err := os.Stat(filepath.Join(root, "plugins", name)); !os.IsNotExist(err) {
			t.Fatalf("real plugin execution wrote runtime data into configuration: %s, %v", name, err)
		}
	}
}

func TestPluginInvocationArgumentSemantics(t *testing.T) {
	for _, tc := range []struct{ text, args, want string }{
		{"$0|$1|$ARGUMENTS[1]", `first "two words"`, "first|two words|two words"},
		{"$2", "one", "$2\n\nARGUMENTS: one"},
		{`\$0 $ARGUMENTS`, "one", "$0 one"},
		{"$named/$missing", "one", "one/"},
		{"Review $ARGUMENTS", "the repo's configuration", "Review the repo's configuration"},
		{"Review configuration", "the repo's configuration", "Review configuration\n\nARGUMENTS: the repo's configuration"},
	} {
		got, err := substituteInvocation(tc.text, tc.args, []string{"named", "missing"})
		if err != nil || got != tc.want {
			t.Errorf("%q: got=%q want=%q err=%v", tc.text, got, tc.want, err)
		}
	}
}

func TestSessionBackgroundTaskSurvivesModelTurn(t *testing.T) {
	core, _ := resourceCore(t)
	session := newTestSession(t, core, "", "background owner")
	core.mu.Lock()
	session.CWD = t.TempDir()
	core.state.Sessions[core.sessionIndex(session.ID)] = session
	core.mu.Unlock()
	tools, err := core.sessionRuntimeTools(session, "")
	if err != nil {
		t.Fatal(err)
	}
	var bash, output interface {
		Execute(context.Context, string, json.RawMessage) (model.ToolOutput, error)
	}
	for _, tool := range tools {
		if tool.Definition().Name == "Bash" {
			bash = tool
		}
		if tool.Definition().Name == "TaskOutput" {
			output = tool
		}
	}
	result, err := bash.Execute(context.Background(), session.CWD, json.RawMessage(`{"command":"sleep 0.2; printf SESSION_TASK","run_in_background":true}`))
	if err != nil {
		t.Fatal(err)
	}
	task, _ := result.Data.(map[string]any)
	taskID, _ := task["backgroundTaskId"].(string)
	if taskID == "" {
		t.Fatalf("missing background task ID: %s", result.Text)
	}
	next, _ := core.sessionRuntimeTools(session, "")
	if len(next) != len(tools) {
		t.Fatal("session changed its task owner")
	}
	raw, _ := json.Marshal(map[string]any{"task_id": taskID, "block": true, "timeout": 5000})
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	value, err := output.Execute(ctx, session.CWD, raw)
	if err != nil || !strings.Contains(value.Text, "SESSION_TASK") {
		t.Fatalf("background session task lost: %q %v", value.Text, err)
	}
}
