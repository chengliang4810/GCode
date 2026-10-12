package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/appmodel"
	"gcode/internal/mcp"
	"gcode/internal/sessions"
	"gcode/internal/tools"
	"gcode/internal/zeroruntime"
)

// TestRuntimeLoadsAndRunsConfiguredExtensions is deliberately an integration
// test: the provider is scripted, but every extension boundary is real. MCP,
// plugin, and hook commands are separate invocations of this test binary, while
// skills and specialists are loaded from the same DataDir/workspace roots that
// the application runtime uses.
func TestRuntimeLoadsAndRunsConfiguredExtensions(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".gcode"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "plugins", "gcode-runtime-test"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "plugins", "broken-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "skills", "runtime-skill"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "specialists"), 0o700); err != nil {
		t.Fatal(err)
	}

	writeRuntimeFile(t, filepath.Join(dataDir, "skills", "runtime-skill", "SKILL.md"), `---
name: runtime-skill
description: Runtime extension verification skill
---

Runtime skill body reached the tool.
`)
	writeRuntimeFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Runtime child verification specialist
tools:
  - "read-only"
---

Child specialist prompt.
`)
	writeRuntimeFile(t, filepath.Join(dataDir, "specialists", "runtime-warning.md"), `---
name: runtime-warning
description: Runtime specialist warning fixture
unexpected: warning
---

