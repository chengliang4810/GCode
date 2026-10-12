package application

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"gcode/internal/mcp"
	"gcode/internal/sessions"
	"gcode/internal/tools"
)

// TestServiceExtensionsIntegration covers the application boundary that the
// runtime-only extension tests cannot reach: Service -> Controller -> Runtime
// -> the real Chat Completions provider. The provider is a local HTTP fixture;
// MCP, plugin, Hook, Skill and specialist commands all use real subprocesses.
func TestServiceExtensionsIntegration(t *testing.T) {
	project := t.TempDir()
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".gcode"), 0o700); err != nil {
		t.Fatal(err)
	}

	pluginLog := filepath.Join(t.TempDir(), "plugin.jsonl")
	hookLog := filepath.Join(t.TempDir(), "hook.jsonl")
	userMCPPID := filepath.Join(t.TempDir(), "user-mcp.pid")
	projectMCPPID := filepath.Join(t.TempDir(), "project-mcp.pid")
	userMCPDone := userMCPPID + ".done"
	projectMCPDone := projectMCPPID + ".done"
	secret := "secret-application-provider-key"

	writeApplicationFile(t, filepath.Join(dataDir, "skills", "runtime-skill", "SKILL.md"), `---
name: runtime-skill
description: Application extension integration skill
---

Application skill body reached the tool.
`)
	writeApplicationFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Application extension child
tools:
  - "read-only"
---

Application child specialist prompt.
`)
	writeApplicationFile(t, filepath.Join(dataDir, "specialists", "runtime-warning.md"), `---
name: runtime-warning
description: Application specialist warning fixture
unexpected: warning
---

