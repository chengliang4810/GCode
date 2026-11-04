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
	"gcode/internal/storage"
)

func resourceCore(t *testing.T) (*Engine, string) {
	t.Helper()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	return core, store.Directory()
}

func saveResource(t *testing.T, core *Engine, kind string, resource *contracts.ResourceInput) {
	t.Helper()
	if err := core.Execute(context.Background(), contracts.Command{Kind: kind, Resource: resource}); err != nil {
		t.Fatal(err)
	}
}

func TestUserSkillImportEditToggleAndDelete(t *testing.T) {
	core, root := resourceCore(t)
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: 文档处理\ndescription: Read a document on demand\n---\nPRIVATE SKILL BODY"
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "helper.txt"), []byte("support content"), 0600); err != nil {
		t.Fatal(err)
	}
	saveResource(t, core, "resource.skill.import", &contracts.ResourceInput{
		Skill: &contracts.UserSkill{ID: "documents", Enabled: true}, ImportPath: source,
	})
	snapshot, _ := core.Snapshot(context.Background())
	skill := snapshot.UserResources.Skills[0]
	if skill.Name != "文档处理" || skill.Description != "Read a document on demand" || len(skill.Files) != 1 || skill.Files[0] != "scripts/helper.txt" {
		t.Fatalf("import metadata/support files lost: %+v", skill)
	}
	actual, err := os.ReadFile(filepath.Join(root, "skills", "documents", "scripts", "helper.txt"))
	if err != nil || string(actual) != "support content" {
		t.Fatalf("support file was not imported: %s %v", actual, err)
	}
	instructions := skillInstructions(root, t.TempDir())
	if !strings.Contains(instructions, "documents") || strings.Contains(instructions, "PRIVATE SKILL BODY") {
		t.Fatalf("skill catalog must advertise on-demand content: %s", instructions)
	}
	expanded, err := expandPrompt(root, "", "/skill:documents task")
	if err != nil || !strings.Contains(expanded, "PRIVATE SKILL BODY") || !strings.HasSuffix(expanded, "task") {
		t.Fatalf("skill invocation: %q %v", expanded, err)
	}
	skill.Content = "UPDATED CONTENT"
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &skill})
	actual, err = os.ReadFile(filepath.Join(root, "skills", "documents", "SKILL.md"))
	if err != nil || string(actual) != "UPDATED CONTENT" {
		t.Fatalf("edited skill was not materialized: %q %v", actual, err)
	}
	actual, err = os.ReadFile(filepath.Join(root, "skills", "documents", "scripts", "helper.txt"))
	if err != nil || string(actual) != "support content" {
		t.Fatalf("editing deleted support files: %q %v", actual, err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.skill.toggle", ID: skill.ID, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(skillInstructions(root, ""), "documents") {
		t.Fatal("disabled skill was advertised")
	}
	if _, err := expandPrompt(root, "", "/skill:documents"); err == nil {
		t.Fatal("disabled skill remained callable")
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.skill.delete", ID: skill.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", skill.ID)); !os.IsNotExist(err) {
		t.Fatalf("deleted skill directory remains: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "skills"))
	if len(entries) != 0 {
		t.Fatalf("staging/backup directories leaked: %+v", entries)
	}
	original, _ := os.ReadFile(filepath.Join(source, "SKILL.md"))
	if string(original) != content {
		t.Fatal("import modified its source")
	}
}

func TestUserCommandsExpandArgumentsAndIgnoreProjectResources(t *testing.T) {
	core, root := resourceCore(t)
	project := t.TempDir()
	for _, dir := range []string{filepath.Join(project, ".agents", "prompts"), filepath.Join(project, ".agents", "skills", "project-only")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(project, ".agents", "prompts", "review.md"), []byte("PROJECT COMMAND"), 0600)
	_ = os.WriteFile(filepath.Join(project, ".agents", "skills", "project-only", "SKILL.md"), []byte("PROJECT SKILL"), 0600)
	command := contracts.UserCommand{ID: "review-id", Name: "review", Description: "Review code",
		ArgumentHint: "<file> <branch>", Content: "All=$ARGUMENTS; first=$0; second=$1; missing=$2", Enabled: true}
	saveResource(t, core, "resource.command.save", &contracts.ResourceInput{Command: &command})
	actual, err := expandPrompt(root, project, "/review alpha beta")
	if err != nil || actual != "All=alpha beta; first=alpha; second=beta; missing=$2" {
		t.Fatalf("command arguments = %q %v", actual, err)
	}
	if strings.Contains(skillInstructions(root, project), "project-only") {
		t.Fatal("project skill appeared in user-only catalog")
	}
	if _, err := expandPrompt(root, project, "/skill:project-only"); err == nil {
		t.Fatal("project skill remained callable")
	}
	command.Content = "No placeholder"
	saveResource(t, core, "resource.command.save", &contracts.ResourceInput{Command: &command})
	actual, err = expandPrompt(root, project, "/review alpha beta")
	if err != nil || actual != "No placeholder\n\nARGUMENTS: alpha beta" {
		t.Fatalf("literal command arguments = %q %v", actual, err)
	}
	command.Content = "$0 / $1 / $ARGUMENTS"
	saveResource(t, core, "resource.command.save", &contracts.ResourceInput{Command: &command})
	actual, err = expandPrompt(root, project, "/review $2 test")
	if err != nil || actual != "$2 / test / $2 test" {
		t.Fatalf("replacement recursively expanded user text: %q %v", actual, err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.command.toggle", ID: command.ID, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := expandPrompt(root, project, "/review"); err == nil {
		t.Fatal("disabled command remained callable")
	}
	catalog, err := readResourceCatalog(root)
	if err != nil || len(catalog.Commands) != 1 || catalog.Commands[0].ArgumentHint != "<file> <branch>" {
		t.Fatalf("command metadata was not persisted: %+v %v", catalog, err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.command.delete", ID: command.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := expandPrompt(root, project, "/review"); err == nil {
		t.Fatal("project prompt overrode a deleted user command")
	}
}

func TestUserSkillRejectsPathEscapeAndRollsBackFailedCatalogCommit(t *testing.T) {
	core, root := resourceCore(t)
	for _, name := range []string{"../escape", `..\escape`, "C:escape", ".", "a/b"} {
		t.Run(name, func(t *testing.T) {
			err := core.Execute(context.Background(), contracts.Command{Kind: "resource.skill.save", Resource: &contracts.ResourceInput{
				Skill: &contracts.UserSkill{ID: name, Name: "Invalid", Content: "body", Enabled: true},
			}})
			if err == nil {
				t.Fatal("unsafe resource ID accepted")
			}
		})
	}
	skill := contracts.UserSkill{ID: "safe", Name: "Safe", Content: "ORIGINAL", Enabled: true}
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &skill})
	catalogPath := filepath.Join(root, resourceCatalogFile)
	if err := os.Remove(catalogPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(catalogPath, 0700); err != nil {
		t.Fatal(err)
	}
	skill.Content = "MUST ROLLBACK"
	err := core.Execute(context.Background(), contracts.Command{Kind: "resource.skill.save", Resource: &contracts.ResourceInput{Skill: &skill}})
	if err == nil {
		t.Fatal("saving over a directory should fail")
	}
	actual, err := os.ReadFile(filepath.Join(root, "skills", skill.ID, "SKILL.md"))
	if err != nil || string(actual) != "ORIGINAL" {
		t.Fatalf("failed commit changed live skill: %q %v", actual, err)
	}
	snapshot, _ := core.Snapshot(context.Background())
	if snapshot.UserResources.Skills[0].Content != "ORIGINAL" {
		t.Fatal("failed resource commit mutated public state")
	}
}

func TestUserSkillImportRejectsSymlinks(t *testing.T) {
	core, _ := resourceCore(t)
	source, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("safe content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("must not import"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(source, "link")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	err := core.Execute(context.Background(), contracts.Command{Kind: "resource.skill.import", Resource: &contracts.ResourceInput{
		Skill: &contracts.UserSkill{ID: "linked", Enabled: true}, ImportPath: source,
	}})
	if err == nil {
		t.Fatal("skill import followed a symlink")
	}
}

