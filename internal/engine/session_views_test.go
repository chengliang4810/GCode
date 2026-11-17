package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func seedViewedConversations(t *testing.T, home string) (string, string) {
	t.Helper()
	core := openStorageTestCore(t, home)
	first := newTestSession(t, core, "", "first")
	second := newTestSession(t, core, "", "second")
	storeTestMessage(t, core, first.ID, "first-body")
	storeTestMessage(t, core, second.ID, "second-body")
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	return first.ID, second.ID
}

func TestConversationStartupReadsOnlyIndexAndRequestedBody(t *testing.T) {
	home := t.TempDir()
	first, second := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	if len(core.loadedSessions) != 0 || len(core.history) != 0 || len(core.state.Messages) != 0 {
		t.Fatal("startup loaded historical conversation bodies")
	}
	summary, err := core.Bootstrap(t.Context())
	if err != nil || len(summary.Sessions) != 2 || len(summary.Messages) != 0 {
		t.Fatalf("invalid summary: %+v, %v", summary, err)
	}
	view, err := core.Snapshot(t.Context(), first)
	if err != nil || len(view.Messages[first]) != 1 || len(view.Messages[second]) != 0 {
		t.Fatalf("view contains wrong conversation details: %+v, %v", view.Messages, err)
	}
	if len(core.loadedSessions) != 0 || len(core.history) != 0 {
		t.Fatal("one-shot snapshot retained unreferenced conversation bodies")
	}
	if view.Messages[first][0].Blocks[0].Text != "first-body" {
		t.Fatal("released engine state invalidated its owned snapshot")
	}
}

