package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"gcode/internal/appmodel"
)

func TestServiceRuntimeBoundaryRedactsCredentialFromSnapshotsAndHistory(t *testing.T) {
	const secret = "fixture-boundary-secret-20261012-abcdef"
	var protocolErrors atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+secret {
			protocolErrors.Add(1)
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(writer, `{"error":{"message":"vendor rejected credential %s"}}`, secret)
	}))
	defer server.Close()

	dataDir := t.TempDir()
	project := t.TempDir()
	service, sessionID := newRuntimeBoundarySession(t, dataDir, project, appmodel.Provider{
		ID: "boundary-error", Name: "边界错误替身", Format: appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1", Model: "fixture-model",
	}, secret)
	ctx := context.Background()
	t.Cleanup(func() { _ = service.Close(ctx) })

	if err := service.Send(ctx, sessionID, "触发厂商错误", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	failed := waitForRuntimeBoundarySession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.ActiveRun == nil && session.State == appmodel.Failed
	})
	if failed.Error == "" || strings.Contains(failed.Error, secret) {
		t.Fatalf("snapshot error leaked credential or was empty: %q", failed.Error)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshotData, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(snapshotData), secret) {
		t.Fatal("application snapshot contains the provider credential")
	}
	if protocolErrors.Load() != 0 {
		t.Fatalf("provider protocol errors = %d", protocolErrors.Load())
	}

	assertRuntimeBoundaryFilesDoNotContain(t, dataDir, secret)
	if _, err := os.Stat(filepath.Join(dataDir, "controller.json")); err != nil {
		t.Fatalf("controller.json is missing: %v", err)
	}
	historyPath := filepath.Join(dataDir, "sessions", sessionID, "events.jsonl")
	history, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read session history: %v", err)
	}
	if !strings.Contains(string(history), "provider") && !strings.Contains(string(history), "auth") {
		t.Fatalf("session history has no durable provider failure evidence: %q", history)
	}
}

func TestServiceSendPersistenceFailureRollsBackAndRestartDoesNotDuplicate(t *testing.T) {
	const secret = "fixture-recovery-secret-20261012-abcdef"
	var requests atomic.Int32
	var protocolErrors atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+secret {
			protocolErrors.Add(1)
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		requests.Add(1)
		writer.Header().Set("Content-Type", "text/event-stream")
		writeChatEvent(writer, map[string]any{"content": "fixture completed"}, "stop")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dataDir := t.TempDir()
	service, sessionID := newRuntimeBoundarySession(t, dataDir, t.TempDir(), appmodel.Provider{
		ID: "boundary-recovery", Name: "恢复替身", Format: appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1", Model: "fixture-model",
	}, secret)
	ctx := context.Background()
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = service.Close(ctx)
		}
	})

	statePath := makeControllerStatePathUnavailable(t, dataDir)
	if err := service.Send(ctx, sessionID, "持久化失败后不应执行", appmodel.Correction); err == nil {
		t.Fatal("Send succeeded while controller.json was a directory")
	}
	rolledBack := waitForRuntimeBoundarySession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.ActiveRun == nil && session.State == appmodel.Idle
	})
	if len(rolledBack.Messages) != 0 || len(rolledBack.Queue) != 0 {
		t.Fatalf("failed send changed session state: %#v", rolledBack)
	}
	if requests.Load() != 0 {
		t.Fatalf("failed send reached provider %d time(s)", requests.Load())
	}
	removeUnavailableControllerState(t, statePath)

	if err := service.Send(ctx, sessionID, "恢复后执行一次", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, service, sessionID)
	if requests.Load() != 1 {
		t.Fatalf("provider requests after recovered send = %d, want 1", requests.Load())
	}
	if protocolErrors.Load() != 0 {
		t.Fatalf("provider protocol errors = %d", protocolErrors.Load())
	}
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	closed = true

	restarted, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	restartedClosed := false
	t.Cleanup(func() {
		if !restartedClosed {
			_ = restarted.Close(ctx)
		}
	})
	if _, err := restarted.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("restart replayed a completed task; provider requests = %d", requests.Load())
	}
	if err := restarted.Send(ctx, sessionID, "重启后执行一次", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, restarted, sessionID)
	if requests.Load() != 2 {
		t.Fatalf("provider requests after explicit resumed send = %d, want 2", requests.Load())
	}
	if err := restarted.Close(ctx); err != nil {
		t.Fatal(err)
	}
	restartedClosed = true
}