Application specialist warning body.
`)
	writeApplicationFile(t, filepath.Join(dataDir, "plugins", "application-runtime-test", "plugin.json"), fmt.Sprintf(`{
  "schemaVersion": 1,
  "id": "application-runtime-test",
  "name": "Application runtime test plugin",
  "version": "1.0.0",
  "enabled": true,
  "tools": [{
    "name": "plugin_runtime_echo",
    "description": "Application runtime plugin tool",
    "command": %q,
    "args": ["-test.run=TestApplicationPluginHelper", "--", %q, "plugin-helper"],
    "inputSchema": {
      "type": "object",
      "properties": {"message": {"type": "string"}},
      "required": ["message"]
    },
    "permission": "prompt"
  }]
}`, os.Args[0], pluginLog))
	// A malformed plugin must become a visible warning without disabling the
	// valid plugin in the same user root.
	writeApplicationFile(t, filepath.Join(dataDir, "plugins", "broken-plugin", "plugin.json"), "{not-json")

	// The malformed user layer must be diagnosed while the valid project layer
	// remains active and executes the afterTool child process.
	writeApplicationFile(t, filepath.Join(dataDir, "hooks.json"), "{not-json")
	writeApplicationFile(t, filepath.Join(project, ".gcode", "hooks.json"), fmt.Sprintf(`{
  "enabled": true,
  "hooks": [{
    "id": "application-after-tool",
    "event": "afterTool",
    "matcher": "*",
    "command": %q,
    "args": ["-test.run=TestApplicationHookHelper", "--", %q, "hook-helper"]
  }]
}`, os.Args[0], hookLog))

	writeApplicationFile(t, filepath.Join(dataDir, "config.json"), fmt.Sprintf(`{
  "mcp": {
    "servers": {
      "exa": {"disabled": true},
      "user-demo": {
        "type": "stdio",
        "command": %q,
        "args": ["-test.run=TestApplicationMCPHelper", "--", "mcp-helper", "user-demo", %q, %q],
        "env": {"GCODE_APPLICATION_MCP_HELPER": "1"}
      }
    }
  }
}`, os.Args[0], userMCPPID, userMCPDone))
	writeApplicationFile(t, filepath.Join(project, ".gcode", "config.json"), fmt.Sprintf(`{
  "mcp": {
    "servers": {
      "project-demo": {
        "type": "stdio",
        "command": %q,
        "args": ["-test.run=TestApplicationMCPHelper", "--", "mcp-helper", "project-demo", %q, %q],
        "env": {"GCODE_APPLICATION_MCP_HELPER": "1"}
      }
    }
  }
}`, os.Args[0], projectMCPPID, projectMCPDone))

	var requestMu sync.Mutex
	var requests []applicationProviderRequest
	parentStep := 0
	childRequests := 0
	protocolErrors := make(chan error, 8)
	providerServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+secret {
			protocolErrors <- fmt.Errorf("unexpected provider request: %s %s auth=%q", request.Method, request.URL.Path, request.Header.Get("Authorization"))
			http.Error(writer, "unexpected endpoint or authorization", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			protocolErrors <- fmt.Errorf("read provider request: %w", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			protocolErrors <- fmt.Errorf("decode provider request: %w", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		isChild := strings.Contains(string(body), "# Specialist Invocation")
		requestMu.Lock()
		step := 0
		if isChild {
			childRequests++
		} else {
			parentStep++
			step = parentStep
		}
		requests = append(requests, applicationProviderRequest{Body: decoded, Raw: append([]byte(nil), body...), Child: isChild, Step: step})
		requestMu.Unlock()

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		if isChild {
			writeChatEvent(writer, map[string]any{"content": "application child final"}, "stop")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
			return
		}
		switch step {
		case 1:
			writeToolStream(writer, "application-plugin-allow", "plugin_runtime_echo", map[string]any{"message": "application-plugin"})
		case 2:
			writeToolStream(writer, "application-plugin-deny", "plugin_runtime_echo", map[string]any{"message": "application-plugin-denied"})
		case 3:
			writeToolStream(writer, "application-mcp-user", "mcp_user_demo_echo", map[string]any{"message": "application-user"})
		case 4:
			writeToolStream(writer, "application-mcp-user-deny", "mcp_user_demo_echo", map[string]any{"message": "application-user-denied"})
		case 5:
			writeToolStream(writer, "application-mcp-project", "mcp_project_demo_echo", map[string]any{"message": "application-project"})
		case 6:
			writeToolStream(writer, "application-skill", "skill", map[string]any{"name": "runtime-skill"})
		case 7:
			writeToolStream(writer, "application-task", "Task", map[string]any{"name": "runtime-child", "prompt": "child application work", "description": "application child"})
		case 8:
			writeChatEvent(writer, map[string]any{"content": "application extensions complete"}, "stop")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		default:
			protocolErrors <- fmt.Errorf("unexpected parent provider step %d", step)
			http.Error(writer, "unexpected provider step", http.StatusBadRequest)
		}
	}))

	ctx := context.Background()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	cleanupService := true
	t.Cleanup(func() {
		if cleanupService {
			_ = service.Close(ctx)
		}
	})
	provider := appmodel.Provider{
		ID:      "application-fixture",
		Name:    "应用扩展协议替身",
		Format:  appmodel.ChatCompletions,
		BaseURL: providerServer.URL + "/v1",
		Model:   "application-fixture-model",
	}
	if err := service.SaveProvider(ctx, provider, secret); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("project snapshot = %#v, error = %v", snapshot.Projects, err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{
		ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: false, Title: "应用扩展集成",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatalf("session snapshot = %#v, error = %v", snapshot.Sessions, err)
	}
	sessionID := snapshot.Sessions[0].ID
	if snapshot.Sessions[0].FullAccess {
		t.Fatal("integration session unexpectedly enabled full access")
	}
	if err := service.Send(ctx, sessionID, "运行应用扩展集成流程。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}

	for index, expected := range []string{"mcp_user_demo_echo", "mcp_user_demo_echo", "mcp_project_demo_echo"} {
		choice := "allow"
		if index == 1 {
			// The first user MCP call is allowed and the second is deliberately
			// denied. The resulting tool history proves the denied call did not
			// invoke the remote stdio server.
			choice = "deny"
		}
		waitAndReplyApplicationPermission(t, service, sessionID, expected, choice)
	}

	completed := waitForIdle(t, service, sessionID)
	requestSnapshot, parentCount, childCount := applicationProviderRequests(&requestMu, requests, &parentStep, &childRequests)
	if parentCount != 8 || childCount != 1 || len(requestSnapshot) != 9 {
		t.Fatalf("provider requests = %d (parents=%d children=%d), want 9 (8+1)", len(requestSnapshot), parentCount, childCount)
	}
	for _, name := range []string{"plugin_runtime_echo", "mcp_user_demo_echo", "mcp_project_demo_echo", "skill", "Task"} {
		if !applicationProviderHasTool(requestSnapshot[0].Body, name) {
			t.Fatalf("first provider request does not advertise %q", name)
		}
	}

	toolCalls := map[string][]appmodel.ToolCall{}
	for _, message := range completed.Messages {
		if message.Role == "assistant" {
			for _, call := range message.Tools {
				toolCalls[call.Name] = append(toolCalls[call.Name], call)
			}
		}
	}
	if len(toolCalls["plugin_runtime_echo"]) != 2 {
		t.Fatalf("plugin tool call history = %#v, want allow and deny calls", toolCalls["plugin_runtime_echo"])
	}
	if !strings.Contains(toolCalls["plugin_runtime_echo"][0].Output, "plugin:application-plugin") {
		t.Fatalf("allowed plugin output = %q", toolCalls["plugin_runtime_echo"][0].Output)
	}
	for name, expected := range map[string]string{
		"mcp_project_demo_echo": "mcp:project-demo:application-project",
		"skill":                 "Application skill body reached the tool.",
		"Task":                  "application child final",
	} {
		if len(toolCalls[name]) != 1 || !strings.Contains(toolCalls[name][0].Output, expected) {
			t.Fatalf("tool %s history = %#v, want output containing %q", name, toolCalls[name], expected)
		}
	}
	if len(toolCalls["mcp_user_demo_echo"]) != 2 || !strings.Contains(toolCalls["mcp_user_demo_echo"][0].Output, "mcp:user-demo:application-user") || !strings.Contains(strings.ToLower(toolCalls["mcp_user_demo_echo"][1].Output), "permission denied") {
		t.Fatalf("denied MCP output = %#v", toolCalls["mcp_user_demo_echo"])
	}

	pluginLines := readApplicationLines(t, pluginLog)
	if len(pluginLines) != 2 {
		t.Fatalf("plugin invocation count = %d, want two successful invocations: %#v", len(pluginLines), pluginLines)
	}
	for index, expected := range []string{"application-plugin", "application-plugin-denied"} {
		var pluginRecord struct {
			Message string `json:"message"`
			Cwd     string `json:"cwd"`
		}
		if err := json.Unmarshal([]byte(pluginLines[index]), &pluginRecord); err != nil {
			t.Fatalf("decode plugin marker: %v", err)
		}
		if pluginRecord.Message != expected || !sameApplicationPath(pluginRecord.Cwd, project) {
			t.Fatalf("plugin record = %#v, want message %q in project cwd", pluginRecord, expected)
		}
	}

	hookLines := readApplicationLines(t, hookLog)
	if len(hookLines) < 5 {
		t.Fatalf("Hook invocation count = %d, want at least five: %#v", len(hookLines), hookLines)
	}
	sawSkillHook := false
	for _, line := range hookLines {
		var record struct {
			Cwd     string `json:"cwd"`
			Payload struct {
				Tool string `json:"tool"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode Hook marker %q: %v", line, err)
		}
		if !sameApplicationPath(record.Cwd, project) {
			t.Fatalf("Hook cwd = %q, want %q", record.Cwd, project)
		}
		if record.Payload.Tool == "skill" {
			sawSkillHook = true
		}
	}
	if !sawSkillHook {
		t.Fatalf("Hook log did not record the Skill call: %#v", hookLines)
	}

	for _, marker := range []string{userMCPPID, projectMCPPID} {
		waitForApplicationFile(t, marker)
	}
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cleanupService = false
	for _, marker := range []string{userMCPPID, projectMCPPID} {
		waitForApplicationProcessExit(t, marker)
	}

	finalSnapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finalSession := applicationSession(finalSnapshot, sessionID)
	if finalSession == nil {
		t.Fatalf("session %s disappeared from final snapshot", sessionID)
	}
	warningCount := 0
	for _, message := range finalSession.Messages {
		if message.Role == "warning" {
			warningCount++
			if strings.Contains(message.Text, secret) {
				t.Fatalf("warning leaked provider key: %q", message.Text)
			}
		}
	}
	if warningCount < 3 {
		t.Fatalf("warning messages = %d, want malformed plugin, malformed Hook and specialist diagnostics: %#v", warningCount, finalSession.Messages)
	}
	finalJSON, _ := json.Marshal(finalSnapshot)
	if strings.Contains(string(finalJSON), secret) {
		t.Fatalf("provider key leaked into application snapshot")
	}

	store := sessions.NewStore(sessions.StoreOptions{RootDir: filepath.Join(dataDir, "sessions")})
	children, err := store.ListChildren(sessionID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(children) != 1 || children[0].ParentSessionID != sessionID || !sameApplicationPath(children[0].Cwd, project) {
		t.Fatalf("child metadata = %#v, want one child bound to parent/project", children)
	}
	childEvents, err := store.ReadRehydratedEvents(children[0].SessionID)
	if err != nil {
		t.Fatalf("Read child events: %v", err)
	}
	if !applicationEventsContainRoleText(childEvents, sessions.EventMessage, "application child final") {
		t.Fatalf("child history does not contain final child answer: %#v", childEvents)
	}

	eventsPath := filepath.Join(dataDir, "sessions", sessionID, sessions.EventsFile)
	eventLines := readApplicationLines(t, eventsPath)
	sawWarningEvent := false
	sawToolResultEvent := false
	for _, line := range eventLines {
		var event struct {
			Type    sessions.EventType `json:"type"`
			Payload json.RawMessage    `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode session event: %v", err)
		}
		if event.Type == sessions.EventToolResult {
			sawToolResultEvent = true
		}
		if event.Type == sessions.EventMessage {
			var payload struct {
				Role string `json:"role"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode message payload: %v", err)
			}
			if payload.Role == "warning" {
				sawWarningEvent = true
			}
		}
	}
	if !sawWarningEvent || !sawToolResultEvent {
		t.Fatalf("events.jsonl missing warning message/tool result: warning=%v tool_result=%v", sawWarningEvent, sawToolResultEvent)
	}
	select {
	case protocolErr := <-protocolErrors:
		t.Fatal(protocolErr)
	default:
	}
}