func TestConversationSubscribersOwnIndependentScopesAndReleaseBodies(t *testing.T) {
	home := t.TempDir()
	first, second := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	ctxA, cancelA := context.WithCancel(t.Context())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(t.Context())
	defer cancelB()
	a, err := core.Subscribe(ctxA, first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := core.Subscribe(ctxB, second)
	if err != nil {
		t.Fatal(err)
	}
	viewA, viewB := <-a, <-b
	if len(viewA.Snapshot.Messages[first]) != 1 || len(viewA.Snapshot.Messages[second]) != 0 ||
		len(viewB.Snapshot.Messages[second]) != 1 || len(viewB.Snapshot.Messages[first]) != 0 {
		t.Fatal("subscribers received another viewer's conversation")
	}
	cancelA()
	deadline := time.Now().Add(5 * time.Second)
	for {
		core.mu.Lock()
		released, retained := !core.loadedSessions[first], core.loadedSessions[second]
		core.mu.Unlock()
		if released && retained {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("canceling one view failed to release only its body")
		}
		time.Sleep(time.Millisecond)
	}
	core.mu.Lock()
	core.publishLocked(contracts.Event{Kind: "test.refresh"})
	core.mu.Unlock()
	event := <-b
	if len(event.Snapshot.Messages[first]) != 0 || len(event.Snapshot.Messages[second]) != 1 {
		t.Fatal("later events lost their subscriber scope")
	}
}

func TestClosingViewRetainsExecutingAndUnsavedConversation(t *testing.T) {
	home := t.TempDir()
	first, _ := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	if err := core.ensureSessionDataLocked(first); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.runs[first] = &activeRun{}
	core.evictIdleSessionsLocked()
	if !core.loadedSessions[first] {
		core.mu.Unlock()
		t.Fatal("closing a viewer evicted an executing conversation")
	}
	delete(core.runs, first)
	core.markSessionDirtyLocked(first)
	core.evictIdleSessionsLocked()
	if !core.loadedSessions[first] {
		core.mu.Unlock()
		t.Fatal("closing a viewer evicted uncommitted state")
	}
	if err := core.persistLocked(); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.evictIdleSessionsLocked()
	if core.loadedSessions[first] {
		core.mu.Unlock()
		t.Fatal("saved, idle conversation still retains its body")
	}
	core.mu.Unlock()
}

func TestUnopenedDamagedConversationDoesNotBlockAnotherView(t *testing.T) {
	home := t.TempDir()
	first, second := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	record := core.sessionRecords[second]
	path := filepath.Join(core.storage.Directory(), "sessions", second, record+".json")
	if err := os.WriteFile(path, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	view, err := core.Snapshot(t.Context(), first)
	if err != nil || len(view.Messages[first]) != 1 {
		t.Fatalf("unopened damaged history blocked another view: %v", err)
	}
	damaged, err := core.Snapshot(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range damaged.Sessions {
		if session.ID == second && !strings.Contains(session.StorageError, "checksum") {
			t.Fatal("damaged history was presented as an empty usable conversation")
		}
	}
}

func TestApplicationUsageReadsUnloadedHistoryWithoutRetainingBodies(t *testing.T) {
	home := t.TempDir()
	first, second := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	usage, err := core.ReadApplicationUsage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if core.state.ApplicationUsage == nil || len(core.state.ApplicationUsage.Records[first]) != 1 || len(core.state.ApplicationUsage.Records[second]) != 1 {
		t.Fatal("saved usage omitted unloaded conversations")
	}
	if len(usage.Records) != 0 || len(usage.Runs) != 0 {
		t.Fatal("usage response exposed the ledger instead of the report")
	}
	if len(core.loadedSessions) != 0 || len(core.state.Messages) != 0 {
		t.Fatal("reading usage installed historical bodies")
	}
	if _, err := core.Snapshot(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	after, err := core.ReadApplicationUsage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Report.Summary != after.Report.Summary {
		t.Fatal("opening history changed the saved application totals")
	}

}

func TestCanceledNamingKeepsBodyUntilRequestAccountingCommits(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	session := newTestSession(t, core, "", "Original title")
	storeTestMessage(t, core, session.ID, "original-body")
	core.mu.Lock()
	core.state.Settings.General.ThreadNamingModel = "checks/selected/model"
	core.startNamingLocked(session, "original-body")
	core.mu.Unlock()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("naming HTTP request did not start")
	}
	core.mu.Lock()
	core.cancelNamingLocked(session.ID)
	core.evictIdleSessionsLocked()
	retained := core.loadedSessions[session.ID]
	core.mu.Unlock()
	if !retained {
		t.Fatal("canceled request still needs to commit accounting, but its body was evicted")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		core.mu.Lock()
		pending := len(core.state.ModelRequests) != 0
		released := !core.loadedSessions[session.ID]
		core.mu.Unlock()
		if released && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("completed naming did not release its retained body")
		}
		time.Sleep(time.Millisecond)
	}
	view, err := core.Snapshot(t.Context(), session.ID)
	if err != nil || len(view.Messages[session.ID]) != 1 || len(view.ModelRequests) != 1 {
		t.Fatalf("canceled request lost its original history or accounting: %v", err)
	}
}

func TestAcceptedRequestRemainsIdempotentAfterBodyEvictionAndRestart(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		goalResponse(w, model.Chat, "Saved response", nil, true)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	command := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		RequestID: "durable-accepted-request", ModelID: "checks/unselected", Text: "Run once",
	}}
	if err := core.Execute(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	state := waitIdle(t, core)
	if len(state.Sessions) != 1 {
		t.Fatal("initial request did not create one conversation")
	}
	core.mu.Lock()
	core.evictIdleSessionsLocked()
	resident := core.loadedSessions[state.Sessions[0].ID]
	core.mu.Unlock()
	if resident {
		t.Fatal("completed conversation body is still resident")
	}
	if err := core.Execute(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(core.state.Sessions) != 1 {
		t.Fatal("retrying an accepted request after body eviction repeated execution")
	}
	home := filepath.Dir(core.storage.ConfigDirectory())
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openStorageTestCore(t, home)
	defer reopened.Close()
	if len(reopened.loadedSessions) != 0 {
		t.Fatal("restart eagerly loaded body to reconstruct request identity")
	}
	if err := reopened.Execute(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(reopened.state.Sessions) != 1 {
		t.Fatal("restart forgot the accepted request identity")
	}
}

func TestHiddenSessionCommandReleasesItsCommittedBody(t *testing.T) {
	home := t.TempDir()
	first, _ := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	title := "Renamed while hidden"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: first, Title: &title}}); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	resident := core.loadedSessions[first]
	core.mu.Unlock()
	if resident {
		t.Fatal("a command retained an unviewed conversation after committing")
	}
	snapshot, err := core.Snapshot(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Messages[first]) != 1 || len(snapshot.Messages[first][0].Blocks) != 1 || snapshot.Messages[first][0].Blocks[0].Text != "first-body" {
		t.Fatal("reloading the renamed conversation lost its original body")
	}
	if snapshot.Sessions[core.sessionIndex(first)].Title != "Renamed while hidden" {
		t.Fatal("reloading the conversation lost its committed title")
	}
}

