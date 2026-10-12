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
	"gcode/internal/sessions"
)

func TestServiceRunIdentityStaysStableAcrossControllerRuntimeAndSuccessiveRuns(t *testing.T) {
	const providerKey = "run-identity-fixture-key"
	project := t.TempDir()
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".gcode"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A non-fatal extension diagnostic makes the warning event part of the same
	// end-to-end identity assertion as user, tool, permission, and run-end data.
	if err := os.WriteFile(filepath.Join(project, ".gcode", "hooks.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	providerSteps := make(chan int, 4)
	protocolErrors := make(chan error, 8)
	var requestsMu sync.Mutex
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+providerKey {
			runIdentityRecordError(protocolErrors, fmt.Errorf("unexpected provider request: %s %s", request.Method, request.URL.Path))
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		if _, err := os.ReadFile(filepath.Join(project, ".gcode", "hooks.json")); err != nil {
			runIdentityRecordError(protocolErrors, fmt.Errorf("read fixture hook config: %w", err))
		}
		requestsMu.Lock()
		requestCount++
		step := requestCount
		requestsMu.Unlock()
		select {
		case providerSteps <- step:
		default:
		}

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		switch step {
		case 1:
			writeToolStream(writer, "run-identity-tool", "exec_command", map[string]any{
				"cmd":                 "printf 'run identity\n'",
				"sandbox_permissions": "require_escalated",
				"yield_time_ms":       1000,
			})
		case 2:
			writeChatEvent(writer, map[string]any{"content": "first run complete"}, "stop")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		case 3:
			writeChatEvent(writer, map[string]any{"content": "second run complete"}, "stop")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		default:
			runIdentityRecordError(protocolErrors, fmt.Errorf("unexpected provider request step %d", step))
			http.Error(writer, "unexpected provider step", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close(ctx) }()
	provider := appmodel.Provider{
		ID:      "run-identity-fixture",
		Name:    "Run identity fixture",
		Format:  appmodel.ChatCompletions,
		BaseURL: server.URL + "/v1",
		Model:   "run-identity-model",
	}
	if err := service.SaveProvider(ctx, provider, providerKey); err != nil {
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
		ProjectID:  snapshot.Projects[0].ID,
		ProviderID: provider.ID,
		Title:      "Run identity integration",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatalf("session snapshot = %#v, error = %v", snapshot.Sessions, err)
	}
	sessionID := snapshot.Sessions[0].ID

	updates := make(chan appmodel.Snapshot, 256)
	unsub := service.Subscribe(func(snapshot appmodel.Snapshot) {
		select {
		case updates <- snapshot:
		default:
		}
	})
	defer unsub()

	if err := service.Send(ctx, sessionID, "first run", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitRunIdentityProviderStep(t, providerSteps, 1)
	pending := waitRunIdentitySession(t, updates, sessionID, func(session appmodel.Session) bool {
		return session.State == appmodel.WaitingPermission && len(session.Permissions) == 1
	})
	oldRunID := pending.RunID
	if oldRunID == "" {
		t.Fatal("pending permission has no active run id")
	}
	if pending.Permissions[0].RunID != oldRunID {
		t.Fatalf("pending permission run id = %q, session run id = %q", pending.Permissions[0].RunID, oldRunID)
	}
	if runID := runIdentityMessageID(pending, "user", "first run"); runID != oldRunID {
		t.Fatalf("first user message run id = %q, want %q", runID, oldRunID)
	}

	if err := service.ReplyPermission(ctx, sessionID, pending.Permissions[0].ID, "allow"); err != nil {
		t.Fatal(err)
	}
	waitRunIdentityProviderStep(t, providerSteps, 2)
	waitRunIdentitySession(t, updates, sessionID, func(session appmodel.Session) bool {
		return session.State == appmodel.Idle && session.ActiveRun == nil && runIdentityHasText(session, "first run complete")
	})

	if err := service.Send(ctx, sessionID, "second run", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitRunIdentityProviderStep(t, providerSteps, 3)
	secondStarted := waitRunIdentitySession(t, updates, sessionID, func(session appmodel.Session) bool {
		return runIdentityMessageID(session, "user", "second run") != ""
	})
	newRunID := runIdentityMessageID(secondStarted, "user", "second run")
	if newRunID == "" || newRunID == oldRunID {
		t.Fatalf("second run id = %q, old run id = %q", newRunID, oldRunID)
	}
	waitRunIdentitySession(t, updates, sessionID, func(session appmodel.Session) bool {
		return session.State == appmodel.Idle && session.ActiveRun == nil && runIdentityHasText(session, "second run complete")
	})

	finalSnapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finalSession := runIdentityFindSession(finalSnapshot, sessionID)
	if finalSession == nil {
		t.Fatalf("session %s disappeared from final snapshot", sessionID)
	}
	for _, message := range finalSession.Messages {
		if message.RunID == "" {
			t.Fatalf("snapshot message has no run id: %#v", message)
		}
		if message.RunID != oldRunID && message.RunID != newRunID {
			t.Fatalf("snapshot message has foreign run id %q: %#v", message.RunID, message)
		}
	}
	if runID := runIdentityMessageID(*finalSession, "user", "first run"); runID != oldRunID {
		t.Fatalf("final first user run id = %q, want %q", runID, oldRunID)
	}
	if runID := runIdentityMessageID(*finalSession, "user", "second run"); runID != newRunID {
		t.Fatalf("final second user run id = %q, want %q", runID, newRunID)
	}

	eventsPath := filepath.Join(dataDir, "sessions", sessionID, sessions.EventsFile)
	eventData, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]bool{oldRunID: {}, newRunID: {}}
	for _, line := range strings.Split(string(eventData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event sessions.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode events.jsonl line %q: %v", line, err)
		}
		var payload struct {
			RunID string `json:"runId"`
			Role  string `json:"role"`
			Text  string `json:"content"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode event payload for %s: %v", event.Type, err)
		}
		if payload.RunID == "" {
			t.Fatalf("event %s has no run id: %s", event.Type, line)
		}
		bucket, ok := seen[payload.RunID]
		if !ok {
			t.Fatalf("event %s has foreign run id %q", event.Type, payload.RunID)
		}
		switch event.Type {
		case sessions.EventMessage:
			bucket["message"] = true
			switch payload.Role {
			case "warning":
				bucket["warning"] = true
			case "user":
				bucket["user"] = true
			case "assistant":
				bucket["assistant"] = true
			case "run_end":
				bucket["run_end"] = true
			}
		case sessions.EventToolCall:
			bucket["tool_call"] = true
		case sessions.EventToolResult:
			bucket["tool_result"] = true
		case sessions.EventPermissionRequest:
			bucket["permission_request"] = true
		case sessions.EventPermissionDecision:
			bucket["permission_decision"] = true
		case sessions.EventPermission:
			bucket["permission"] = true
		}
	}
	for _, key := range []string{"warning", "user", "assistant", "run_end"} {
		if !seen[oldRunID][key] {
			t.Fatalf("old run %q missing event role %q: %#v", oldRunID, key, seen[oldRunID])
		}
	}
	for _, key := range []string{"warning", "user", "assistant", "run_end"} {
		if !seen[newRunID][key] {
			t.Fatalf("new run %q missing event role %q: %#v", newRunID, key, seen[newRunID])
		}
	}
	for _, key := range []string{"tool_call", "tool_result", "permission_request", "permission_decision", "permission"} {
		if !seen[oldRunID][key] {
			t.Fatalf("old run %q missing event %q: %#v", oldRunID, key, seen[oldRunID])
		}
		if seen[newRunID][key] {
			t.Fatalf("new run %q incorrectly contains old-run event %q: %#v", newRunID, key, seen[newRunID])
		}
	}
	t.Logf("run identity evidence: old=%s new=%s journal=%s", oldRunID, newRunID, eventsPath)
	select {
	case protocolErr := <-protocolErrors:
		t.Fatal(protocolErr)
	default:
	}
}

func waitRunIdentityProviderStep(t *testing.T, steps <-chan int, want int) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case got := <-steps:
		if got != want {
			t.Fatalf("provider step = %d, want %d", got, want)
		}
	case <-timer.C:
		t.Fatalf("provider did not reach step %d", want)
	}
}

func waitRunIdentitySession(t *testing.T, updates <-chan appmodel.Snapshot, sessionID string, predicate func(appmodel.Session) bool) appmodel.Session {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case snapshot := <-updates:
			session := runIdentityFindSession(snapshot, sessionID)
			if session == nil {
				continue
			}
			if session.State == appmodel.Failed {
				t.Fatalf("session failed while waiting for run identity state: %s", session.Error)
			}
			if predicate(*session) {
				return *session
			}
		case <-timer.C:
			t.Fatalf("session %s did not reach the expected run identity state", sessionID)
		}
	}
}

func runIdentityFindSession(snapshot appmodel.Snapshot, sessionID string) *appmodel.Session {
	for index := range snapshot.Sessions {
		if snapshot.Sessions[index].ID == sessionID {
			return &snapshot.Sessions[index]
		}
	}
	return nil
}

func runIdentityMessageID(session appmodel.Session, role, text string) string {
	for _, message := range session.Messages {
		if message.Role == role && message.Text == text {
			return message.RunID
		}
	}
	return ""
}

func runIdentityHasText(session appmodel.Session, text string) bool {
	for _, message := range session.Messages {
		if strings.Contains(message.Text, text) {
			return true
		}
	}
	return false
}

func runIdentityRecordError(errors chan<- error, err error) {
	if err == nil {
		return
	}
	select {
	case errors <- err:
	default:
	}
}
