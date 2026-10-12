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

	"gcode/internal/appmodel"
)

const (
	backgroundIsolationStartPrompt         = "启动隔离本实例后台任务。"
	backgroundIsolationOwnOutputPrompt     = "查询隔离本实例后台任务。"
	backgroundIsolationForeignOutputPrompt = "查询隔离另一实例后台任务。"
	backgroundIsolationForeignStopPrompt   = "停止隔离另一实例后台任务。"
)

type backgroundIsolationFixture struct {
	mu                sync.Mutex
	taskID            string
	foreignTaskID     string
	ownOutput         string
	foreignOutput     string
	foreignStopOutput string
	childStarted      chan struct{}
	childCanceled     chan struct{}
	childStartOnce    sync.Once
	childStopOnce     sync.Once
}

func newBackgroundIsolationFixture() *backgroundIsolationFixture {
	return &backgroundIsolationFixture{
		childStarted:  make(chan struct{}),
		childCanceled: make(chan struct{}),
	}
}

func (fixture *backgroundIsolationFixture) handler(writer http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(writer, "invalid provider request", http.StatusBadRequest)
		return
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "# Specialist Invocation") {
		fixture.childStartOnce.Do(func() { close(fixture.childStarted) })
		writer.Header().Set("Content-Type", "text/event-stream")
		writeChatEvent(writer, map[string]any{"content": "isolated background progress"}, "")
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
	case strings.Contains(lastUser, backgroundIsolationStartPrompt) && !hasToolResult:
		writeToolStream(writer, "call-isolation-background-task", "Task", map[string]any{
			"name":              "runtime-child",
			"prompt":            strings.Repeat("保持隔离后台 specialist 运行。", 400),
			"description":       "DataDir isolation",
			"run_in_background": true,
		})
	case strings.Contains(lastUser, backgroundIsolationStartPrompt) && hasToolResult:
		fixture.setTaskID(extractTaskID(body))
		writeChatEvent(writer, map[string]any{"content": "隔离后台任务已启动。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	case strings.Contains(lastUser, backgroundIsolationOwnOutputPrompt) && !hasToolResult:
		writeToolStream(writer, "call-isolation-own-output", "TaskOutput", map[string]any{
			"task_id": fixture.getTaskID(),
		})
	case strings.Contains(lastUser, backgroundIsolationOwnOutputPrompt) && hasToolResult:
		fixture.setOwnOutput(extractToolOutput(body))
		writeChatEvent(writer, map[string]any{"content": "本实例后台任务查询完成。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	case strings.Contains(lastUser, backgroundIsolationForeignOutputPrompt) && !hasToolResult:
		writeToolStream(writer, "call-isolation-foreign-output", "TaskOutput", map[string]any{
			"task_id": fixture.getForeignTaskID(),
		})
	case strings.Contains(lastUser, backgroundIsolationForeignOutputPrompt) && hasToolResult:
		fixture.setForeignOutput(extractToolOutput(body))
		writeChatEvent(writer, map[string]any{"content": "另一实例后台任务查询完成。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	case strings.Contains(lastUser, backgroundIsolationForeignStopPrompt) && !hasToolResult:
		writeToolStream(writer, "call-isolation-foreign-stop", "TaskStop", map[string]any{
			"task_id": fixture.getForeignTaskID(),
		})
	case strings.Contains(lastUser, backgroundIsolationForeignStopPrompt) && hasToolResult:
		fixture.setForeignStopOutput(extractToolOutput(body))
		writeChatEvent(writer, map[string]any{"content": "另一实例后台任务停止请求完成。"}, "stop")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	default:
		http.Error(writer, "unexpected provider request", http.StatusBadRequest)
	}
}

func (fixture *backgroundIsolationFixture) setTaskID(taskID string) {
	fixture.mu.Lock()
	fixture.taskID = strings.TrimSpace(taskID)
	fixture.mu.Unlock()
}

func (fixture *backgroundIsolationFixture) getTaskID() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.taskID
}

func (fixture *backgroundIsolationFixture) setForeignTaskID(taskID string) {
	fixture.mu.Lock()
	fixture.foreignTaskID = strings.TrimSpace(taskID)
	fixture.mu.Unlock()
}

func (fixture *backgroundIsolationFixture) getForeignTaskID() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.foreignTaskID
}

func (fixture *backgroundIsolationFixture) setOwnOutput(output string) {
	fixture.mu.Lock()
	fixture.ownOutput = output
	fixture.mu.Unlock()
}

func (fixture *backgroundIsolationFixture) getOwnOutput() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.ownOutput
}

func (fixture *backgroundIsolationFixture) setForeignOutput(output string) {
	fixture.mu.Lock()
	fixture.foreignOutput = output
	fixture.mu.Unlock()
}

func (fixture *backgroundIsolationFixture) getForeignOutput() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.foreignOutput
}

func (fixture *backgroundIsolationFixture) setForeignStopOutput(output string) {
	fixture.mu.Lock()
	fixture.foreignStopOutput = output
	fixture.mu.Unlock()
}

func (fixture *backgroundIsolationFixture) getForeignStopOutput() string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.foreignStopOutput
}