// TestServiceExtensionChildCancellationOnStopAndClose proves that a child
// specialist remains inside the parent runtime context. Both Stop and Close
// must cancel the child provider request, and Close must release a live MCP
// stdio process before returning.
func TestServiceExtensionChildCancellationOnStopAndClose(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		useMCP    bool
		stop      bool
		keySuffix string
	}{
		{name: "stop", stop: true, keySuffix: "stop"},
		{name: "close", useMCP: true, keySuffix: "close"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			project := t.TempDir()
			dataDir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(project, ".gcode"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeApplicationFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Cancellation child
tools:
  - "read-only"
---

Cancellation child prompt.
`)
			mcpPID := filepath.Join(t.TempDir(), "mcp.pid")
			mcpDone := mcpPID + ".done"
			if testCase.useMCP {
				writeApplicationFile(t, filepath.Join(dataDir, "config.json"), fmt.Sprintf(`{
  "mcp": {"servers": {
    "close-demo": {
      "type": "stdio",
      "command": %q,
      "args": ["-test.run=TestApplicationMCPHelper", "--", "mcp-helper", "close-demo", %q, %q],
      "env": {"GCODE_APPLICATION_MCP_HELPER": "1"}
    }
  }}
}`, os.Args[0], mcpPID, mcpDone))
			}

			childStarted := make(chan struct{})
			childCanceled := make(chan struct{})
			var childStartOnce sync.Once
			var childCancelOnce sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					return
				}
				if strings.Contains(string(body), "# Specialist Invocation") {
					childStartOnce.Do(func() { close(childStarted) })
					<-request.Context().Done()
					childCancelOnce.Do(func() { close(childCanceled) })
					return
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				writeToolStream(writer, "cancellation-task", "Task", map[string]any{
					"name": "runtime-child", "prompt": "wait for cancellation", "description": "cancellation child",
				})
			}))

			ctx := context.Background()
			service, err := New(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			closed := false
			t.Cleanup(func() {
				if !closed {
					_ = service.Close(ctx)
				}
			})
			provider := appmodel.Provider{
				ID:      "application-cancel-" + testCase.keySuffix,
				Name:    "取消协议替身",
				Format:  appmodel.ChatCompletions,
				BaseURL: server.URL + "/v1",
				Model:   "application-cancel-model",
			}
			if err := service.SaveProvider(ctx, provider, "cancel-key-"+testCase.keySuffix); err != nil {
				t.Fatal(err)
			}
			if err := service.AddProject(ctx, project); err != nil {
				t.Fatal(err)
			}
			snapshot, err := service.Snapshot(ctx)
			if err != nil || len(snapshot.Projects) != 1 {
				t.Fatalf("project snapshot = %#v, error = %v", snapshot.Projects, err)
			}
			if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: false}); err != nil {
				t.Fatal(err)
			}
			snapshot, _ = service.Snapshot(ctx)
			sessionID := snapshot.Sessions[0].ID
			if err := service.Send(ctx, sessionID, "启动一个可取消的子代理。", appmodel.Correction); err != nil {
				t.Fatal(err)
			}
			select {
			case <-childStarted:
			case <-time.After(20 * time.Second):
				t.Fatal("child provider request did not start")
			}
			if testCase.useMCP {
				waitForApplicationFile(t, mcpPID)
			}

			if testCase.stop {
				if err := service.Stop(ctx, sessionID); err != nil {
					t.Fatal(err)
				}
				waitForApplicationState(t, service, sessionID, appmodel.Interrupted)
			} else {
				closeResult := make(chan error, 1)
				go func() { closeResult <- service.Close(ctx) }()
				select {
				case err := <-closeResult:
					if err != nil {
						t.Fatalf("Service.Close: %v", err)
					}
				case <-time.After(20 * time.Second):
					t.Fatal("Service.Close did not complete after child cancellation")
				}
				closed = true
			}
			select {
			case <-childCanceled:
			case <-time.After(20 * time.Second):
				t.Fatal("child provider request was not canceled")
			}
			if testCase.stop {
				if err := service.Close(ctx); err != nil {
					t.Fatal(err)
				}
				closed = true
			}
			if testCase.useMCP {
				waitForApplicationProcessExit(t, mcpPID)
			}
		})
	}
}

type applicationProviderRequest struct {
	Body  map[string]any
	Raw   []byte
	Child bool
	Step  int
}

func applicationProviderRequests(mu *sync.Mutex, requests []applicationProviderRequest, parentCount, childCount *int) ([]applicationProviderRequest, int, int) {
	mu.Lock()
	defer mu.Unlock()
	return append([]applicationProviderRequest(nil), requests...), *parentCount, *childCount
}

func applicationProviderHasTool(body map[string]any, name string) bool {
	entries, _ := body["tools"].([]any)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		function, _ := entry["function"].(map[string]any)
		if function["name"] == name {
			return true
		}
	}
	return false
}

func waitAndReplyApplicationPermission(t *testing.T, service *Service, sessionID, tool, choice string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last appmodel.Session
	for time.Now().Before(deadline) {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		session := applicationSession(snapshot, sessionID)
		if session != nil {
			last = *session
			for _, permission := range session.Permissions {
				if permission.Tool != tool {
					continue
				}
				if err := service.ReplyPermission(context.Background(), sessionID, permission.ID, choice); err != nil {
					t.Fatal(err)
				}
				return
			}
			if session.State == appmodel.Failed {
				t.Fatalf("session failed while waiting for %s permission: %s", tool, session.Error)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("permission %s did not become visible; last session = %#v", tool, last)
}

func waitForApplicationState(t *testing.T, service *Service, sessionID string, state appmodel.RunState) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		session := applicationSession(snapshot, sessionID)
		if session != nil && session.State == state && session.ActiveRun == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach %s", sessionID, state)
}

func applicationSession(snapshot appmodel.Snapshot, id string) *appmodel.Session {
	for index := range snapshot.Sessions {
		if snapshot.Sessions[index].ID == id {
			return &snapshot.Sessions[index]
		}
	}
	return nil
}

func applicationEventsContainRoleText(events []sessions.Event, eventType sessions.EventType, expected string) bool {
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		for _, key := range []string{"content", "text", "output"} {
			if value, ok := payload[key].(string); ok && strings.Contains(value, expected) {
				return true
			}
		}
	}
	return false
}

func writeApplicationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readApplicationLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return lines
}

func waitForApplicationFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat %s: %v", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %s did not appear", path)
}

func waitForApplicationProcessExit(t *testing.T, pidPath string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read process marker %s: %v", pidPath, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid process marker %s: %q", pidPath, data)
	}
	for time.Now().Before(deadline) {
		process, findErr := os.FindProcess(pid)
		if findErr != nil {
			return
		}
		signalErr := process.Signal(syscall.Signal(0))
		if signalErr != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("MCP process %d from %s did not exit", pid, pidPath)
}

func sameApplicationPath(left, right string) bool {
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftResolved) == filepath.Clean(rightResolved)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func applicationHelperArg(value string) bool {
	for _, arg := range os.Args {
		if arg == value {
			return true
		}
	}
	return false
}

func applicationHelperArgAfter(marker string, offset int) string {
	for index, arg := range os.Args {
		if arg == marker && index+1+offset < len(os.Args) {
			return os.Args[index+1+offset]
		}
	}
	return ""
}

// TestApplicationMCPHelper is launched by the integration test as a real MCP
// stdio child. It intentionally lives in this package's test binary so the
// production MCP client exercises process startup and shutdown without a
// network dependency.
func TestApplicationMCPHelper(t *testing.T) {
	if !applicationHelperArg("mcp-helper") {
		return
	}
	serverName := applicationHelperArgAfter("mcp-helper", 0)
	pidPath := applicationHelperArgAfter("mcp-helper", 1)
	donePath := applicationHelperArgAfter("mcp-helper", 2)
	if serverName == "" || pidPath == "" || donePath == "" {
		t.Fatal("MCP helper arguments are incomplete")
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	registry.Register(applicationMCPTool{server: serverName})
	serveErr := mcp.Serve(context.Background(), os.Stdin, os.Stdout, registry, mcp.ServeOptions{Name: "gcode-application-test-mcp", Version: "1.0.0"})
	if serveErr != nil && !errors.Is(serveErr, io.EOF) {
		t.Fatal(serveErr)
	}
	_ = os.WriteFile(donePath, []byte("done\n"), 0o600)
}

type applicationMCPTool struct{ server string }

func (tool applicationMCPTool) Name() string { return "echo" }

func (tool applicationMCPTool) Description() string { return "application MCP echo" }

func (tool applicationMCPTool) Parameters() tools.Schema {
	return tools.Schema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"message": {Type: "string"},
		},
		Required:             []string{"message"},
		AdditionalProperties: false,
	}
}

func (tool applicationMCPTool) Safety() tools.Safety {
	return tools.Safety{SideEffect: tools.SideEffectNone, Permission: tools.PermissionAllow}
}

func (tool applicationMCPTool) Run(ctx context.Context, args map[string]any) tools.Result {
	if err := ctx.Err(); err != nil {
		return tools.Result{Status: tools.StatusError, Output: err.Error()}
	}
	cwd, _ := os.Getwd()
	return tools.Result{Status: tools.StatusOK, Output: fmt.Sprintf("mcp:%s:%v:cwd=%s", tool.server, args["message"], cwd)}
}

// TestApplicationPluginHelper is launched by the production plugin runner.
func TestApplicationPluginHelper(t *testing.T) {
	if !applicationHelperArg("plugin-helper") {
		return
	}
	marker := applicationHelperArgAfter("--", 0)
	if marker == "" {
		t.Fatal("plugin marker path is missing")
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal(body, &args); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	markerRecord, err := json.Marshal(map[string]any{"message": args["message"], "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(markerRecord, '\n')); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "plugin:"+fmt.Sprint(args["message"]))
}

// TestApplicationHookHelper is launched by the production Hook dispatcher.
func TestApplicationHookHelper(t *testing.T) {
	if !applicationHelperArg("hook-helper") {
		return
	}
	marker := applicationHelperArgAfter("--", 0)
	if marker == "" {
		t.Fatal("Hook marker path is missing")
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
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
}
