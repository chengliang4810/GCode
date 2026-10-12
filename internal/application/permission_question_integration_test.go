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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/appmodel"
)

const permissionQuestionFixtureKey = "permission-question-fixture-key"

// TestServicePermissionIntegrationWritesOnlyAfterTheRealApproval exercises the
// complete application path with the production exec_command tool. The local HTTP
// fixture supplies model tool calls; it never decides the permission response.
func TestServicePermissionIntegrationWritesOnlyAfterTheRealApproval(t *testing.T) {
	var requestMu sync.Mutex
	requestCount := 0
	protocolErrors := make(chan error, 8)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+permissionQuestionFixtureKey {
			permissionQuestionRecordError(protocolErrors, errors.New("unexpected provider request"))
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			permissionQuestionRecordError(protocolErrors, errors.New("unable to read provider request"))
			http.Error(writer, "invalid provider request", http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			permissionQuestionRecordError(protocolErrors, errors.New("invalid provider JSON"))
			http.Error(writer, "invalid provider request", http.StatusBadRequest)
			return
		}
		requestMu.Lock()
		requestCount++
		step := requestCount
		requestMu.Unlock()

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		switch step {
		case 1:
			writeToolStream(writer, "permission-allow", "exec_command", map[string]any{
				"cmd":                 "printf 'approved content\\n' > permission-allowed.txt",
				"sandbox_permissions": "require_escalated", "yield_time_ms": 1000,
			})
		case 2:
			writeToolStream(writer, "permission-deny", "exec_command", map[string]any{
				"cmd":                 "printf 'must not be written\\n' > permission-denied.txt",
				"sandbox_permissions": "require_escalated", "yield_time_ms": 1000,
			})
		case 3:
			if !strings.Contains(strings.ToLower(string(body)), "permission denied") {
				permissionQuestionRecordError(protocolErrors, errors.New("denied write result missing from provider context"))
			}
			writeChatEvent(writer, map[string]any{"content": "permission flow completed"}, "stop")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		default:
			permissionQuestionRecordError(protocolErrors, fmt.Errorf("unexpected provider request step %d", step))
			http.Error(writer, "unexpected provider step", http.StatusBadRequest)
		}
	}))
	t.Cleanup(func() { server.Close() })

	fixture := newPermissionQuestionFixture(t, server.URL, "permission-allow-deny")
	if err := fixture.service.Send(fixture.ctx, fixture.sessionID, "permission-allow-deny", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	first := waitPermissionQuestionPermission(t, fixture.service, fixture.sessionID, "exec_command")
	if err := fixture.service.ReplyPermission(fixture.ctx, fixture.sessionID, first.ID, "allow"); err != nil {
		t.Fatal(err)
	}
	second := waitPermissionQuestionPermission(t, fixture.service, fixture.sessionID, "exec_command")
	if first.ID == second.ID {
		t.Fatalf("permission request id was reused: %q", first.ID)
	}
	if err := fixture.service.ReplyPermission(fixture.ctx, fixture.sessionID, second.ID, "deny"); err != nil {
		t.Fatal(err)
	}
	completed := waitForIdle(t, fixture.service, fixture.sessionID)

	allowed, err := os.ReadFile(filepath.Join(fixture.project, "permission-allowed.txt"))
	if err != nil || string(allowed) != "approved content\n" {
		t.Fatalf("allowed file = %q, error = %v", allowed, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.project, "permission-denied.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("denied file exists or stat failed: %v", err)
	}

	var writeCalls []appmodel.ToolCall
	for _, message := range completed.Messages {
		if message.Role != "assistant" {
			continue
		}
		for _, call := range message.Tools {
			if call.Name == "exec_command" {
				writeCalls = append(writeCalls, call)
			}
		}
	}
	if len(writeCalls) != 2 {
		t.Fatalf("exec_command history = %#v, want allow and deny calls", writeCalls)
	}
	if strings.Contains(strings.ToLower(writeCalls[0].Output), "permission denied") || !strings.Contains(writeCalls[0].Arguments, "permission-allowed.txt") {
		t.Fatalf("allowed write result = %#v", writeCalls[0])
	}
	if !strings.Contains(strings.ToLower(writeCalls[1].Output), "permission denied") {
		t.Fatalf("denied write result = %q", writeCalls[1].Output)
	}
	permissionQuestionAssertNoProviderErrors(t, protocolErrors)
}

// TestServiceAskUserIntegrationThreadsAnswersThroughContextAndHistory proves
// that a real application reply is both sent to the next provider turn and
// durably recorded in the session event stream.
func TestServiceAskUserIntegrationThreadsAnswersThroughContextAndHistory(t *testing.T) {
	var requestMu sync.Mutex
	var requestBodies [][]byte
	protocolErrors := make(chan error, 8)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+permissionQuestionFixtureKey {
			permissionQuestionRecordError(protocolErrors, errors.New("unexpected provider request"))
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			permissionQuestionRecordError(protocolErrors, errors.New("unable to read provider request"))
			http.Error(writer, "invalid provider request", http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			permissionQuestionRecordError(protocolErrors, errors.New("invalid provider JSON"))
			http.Error(writer, "invalid provider request", http.StatusBadRequest)
			return
		}
		requestMu.Lock()
		requestBodies = append(requestBodies, append([]byte(nil), body...))
		step := len(requestBodies)
		requestMu.Unlock()

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		switch step {
		case 1:
			writeToolStream(writer, "ask-history", "ask_user", map[string]any{
				"header": "部署环境",
				"questions": []any{map[string]any{
					"header":   "环境",
					"question": "选择要部署的环境。",
					"options":  []any{"staging", "production"},
				}},
			})
		case 2:
			if !strings.Contains(string(body), "production") {
				permissionQuestionRecordError(protocolErrors, errors.New("ask answer missing from provider context"))
			}
			writeChatEvent(writer, map[string]any{"content": "已记录部署环境 production"}, "stop")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		default:
			permissionQuestionRecordError(protocolErrors, fmt.Errorf("unexpected provider request step %d", step))
			http.Error(writer, "unexpected provider step", http.StatusBadRequest)
		}
	}))
	t.Cleanup(func() { server.Close() })

	fixture := newPermissionQuestionFixture(t, server.URL, "ask-history")
	if err := fixture.service.Send(fixture.ctx, fixture.sessionID, "ask-history", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	question := waitPermissionQuestionAsk(t, fixture.service, fixture.sessionID)
	if len(question.Questions) != 1 || question.Questions[0].Text != "选择要部署的环境。" {
		t.Fatalf("question = %#v", question)
	}
	if err := fixture.service.ReplyAsk(fixture.ctx, fixture.sessionID, question.ID, []string{"production"}); err != nil {
		t.Fatal(err)
	}
	completed := waitForIdle(t, fixture.service, fixture.sessionID)

	var askCalls []appmodel.ToolCall
	for _, message := range completed.Messages {
		if message.Role != "assistant" {
			continue
		}
		for _, call := range message.Tools {
			if call.Name == "ask_user" {
				askCalls = append(askCalls, call)
			}
		}
	}
	if len(askCalls) != 1 || !strings.Contains(askCalls[0].Output, "production") {
		t.Fatalf("ask_user history = %#v", askCalls)
	}
	if len(completed.Messages) == 0 || !strings.Contains(completed.Messages[len(completed.Messages)-1].Text, "production") {
		t.Fatalf("final answer did not retain answer: %#v", completed.Messages)
	}
	requestMu.Lock()
	bodyCount := len(requestBodies)
	secondBody := append([]byte(nil), requestBodies[len(requestBodies)-1]...)
	requestMu.Unlock()
	if bodyCount != 2 || !strings.Contains(string(secondBody), "production") {
		t.Fatalf("provider request history count=%d, last=%q", bodyCount, secondBody)
	}
	historyPath := filepath.Join(fixture.dataDir, "sessions", fixture.sessionID, "events.jsonl")
	history, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), "ask_user_answers") || !strings.Contains(string(history), "production") {
		t.Fatalf("ask answer was not persisted in session history: %q", history)
	}
	permissionQuestionAssertNoProviderErrors(t, protocolErrors)
}

