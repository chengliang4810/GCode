//go:build integration

package corerunner

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/model"
	"gcode/internal/plugins"
	"gcode/scripts/plugin-acceptance/acceptance"
)

func modelTool(w http.ResponseWriter, name, args string) {
	w.Header().Set("Content-Type", "text/event-stream")
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "fixture-call", "function": map[string]any{"name": name, "arguments": args}}}}, "finish_reason": "tool_calls"}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
}
func modelText(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": text}, "finish_reason": "stop"}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
}
func put(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func runnerRequest(t *testing.T, root, url string, component acceptance.Component, probe ProbePlan) acceptance.RunRequest {
	t.Helper()
	bundle, err := plugins.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	bundle.ID = "demo@fixture"
	for _, actual := range acceptance.ComponentsForBundle(bundle) {
		if actual.ID == component.ID {
			component = actual
			break
		}
	}
	artifact, err := acceptance.ArtifactDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	request := acceptance.RunRequest{RunID: "fixture-run", PluginID: "demo@fixture", MarketplaceCommit: strings.Repeat("a", 40), SourceCommit: strings.Repeat("b", 40), ArtifactDigest: artifact,
		Platform: runtime.GOOS + "/" + runtime.GOARCH, Model: "fixture/model", Bundle: bundle, Components: []acceptance.Component{component}, Workspace: t.TempDir(), EvidenceDir: t.TempDir(), ProbeDir: t.TempDir(),
		Provider: acceptance.ProviderConfig{Provider: model.Provider{ID: "fixture", Name: "Local HTTP fixture", Protocol: model.Chat, BaseURL: url, APIKey: "private-test-key", Models: []model.Model{{ID: "model"}}}, ModelID: "fixture/model"}}
	plan := Plan{PluginID: request.PluginID, SourceCommit: request.SourceCommit, Probes: []ProbePlan{probe}}
	if component.Kind == "hook" {
		plan.AllowHooks = []string{component.ID}
	}
	if err := saveJSON(filepath.Join(request.ProbeDir, request.PluginID+".json"), plan); err != nil {
		t.Fatal(err)
	}
	return request
}

func runAndValidate(t *testing.T, request acceptance.RunRequest) acceptance.RuntimeEvidence {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	evidence, err := (InProcessRunner{}).Run(ctx, request)
	assertProbeHomesRemoved(t, request.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Probes) != 1 || evidence.Probes[0].Outcome != "passed" {
		t.Fatalf("actual core probe did not pass: %+v", evidence)
	}
	report := acceptance.Report{Denominator: 1, RequiredPasses: 1, Rows: []acceptance.Row{{ID: request.PluginID, MarketplaceCommit: request.MarketplaceCommit, FixedSource: acceptance.FixedSource{Commit: request.SourceCommit},
		ArtifactDigest: request.ArtifactDigest, Platform: request.Platform, Model: request.Model, Source: acceptance.Stage{Status: "verified"}, Install: acceptance.Stage{Status: "passed"},
		Discover: acceptance.Stage{Status: "passed"}, Runtime: acceptance.Stage{Status: "not_tested"}, Components: request.Components}}}
	file := filepath.Join(t.TempDir(), "runtime.json")
	if err := saveJSON(file, evidence); err != nil {
		t.Fatal(err)
	}
	if err := acceptance.ImportEvidence(&report, file); err != nil {
		t.Fatal("actual core trace rejected:", err)
	}
	if report.Passed != 1 {
		t.Fatal("valid deterministic local-core evidence did not meet validator contract")
	}
	return evidence
}

func TestInProcessRunnerInvokesRealSkillAndChecksWrittenFile(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer private-test-key" {
			t.Error("did not use explicitly supplied fixture credential")
		}
		if calls.Add(1) == 1 {
			data, _ := json.Marshal(body)
			if !strings.Contains(string(data), "FIXTURE_SKILL_BODY") {
				t.Error("real skill content did not reach the core model")
			}
			modelTool(w, "Write", `{"file_path":"result.txt","content":"42"}`)
		} else {
			modelText(w, "Business result is 42")
		}
	}))
	defer server.Close()
	root := t.TempDir()
	put(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	put(t, root, "skills/run/SKILL.md", "---\nname: run\ndescription: fixture task\n---\nFIXTURE_SKILL_BODY: write the answer into result.txt.")
	component := acceptance.Component{ID: "skill:demo:run", Kind: "skill", Name: "demo:run", Digest: strings.Repeat("c", 64), Status: "not_tested"}
	expected := "42"
	request := runnerRequest(t, root, server.URL, component, ProbePlan{ID: "skill-file", ComponentID: component.ID, Prompt: "/demo:run", Assertions: []Assertion{{Kind: "file", Path: "result.txt", Equals: &expected}}})
	evidence := runAndValidate(t, request)
	data, err := os.ReadFile(evidence.Probes[0].Log.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-test-key") {
		t.Fatal("credential leaked into evidence")
	}
	if calls.Load() < 2 {
		t.Fatal("did not exercise the real model-tool-model loop")
	}
}