func TestMCPUserSettingsHTTPTestAndRuntimeTools(t *testing.T) {
	var requests atomic.Int32
	block := make(chan struct{})
	entered := make(chan struct{}, 1)
	var blockList atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer user-resource-secret" {
			t.Error("MCP credential was not used")
		}
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch request.Method {
		case "server/discover":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32601, "message": "method not found"}})
			return
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25"}
		case "tools/list":
			if blockList.Load() {
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-block:
				case <-r.Context().Done():
					return
				}
			}
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo a message", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "real user MCP output"}}}
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	core, root := resourceCore(t)
	input := contracts.MCPServerInput{ID: "mcp-id", Name: "custom", Transport: "http", URL: server.URL, Enabled: true,
		Headers: map[string]string{"Authorization": "Bearer user-resource-secret"}}
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
	input.Headers = nil
	input.Name = "renamed"
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
	snapshot, _ := core.Snapshot(context.Background())
	data, _ := json.Marshal(snapshot)
	if strings.Contains(string(data), "user-resource-secret") || len(snapshot.UserResources.MCPServers[0].HeaderKeys) != 1 {
		t.Fatal("public MCP settings leaked credentials or lost key presence")
	}
	// Saving another configuration field preserves omitted credentials.
	blockList.Store(true)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.test", ID: input.ID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP test did not connect")
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil || snapshot.UserResources.MCPServers[0].Test.State != "testing" {
		t.Fatalf("test blocked the core lock: %+v %v", snapshot.UserResources, err)
	}
	blockList.Store(false)
	close(block)
	snapshot = waitResourceState(t, core, func(s contracts.Snapshot) bool { return s.UserResources.MCPServers[0].Test.State != "testing" })
	test := snapshot.UserResources.MCPServers[0].Test
	if test.State != "ready" || len(test.Tools) != 1 || test.Tools[0].Name != "mcp__renamed__echo" {
		t.Fatalf("MCP test did not discover real tools: %+v", test)
	}
	tools, closeTools, _, err := loadRunTools(context.Background(), root, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools {
		if tool.Definition().Name == "mcp__renamed__echo" {
			found = true
			output, err := tool.Execute(context.Background(), root, json.RawMessage(`{}`))
			if err != nil || !strings.Contains(output.Text, "real user MCP output") {
				t.Fatalf("MCP runtime execution: %+v %v", output, err)
			}
		}
	}
	closeTools()
	if !found {
		t.Fatal("saved MCP tool was not available at runtime")
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.toggle", ID: input.ID, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	tools, closeTools, _, err = loadRunTools(context.Background(), root, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	closeTools()
	if requests.Load() != before {
		t.Fatal("disabled MCP server was connected")
	}
	for _, tool := range tools {
		if strings.HasPrefix(tool.Definition().Name, "mcp__") {
			t.Fatal("disabled MCP tool was advertised")
		}
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.delete", ID: input.ID}); err != nil {
		t.Fatal(err)
	}
}

func waitResourceState(t *testing.T, core *Engine, predicate func(contracts.Snapshot) bool) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("resource operation did not finish")
	return contracts.Snapshot{}
}

