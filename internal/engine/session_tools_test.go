package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func sessionTestTool(t *testing.T, tools []agent.Tool, name string) agent.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition().Name == name {
			return tool
		}
	}
	t.Fatalf("missing session tool %s", name)
	return nil
}

func callSessionTestTool(t *testing.T, tools []agent.Tool, name string, args map[string]any) model.ToolOutput {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := sessionTestTool(t, tools, name).Execute(context.Background(), "", raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSessionDeleteJoinsBackgroundOwnerOutsideEngineLock(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	for range 2 {
		if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, other := core.state.Sessions[0], core.state.Sessions[1]
	tools, err := core.sessionRuntimeTools(deleted, "")
	if err != nil {
		t.Fatal(err)
	}
	otherTools, err := core.sessionRuntimeTools(other, "")
	if err != nil {
		t.Fatal(err)
	}
	started := callSessionTestTool(t, tools, "Bash", map[string]any{"command": "sleep 30", "run_in_background": true})
	taskID := started.Data.(map[string]any)["backgroundTaskId"].(string)
	otherStarted := callSessionTestTool(t, otherTools, "Bash", map[string]any{"command": "sleep 0.1; printf survived", "run_in_background": true})
	otherID := otherStarted.Data.(map[string]any)["backgroundTaskId"].(string)
	lockReleased := false
	core.mu.Lock()
	owner := core.sessionTools[deleted.ID]
	cleanup := owner.cleanup
	owner.cleanup = func() {
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if core.mu.TryLock() {
				lockReleased = true
				core.mu.Unlock()
				break
			}
			time.Sleep(time.Millisecond)
		}
		cleanup()
	}
	core.sessionHookOnce[deleted.ID], core.sessionHookOnce[other.ID] = &agent.HookOnceState{}, &agent.HookOnceState{}
	core.sessionStarted[deleted.ID], core.sessionStarted[other.ID] = true, true
	core.mu.Unlock()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.delete", ID: deleted.ID}); err != nil {
		t.Fatal(err)
	}
	if !lockReleased {
		t.Fatal("background cleanup ran while engine lock was held")
	}
	core.mu.Lock()
	detached := core.sessionTools[deleted.ID] == nil && core.sessionHookOnce[deleted.ID] == nil && !core.sessionStarted[deleted.ID]
	preserved := core.sessionTools[other.ID] != nil && core.sessionHookOnce[other.ID] != nil && core.sessionStarted[other.ID]
	core.mu.Unlock()
	if !detached || !preserved {
		t.Fatal("delete leaked ownership state or removed unrelated session state")
	}
	out := callSessionTestTool(t, tools, "TaskOutput", map[string]any{"task_id": taskID, "block": false})
	if out.Data.(map[string]any)["task"].(map[string]any)["status"] != "killed" {
		t.Fatalf("delete returned before owned task exited: %s", out.Text)
	}
	out = callSessionTestTool(t, otherTools, "TaskOutput", map[string]any{"task_id": otherID, "timeout": 10000})
	if out.Data.(map[string]any)["task"].(map[string]any)["output"] != "survived" {
		t.Fatalf("unrelated owner was cancelled: %s", out.Text)
	}
	out = callSessionTestTool(t, otherTools, "Bash", map[string]any{"command": "printf still-open"})
	if out.Text != "still-open" {
		t.Fatalf("unrelated owner cannot accept further work: %s", out.Text)
	}
	if _, err := core.sessionRuntimeTools(deleted, ""); err == nil {
		t.Fatal("deleted session could recreate an owner")
	}
}

func TestSessionDeletePersistenceFailureKeepsBackgroundOwner(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	tools, err := core.sessionRuntimeTools(session, "")
	if err != nil {
		t.Fatal(err)
	}
	started := callSessionTestTool(t, tools, "Bash", map[string]any{"command": "sleep 30", "run_in_background": true})
	id := started.Data.(map[string]any)["backgroundTaskId"].(string)
	core.mu.Lock()
	core.sessionStarted[session.ID] = true
	core.sessionHookOnce[session.ID] = &agent.HookOnceState{}
	core.mu.Unlock()
	path := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	backup := path + ".test-backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		_ = os.Rename(backup, path)
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
		if err := os.Rename(backup, path); err != nil {
			t.Error(err)
		}
	}()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.delete", ID: session.ID}); err == nil {
		t.Fatal("delete succeeded without saving the index")
	}
	core.mu.Lock()
	kept := core.sessionIndex(session.ID) >= 0 && core.sessionTools[session.ID] != nil && core.sessionHookOnce[session.ID] != nil && core.sessionStarted[session.ID]
	core.mu.Unlock()
	if !kept {
		t.Fatal("failed delete discarded live session ownership")
	}
	out := callSessionTestTool(t, tools, "TaskOutput", map[string]any{"task_id": id, "block": false})
	if !strings.Contains(out.Text, `"status":"running"`) {
		t.Fatalf("failed delete killed accepted background work: %s", out.Text)
	}
	out = callSessionTestTool(t, tools, "Bash", map[string]any{"command": "printf retained"})
	if out.Text != "retained" {
		t.Fatalf("failed delete closed its owner: %s", out.Text)
	}
}

func TestHostCloseWaitsForDetachedSessionOwnerCleanup(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	if _, err := core.sessionRuntimeTools(session, ""); err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	core.mu.Lock()
	owner := core.sessionTools[session.ID]
	cleanup := owner.cleanup
	owner.cleanup = func() { close(entered); <-release; cleanup() }
	core.mu.Unlock()
	deleted := make(chan error, 1)
	go func() {
		deleted <- core.Execute(context.Background(), contracts.Command{Kind: "session.delete", ID: session.ID})
	}()
	select {
	case <-entered:
	case err := <-deleted:
		close(release)
		_ = core.Close()
		t.Fatalf("delete did not invoke cleanup: %v", err)
	case <-time.After(5 * time.Second):
		close(release)
		_ = core.Close()
		t.Fatal("delete did not reach detached owner cleanup")
	}
	closed := make(chan error, 1)
	go func() { closed <- core.Close() }()
	select {
	case err := <-closed:
		close(release)
		<-deleted
		t.Fatalf("host closed while deleted-session cleanup was still pending: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("host close did not finish after cleanup")
	}
}