func TestBackgroundSubagentRetainsItsIdleParentsBody(t *testing.T) {
	home := t.TempDir()
	parent, child := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	if err := core.ensureSessionDataLocked(parent); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.runs[child] = &activeRun{subagent: &subagentExecution{parentSessionID: parent, background: true}}
	core.updateRunningSubagentsLocked(parent)
	core.evictIdleSessionsLocked()
	retained := core.loadedSessions[parent]
	delete(core.runs, child)
	core.updateRunningSubagentsLocked(parent)
	core.evictIdleSessionsLocked()
	released := !core.loadedSessions[parent]
	core.mu.Unlock()
	if !retained || !released {
		t.Fatal("parent body lifetime did not follow its background child")
	}
}

func TestSubagentCompletionPersistsItsParentsUpdatedBlock(t *testing.T) {
	home := t.TempDir()
	parent, child := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	if err := core.ensureSessionDataLocked(parent); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.state.Messages[parent][0].Blocks = append(core.state.Messages[parent][0].Blocks, contracts.Block{
		ID: "child-result", Kind: "subagent", Subagent: &contracts.Subagent{SessionID: child, Status: "running"},
	})
	if err := core.persistLocked(parent); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.finishSubagentLocked(contracts.Session{ID: child}, &activeRun{
		output: "Saved child answer", subagent: &subagentExecution{parentSessionID: parent, parentBlockID: "child-result"},
	}, nil)
	err := core.persistLocked(child)
	core.evictIdleSessionsLocked()
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(t.Context(), parent)
	if err != nil {
		t.Fatal(err)
	}
	result := snapshot.Messages[parent][0].Blocks[1].Subagent
	if result.Status != "completed" || result.Summary != "Saved child answer" {
		t.Fatal("child completion did not commit its parent's updated tool result")
	}
}

func TestTaskOutputReloadsCompletedSubagentWithoutRetainingItsBody(t *testing.T) {
	home := t.TempDir()
	parent, child := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	if err := core.ensureSessionDataLocked(child); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.state.Sessions[core.sessionIndex(child)].ParentSessionID = parent
	core.state.Sessions[core.sessionIndex(child)].SubagentType = "worker"
	core.history[child] = append(core.history[child], model.Message{Role: "assistant", Text: "Completed child answer"})
	err := core.persistLocked(child)
	core.evictIdleSessionsLocked()
	parentSession := core.state.Sessions[core.sessionIndex(parent)]
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := core.sessionRuntimeTools(parentSession, "")
	if err != nil {
		t.Fatal(err)
	}
	result := callSessionTestTool(t, tools, "TaskOutput", map[string]any{"task_id": child, "block": false})
	if !strings.Contains(result.Text, "Completed child answer") {
		t.Fatal("TaskOutput lost the released child's saved response")
	}
	core.mu.Lock()
	resident := core.loadedSessions[child]
	core.mu.Unlock()
	if resident {
		t.Fatal("reading TaskOutput retained the completed child's body")
	}
}