func TestMCPUserSettingsStdioAndCredentialRemoval(t *testing.T) {
	core, root := resourceCore(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input := contracts.MCPServerInput{ID: "stdio-id", Name: "stdio", Transport: "stdio", Command: executable,
		Args: []string{"-test.run=^TestUserResourceStdioHelper$"}, Env: map[string]string{"RCODE_TEST_USER_MCP": "yes", "REMOVE_ME": "secret"}, Enabled: true}
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
	input.Env = nil
	input.RemoveEnvKeys = []string{"REMOVE_ME"}
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
	catalog, err := readResourceCatalog(root)
	if err != nil || catalog.MCP[0].Env["RCODE_TEST_USER_MCP"] != "yes" || catalog.MCP[0].Env["REMOVE_ME"] != "" {
		t.Fatalf("credential merge/removal failed: %+v %v", catalog.MCP, err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.test", ID: input.ID}); err != nil {
		t.Fatal(err)
	}
	state := waitResourceState(t, core, func(s contracts.Snapshot) bool { return s.UserResources.MCPServers[0].Test.State != "testing" })
	if test := state.UserResources.MCPServers[0].Test; test.State != "ready" || len(test.Tools) != 1 {
		t.Fatalf("stdio test did not use the configured child process: %+v", test)
	}
}

func TestUserResourceStdioHelper(t *testing.T) {
	if os.Getenv("RCODE_TEST_USER_MCP") != "yes" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == 0 {
			continue
		}
		result := `{}`
		switch request.Method {
		case "initialize":
			result = `{"protocolVersion":"2025-11-25"}`
		case "tools/list":
			result = `{"tools":[{"name":"stdio_echo","description":"Child process tool","inputSchema":{"type":"object"}}]}`
		}
		fmt.Fprintf(os.Stdout, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":%s}\n", request.ID, result)
	}
	os.Exit(0)
}

func TestMCPRuntimeFailuresCannotLeakConfiguredCredentials(t *testing.T) {
	for _, phase := range []string{"initialize", "tools/list", "tools/call"} {
		t.Run(phase, func(t *testing.T) {
			mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     uint64 `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.ID == 0 {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if request.Method == phase {
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID,
						"error": map[string]any{"code": -32000, "message": "unauthorized Bearer runtime-private-token"}})
					return
				}
				var result any
				switch request.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": "2025-11-25"}
				case "tools/list":
					result = map[string]any{"tools": []any{map[string]any{"name": "echo", "inputSchema": map[string]any{"type": "object"}}}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			}))
			defer mcpServer.Close()
			var modelCalls atomic.Int32
			core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				encoded, _ := json.Marshal(body)
				if strings.Contains(string(encoded), "runtime-private-token") {
					t.Error("MCP tool error leaked credentials into model history")
				}
				if modelCalls.Add(1) == 1 {
					subagentWireCall(w, model.Chat, "mcp__runtime__echo", `{}`)
				} else {
					subagentText(w, model.Chat, "Handled the MCP failure.")
				}
			}))
			saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &contracts.MCPServerInput{
				ID: "runtime", Name: "runtime", Transport: "http", URL: mcpServer.URL, Enabled: true,
				Headers: map[string]string{"Authorization": "Bearer runtime-private-token"},
			}})
			sendGoal(t, core, "Use the configured tool")
			snapshot := waitSubagentState(t, core, runsFinished)
			encoded, _ := json.Marshal(snapshot)
			if strings.Contains(string(encoded), "runtime-private-token") {
				t.Fatalf("MCP %s leaked into public state: %s", phase, encoded)
			}
			if phase == "tools/call" && modelCalls.Load() != 2 {
				t.Fatalf("remote tool failure did not reach the Agent: %d calls", modelCalls.Load())
			}
		})
	}
}

func TestUserResourcesSurviveRestartWithSecretsKeptPrivate(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	root := store.Directory()
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &contracts.UserSkill{
		ID: "reloaded", Name: "Reloaded", Content: "RESTART SKILL", Enabled: true,
	}})
	saveResource(t, core, "resource.command.save", &contracts.ResourceInput{Command: &contracts.UserCommand{
		ID: "reload-command", Name: "reload", Content: "RESTART $ARGUMENTS", Enabled: true,
	}})
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &contracts.MCPServerInput{
		ID: "reload-mcp", Name: "reload", Transport: "stdio", Command: "user-configured-executable",
		Env: map[string]string{"API_TOKEN": "private-restart-secret"}, Enabled: false,
	}})
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	engineFile, err := os.ReadFile(filepath.Join(root, "engine.json"))
	if err != nil || strings.Contains(string(engineFile), "private-restart-secret") {
		t.Fatalf("public persisted engine state leaked MCP secrets: %v", err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snapshot, _ := reopened.Snapshot(context.Background())
	if len(snapshot.UserResources.Skills) != 1 || len(snapshot.UserResources.Commands) != 1 || len(snapshot.UserResources.MCPServers) != 1 {
		t.Fatalf("user resources not rebuilt from catalog: %+v", snapshot.UserResources)
	}
	expanded, err := expandPrompt(root, "", "/reload after restart")
	if err != nil || expanded != "RESTART after restart" {
		t.Fatalf("reloaded command: %q %v", expanded, err)
	}
	catalog, err := readResourceCatalog(root)
	if err != nil || catalog.MCP[0].Env["API_TOKEN"] != "private-restart-secret" || !catalog.MCP[0].Disabled {
		t.Fatalf("runtime configuration did not preserve private values/state: %v", err)
	}
}

func TestMCPTestCannotOverwriteEditedOrDeletedConfiguration(t *testing.T) {
	for _, operation := range []string{"save", "delete", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     uint64 `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.ID == 0 {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				result := any(map[string]any{"protocolVersion": "2025-11-25"})
				if request.Method == "tools/list" {
					close(entered)
					<-release
					defer close(finished)
					result = map[string]any{"tools": []any{map[string]any{"name": "stale", "inputSchema": map[string]any{"type": "object"}}}}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			}))
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
				server.Close()
			}()
			core, _ := resourceCore(t)
			input := contracts.MCPServerInput{ID: "stable-id", Name: "stable", Transport: "http", URL: server.URL, Enabled: true}
			saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
			if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.test", ID: input.ID}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("MCP discovery did not start")
			}
			switch operation {
			case "save":
				// An identical save still invalidates the previous diagnostic.
				saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &input})
			case "delete":
				if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.mcp.delete", ID: input.ID}); err != nil {
					t.Fatal(err)
				}
			case "refresh":
				if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.refresh"}); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			<-finished
			core.wg.Wait()
			snapshot, _ := core.Snapshot(context.Background())
			if operation == "delete" {
				if len(snapshot.UserResources.MCPServers) != 0 {
					t.Fatal("late diagnostic resurrected a deleted MCP server")
				}
			} else if snapshot.UserResources.MCPServers[0].Test.State != "idle" {
				t.Fatalf("late diagnostic overwrote current configuration: %+v", snapshot.UserResources.MCPServers[0].Test)
			}
		})
	}
}
