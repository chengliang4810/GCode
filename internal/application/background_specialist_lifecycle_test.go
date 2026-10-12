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

type backgroundSpecialistFixture struct {
	mu             sync.Mutex
	taskID         string
	queryOutput    string
	stopOutput     string
	childStarted   chan struct{}
	childCanceled  chan struct{}
	childStartOnce sync.Once
	childStopOnce  sync.Once
}

func newBackgroundSpecialistFixture() *backgroundSpecialistFixture {
	return &backgroundSpecialistFixture{
		childStarted:  make(chan struct{}),
		childCanceled: make(chan struct{}),
	}
}

func (fixture *backgroundSpecialistFixture) handler(writer http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(writer, "invalid provider request", http.StatusBadRequest)
		return
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "# Specialist Invocation") {
		fixture.childStartOnce.Do(func() { close(fixture.childStarted) })
		writer.Header().Set("Content-Type", "text/event-stream")
		writeChatEvent(writer, map[string]any{"content": "background progress"}, "")
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		<-request.Context().Done()
		fixture.childStopOnce.Do(func() { close(fixture.childCanceled) })
		return
	}

	lastUser := lastUserContent(body)
	hasToolResult := requestHasRole(body, "tool")
	writer.Header().Set("Content-Type", "text/event-stream")
	switch {
	case strings.Contains(lastUser, "启动跨 parent 后台任务") && !hasToolResult:
		writeToolStream(writer, "call-background-task", "Task", map[string]any{
			"name":              "runtime-child",
			"prompt":            strings.Repeat("保持后台 specialist 运行。", 400),
			"description":       "跨 parent lifecycle",
			"run_in_background": true,
		})
	case strings.Contains(lastUser, "启动跨 parent 后台任务") && hasToolResult:
		fixture.setTaskID(extractTaskID(body))
		writeChatEvent(writer, map[string]any{"content": "parent A 已完成。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	case strings.Contains(lastUser, "查询跨 parent 后台任务") && !hasToolResult:
		writeToolStream(writer, "call-background-output", "TaskOutput", map[string]any{
			"task_id": fixture.getTaskID(),
		})
	case strings.Contains(lastUser, "查询跨 parent 后台任务") && hasToolResult:
		fixture.setQueryOutput(extractToolOutput(body))
		writeChatEvent(writer, map[string]any{"content": "parent B 已读取后台输出。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	case strings.Contains(lastUser, "停止跨 parent 后台任务") && !hasToolResult:
		writeToolStream(writer, "call-background-stop", "TaskStop", map[string]any{
			"task_id": fixture.getTaskID(),
		})
	case strings.Contains(lastUser, "停止跨 parent 后台任务") && hasToolResult:
		fixture.setStopOutput(extractToolOutput(body))
		writeChatEvent(writer, map[string]any{"content": "parent B 已停止后台任务。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	default:
		http.Error(writer, "unexpected provider request", http.StatusBadRequest)
	}
}

func TestServiceBackgroundSpecialistSurvivesParentAndStopsAcrossParents(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	project := t.TempDir()
	dataDir := t.TempDir()
	writeApplicationFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Long lived background child
tools:
  - "read-only"
---

Background specialist fixture.
`)
	fixture := newBackgroundSpecialistFixture()
	server := httptest.NewServer(http.HandlerFunc(fixture.handler))
	defer server.Close()

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
		ID:      "background-lifecycle",
		Name:    "后台生命周期替身",
		Format:  appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1",
		Model:   "background-lifecycle-model",
	}
	if err := service.SaveProvider(ctx, provider, "background-lifecycle-key"); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("projects = %#v, error = %v", snapshot.Projects, err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: false, Title: "parent A"}); err != nil {
		t.Fatal(err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: false, Title: "parent B"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 2 {
		t.Fatalf("sessions = %#v, error = %v", snapshot.Sessions, err)
	}
	parentA := snapshot.Sessions[0].ID
	parentB := snapshot.Sessions[1].ID
	if err := service.Send(ctx, parentA, "启动跨 parent 后台任务。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, service, parentA)
	waitForBackgroundChannel(t, fixture.childStarted, "background child start")
	taskID := fixture.getTaskID()
	if taskID == "" {
		t.Fatal("parent A did not return a background task id")
	}

	if err := service.Send(ctx, parentB, "查询跨 parent 后台任务。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, service, parentB)
	queryOutput := fixture.getQueryOutput()
	if !strings.Contains(queryOutput, "status: running") || !strings.Contains(queryOutput, "background progress") {
		t.Fatalf("cross-parent TaskOutput = %q", queryOutput)
	}

	if err := service.Send(ctx, parentB, "停止跨 parent 后台任务。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdleAfterOptionalTaskStopPermission(t, service, parentB)
	stopOutput := fixture.getStopOutput()
	if !strings.Contains(stopOutput, "status: killed") {
		t.Fatalf("cross-parent TaskStop = %q", stopOutput)
	}
	waitForBackgroundChannel(t, fixture.childCanceled, "background child cancellation")
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	closed = true
}

func TestServiceCloseStopsBackgroundSpecialistAndCleansData(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	project := t.TempDir()
	dataDir := t.TempDir()
	writeApplicationFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Long lived background child
tools:
  - "read-only"
---

Background specialist fixture.
`)
	fixture := newBackgroundSpecialistFixture()
	server := httptest.NewServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
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
		ID:      "background-close",
		Name:    "后台关闭替身",
		Format:  appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1",
		Model:   "background-close-model",
	}
	if err := service.SaveProvider(ctx, provider, "background-close-key"); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("projects = %#v, error = %v", snapshot.Projects, err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: false, Title: "close parent"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = service.Snapshot(ctx)
	parentID := snapshot.Sessions[0].ID
	if err := service.Send(ctx, parentID, "启动跨 parent 后台任务。", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, service, parentID)
	waitForBackgroundChannel(t, fixture.childStarted, "background child start before close")

	closeResult := make(chan error, 1)
	go func() { closeResult <- service.Close(ctx) }()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Service.Close did not complete")
	}
	closed = true
	waitForBackgroundChannel(t, fixture.childCanceled, "background child cancellation on close")
	if err := service.Close(ctx); err != nil {
		t.Fatalf("repeated Service.Close: %v", err)
	}

	backgroundDir := filepath.Join(dataDir, "background")
	entries, err := os.ReadDir(backgroundDir)
	if err != nil {
		t.Fatalf("read background directory: %v", err)
	}
	metadataFound := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		metadataFound = true
		data, readErr := os.ReadFile(filepath.Join(backgroundDir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var task struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(data, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status == "running" {
			t.Fatalf("background task remained running after close: %s", entry.Name())
		}
	}
	if !metadataFound {
		t.Fatal("Service.Close did not leave background task metadata in the Engine DataDir")
	}
}

func (fixture *backgroundSpecialistFixture) setTaskID(taskID string) {
	fixture.mu.Lock()
	fixture.taskID = strings.TrimSpace(taskID)
	fixture.mu.Unlock()
}

func (fixture *backgroundSpecialistFixture) getTaskID() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.taskID
}

func (fixture *backgroundSpecialistFixture) setQueryOutput(output string) {
	fixture.mu.Lock()
	fixture.queryOutput = output
	fixture.mu.Unlock()
}

func (fixture *backgroundSpecialistFixture) getQueryOutput() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.queryOutput
}

func (fixture *backgroundSpecialistFixture) setStopOutput(output string) {
	fixture.mu.Lock()
	fixture.stopOutput = output
	fixture.mu.Unlock()
}

func (fixture *backgroundSpecialistFixture) getStopOutput() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.stopOutput
}

func lastUserContent(body map[string]any) string {
	messages, _ := body["messages"].([]any)
	for index := len(messages) - 1; index >= 0; index-- {
		message, _ := messages[index].(map[string]any)
		if message["role"] != "user" {
			continue
		}
		content, _ := message["content"].(string)
		return content
	}
	return ""
}

func requestHasRole(body map[string]any, role string) bool {
	messages, _ := body["messages"].([]any)
	lastUser := -1
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "user" {
			lastUser = index
		}
	}
	for _, raw := range messages[lastUser+1:] {
		message, _ := raw.(map[string]any)
		if message["role"] == role {
			return true
		}
	}
	return false
}

func extractTaskID(body map[string]any) string {
	output := extractToolOutput(body)
	marker := "task_id:"
	index := strings.Index(output, marker)
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(output[index+len(marker):])
	if lineEnd := strings.IndexAny(value, "\r\n"); lineEnd >= 0 {
		value = value[:lineEnd]
	}
	return strings.TrimSpace(value)
}

func extractToolOutput(body map[string]any) string {
	messages, _ := body["messages"].([]any)
	for index := len(messages) - 1; index >= 0; index-- {
		message, _ := messages[index].(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		content, _ := message["content"].(string)
		return content
	}
	return ""
}

func waitForBackgroundChannel(t *testing.T, channel <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not occur", description)
	}
}

func waitForIdleAfterOptionalTaskStopPermission(t *testing.T, service *Service, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	replied := false
	for time.Now().Before(deadline) {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		session := applicationSession(snapshot, sessionID)
		if session == nil {
			t.Fatal("TaskStop session disappeared")
		}
		if !replied {
			for _, permission := range session.Permissions {
				if permission.Tool != "TaskStop" {
					continue
				}
				if err := service.ReplyPermission(context.Background(), sessionID, permission.ID, "allow"); err != nil {
					t.Fatal(err)
				}
				replied = true
				break
			}
		}
		if session.State == appmodel.Failed {
			t.Fatalf("TaskStop session failed: %s", session.Error)
		}
		if session.State == appmodel.Idle && session.ActiveRun == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("TaskStop session did not become idle")
}
