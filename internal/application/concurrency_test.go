package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/appmodel"
)

// TestThreeSessionsOwnTheirCommands exercises the complete desktop path:
// Service -> Controller -> Runtime -> the real Chat Completions provider ->
// exec_command. The provider is an HTTP protocol substitute; no vendor API is
// contacted.
func TestThreeSessionsOwnTheirCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses the POSIX shell and sleep command")
	}
	project := t.TempDir()
	dataDir := t.TempDir()
	canonicalProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	markers := map[string]string{
		"A": filepath.Join(project, "a.started"),
		"B": filepath.Join(project, "b.started"),
		"C": filepath.Join(project, "c.started"),
	}
	doneMarkers := map[string]string{
		"A": filepath.Join(project, "a.done"),
		"B": filepath.Join(project, "b.done"),
		"C": filepath.Join(project, "c.done"),
	}
	childMarker := filepath.Join(project, "a.child.done")
	terminalMarker := filepath.Join(project, "terminal.done")

	var requestMu sync.Mutex
	requestSteps := make(map[string]int)
	aFollowUp := make(chan struct{})
	var aFollowUpOnce sync.Once
	protocolErrors := make(chan error, 8)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer fixture-concurrency-key" {
			protocolErrors <- fmt.Errorf("unexpected provider request: %s %s", request.Method, request.URL.Path)
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
		requestText := string(body)
		session := ""
		for _, candidate := range []string{"A", "B", "C"} {
			if strings.Contains(requestText, "fixture-"+candidate) {
				session = candidate
				break
			}
		}
		if session == "" {
			protocolErrors <- fmt.Errorf("provider request did not carry a fixture session: %s", requestText)
			http.Error(writer, "missing fixture session", http.StatusBadRequest)
			return
		}

		requestMu.Lock()
		requestSteps[session]++
		step := requestSteps[session]
		requestMu.Unlock()

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		if step == 1 {
			command := concurrencyFixtureCommand(session)
			writeToolStream(writer, "call-"+strings.ToLower(session), "exec_command", map[string]any{
				"cmd":           command,
				"workdir":       ".",
				"yield_time_ms": 250,
			})
			return
		}

		if session == "A" {
			aFollowUpOnce.Do(func() { close(aFollowUp) })
			<-request.Context().Done()
			return
		}

		if err := waitForConcurrencyMarker(request.Context(), doneMarkers[session]); err != nil {
			return
		}
		writeChatEvent(writer, map[string]any{"content": "fixture-" + session + " completed"}, "stop")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		_ = decoded // Keep decoding explicit: malformed protocol input fails above.
	}))
	defer server.Close()

	ctx := context.Background()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(ctx)
	provider := appmodel.Provider{
		ID:      "fixture-concurrency",
		Name:    "并发协议替身",
		Format:  appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1",
		Model:   "fixture-concurrency-model",
	}
	if err := service.SaveProvider(ctx, provider, "fixture-concurrency-key"); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("project snapshot = %#v, error = %v", snapshot.Projects, err)
	}
	projectID := snapshot.Projects[0].ID
	for _, session := range []string{"A", "B", "C"} {
		if err := service.NewSession(ctx, appmodel.NewSessionOptions{
			ProjectID: projectID, ProviderID: provider.ID, FullAccess: true, Title: "fixture-" + session,
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 3 {
		t.Fatalf("session snapshot = %#v, error = %v", snapshot.Sessions, err)
	}
	ids := make(map[string]string, 3)
	for _, session := range snapshot.Sessions {
		name := strings.TrimPrefix(session.Title, "fixture-")
		ids[name] = session.ID
		if session.Cwd != canonicalProject {
			t.Fatalf("session %s cwd = %q, want shared project %q", name, session.Cwd, canonicalProject)
		}
	}

	sendErrors := make(chan error, 3)
	var sendWait sync.WaitGroup
	for _, session := range []string{"A", "B", "C"} {
		session := session
		sendWait.Add(1)
		go func() {
			defer sendWait.Done()
			sendErrors <- service.Send(ctx, ids[session], "fixture-"+session+" run its command", appmodel.Correction)
		}()
	}
	sendWait.Wait()
	close(sendErrors)
	for sendErr := range sendErrors {
		if sendErr != nil {
			t.Fatal(sendErr)
		}
	}

	waitForConcurrencyCondition(t, 10*time.Second, func() bool {
		for _, path := range markers {
			if _, err := os.Stat(path); err != nil {
				return false
			}
		}
		return true
	})
	select {
	case <-aFollowUp:
	case <-time.After(10 * time.Second):
		t.Fatal("session A did not reach the retained-command provider turn")
	}
	for _, path := range doneMarkers {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a short/long command finished before cancellation: %s, error=%v", path, err)
		}
	}

	terminalCommand := exec.Command("sh", "-c", "sleep 4; printf done > terminal.done")
	terminalCommand.Dir = project
	if err := terminalCommand.Start(); err != nil {
		t.Fatal(err)
	}
	terminalWait := make(chan error, 1)
	go func() { terminalWait <- terminalCommand.Wait() }()

	if err := service.Stop(ctx, ids["A"]); err != nil {
		t.Fatal(err)
	}
	waitForConcurrencyCondition(t, 10*time.Second, func() bool {
		snapshot, snapshotErr := service.Snapshot(ctx)
		if snapshotErr != nil {
			return false
		}
		for _, session := range snapshot.Sessions {
			if session.ID == ids["A"] {
				return session.ActiveRun == nil && session.State == appmodel.Interrupted
			}
		}
		return false
	})

	waitForIdle(t, service, ids["B"])
	waitForIdle(t, service, ids["C"])
	if err := <-terminalWait; err != nil {
		t.Fatalf("independent terminal exited with error: %v", err)
	}
	for _, session := range []string{"B", "C"} {
		if _, err := os.Stat(doneMarkers[session]); err != nil {
			t.Fatalf("session %s did not finish its command: %v", session, err)
		}
	}
	if _, err := os.Stat(terminalMarker); err != nil {
		t.Fatalf("independent terminal resource was affected: %v", err)
	}
	if _, err := os.Stat(doneMarkers["A"]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session A wrote done after cancellation: error=%v", err)
	}
	if _, err := os.Stat(childMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session A descendant survived cancellation and wrote its marker: error=%v", err)
	}

	select {
	case protocolErr := <-protocolErrors:
		t.Fatal(protocolErr)
	default:
	}
}

func concurrencyFixtureCommand(session string) string {
	session = strings.ToLower(session)
	switch session {
	case "a":
		return "printf started > a.started; (sleep 30; printf done > a.done) & (sleep 3; printf done > a.child.done) & wait"
	case "b", "c":
		return fmt.Sprintf("printf started > %s.started; sleep 2; printf done > %s.done", session, session)
	default:
		panic("unknown concurrency fixture session")
	}
}

func waitForConcurrencyMarker(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForConcurrencyCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("concurrency fixture condition was not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