func TestInProcessRunnerMatchesExactMCPServerAndCallsRealHTTPTool(t *testing.T) {
	var toolCalls, modelCalls atomic.Int32
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": request.Params["protocolVersion"], "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo a fixture value", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}}}}}}
		case "tools/call":
			toolCalls.Add(1)
			if request.Params["name"] != "echo" {
				t.Error("wrong MCP tool")
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "echo-proof"}}}
		default:
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer mcpServer.Close()
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if modelCalls.Add(1) == 1 {
			modelTool(w, "mcp__plugin_demo_fixture__echo", `{"message":"echo-proof"}`)
		} else {
			modelText(w, "Echo complete")
		}
	}))
	defer modelServer.Close()
	root := t.TempDir()
	put(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	config, _ := json.Marshal(map[string]any{"fixture": map[string]any{"type": "http", "url": mcpServer.URL}})
	put(t, root, ".mcp.json", string(config))
	component := acceptance.Component{ID: "mcp:fixture", Kind: "mcp", Name: "fixture", Digest: strings.Repeat("c", 64), Status: "not_tested"}
	request := runnerRequest(t, root, modelServer.URL, component, ProbePlan{ID: "mcp-echo", ComponentID: component.ID, Prompt: "Call the fixture MCP echo tool with echo-proof.", MCPTool: "echo",
		Assertions: []Assertion{{Kind: "text", Source: "tool", ToolName: "mcp__plugin_demo_fixture__echo", Contains: "echo-proof"}}})
	runAndValidate(t, request)
	if toolCalls.Load() != 1 {
		t.Fatalf("real MCP tools/call count=%d", toolCalls.Load())
	}
}