func TestServiceStopPersistenceFailureRetainsRunUntilSuccessfulStop(t *testing.T) {
	const secret = "fixture-stop-secret-20261012-abcdef"
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var releaseOnce sync.Once
	var protocolErrors atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+secret {
			protocolErrors.Add(1)
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		startOnce.Do(func() { close(started) })
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	releaseHandler := func() {
		releaseOnce.Do(func() { close(release) })
	}
	t.Cleanup(releaseHandler)

	dataDir := t.TempDir()
	service, sessionID := newRuntimeBoundarySession(t, dataDir, t.TempDir(), appmodel.Provider{
		ID: "boundary-stop", Name: "停止替身", Format: appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1", Model: "fixture-model",
	}, secret)
	ctx := context.Background()
	t.Cleanup(func() { _ = service.Close(ctx) })
	if err := service.Send(ctx, sessionID, "启动长请求", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider request did not start")
	}

	statePath := makeControllerStatePathUnavailable(t, dataDir)
	if err := service.Stop(ctx, sessionID); err == nil {
		t.Fatal("Stop succeeded while controller.json was a directory")
	}
	running := waitForRuntimeBoundarySession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.ActiveRun != nil && session.State == appmodel.Running
	})
	if running.QueuePaused {
		t.Fatalf("failed Stop paused the active run: %#v", running)
	}
	removeUnavailableControllerState(t, statePath)

	if err := service.Stop(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	releaseHandler()
	waitForRuntimeBoundarySession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.ActiveRun == nil && session.State == appmodel.Interrupted
	})
	if protocolErrors.Load() != 0 {
		t.Fatalf("provider protocol errors = %d", protocolErrors.Load())
	}
}

func TestServiceSettingsPersistenceFailureRollsBackAndRecovers(t *testing.T) {
	dataDir := t.TempDir()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	t.Cleanup(func() { _ = service.Close(ctx) })
	beforeSnapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed := beforeSnapshot.Settings
	changed.Theme = "dark"
	statePath := makeControllerStatePathUnavailable(t, dataDir)
	if err := service.SaveSettings(ctx, changed); err == nil {
		t.Fatal("SaveSettings succeeded while controller.json was a directory")
	}
	afterFailure, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterFailure.Settings, beforeSnapshot.Settings) {
		t.Fatalf("settings rollback failed: before=%#v after=%#v", beforeSnapshot.Settings, afterFailure.Settings)
	}
	removeUnavailableControllerState(t, statePath)
	if err := service.SaveSettings(ctx, changed); err != nil {
		t.Fatal(err)
	}
	afterSuccess, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterSuccess.Settings.Theme != "dark" {
		t.Fatalf("settings success was not reported in snapshot: %#v", afterSuccess.Settings)
	}
}

func TestServiceRestoresProviderCredentialPresenceAfterRestart(t *testing.T) {
	t.Setenv("GCODE_CRED_STORAGE", "encrypted-file")
	const secret = "fixture-restart-credential-20261012-abcdef"
	dataDir := t.TempDir()
	ctx := context.Background()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProvider(ctx, appmodel.Provider{
		ID: "boundary-restart-key", Name: "重启有密钥", Format: appmodel.ChatCompletions,
		BaseURL: "https://provider.example/v1", Model: "fixture-key-model",
	}, secret); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	if err := service.SaveProvider(ctx, appmodel.Provider{
		ID: "boundary-restart-empty", Name: "重启无密钥", Format: appmodel.ChatCompletions,
		BaseURL: "https://provider.example/v1", Model: "fixture-empty-model",
	}, ""); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	before, err := service.Snapshot(ctx)
	if err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	assertProviderCredentialPresence(t, before.Settings.Providers, map[string]bool{
		"boundary-restart-key":   true,
		"boundary-restart-empty": false,
	})
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}

	controllerData, err := os.ReadFile(filepath.Join(dataDir, "controller.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(controllerData), secret) {
		t.Fatal("controller.json contains the provider credential")
	}
	if strings.Contains(string(controllerData), "has_api_key") {
		t.Fatal("controller.json persisted the derived HasAPIKey marker")
	}

	restarted, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close(ctx)
	after, err := restarted.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertProviderCredentialPresence(t, after.Settings.Providers, map[string]bool{
		"boundary-restart-key":   true,
		"boundary-restart-empty": false,
	})
}