func TestServiceBackgroundSpecialistIsolationAcrossDataDirs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ctx := context.Background()
	projectA, projectB := t.TempDir(), t.TempDir()
	dataDirA, dataDirB := t.TempDir(), t.TempDir()
	for _, dataDir := range []string{dataDirA, dataDirB} {
		writeApplicationFile(t, filepath.Join(dataDir, "specialists", "runtime-child.md"), `---
name: runtime-child
description: Long lived isolated background child
tools:
  - "read-only"
---

Background specialist isolation fixture.
`)
	}

	fixtureA, fixtureB := newBackgroundIsolationFixture(), newBackgroundIsolationFixture()
	serverA := httptest.NewServer(http.HandlerFunc(fixtureA.handler))
	serverB := httptest.NewServer(http.HandlerFunc(fixtureB.handler))
	t.Cleanup(serverA.Close)
	t.Cleanup(serverB.Close)

	serviceA, err := New(dataDirA)
	if err != nil {
		t.Fatalf("New service A: %v", err)
	}
	serviceB, err := New(dataDirB)
	if err != nil {
		_ = serviceA.Close(ctx)
		t.Fatalf("New service B: %v", err)
	}
	closedA, closedB := false, false
	t.Cleanup(func() {
		if !closedB {
			_ = serviceB.Close(ctx)
		}
		if !closedA {
			_ = serviceA.Close(ctx)
		}
	})

	sessionA := newIsolationServiceSession(t, ctx, serviceA, projectA, serverA.URL, "background-isolation-a", "隔离实例 A")
	sessionB := newIsolationServiceSession(t, ctx, serviceB, projectB, serverB.URL, "background-isolation-b", "隔离实例 B")

	if err := serviceA.Send(ctx, sessionA, backgroundIsolationStartPrompt, appmodel.Correction); err != nil {
		t.Fatalf("start service A background task: %v", err)
	}
	waitForIdle(t, serviceA, sessionA)
	waitForBackgroundChannel(t, fixtureA.childStarted, "service A child start")
	taskA := fixtureA.getTaskID()
	if taskA == "" {
		t.Fatal("service A did not return a background task id")
	}

	if err := serviceB.Send(ctx, sessionB, backgroundIsolationStartPrompt, appmodel.Correction); err != nil {
		t.Fatalf("start service B background task: %v", err)
	}
	waitForIdle(t, serviceB, sessionB)
	waitForBackgroundChannel(t, fixtureB.childStarted, "service B child start")
	taskB := fixtureB.getTaskID()
	if taskB == "" {
		t.Fatal("service B did not return a background task id")
	}
	if taskA == taskB {
		t.Fatalf("independent services returned the same task id: %q", taskA)
	}
	fixtureA.setForeignTaskID(taskB)
	fixtureB.setForeignTaskID(taskA)

	assertBackgroundMetadataIsolation(t, dataDirA, dataDirB, taskA, taskB)

	if err := serviceA.Send(ctx, sessionA, backgroundIsolationForeignOutputPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service A cross-instance TaskOutput: %v", err)
	}
	waitForIdle(t, serviceA, sessionA)
	if output := fixtureA.getForeignOutput(); !strings.Contains(output, "background task not found") {
		t.Fatalf("service A cross-instance TaskOutput = %q", output)
	}

	if err := serviceB.Send(ctx, sessionB, backgroundIsolationForeignOutputPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service B cross-instance TaskOutput: %v", err)
	}
	waitForIdle(t, serviceB, sessionB)
	if output := fixtureB.getForeignOutput(); !strings.Contains(output, "background task not found") {
		t.Fatalf("service B cross-instance TaskOutput = %q", output)
	}

	if err := serviceA.Send(ctx, sessionA, backgroundIsolationForeignStopPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service A cross-instance TaskStop: %v", err)
	}
	waitForIdleAfterOptionalTaskStopPermission(t, serviceA, sessionA)
	if output := fixtureA.getForeignStopOutput(); !strings.Contains(output, "background task not found") {
		t.Fatalf("service A cross-instance TaskStop = %q", output)
	}
	assertBackgroundChildStillRunning(t, fixtureB, "service B child after service A cross-instance TaskStop")

	if err := serviceB.Send(ctx, sessionB, backgroundIsolationForeignStopPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service B cross-instance TaskStop: %v", err)
	}
	waitForIdleAfterOptionalTaskStopPermission(t, serviceB, sessionB)
	if output := fixtureB.getForeignStopOutput(); !strings.Contains(output, "background task not found") {
		t.Fatalf("service B cross-instance TaskStop = %q", output)
	}
	assertBackgroundChildStillRunning(t, fixtureA, "service A child after service B cross-instance TaskStop")

	if err := serviceA.Send(ctx, sessionA, backgroundIsolationOwnOutputPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service A own TaskOutput: %v", err)
	}
	waitForIdle(t, serviceA, sessionA)
	if output := fixtureA.getOwnOutput(); !strings.Contains(output, "status: running") || !strings.Contains(output, "isolated background progress") {
		t.Fatalf("service A own TaskOutput = %q", output)
	}
	if err := serviceB.Send(ctx, sessionB, backgroundIsolationOwnOutputPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service B own TaskOutput: %v", err)
	}
	waitForIdle(t, serviceB, sessionB)
	if output := fixtureB.getOwnOutput(); !strings.Contains(output, "status: running") || !strings.Contains(output, "isolated background progress") {
		t.Fatalf("service B own TaskOutput = %q", output)
	}

	if err := serviceA.Close(ctx); err != nil {
		t.Fatalf("close service A: %v", err)
	}
	closedA = true
	waitForBackgroundChannel(t, fixtureA.childCanceled, "service A child cancellation on close")
	assertBackgroundChildStillRunning(t, fixtureB, "service B child after service A close")

	if err := serviceB.Send(ctx, sessionB, backgroundIsolationOwnOutputPrompt, appmodel.Correction); err != nil {
		t.Fatalf("service B TaskOutput after service A close: %v", err)
	}
	waitForIdle(t, serviceB, sessionB)
	if output := fixtureB.getOwnOutput(); !strings.Contains(output, "status: running") || !strings.Contains(output, "isolated background progress") {
		t.Fatalf("service B TaskOutput after service A close = %q", output)
	}

	if err := serviceB.Close(ctx); err != nil {
		t.Fatalf("close service B: %v", err)
	}
	closedB = true
	waitForBackgroundChannel(t, fixtureB.childCanceled, "service B child cancellation on close")
}