func TestBackgroundHookCommitsQueueAfterConversationBodyWasReleased(t *testing.T) {
	home := t.TempDir()
	session, _ := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	err := core.queueHookResult(session, agent.HookExecution{
		Result: agent.HookResult{Context: "Saved background hook result"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openStorageTestCore(t, home)
	defer reopened.Close()
	view, err := reopened.Snapshot(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Queue) != 1 || !strings.Contains(view.Queue[0].Text, "Saved background hook result") {
		t.Fatal("background hook result was lost after committing an unloaded conversation")
	}
	if len(view.Messages[session]) != 1 || view.Messages[session][0].Blocks[0].Text != "first-body" {
		t.Fatal("background hook result overwrote its conversation's saved history")
	}
}

func TestAsyncHookCompletionCommitsRecordAfterConversationBodyWasReleased(t *testing.T) {
	home := t.TempDir()
	session, _ := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	sessionIndex := core.sessionIndex(session)
	if sessionIndex < 0 {
		core.mu.Unlock()
		t.Fatal("seeded session is missing")
	}
	sessionMetadata := core.state.Sessions[sessionIndex]
	core.mu.Unlock()
	runner, invoke := core.runHookRunner(sessionMetadata, &activeRun{
		id:      "async-hook-run",
		pending: map[string]contracts.QueuedMessage{},
	}, []contracts.HookRule{{
		ID: "async-record", Event: contracts.HookStop, Type: "command", Command: program,
		Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper",
			filepath.Join(home, "hook-input.json"), `{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"saved async hook"}}`},
		Enabled: true, Async: true,
	}})
	if _, err := invoke(t.Context(), agent.HookInput{Event: contracts.HookStop}); err != nil {
		t.Fatal(err)
	}
	if err := runner.WaitBackground(); err != nil {
		t.Fatal(err)
	}
	view, err := core.Snapshot(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	foundHook, foundQueue := false, false
	for _, message := range view.Messages[session] {
		for _, block := range message.Blocks {
			if block.Kind == "hook" && block.Hook != nil && block.Hook.RuleID == "async-record" {
				foundHook = true
			}
		}
	}
	for _, row := range view.Queue {
		if row.SessionID == session && strings.Contains(row.Text, "saved async hook") {
			foundQueue = true
		}
	}
	if !foundHook || !foundQueue {
		t.Fatalf("async hook completion was not committed with the released body: hook=%v queue=%v messages=%+v queueRows=%+v", foundHook, foundQueue, view.Messages[session], view.Queue)
	}
}

func TestLateAsyncHookAfterSessionDeleteDoesNotRecreateState(t *testing.T) {
	home := t.TempDir()
	session, _ := seedViewedConversations(t, home)
	core := openStorageTestCore(t, home)
	defer core.Close()
	startedPath := filepath.Join(home, "hook-started")
	releasePath := filepath.Join(home, "hook-release")
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	sessionIndex := core.sessionIndex(session)
	if sessionIndex < 0 {
		core.mu.Unlock()
		t.Fatal("seeded session is missing")
	}
	sessionMetadata := core.state.Sessions[sessionIndex]
	core.mu.Unlock()
	runner, invoke := core.runHookRunner(sessionMetadata, &activeRun{
		id:      "late-hook-run",
		pending: map[string]contracts.QueuedMessage{},
	}, []contracts.HookRule{{
		ID: "late-record", Event: contracts.HookStop, Type: "command", Command: program,
		Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper",
			filepath.Join(home, "hook-input.json"), `{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"late hook"}}`, "", startedPath, releasePath},
		Enabled: true, Async: true,
	}})
	if _, err := invoke(t.Context(), agent.HookInput{Event: contracts.HookStop}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("asynchronous hook did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.delete", ID: session}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runner.WaitBackground(); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	if core.sessionIndex(session) >= 0 {
		t.Fatal("deleted session was recreated by its late hook")
	}
	if _, ok := core.state.Messages[session]; ok {
		t.Fatal("late hook recreated an orphan message map")
	}
	for _, row := range core.state.Queue {
		if row.SessionID == session {
			t.Fatal("late hook recreated an orphan queue row")
		}
	}
}