// TestServicePendingInteractionCancellationAndSessionOwnership covers the
// controller boundary that is difficult to prove with a fake Runner: a
// correction cancels an actual runtime wait, the replacement starts only after
// that cancellation, stale replies are rejected, and two sessions cannot reply
// to one another's pending request.
func TestServicePendingInteractionCancellationAndSessionOwnership(t *testing.T) {
	var requestMu sync.Mutex
	requestCounts := map[string]int{}
	protocolErrors := make(chan error, 16)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+permissionQuestionFixtureKey {
			permissionQuestionRecordError(protocolErrors, errors.New("unexpected provider request"))
			http.Error(writer, "unexpected provider request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			permissionQuestionRecordError(protocolErrors, errors.New("unable to read provider request"))
			http.Error(writer, "invalid provider request", http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			permissionQuestionRecordError(protocolErrors, errors.New("invalid provider JSON"))
			http.Error(writer, "invalid provider request", http.StatusBadRequest)
			return
		}
		marker := permissionQuestionMarker(string(body))
		if marker == "" {
			permissionQuestionRecordError(protocolErrors, errors.New("provider request lost scenario marker"))
			http.Error(writer, "missing scenario marker", http.StatusBadRequest)
			return
		}
		requestMu.Lock()
		requestCounts[marker]++
		step := requestCounts[marker]
		requestMu.Unlock()

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		switch marker {
		case "permission-old":
			if step == 1 {
				writeToolStream(writer, "permission-old-call", "exec_command", map[string]any{
					"cmd":                 "printf 'old\\n' > permission-old.txt",
					"sandbox_permissions": "require_escalated", "yield_time_ms": 1000,
				})
			} else {
				writeChatEvent(writer, map[string]any{"content": "old run unexpectedly continued"}, "stop")
				_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
			}
		case "permission-new":
			if step == 1 {
				writeToolStream(writer, "permission-new-call", "exec_command", map[string]any{
					"cmd":                 "printf 'new\\n' > permission-new.txt",
					"sandbox_permissions": "require_escalated", "yield_time_ms": 1000,
				})
			} else {
				writeChatEvent(writer, map[string]any{"content": "permission replacement completed"}, "stop")
				_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
			}
		case "ask-old", "ask-new":
			if step == 1 {
				writeToolStream(writer, marker+"-call", "ask_user", map[string]any{
					"questions": []any{map[string]any{"question": marker + " question"}},
				})
			} else {
				if marker == "ask-new" && strings.Contains(string(body), "stale") {
					permissionQuestionRecordError(protocolErrors, errors.New("stale ask answer reached replacement provider turn"))
				}
				writeChatEvent(writer, map[string]any{"content": marker + " completed"}, "stop")
				_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
			}
		case "isolated-a", "isolated-b":
			if step == 1 {
				writeToolStream(writer, marker+"-call", "exec_command", map[string]any{
					"cmd":                 "printf '" + marker + " content\\n' > " + marker + ".txt",
					"sandbox_permissions": "require_escalated", "yield_time_ms": 1000,
				})
			} else {
				writeChatEvent(writer, map[string]any{"content": marker + " completed"}, "stop")
				_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
			}
		default:
			permissionQuestionRecordError(protocolErrors, fmt.Errorf("unknown scenario marker %q", marker))
			http.Error(writer, "unknown scenario", http.StatusBadRequest)
		}
	}))
	t.Cleanup(func() { server.Close() })

	t.Run("permission correction rejects stale reply", func(t *testing.T) {
		fixture := newPermissionQuestionFixture(t, server.URL, "permission-correction")
		if err := fixture.service.Send(fixture.ctx, fixture.sessionID, "permission-old", appmodel.Correction); err != nil {
			t.Fatal(err)
		}
		oldRequest := waitPermissionQuestionPermission(t, fixture.service, fixture.sessionID, "exec_command")
		if err := fixture.service.Send(fixture.ctx, fixture.sessionID, "permission-new", appmodel.Correction); err != nil {
			t.Fatal(err)
		}
		newRequest := waitPermissionQuestionPermissionAfter(t, fixture.service, fixture.sessionID, "exec_command", oldRequest.ID)
		if oldRequest.ID == newRequest.ID {
			t.Fatalf("replacement reused permission id %q", oldRequest.ID)
		}
		if err := fixture.service.ReplyPermission(fixture.ctx, fixture.sessionID, oldRequest.ID, "allow"); err == nil {
			t.Fatal("stale permission reply was accepted")
		}
		if err := fixture.service.ReplyPermission(fixture.ctx, fixture.sessionID, newRequest.ID, "allow"); err != nil {
			t.Fatal(err)
		}
		waitForIdle(t, fixture.service, fixture.sessionID)
		if _, err := os.Stat(filepath.Join(fixture.project, "permission-old.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old permission run wrote a file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(fixture.project, "permission-new.txt")); err != nil {
			t.Fatalf("replacement permission run did not write a file: %v", err)
		}
	})

	t.Run("ask correction rejects stale reply", func(t *testing.T) {
		fixture := newPermissionQuestionFixture(t, server.URL, "ask-correction")
		if err := fixture.service.Send(fixture.ctx, fixture.sessionID, "ask-old", appmodel.Correction); err != nil {
			t.Fatal(err)
		}
		oldRequest := waitPermissionQuestionAsk(t, fixture.service, fixture.sessionID)
		if err := fixture.service.Send(fixture.ctx, fixture.sessionID, "ask-new", appmodel.Correction); err != nil {
			t.Fatal(err)
		}
		newRequest := waitPermissionQuestionAskAfter(t, fixture.service, fixture.sessionID, oldRequest.ID)
		if oldRequest.ID == newRequest.ID {
			t.Fatalf("replacement reused ask id %q", oldRequest.ID)
		}
		if err := fixture.service.ReplyAsk(fixture.ctx, fixture.sessionID, oldRequest.ID, []string{"stale"}); err == nil {
			t.Fatal("stale ask reply was accepted")
		}
		if err := fixture.service.ReplyAsk(fixture.ctx, fixture.sessionID, newRequest.ID, []string{"replacement answer"}); err != nil {
			t.Fatal(err)
		}
		completed := waitForIdle(t, fixture.service, fixture.sessionID)
		if len(completed.Messages) == 0 || !strings.Contains(completed.Messages[len(completed.Messages)-1].Text, "ask-new completed") {
			t.Fatalf("replacement ask run did not complete: %#v", completed.Messages)
		}
	})

	t.Run("sessions own pending permissions", func(t *testing.T) {
		fixture := newPermissionQuestionFixture(t, server.URL, "session-isolation")
		if err := fixture.service.NewSession(fixture.ctx, appmodel.NewSessionOptions{
			ProjectID: fixture.projectID, ProviderID: fixture.providerID, Title: "isolated-b",
		}); err != nil {
			t.Fatal(err)
		}
		snapshot, err := fixture.service.Snapshot(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Sessions) != 2 {
			t.Fatalf("sessions = %#v", snapshot.Sessions)
		}
		firstID := fixture.sessionID
		secondID := ""
		for _, session := range snapshot.Sessions {
			if session.ID != firstID {
				secondID = session.ID
			}
		}
		if secondID == "" {
			t.Fatal("second session was not created")
		}
		if err := fixture.service.Send(fixture.ctx, firstID, "isolated-a", appmodel.Correction); err != nil {
			t.Fatal(err)
		}
		if err := fixture.service.Send(fixture.ctx, secondID, "isolated-b", appmodel.Correction); err != nil {
			t.Fatal(err)
		}
		firstPermission := waitPermissionQuestionPermission(t, fixture.service, firstID, "exec_command")
		secondPermission := waitPermissionQuestionPermission(t, fixture.service, secondID, "exec_command")
		if firstPermission.ID == secondPermission.ID {
			t.Fatalf("two sessions shared permission id %q", firstPermission.ID)
		}
		if err := fixture.service.ReplyPermission(fixture.ctx, firstID, secondPermission.ID, "allow"); err == nil {
			t.Fatal("session A accepted session B permission reply")
		}
		if err := fixture.service.ReplyPermission(fixture.ctx, secondID, firstPermission.ID, "allow"); err == nil {
			t.Fatal("session B accepted session A permission reply")
		}
		if err := fixture.service.ReplyPermission(fixture.ctx, firstID, firstPermission.ID, "allow"); err != nil {
			t.Fatal(err)
		}
		if err := fixture.service.ReplyPermission(fixture.ctx, secondID, secondPermission.ID, "allow"); err != nil {
			t.Fatal(err)
		}
		waitForIdle(t, fixture.service, firstID)
		waitForIdle(t, fixture.service, secondID)
		for _, name := range []string{"isolated-a.txt", "isolated-b.txt"} {
			if _, err := os.Stat(filepath.Join(fixture.project, name)); err != nil {
				t.Fatalf("session-owned file %s missing: %v", name, err)
			}
		}
	})

	permissionQuestionAssertNoProviderErrors(t, protocolErrors)
}

