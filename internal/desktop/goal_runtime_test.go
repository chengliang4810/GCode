package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestGoalRealCoreComposerCreationPauseResumeAndStop(t *testing.T) {
	// This exercises the actual core, storage and Chat Completions adapter.
	// The HTTP provider is a local fixture; no external model is contacted.
	verifying := make(chan struct{}, 4)
	cancelled := make(chan struct{}, 4)
	var workCalls, verificationCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var system string
		if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
			_ = json.Unmarshal(request.Messages[0].Content, &system)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.HasPrefix(system, "Review whether the objective below is fully satisfied") {
			if len(request.Tools) != 0 {
				http.Error(w, "the verifier must not receive tools", http.StatusBadRequest)
				return
			}
			verificationCalls.Add(1)
			// Report both counters so the adapter can distinguish known zero
			// output from missing usage, then hold verification until cancelled.
			fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":0}}\n\n")
			w.(http.Flusher).Flush()
			verifying <- struct{}{}
			<-r.Context().Done()
			cancelled <- struct{}{}
			return
		}
		workCalls.Add(1)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"WORK_EVIDENCE_READY\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	// Close the core before the HTTP server so blocked verifier requests are
	// cancelled even when an assertion aborts the test.
	t.Cleanup(provider.Close)
	app, core := runtimeSettingsTestApp(t)
	ctx := context.Background()
	config := model.Provider{
		ID: "goal-ui-fixture", Name: "Goal UI fixture", Protocol: model.Chat,
		BaseURL: provider.URL, Models: []model.Model{{ID: "goal-model"}},
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	app.modelID = "goal-ui-fixture/goal-model"
	tt := ui.NewTester(app.View, 1320, 880)
	if app.sessionID != "" || len(snapshot.Sessions) != 0 {
		t.Fatal("the goal must start from the new-session composer")
	}
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/go")
	tt.Key(0, ui.KeyEnter)
	const objective = "Verify goal commands through the native composer"
	tt.Type(objective)
	tt.Key(0, ui.KeyEnter)
	if app.errorText != "" || app.pendingNew || app.sessionID == "" {
		t.Fatalf("new goal did not take over its accepted session: error=%q pending=%v session=%q", app.errorText, app.pendingNew, app.sessionID)
	}
	sessionID := app.sessionID

	waitFor := func(description string, predicate func(contracts.Snapshot) bool) contracts.Snapshot {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		var latest contracts.Snapshot
		for time.Now().Before(deadline) {
			current, snapshotErr := core.Snapshot(ctx, sessionID)
			if snapshotErr != nil {
				t.Fatal(snapshotErr)
			}
			if predicate(current) {
				app.apply(current)
				tt.Frame()
				return current
			}
			latest = current
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s: sessions=%+v runs=%+v error=%q", description, latest.Sessions, latest.Runs, app.errorText)
		return contracts.Snapshot{}
	}
	waitVerifier := func() contracts.Snapshot {
		t.Helper()
		select {
		case <-verifying:
		case <-time.After(5 * time.Second):
			current, snapshotErr := core.Snapshot(ctx, sessionID)
			t.Fatalf("the core did not send a verifier request: work=%d verification=%d error=%v state=%+v", workCalls.Load(), verificationCalls.Load(), snapshotErr, current)
		}
		return waitFor("the verifying goal projection", func(current contracts.Snapshot) bool {
			return len(current.Sessions) == 1 && current.Sessions[0].ID == sessionID &&
				current.Sessions[0].Goal != nil && current.Sessions[0].Goal.Status == contracts.GoalVerifying
		})
	}
	waitCancellation := func() {
		t.Helper()
		select {
		case <-cancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("the blocked verifier HTTP request was not cancelled")
		}
	}
	first := waitVerifier()
	goalID := first.Sessions[0].Goal.ID
	if len(first.Runs) != 1 || first.Runs[0].SessionID != sessionID || !strings.HasPrefix(first.Runs[0].RequestID, "primary:") ||
		first.Sessions[0].Goal.Objective != objective || first.Sessions[0].Goal.TokensUsed < 14 ||
		app.currentSession().Goal.ID != goalID || !tt.HasText("Verifying") || !tt.HasText("WORK_EVIDENCE_READY") {
		t.Fatalf("typed creation lost RequestID, goal identity, known work usage or its transcript: %+v", first)
	}
	if err := tt.Click("Pause goal"); err != nil {
		t.Fatal(err)
	}
	paused := waitFor("the paused idle session", func(current contracts.Snapshot) bool {
		return current.Sessions[0].Goal.Status == contracts.GoalPaused && current.Sessions[0].Status == contracts.Idle && current.Runs[0].Status != "running"
	})
	waitCancellation()
	if paused.Sessions[0].Goal.ID != goalID || app.sessionID != sessionID || !tt.HasText("Paused") {
		t.Fatal("pause replaced the goal or navigated away from its session")
	}
	if err := tt.Click("Resume goal"); err != nil {
		t.Fatal(err)
	}
	second := waitVerifier()
	if len(second.Runs) != 2 || second.Runs[1].SessionID != sessionID || second.Sessions[0].Goal.ID == goalID ||
		second.Sessions[0].Goal.Objective != objective ||
		second.Sessions[0].Goal.TokensUsed < 28 || workCalls.Load() != 2 || verificationCalls.Load() != 2 {
		t.Fatal("resume lost the objective/accounting or reused the cancelled execution identity")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "goal.pause", SessionID: sessionID, ID: goalID}); err == nil {
		t.Fatal("a stale control paused the resumed execution")
	}
	goalID = second.Sessions[0].Goal.ID
	if err := tt.Click("Stop response"); err != nil {
		t.Fatal(err)
	}
	stopped := waitFor("Stop pausing the goal", func(current contracts.Snapshot) bool {
		return len(current.Runs) == 2 && current.Runs[1].Status != "running" &&
			current.Sessions[0].Goal.Status == contracts.GoalPaused && current.Sessions[0].Status == contracts.Idle
	})
	waitCancellation()
	if stopped.Sessions[0].Goal.ID != goalID || len(stopped.Sessions[0].Goal.Verifications) != 0 ||
		app.currentSession().Goal.Status != contracts.GoalPaused || !tt.HasText("Paused") {
		t.Fatal("ordinary Stop did not persist and display the same paused goal")
	}
	t.Logf("actual core and local HTTP adapter: session=%s creationRequest=%s goal=%s workCalls=%d verifierCalls=%d tokens=%d",
		sessionID, first.Runs[0].RequestID, goalID, workCalls.Load(), verificationCalls.Load(), stopped.Sessions[0].Goal.TokensUsed)
}