func TestInProcessRunnerDoesNotAcceptModelClaimWithoutFileEffect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { modelText(w, "PASS: I wrote the requested file") }))
	defer server.Close()
	root := t.TempDir()
	put(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	put(t, root, "skills/run/SKILL.md", "---\nname: run\n---\nWrite a result file.")
	component := acceptance.Component{ID: "skill:demo:run", Kind: "skill", Name: "demo:run", Digest: strings.Repeat("c", 64)}
	request := runnerRequest(t, root, server.URL, component, ProbePlan{ID: "claim", ComponentID: component.ID, Prompt: "/demo:run", Assertions: []Assertion{{Kind: "file-exists", Path: "result.txt"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	evidence, err := (InProcessRunner{}).Run(ctx, request)
	assertProbeHomesRemoved(t, request.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Probes) != 1 || evidence.Probes[0].Outcome == "passed" {
		t.Fatal("model's unsupported PASS claim became business acceptance")
	}
}

func assertProbeHomesRemoved(t *testing.T, workspace string) {
	t.Helper()
	homes, err := filepath.Glob(filepath.Join(workspace, "probe-*", "home"))
	if err != nil || len(homes) != 0 {
		t.Fatal("successful or failed probe retained private credential storage:", homes, err)
	}
	if err := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "private-test-key") {
			return fmt.Errorf("copied test credential remains in an owned artifact")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInProcessRunnerRedactsProviderFailureAndRemovesFailedPrivateHome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "account rejected private-test-key")
	}))
	defer server.Close()
	root := t.TempDir()
	put(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	put(t, root, "skills/run/SKILL.md", "---\nname: run\n---\nComplete a fixture task.")
	component := acceptance.Component{ID: "skill:demo:run", Kind: "skill", Name: "demo:run"}
	request := runnerRequest(t, root, server.URL, component, ProbePlan{ID: "provider-error", ComponentID: component.ID, Prompt: "/demo:run",
		Assertions: []Assertion{{Kind: "text", Contains: "completed"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	evidence, err := (InProcessRunner{}).Run(ctx, request)
	if err != nil || len(evidence.Probes) != 1 || evidence.Probes[0].Outcome != "failed" {
		t.Fatalf("provider failure was not recorded: %+v %v", evidence, err)
	}
	if strings.Contains(evidence.Probes[0].Error, "private-test-key") || !strings.Contains(evidence.Probes[0].Error, "401") {
		t.Fatal("provider failure lost its useful cause or exposed the credential")
	}
	assertProbeHomesRemoved(t, request.Workspace)
}

func TestAcceptanceHookHelper(t *testing.T) {
	for i, arg := range os.Args {
		if arg == "--acceptance-hook-helper" && i+1 < len(os.Args) {
			value := map[string]string{"home": os.Getenv("HOME"), "inheritedSecret": os.Getenv("ACCEPTANCE_AMBIENT_SECRET")}
			data, _ := json.Marshal(value)
			if err := os.WriteFile(os.Args[i+1], data, 0600); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
}

func TestInProcessRunnerRunsExactHookWithPrivateHomeAndActualExitCode(t *testing.T) {
	t.Setenv("ACCEPTANCE_AMBIENT_SECRET", "do-not-inherit")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { modelText(w, "HOOK business task completed") }))
	defer server.Close()
	root := t.TempDir()
	put(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	hooks := map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{
		"type": "command", "command": binary, "args": []string{"-test.run=^TestAcceptanceHookHelper$", "--", "--acceptance-hook-helper", "${CLAUDE_PROJECT_DIR}/hook-proof.txt"},
	}}}}}}
	data, _ := json.Marshal(hooks)
	put(t, root, "hooks/hooks.json", string(data))
	component := acceptance.Component{ID: "hook:SessionStart/0/0", Kind: "hook", Name: "SessionStart/0/0", Digest: strings.Repeat("c", 64)}
	code := 0
	request := runnerRequest(t, root, server.URL, component, ProbePlan{ID: "hook-helper", ComponentID: component.ID, Prompt: "Complete the hook fixture task.",
		Assertions: []Assertion{{Kind: "file", Path: "hook-proof.txt", Contains: `"inheritedSecret":""`}, {Kind: "exit", ExitCode: &code}}})
	runAndValidate(t, request)
	if os.Getenv("ACCEPTANCE_AMBIENT_SECRET") != "do-not-inherit" {
		t.Fatal("probe environment did not restore the invoking process")
	}
}

func TestInProcessRunnerUsesRealConfiguredChildAgent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			modelTool(w, "Agent", `{"description":"Write fixture answer","prompt":"Write CHILD_FIXTURE into child.txt","subagent_type":"demo:worker"}`)
		case 2:
			modelTool(w, "Write", `{"file_path":"child.txt","content":"CHILD_FIXTURE"}`)
		case 3:
			modelText(w, "CHILD TASK FINISHED")
		default:
			modelText(w, "PARENT TASK FINISHED")
		}
	}))
	defer server.Close()
	root := t.TempDir()
	put(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	put(t, root, "agents/worker.md", "---\nname: worker\ndescription: Complete an independent fixture task\n---\nWrite the requested fixture file.")
	component := acceptance.Component{ID: "agent:demo:worker", Kind: "agent", Name: "demo:worker", Digest: strings.Repeat("c", 64)}
	expected := "CHILD_FIXTURE"
	request := runnerRequest(t, root, server.URL, component, ProbePlan{ID: "child-writer", ComponentID: component.ID, Prompt: "Delegate to demo:worker to write CHILD_FIXTURE into child.txt.",
		Assertions: []Assertion{{Kind: "file", Path: "child.txt", Equals: &expected}}})
	runAndValidate(t, request)
	if calls.Load() < 4 {
		t.Fatal("did not exercise independent parent and child model/tool loops")
	}
}