type permissionQuestionFixture struct {
	service    *Service
	ctx        context.Context
	project    string
	dataDir    string
	projectID  string
	providerID string
	sessionID  string
}

func newPermissionQuestionFixture(t *testing.T, serverURL, title string) permissionQuestionFixture {
	t.Helper()
	ctx := context.Background()
	project := t.TempDir()
	dataDir := t.TempDir()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(ctx); err != nil {
			t.Errorf("close service: %v", err)
		}
	})
	providerID := "permission-question-fixture"
	provider := appmodel.Provider{
		ID: providerID, Name: "授权提问协议替身", Format: appmodel.ChatCompletions,
		BaseURL: serverURL + "/v1", Model: "permission-question-fixture-model",
	}
	if err := service.SaveProvider(ctx, provider, permissionQuestionFixtureKey); err != nil {
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
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{
		ProjectID: projectID, ProviderID: providerID, Title: title, FullAccess: false,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) == 0 {
		t.Fatalf("session snapshot = %#v, error = %v", snapshot.Sessions, err)
	}
	return permissionQuestionFixture{
		service: service, ctx: ctx, project: project, dataDir: dataDir,
		projectID: projectID, providerID: providerID, sessionID: snapshot.Sessions[len(snapshot.Sessions)-1].ID,
	}
}