func TestServiceClosePersistenceFailureConvergesRealShellResource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell and kill(2)")
	}
	const secret = "fixture-close-secret-20261012-abcdef"
	project := t.TempDir()
	pidPath := filepath.Join(project, "runtime-boundary.pid")
	donePath := filepath.Join(project, "runtime-boundary.done")
	secondRequest := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var secondOnce sync.Once
	var requestNumber atomic.Int32
	var protocolErrors atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+secret {
			protocolErrors.Add(1)
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		if requestNumber.Add(1) == 1 {
			writer.Header().Set("Content-Type", "text/event-stream")
			command := fmt.Sprintf("echo $$ > %s; sleep 30; echo done > %s", runtimeBoundaryShellQuote(pidPath), runtimeBoundaryShellQuote(donePath))
			writeToolStream(writer, "close-shell", "exec_command", map[string]any{"cmd": command, "yield_time_ms": 1000})
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
			return
		}
		secondOnce.Do(func() { close(secondRequest) })
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	releaseHandler := func() {
		releaseOnce.Do(func() { close(release) })
	}
	t.Cleanup(releaseHandler)

	dataDir := t.TempDir()
	service, sessionID := newRuntimeBoundarySession(t, dataDir, project, appmodel.Provider{
		ID: "boundary-close", Name: "关闭替身", Format: appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1", Model: "fixture-model",
	}, secret)
	ctx := context.Background()
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = service.Close(ctx)
		}
	})
	if err := service.Send(ctx, sessionID, "启动真实 shell 并等待关闭", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondRequest:
	case <-time.After(10 * time.Second):
		t.Fatal("provider follow-up request did not remain active")
	}
	pid := waitForRuntimeBoundaryPID(t, pidPath)
	if !runtimeBoundaryProcessAlive(pid) {
		t.Fatalf("shell PID %d exited before Close", pid)
	}
	t.Logf("real shell pid=%d alive before Close=%v", pid, runtimeBoundaryProcessAlive(pid))

	_ = makeControllerStatePathUnavailable(t, dataDir)
	if err := service.Close(ctx); err == nil {
		t.Fatal("Close succeeded while controller.json was a directory")
	}
	closed = true
	// The HTTP client stream is intentionally left without response headers. A
	// failed Close must still converge the real shell even if this test double
	// does not observe transport cancellation immediately.
	releaseHandler()
	waitForRuntimeBoundaryProcessExit(t, pid)
	doneErr := func() error {
		_, err := os.Stat(donePath)
		return err
	}()
	t.Logf("real shell pid=%d alive after Close=%v done marker error=%v", pid, runtimeBoundaryProcessAlive(pid), doneErr)
	if !errors.Is(doneErr, os.ErrNotExist) {
		t.Fatalf("shell cleanup did not stop the real command; done file error = %v", doneErr)
	}
	if protocolErrors.Load() != 0 {
		t.Fatalf("provider protocol errors = %d", protocolErrors.Load())
	}
}

func newRuntimeBoundarySession(t *testing.T, dataDir, project string, provider appmodel.Provider, key string) (*Service, string) {
	t.Helper()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := service.SaveProvider(ctx, provider, key); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		_ = service.Close(ctx)
		t.Fatalf("project setup failed: projects=%#v error=%v", snapshot.Projects, err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: true}); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 1 {
		_ = service.Close(ctx)
		t.Fatalf("session setup failed: sessions=%#v error=%v", snapshot.Sessions, err)
	}
	return service, snapshot.Sessions[0].ID
}

func waitForRuntimeBoundarySession(t *testing.T, service *Service, sessionID string, predicate func(appmodel.Session) bool) appmodel.Session {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range snapshot.Sessions {
			if session.ID == sessionID && predicate(session) {
				return session
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %q did not reach the expected boundary; snapshot=%#v", sessionID, serviceSnapshotForRuntimeBoundary(t, service))
	return appmodel.Session{}
}

func serviceSnapshotForRuntimeBoundary(t *testing.T, service *Service) appmodel.Snapshot {
	t.Helper()
	snapshot, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read final snapshot: %v", err)
	}
	return snapshot
}

func makeControllerStatePathUnavailable(t *testing.T, dataDir string) string {
	t.Helper()
	path := filepath.Join(dataDir, "controller.json")
	if err := os.Remove(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove controller state: %v", err)
		}
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make controller state directory: %v", err)
	}
	return path
}

func removeUnavailableControllerState(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("restore controller state path: %v", err)
	}
}

func assertRuntimeBoundaryFilesDoNotContain(t *testing.T, root, secret string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), secret) {
			return fmt.Errorf("credential found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func waitForRuntimeBoundaryPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("shell PID file %q was not populated", path)
	return 0
}

func waitForRuntimeBoundaryProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !runtimeBoundaryProcessAlive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("shell PID %d is still alive after Close", pid)
}

func runtimeBoundaryProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func runtimeBoundaryShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func assertProviderCredentialPresence(t *testing.T, providers []appmodel.Provider, want map[string]bool) {
	t.Helper()
	got := make(map[string]bool, len(providers))
	for _, provider := range providers {
		got[provider.ID] = provider.HasAPIKey
	}
	for id, expected := range want {
		if actual, ok := got[id]; !ok || actual != expected {
			t.Fatalf("provider %q HasAPIKey = %v (present=%v), want %v; providers=%#v", id, actual, ok, expected, providers)
		}
	}
}
