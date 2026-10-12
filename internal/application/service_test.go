package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/appmodel"
)

func TestCodingFlowRestoresToolHistory(t *testing.T) {
	project := t.TempDir()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "value.txt"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requestMu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer fixture-secret-key" {
			http.Error(writer, "unexpected endpoint or authorization", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		requestMu.Lock()
		requests = append(requests, body)
		step := len(requests)
		requestMu.Unlock()
		writer.Header().Set("Content-Type", "text/event-stream")
		switch step {
		case 1:
			writeToolStream(writer, "call-read", "read_file", map[string]any{"path": "value.txt"})
		case 2:
			writeToolStream(writer, "call-edit", "edit_file", map[string]any{"path": "value.txt", "old_string": "old", "new_string": "new"})
		case 3:
			writeToolStream(writer, "call-exec", "exec_command", map[string]any{"cmd": "cat value.txt", "yield_time_ms": 1000})
		default:
			writeChatEvent(writer, map[string]any{"content": "修改和命令验证完成。"}, "stop")
			fmt.Fprint(writer, "data: [DONE]\n\n")
		}
	}))
	defer server.Close()
	ctx := context.Background()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(ctx) })
	provider := appmodel.Provider{ID: "fixture", Name: "测试服务", Format: appmodel.ChatCompletions, BaseURL: server.URL + "/v1", Model: "fixture-model"}
	if err := service.SaveProvider(ctx, provider, "fixture-secret-key"); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := service.Snapshot(ctx)
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: true}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = service.Snapshot(ctx)
	id := snapshot.Sessions[0].ID
	if err := service.Send(ctx, id, "读取 value.txt，将 old 改为 new，再用命令读取验证。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	completed := waitForIdle(t, service, id)
	if len(completed.Messages) < 2 || len(completed.Messages[1].Tools) != 3 {
		t.Fatalf("tool history missing from UI: %#v", completed.Messages)
	}
	for _, tool := range completed.Messages[1].Tools {
		if tool.Error != "" || strings.Contains(tool.Output, "Error:") {
			t.Fatalf("tool failed: %#v", tool)
		}
	}
	content, err := os.ReadFile(filepath.Join(project, "value.txt"))
	if err != nil || string(content) != "new\n" {
		t.Fatalf("actual file = %q, error = %v", content, err)
	}
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close(ctx)
	if err := restored.Send(ctx, id, "根据刚才的工具结果继续总结。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, restored, id)
	requestMu.Lock()
	defer requestMu.Unlock()
	if len(requests) != 5 {
		t.Fatalf("requests = %d, expected 4 coding turns and 1 resumed turn", len(requests))
	}
	messages, ok := requests[4]["messages"].([]any)
	if !ok {
		t.Fatal("resumed request has no messages")
	}
	toolResults := make(map[string]bool)
	toolCalls := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "tool" {
			id, _ := message["tool_call_id"].(string)
			toolResults[id] = true
		}
		if calls, ok := message["tool_calls"].([]any); ok {
			for _, rawCall := range calls {
				call, _ := rawCall.(map[string]any)
				id, _ := call["id"].(string)
				toolCalls[id] = true
			}
		}
	}
	for _, id := range []string{"call-read", "call-edit", "call-exec"} {
		if !toolResults[id] || !toolCalls[id] {
			t.Fatalf("resumed request lost %s: calls=%v results=%v", id, toolCalls, toolResults)
		}
	}
	state, err := os.ReadFile(filepath.Join(dataDir, "controller.json"))
	if err != nil || strings.Contains(string(state), "fixture-secret-key") {
		t.Fatalf("controller persisted credential or read failed: %v", err)
	}
}

func writeToolStream(writer http.ResponseWriter, id, name string, arguments map[string]any) {
	encoded, _ := json.Marshal(arguments)
	writeChatEvent(writer, map[string]any{"tool_calls": []any{map[string]any{
		"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)},
	}}}, "tool_calls")
	fmt.Fprint(writer, "data: [DONE]\n\n")
}

func writeChatEvent(writer http.ResponseWriter, delta map[string]any, finish string) {
	event := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	encoded, _ := json.Marshal(event)
	fmt.Fprintf(writer, "data: %s\n\n", encoded)
}

func waitForIdle(t *testing.T, service *Service, id string) appmodel.Session {
	t.Helper()
	deadline := time.After(20 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range snapshot.Sessions {
			if session.ID != id {
				continue
			}
			if session.State == appmodel.Failed {
				t.Fatalf("run failed: %s", session.Error)
			}
			if session.ActiveRun == nil && session.State == appmodel.Idle && len(session.Messages) > 0 {
				return session
			}
		}
		select {
		case <-deadline:
			t.Fatal("run did not become idle")
		case <-ticker.C:
		}
	}
}

func TestWorkspaceBrowseUsesProjectBoundary(t *testing.T) {
	project := t.TempDir()
	nested := filepath.Join(project, "src")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	defer service.Close(ctx)
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	adapter := Workspace{Service: service}
	snapshot, err := adapter.Snapshot(ctx, nested)
	if err != nil || len(snapshot.Files) != 1 || snapshot.Files[0].Path != "main.go" || snapshot.Git.IsGit {
		t.Fatalf("nested non-Git browse = %#v, error = %v", snapshot, err)
	}
	preview, err := adapter.ReadFile(ctx, nested, "main.go")
	if err != nil || preview.Text != "package main\n" {
		t.Fatalf("preview = %#v, error = %v", preview, err)
	}
	if _, err := adapter.Snapshot(ctx, t.TempDir()); err == nil {
		t.Fatal("unopened directory allowed")
	}
	if _, err := adapter.ReadFile(ctx, project, "../outside.txt"); err == nil {
		t.Fatal("outside file allowed")
	}
}