func waitPermissionQuestionPermission(t *testing.T, service *Service, sessionID, tool string) appmodel.Permission {
	return waitPermissionQuestionPermissionAfter(t, service, sessionID, tool, "")
}

func waitPermissionQuestionPermissionAfter(t *testing.T, service *Service, sessionID, tool, excludedID string) appmodel.Permission {
	t.Helper()
	var last appmodel.Session
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		session := applicationSession(snapshot, sessionID)
		if session != nil {
			last = *session
			for _, permission := range session.Permissions {
				if permission.Tool == tool && permission.ID != excludedID {
					return permission
				}
			}
			if session.State == appmodel.Failed {
				t.Fatalf("session failed while waiting for %s permission: %s", tool, session.Error)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("permission %s did not become visible for session %s; last=%#v", tool, sessionID, last)
	return appmodel.Permission{}
}

func waitPermissionQuestionAsk(t *testing.T, service *Service, sessionID string) appmodel.AskRequest {
	return waitPermissionQuestionAskAfter(t, service, sessionID, "")
}

func waitPermissionQuestionAskAfter(t *testing.T, service *Service, sessionID, excludedID string) appmodel.AskRequest {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		session := applicationSession(snapshot, sessionID)
		if session != nil {
			for _, question := range session.Questions {
				if question.ID != excludedID {
					return question
				}
			}
			if session.State == appmodel.Failed {
				t.Fatalf("session failed while waiting for question: %s", session.Error)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("question did not become visible for session %s", sessionID)
	return appmodel.AskRequest{}
}

func permissionQuestionMarker(body string) string {
	for _, marker := range []string{
		"permission-new", "permission-old", "ask-new", "ask-old", "isolated-a", "isolated-b",
	} {
		if strings.Contains(body, marker) {
			return marker
		}
	}
	return ""
}

func permissionQuestionRecordError(errors chan<- error, err error) {
	select {
	case errors <- err:
	default:
	}
}

func permissionQuestionAssertNoProviderErrors(t *testing.T, errors <-chan error) {
	t.Helper()
	select {
	case err := <-errors:
		t.Fatal(err)
	default:
	}
}