func newIsolationServiceSession(t *testing.T, ctx context.Context, service *Service, project, serverURL, providerID, title string) string {
	t.Helper()
	provider := appmodel.Provider{
		ID:      providerID,
		Name:    title,
		Format:  appmodel.ChatCompletions,
		BaseURL: serverURL + "/v1",
		Model:   providerID + "-model",
	}
	if err := service.SaveProvider(ctx, provider, providerID+"-key"); err != nil {
		t.Fatalf("save provider %s: %v", providerID, err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatalf("add project %s: %v", providerID, err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("projects for %s = %#v, error = %v", providerID, snapshot.Projects, err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{
		ProjectID:  snapshot.Projects[0].ID,
		ProviderID: provider.ID,
		FullAccess: false,
		Title:      title,
	}); err != nil {
		t.Fatalf("new session %s: %v", providerID, err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatalf("sessions for %s = %#v, error = %v", providerID, snapshot.Sessions, err)
	}
	return snapshot.Sessions[0].ID
}

func assertBackgroundMetadataIsolation(t *testing.T, dataDirA, dataDirB, taskA, taskB string) {
	t.Helper()
	metadataA := backgroundTaskMetadataIDs(t, dataDirA)
	metadataB := backgroundTaskMetadataIDs(t, dataDirB)
	if !metadataA[taskA] {
		t.Fatalf("service A background metadata does not contain %s: %v", taskA, metadataA)
	}
	if metadataA[taskB] {
		t.Fatalf("service A background metadata contains service B task %s: %v", taskB, metadataA)
	}
	if !metadataB[taskB] {
		t.Fatalf("service B background metadata does not contain %s: %v", taskB, metadataB)
	}
	if metadataB[taskA] {
		t.Fatalf("service B background metadata contains service A task %s: %v", taskA, metadataB)
	}
}

func backgroundTaskMetadataIDs(t *testing.T, dataDir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dataDir, "background"))
	if err != nil {
		t.Fatalf("read background metadata in %s: %v", dataDir, err)
	}
	ids := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		ids[strings.TrimSuffix(entry.Name(), ".json")] = true
	}
	return ids
}

func assertBackgroundChildStillRunning(t *testing.T, fixture *backgroundIsolationFixture, description string) {
	t.Helper()
	select {
	case <-fixture.childCanceled:
		t.Fatalf("%s: child was canceled", description)
	default:
	}
}