Specialist warning fixture.
`)
	writeRuntimeFile(t, filepath.Join(dataDir, "plugins", "gcode-runtime-test", "plugin.json"), fmt.Sprintf(`{
  "schemaVersion": 1,
  "id": "gcode-runtime-test",
  "name": "Runtime test plugin",
  "version": "1.0.0",
  "enabled": true,
  "tools": [
    {
      "name": "plugin_runtime_echo",
      "description": "runtime test plugin tool",
      "command": %q,
      "args": ["-test.run=TestRuntimePluginHelper", "--", "plugin-helper"],
      "inputSchema": {
        "type": "object",
        "properties": {"message": {"type": "string"}},
        "required": ["message"]
      },
      "permission": "allow"
    }
  ]
}`, os.Args[0]))
	writeRuntimeFile(t, filepath.Join(dataDir, "plugins", "broken-plugin", "plugin.json"), "{not-json")

	hookLog := filepath.Join(t.TempDir(), "hooks.jsonl")
	writeRuntimeFile(t, filepath.Join(dataDir, "hooks.json"), "{not-json")
	writeRuntimeFile(t, filepath.Join(workspace, ".gcode", "hooks.json"), fmt.Sprintf(`{
  "enabled": true,
  "hooks": [
    {
      "id": "runtime-after-tool",
      "event": "afterTool",
      "matcher": "*",
      "command": %q,
      "args": ["-test.run=TestRuntimeHookHelper", "--", %q, "hook-helper"]
    }
  ]
}`, os.Args[0], hookLog))

	writeRuntimeFile(t, filepath.Join(dataDir, "config.json"), fmt.Sprintf(`{
  "mcp": {
    "servers": {
      "exa": {"disabled": true},
      "user-demo": {
        "type": "stdio",
        "command": %q,
        "args": ["-test.run=TestRuntimeMCPServerHelper", "--", "mcp-helper"],
        "env": {"GCODE_RUNTIME_MCP_HELPER": "1"}
      }
    }
  }
}`, os.Args[0]))
	writeRuntimeFile(t, filepath.Join(workspace, ".gcode", "config.json"), fmt.Sprintf(`{
  "mcp": {
    "servers": {
      "project-demo": {
        "type": "stdio",
        "command": %q,
        "args": ["-test.run=TestRuntimeMCPServerHelper", "--", "mcp-helper"],
        "env": {"GCODE_RUNTIME_MCP_HELPER": "1"}
      }
    }
  }
}`, os.Args[0]))

	provider := newRuntimeScriptedProvider()
	engine, err := New(Options{
		DataDir:  dataDir,
		MaxTurns: 10,
		ProviderFactory: func(_ appmodel.Provider, _ string) (agent.Provider, error) {
			return provider, nil
		},
	})
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	if got := engine.DataDir(); got != dataDir {
		t.Fatalf("DataDir = %q, want %q", got, dataDir)
	}
	if !strings.HasPrefix(engine.SessionStore().RootDir, dataDir+string(os.PathSeparator)) {
		t.Fatalf("session store root %q is outside the runtime DataDir %q", engine.SessionStore().RootDir, dataDir)
	}
	const sessionID = "runtime-extensions"
	if _, err := engine.SessionStore().Create(sessions.CreateInput{
		SessionID: sessionID,
		Cwd:       workspace,
		ModelID:   "scripted",
		Provider:  "scripted",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	var warnings []string
	var toolResults []ToolResult
	result, err := engine.Run(context.Background(), Input{
		SessionID:  sessionID,
		Prompt:     "exercise runtime extensions",
		Cwd:        workspace,
		Provider:   appmodel.Provider{ID: "scripted", Model: "scripted"},
		FullAccess: true,
	}, Callbacks{
		OnWarning: func(warning error) {
			if warning != nil {
				warnings = append(warnings, warning.Error())
			}
		},
		OnToolResult: func(toolResult ToolResult) {
			toolResults = append(toolResults, toolResult)
		},
	})
	if err != nil {
		t.Fatalf("runtime.Run: %v", err)
	}
	if result.FinalAnswer != "runtime extensions complete" {
		t.Fatalf("FinalAnswer = %q, want scripted completion", result.FinalAnswer)
	}
	if result.Turns != 6 {
		t.Fatalf("Turns = %d, want six requests (five tools plus final answer)", result.Turns)
	}
	if len(toolResults) != 5 {
		t.Fatalf("tool result count = %d, want five", len(toolResults))
	}

	requests := provider.requestsSnapshot()
	if len(requests) != 7 {
		t.Fatalf("provider request count = %d, want six parent requests plus one child request", len(requests))
	}
	parentRequest := requests[0]
	if !strings.Contains(parentRequest.Messages[0].Content, "runtime-skill") {
		t.Fatalf("system prompt does not advertise the loaded skill: %q", parentRequest.Messages[0].Content)
	}
	for _, expectedTool := range []string{"skill", "plugin_runtime_echo", "mcp_user_demo_echo", "mcp_project_demo_echo", "Task"} {
		if !requestHasTool(parentRequest, expectedTool) {
			t.Fatalf("initial provider request does not expose %q", expectedTool)
		}
	}
	for _, expected := range []string{
		"Runtime skill body reached the tool.",
		"plugin:runtime-plugin",
		"mcp:user-message",
		"mcp:project-message",
		"child-result:child work",
	} {
		if !anyToolResultContains(toolResults, expected) {
			t.Fatalf("tool results do not contain %q: %#v", expected, toolResults)
		}
	}

	if !hasWarningContaining(warnings, "plugin") {
		t.Fatalf("malformed plugin diagnostic was not reported: %#v", warnings)
	}
	if !hasWarningContaining(warnings, "hook") {
		t.Fatalf("malformed hook diagnostic was not reported: %#v", warnings)
	}
	if !hasWarningContaining(warnings, "specialist") {
		t.Fatalf("specialist manifest warning was not reported: %#v", warnings)
	}

	hookLines := readRuntimeLines(t, hookLog)
	if len(hookLines) < 5 {
		t.Fatalf("hook log has %d lines, want at least one afterTool invocation per tool: %#v", len(hookLines), hookLines)
	}
	var sawSkillHook bool
	for _, line := range hookLines {
		var payload struct {
			Cwd     string `json:"cwd"`
			Payload struct {
				Tool string `json:"tool"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("decode hook log line %q: %v", line, err)
		}
		if !sameRuntimePath(payload.Cwd, workspace) {
			t.Fatalf("hook cwd = %q, want runtime workspace %q", payload.Cwd, workspace)
		}
		if payload.Payload.Tool == "skill" {
			sawSkillHook = true
		}
	}
	if !sawSkillHook {
		t.Fatalf("hook log did not record the skill tool: %#v", hookLines)
	}

	childSessions, err := engine.SessionStore().ListChildren(sessionID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(childSessions) != 1 {
		t.Fatalf("child session count = %d, want one", len(childSessions))
	}
	if childSessions[0].Cwd != workspace {
		t.Fatalf("child cwd = %q, want %q", childSessions[0].Cwd, workspace)
	}

	events, err := engine.SessionStore().ReadRehydratedEvents(sessionID)
	if err != nil {
		t.Fatalf("ReadRehydratedEvents: %v", err)
	}
	if !hasSessionEvent(events, sessions.EventToolResult) || !hasSessionEvent(events, sessions.EventMessage) {
		t.Fatalf("session history does not contain persisted messages and tool results: %#v", events)
	}
}

// TestRuntimeSubagentRunChildCancellationAndPermission proves Task uses the
// runtime's RunChild seam rather than a detached external process. It also
// checks that a normal parent mode gives the child only the read-only tool set,
// then cancels a second child while its provider stream is live.
func TestRuntimeSubagentRunChildCancellationAndPermission(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "specialists"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Runtime child verification specialist
tools:
  - "execute"
---

Child specialist prompt.
`)

	t.Run("permission follows parent", func(t *testing.T) {
		provider := newRuntimeScriptedProvider()
		provider.taskOnly = true
		engine := newRuntimeTestEngine(t, dataDir, provider)
		const sessionID = "runtime-child-permission"
		createRuntimeSession(t, engine, sessionID, workspace)
		permissionRequests := 0
		result, err := engine.Run(context.Background(), Input{
			SessionID:      sessionID,
			Prompt:         "delegate a read-only child",
			Cwd:            workspace,
			Provider:       appmodel.Provider{ID: "scripted", Model: "scripted"},
			PermissionMode: "auto",
		}, Callbacks{
			OnPermissionRequest: func(context.Context, PermissionRequest) (string, error) {
				permissionRequests++
				return "allow", nil
			},
		})
		if err != nil {
			t.Fatalf("runtime.Run: %v", err)
		}
		if result.FinalAnswer != "parent completed after child" {
			t.Fatalf("FinalAnswer = %q", result.FinalAnswer)
		}
		if permissionRequests != 0 {
			t.Fatalf("permission request count = %d, want zero for a sandbox-safe Task dispatch", permissionRequests)
		}
		childRequest, ok := provider.childRequestSnapshot()
		if !ok {
			t.Fatal("child provider request was not observed")
		}
		if !requestHasTool(childRequest, "read_file") {
			t.Fatalf("child request omitted read_file: %#v", childRequest.Tools)
		}
		for _, forbidden := range []string{"bash", "write_file", "edit_file", "Task"} {
			if requestHasTool(childRequest, forbidden) {
				t.Fatalf("child request widened parent auto permission with %q", forbidden)
			}
		}
	})

	t.Run("parent cancellation reaches child", func(t *testing.T) {
		provider := newRuntimeScriptedProvider()
		provider.taskOnly = true
		provider.blockChild = true
		provider.childStarted = make(chan struct{})
		provider.childCanceled = make(chan struct{})
		engine := newRuntimeTestEngine(t, dataDir, provider)
		const sessionID = "runtime-child-cancel"
		createRuntimeSession(t, engine, sessionID, workspace)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		resultCh := make(chan struct {
			result Result
			err    error
		}, 1)
		go func() {
			result, err := engine.Run(ctx, Input{
				SessionID:      sessionID,
				Prompt:         "cancel the delegated child",
				Cwd:            workspace,
				Provider:       appmodel.Provider{ID: "scripted", Model: "scripted"},
				PermissionMode: "auto",
			}, Callbacks{
				OnPermissionRequest: func(context.Context, PermissionRequest) (string, error) {
					return "allow", nil
				},
			})
			resultCh <- struct {
				result Result
				err    error
			}{result: result, err: err}
		}()
		select {
		case <-provider.childStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("child provider did not start")
		}
		cancel()
		select {
		case <-provider.childCanceled:
		case <-time.After(5 * time.Second):
			t.Fatal("child provider did not observe context cancellation")
		}
		select {
		case outcome := <-resultCh:
			if !errors.Is(outcome.err, context.Canceled) {
				t.Fatalf("parent error = %v, want context.Canceled", outcome.err)
			}
			if !outcome.result.Interrupted {
				t.Fatal("canceled parent result is not marked Interrupted")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runtime.Run did not return after parent cancellation")
		}
	})
}

func TestRuntimeConfigurationErrorsAreVisible(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	writeRuntimeFile(t, filepath.Join(dataDir, "config.json"), "{invalid-json")
	provider := newRuntimeScriptedProvider()
	engine := newRuntimeTestEngine(t, dataDir, provider)
	createRuntimeSession(t, engine, "runtime-config-error", workspace)
	_, err := engine.Run(context.Background(), Input{
		SessionID: "runtime-config-error",
		Prompt:    "configuration error must be visible",
		Cwd:       workspace,
		Provider:  appmodel.Provider{ID: "scripted", Model: "scripted"},
	}, Callbacks{})
	if err == nil {
		t.Fatal("runtime.Run succeeded with malformed MCP config")
	}
	if !strings.Contains(err.Error(), "resolve GCode MCP config") {
		t.Fatalf("configuration error = %q, want MCP resolution context", err)
	}
}

func TestRuntimeReportsSkippedMCPServer(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	missingCommand := filepath.Join(t.TempDir(), "missing-mcp-server")
	writeRuntimeFile(t, filepath.Join(dataDir, "config.json"), fmt.Sprintf(`{
  "mcp": {
    "servers": {
      "exa": {"disabled": true},
      "broken-demo": {"type": "stdio", "command": %q}
    }
  }
}`, missingCommand))
	engine, err := New(Options{
		DataDir: dataDir,
		ProviderFactory: func(_ appmodel.Provider, _ string) (agent.Provider, error) {
			return runtimeFinalProvider{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	createRuntimeSession(t, engine, "runtime-mcp-warning", workspace)
	var warnings []string
	result, err := engine.Run(context.Background(), Input{
		SessionID:  "runtime-mcp-warning",
		Prompt:     "report skipped MCP",
		Cwd:        workspace,
		Provider:   appmodel.Provider{ID: "scripted", Model: "scripted"},
		FullAccess: true,
	}, Callbacks{OnWarning: func(warning error) {
		if warning != nil {
			warnings = append(warnings, warning.Error())
		}
	}})
	if err != nil {
		t.Fatalf("runtime.Run: %v", err)
	}
	if result.FinalAnswer != "runtime final" {
		t.Fatalf("FinalAnswer = %q", result.FinalAnswer)
	}
	if !hasWarningContaining(warnings, "MCP server broken-demo was skipped") {
		t.Fatalf("skipped MCP server warning was not reported: %#v", warnings)
	}
}

func TestRuntimeSpecialistLoadFailureIsVisibleAndRedacted(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "specialist-secret")
	workspace := t.TempDir()
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "specialists"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{
		DataDir: dataDir,
		ProviderFactory: func(_ appmodel.Provider, _ string) (agent.Provider, error) {
			return runtimeFinalProvider{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	createRuntimeSession(t, engine, "runtime-specialist-warning", workspace)
	if err := engine.SetCredential("scripted", "specialist-secret"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	var warnings []string
	result, err := engine.Run(context.Background(), Input{
		SessionID: "runtime-specialist-warning",
		Prompt:    "specialist load failure must be visible",
		Cwd:       workspace,
		Provider:  appmodel.Provider{ID: "scripted", Model: "scripted"},
	}, Callbacks{OnWarning: func(warning error) {
		if warning != nil {
			warnings = append(warnings, warning.Error())
		}
	}})
	if err != nil {
		t.Fatalf("runtime.Run: %v", err)
	}
	if result.FinalAnswer != "runtime final" {
		t.Fatalf("FinalAnswer = %q", result.FinalAnswer)
	}
	if !hasWarningContaining(warnings, "load GCode specialists") {
		t.Fatalf("specialist load warning was not reported: %#v", warnings)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "specialist-secret") {
			t.Fatalf("specialist warning leaked credential or secret path: %q", warning)
		}
	}
}

// TestRuntimeRunErrorIsRedacted keeps the runtime's authentication boundary
// observable: the underlying provider error is retained for errors.Is, while
// the public error text cannot leak the credential loaded from DataDir.
func TestRuntimeRunErrorIsRedacted(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	provider := runtimeErrorProvider{err: errors.New("upstream rejected secret-runtime-key")}
	engine, err := New(Options{
		DataDir: dataDir,
		ProviderFactory: func(_ appmodel.Provider, key string) (agent.Provider, error) {
			if key != "secret-runtime-key" {
				return nil, fmt.Errorf("provider factory key = %q, want credential", key)
			}
			return provider, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	createRuntimeSession(t, engine, "runtime-error-redaction", workspace)
	if err := engine.SetCredential("scripted", "secret-runtime-key"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	_, err = engine.Run(context.Background(), Input{
		SessionID: "runtime-error-redaction",
		Prompt:    "provider error",
		Cwd:       workspace,
		Provider:  appmodel.Provider{ID: "scripted", Model: "scripted"},
	}, Callbacks{})
	if err == nil {
		t.Fatal("runtime.Run succeeded despite provider error")
	}
	if strings.Contains(err.Error(), "secret-runtime-key") {
		t.Fatalf("runtime error leaked credential: %q", err)
	}
	if !strings.Contains(err.Error(), "upstream rejected") {
		t.Fatalf("runtime error lost provider context: %q", err)
	}
	if !errors.Is(err, provider.err) {
		t.Fatalf("runtime error does not unwrap provider error: %v", err)
	}
}

// TestRuntimeMCPServerHelper is executed in a real child process by the MCP
// integration test. The helper uses the production MCP stdio server protocol;
// the parent therefore exercises process startup, initialize, tools/list, and
// tools/call through the production MCP client.
func TestRuntimeMCPServerHelper(t *testing.T) {
	if !runtimeHelperArg("mcp-helper") {
		return
	}
	registry := tools.NewRegistry()
	registry.Register(runtimeMCPTestTool{})
	if err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, registry, mcp.ServeOptions{
		Name:    "gcode-runtime-test-mcp",
		Version: "1.0.0",
	}); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("MCP helper: %v", err)
	}
	os.Exit(0)
}

func TestRuntimePluginHelper(t *testing.T) {
	if !runtimeHelperArg("plugin-helper") {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "plugin:%v:cwd=%s\n", args["message"], cwd)
	os.Exit(0)
}

func TestRuntimeHookHelper(t *testing.T) {
	if !runtimeHelperArg("hook-helper") {
		return
	}
	marker := runtimeHelperArgAfter("--", 0)
	if marker == "" {
		t.Fatal("hook helper log path is missing")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	record, err := json.Marshal(map[string]any{"cwd": cwd, "payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(record, '\n')); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

type runtimeMCPTestTool struct{}

func (runtimeMCPTestTool) Name() string { return "echo" }

func (runtimeMCPTestTool) Description() string { return "runtime MCP echo" }

func (runtimeMCPTestTool) Parameters() tools.Schema {
	return tools.Schema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"message": {Type: "string"},
		},
		Required:             []string{"message"},
		AdditionalProperties: false,
	}
}

func (runtimeMCPTestTool) Safety() tools.Safety {
	return tools.Safety{SideEffect: tools.SideEffectNone, Permission: tools.PermissionAllow}
}

func (runtimeMCPTestTool) Run(ctx context.Context, args map[string]any) tools.Result {
	if err := ctx.Err(); err != nil {
		return tools.Result{Status: tools.StatusError, Output: err.Error()}
	}
	cwd, _ := os.Getwd()
	return tools.Result{Status: tools.StatusOK, Output: fmt.Sprintf("mcp:%v:cwd=%s", args["message"], cwd)}
}

type runtimeScriptedProvider struct {
	mu            sync.Mutex
	requests      []zeroruntime.CompletionRequest
	parentCalls   int
	childRequest  *zeroruntime.CompletionRequest
	taskOnly      bool
	blockChild    bool
	childStarted  chan struct{}
	childCanceled chan struct{}
	childStart    sync.Once
	childCancel   sync.Once
}

func newRuntimeScriptedProvider() *runtimeScriptedProvider {
	return &runtimeScriptedProvider{}
}

func (provider *runtimeScriptedProvider) StreamCompletion(ctx context.Context, request zeroruntime.CompletionRequest) (<-chan zeroruntime.StreamEvent, error) {
	snapshot := cloneRuntimeCompletionRequest(request)
	isChild := runtimeRequestIsChild(snapshot)
	provider.mu.Lock()
	provider.requests = append(provider.requests, snapshot)
	step := provider.parentCalls
	if isChild {
		provider.childRequest = &snapshot
		if provider.childStarted != nil {
			provider.childStart.Do(func() { close(provider.childStarted) })
		}
	} else {
		provider.parentCalls++
	}
	blockChild := provider.blockChild && isChild
	provider.mu.Unlock()

	events := make(chan zeroruntime.StreamEvent, 8)
	go func() {
		defer close(events)
		if blockChild {
			select {
			case <-ctx.Done():
				if provider.childCanceled != nil {
					provider.childCancel.Do(func() { close(provider.childCanceled) })
				}
				return
			}
		}
		var scripted []zeroruntime.StreamEvent
		if isChild {
			scripted = []zeroruntime.StreamEvent{
				{Type: zeroruntime.StreamEventText, Content: "child-result:child work"},
				{Type: zeroruntime.StreamEventDone},
			}
		} else {
			scripted = provider.parentEvents(step, snapshot)
		}
		for _, event := range scripted {
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

func (provider *runtimeScriptedProvider) parentEvents(step int, request zeroruntime.CompletionRequest) []zeroruntime.StreamEvent {
	call := func(name string, arguments string) []zeroruntime.StreamEvent {
		return []zeroruntime.StreamEvent{
			{Type: zeroruntime.StreamEventToolCallStart, ToolCallID: fmt.Sprintf("runtime-call-%d", step), ToolName: name},
			{Type: zeroruntime.StreamEventToolCallDelta, ToolCallID: fmt.Sprintf("runtime-call-%d", step), ArgumentsFragment: arguments},
			{Type: zeroruntime.StreamEventToolCallEnd, ToolCallID: fmt.Sprintf("runtime-call-%d", step)},
			{Type: zeroruntime.StreamEventDone},
		}
	}
	if provider.taskOnly {
		if step == 0 {
			return call("Task", `{"name":"runtime-child","prompt":"child work","description":"runtime child"}`)
		}
		return []zeroruntime.StreamEvent{
			{Type: zeroruntime.StreamEventText, Content: "parent completed after child"},
			{Type: zeroruntime.StreamEventDone},
		}
	}
	if step > 0 {
		last := lastRuntimeToolContent(request)
		want := []string{
			"Runtime skill body reached the tool.",
			"plugin:runtime-plugin",
			"mcp:user-message",
			"mcp:project-message",
			"child-result:child work",
		}
		if step-1 < len(want) && !strings.Contains(last, want[step-1]) {
			return []zeroruntime.StreamEvent{{Type: zeroruntime.StreamEventError, Error: fmt.Sprintf("scripted provider expected tool result %q, got %q", want[step-1], last)}}
		}
	}
	switch step {
	case 0:
		return call("skill", `{"name":"runtime-skill"}`)
	case 1:
		return call("plugin_runtime_echo", `{"message":"runtime-plugin"}`)
	case 2:
		return call("mcp_user_demo_echo", `{"message":"user-message"}`)
	case 3:
		return call("mcp_project_demo_echo", `{"message":"project-message"}`)
	case 4:
		return call("Task", `{"name":"runtime-child","prompt":"child work","description":"runtime child"}`)
	default:
		return []zeroruntime.StreamEvent{
			{Type: zeroruntime.StreamEventText, Content: "runtime extensions complete"},
			{Type: zeroruntime.StreamEventDone},
		}
	}
}

func (provider *runtimeScriptedProvider) requestsSnapshot() []zeroruntime.CompletionRequest {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]zeroruntime.CompletionRequest(nil), provider.requests...)
}

func (provider *runtimeScriptedProvider) childRequestSnapshot() (zeroruntime.CompletionRequest, bool) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.childRequest == nil {
		return zeroruntime.CompletionRequest{}, false
	}
	return cloneRuntimeCompletionRequest(*provider.childRequest), true
}

type runtimeErrorProvider struct{ err error }

func (provider runtimeErrorProvider) StreamCompletion(context.Context, zeroruntime.CompletionRequest) (<-chan zeroruntime.StreamEvent, error) {
	return nil, provider.err
}

type runtimeFinalProvider struct{}

func (runtimeFinalProvider) StreamCompletion(ctx context.Context, _ zeroruntime.CompletionRequest) (<-chan zeroruntime.StreamEvent, error) {
	events := make(chan zeroruntime.StreamEvent, 2)
	if err := ctx.Err(); err != nil {
		close(events)
		return events, nil
	}
	events <- zeroruntime.StreamEvent{Type: zeroruntime.StreamEventText, Content: "runtime final"}
	events <- zeroruntime.StreamEvent{Type: zeroruntime.StreamEventDone}
	close(events)
	return events, nil
}

func newRuntimeTestEngine(t *testing.T, dataDir string, provider *runtimeScriptedProvider) *Engine {
	t.Helper()
	engine, err := New(Options{
		DataDir:  dataDir,
		MaxTurns: 8,
		ProviderFactory: func(_ appmodel.Provider, _ string) (agent.Provider, error) {
			return provider, nil
		},
	})
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func createRuntimeSession(t *testing.T, engine *Engine, sessionID string, workspace string) {
	t.Helper()
	if _, err := engine.SessionStore().Create(sessions.CreateInput{
		SessionID: sessionID,
		Cwd:       workspace,
		ModelID:   "scripted",
		Provider:  "scripted",
	}); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

func writeRuntimeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runtimeHelperArg(value string) bool {
	for _, arg := range os.Args {
		if arg == value {
			return true
		}
	}
	return false
}

func runtimeHelperArgAfter(marker string, offset int) string {
	for index, arg := range os.Args {
		if arg == marker && index+1+offset < len(os.Args) {
			return os.Args[index+1+offset]
		}
	}
	return ""
}

func cloneRuntimeCompletionRequest(request zeroruntime.CompletionRequest) zeroruntime.CompletionRequest {
	clone := request
	clone.Messages = append([]zeroruntime.Message(nil), request.Messages...)
	for index := range clone.Messages {
		clone.Messages[index].ToolCalls = append([]zeroruntime.ToolCall(nil), request.Messages[index].ToolCalls...)
		clone.Messages[index].Reasoning = append([]zeroruntime.ReasoningBlock(nil), request.Messages[index].Reasoning...)
		clone.Messages[index].Images = append([]zeroruntime.ImageBlock(nil), request.Messages[index].Images...)
	}
	clone.Tools = append([]zeroruntime.ToolDefinition(nil), request.Tools...)
	return clone
}

func runtimeRequestIsChild(request zeroruntime.CompletionRequest) bool {
	for _, message := range request.Messages {
		if strings.Contains(message.Content, "# Specialist Invocation") {
			return true
		}
	}
	return false
}

func lastRuntimeToolContent(request zeroruntime.CompletionRequest) string {
	for index := len(request.Messages) - 1; index >= 0; index-- {
		if request.Messages[index].Role == zeroruntime.MessageRoleTool {
			return request.Messages[index].Content
		}
	}
	return ""
}

func requestHasTool(request zeroruntime.CompletionRequest, name string) bool {
	for _, tool := range request.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func anyToolResultContains(results []ToolResult, expected string) bool {
	for _, result := range results {
		if strings.Contains(result.Output, expected) {
			return true
		}
	}
	return false
}

func hasWarningContaining(warnings []string, expected string) bool {
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning), strings.ToLower(expected)) {
			return true
		}
	}
	return false
}

func readRuntimeLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			lines = append(lines, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return lines
}

func hasSessionEvent(events []sessions.Event, kind sessions.EventType) bool {
	for _, event := range events {
		if event.Type == kind {
			return true
		}
	}
	return false
}

func sameRuntimePath(left string, right string) bool {
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftResolved) == filepath.Clean(rightResolved)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
