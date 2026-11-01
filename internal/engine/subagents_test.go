package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestSubagentUsesConfiguredProviderAndReasoningAcrossProtocols(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var parentCalls, childCalls atomic.Int32
			parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				w.Header().Set("Content-Type", "text/event-stream")
				if parentCalls.Add(1) == 1 {
					schema := delegationSchema(t, body)
					properties := schema["properties"].(map[string]any)
					if properties["model"] == nil || properties["effort"] != nil {
						t.Error("Agent must expose model roles while reasoning stays in user configuration")
					}
					subagentCall(w, `{"description":"Write the answer","prompt":"CHILD TASK","subagent_type":"worker","model":"haiku"}`)
					return
				}
				data, _ := json.Marshal(body)
				if !strings.Contains(string(data), "CHILD FINISHED") || !strings.Contains(string(data), "child_session_id") {
					t.Error("parent did not receive the child's final result")
				}
				subagentText(w, model.Chat, "PARENT FINISHED")
			}))
			defer parent.Close()
			child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if body["model"] != "child-model" {
					t.Error("child used the wrong model")
				}
				if (protocol == model.Messages && r.Header.Get("x-api-key") != "child-secret") ||
					(protocol != model.Messages && r.Header.Get("Authorization") != "Bearer child-secret") {
					t.Error("child did not use its own provider credentials")
				}
				var effort any
				switch protocol {
				case model.Chat:
					effort = body["reasoning_effort"]
				case model.Responses:
					effort = body["reasoning"].(map[string]any)["effort"]
				case model.Messages:
					effort = body["output_config"].(map[string]any)["effort"]
				}
				if effort != "max" {
					t.Errorf("child effort = %v, want max", effort)
				}
				encoded, _ := json.Marshal(body["tools"])
				if strings.Contains(string(encoded), `"name":"Agent"`) {
					t.Error("child can recursively delegate")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if childCalls.Add(1) == 1 {
					subagentWireCall(w, protocol, "Write", `{"file_path":"child.txt","content":"child content"}`)
				} else {
					subagentText(w, protocol, "CHILD FINISHED")
				}
			}))
			defer child.Close()
			core, workspace := subagentCore(t, parent.URL, child.URL, protocol)
			state, _ := core.Snapshot(context.Background())
			profiles := state.Settings
			profiles.Subagents = []contracts.SubagentProfile{{
				ID: "worker", Name: "Worker", Description: "Implement the assigned task",
				Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Max"},
			}}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &profiles}); err != nil {
				t.Fatal(err)
			}
			startSubagentParent(t, core)
			state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
				return len(s.Runs) == 2 && runsFinished(s)
			})
			if parentCalls.Load() != 2 || childCalls.Load() != 2 {
				t.Fatalf("calls parent=%d child=%d", parentCalls.Load(), childCalls.Load())
			}
			data, err := os.ReadFile(filepath.Join(workspace, "child.txt"))
			if err != nil || string(data) != "child content" {
				t.Fatalf("child did not execute the real write tool: %q %v", data, err)
			}
			var childSession, parentSession contracts.Session
			for _, session := range state.Sessions {
				if session.SubagentType != "" {
					childSession = session
				} else {
					parentSession = session
				}
			}
			if childSession.ModelID != "child/child-model" || childSession.ReasoningLevel != "Max" ||
				childSession.ParentSessionID != parentSession.ID || childSession.CWD != workspace {
				t.Fatalf("child selection or ownership lost: %+v", childSession)
			}
			var link *contracts.Subagent
			for _, message := range state.Messages[parentSession.ID] {
				for _, block := range message.Blocks {
					if block.Subagent != nil {
						link = block.Subagent
					}
				}
			}
			if link == nil || link.Status != "completed" || link.SessionID != childSession.ID ||
				link.ModelID != childSession.ModelID || link.Summary != "CHILD FINISHED" {
				t.Fatalf("parent row is not the actual child projection: %+v", link)
			}
			if err = core.Execute(context.Background(), contracts.Command{
				Kind: "message.send", Send: &contracts.SendRequest{SessionID: childSession.ID, Text: "delegate again"},
			}); err == nil {
				t.Fatal("read-only child session accepted a new root execution")
			}
		})
	}
}

func TestSubagentInvalidSelectionDoesNotFallBack(t *testing.T) {
	for _, selection := range []contracts.ModelSelection{
		{ModelID: "missing/model", Reasoning: "High"},
		{ModelID: "child/child-model", Reasoning: "Unsupported"},
		{ModelID: "child/child-model"},
	} {
		t.Run(selection.ModelID+selection.Reasoning, func(t *testing.T) {
			var parentCalls, childCalls atomic.Int32
			parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				w.Header().Set("Content-Type", "text/event-stream")
				if parentCalls.Add(1) == 1 {
					subagentCall(w, `{"description":"Try selection","prompt":"task","subagent_type":"worker"}`)
				} else {
					data, _ := json.Marshal(body)
					if !strings.Contains(string(data), "subagent") {
						t.Error("parent lost selection failure")
					}
					subagentText(w, model.Chat, "selection unavailable")
				}
			}))
			defer parent.Close()
			child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				childCalls.Add(1)
				subagentText(w, model.Chat, "unexpected")
			}))
			defer child.Close()
			core, _ := subagentCore(t, parent.URL, child.URL, model.Chat)
			state, _ := core.Snapshot(context.Background())
			state.Settings.Subagents = []contracts.SubagentProfile{{
				ID: "worker", Name: "Worker", Description: "A task", Model: &selection,
			}}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
				t.Fatal(err)
			}
			startSubagentParent(t, core)
			state = waitSubagentState(t, core, runsFinished)
			if childCalls.Load() != 0 || parentCalls.Load() != 2 || len(state.Sessions) != 1 {
				t.Fatalf("invalid selection launched a child or fallback: %d %+v", childCalls.Load(), state.Sessions)
			}
		})
	}
}

func TestBackgroundSubagentNotifiesParentAfterItsTurnEnds(t *testing.T) {
	var parentCalls atomic.Int32
	childStarted, releaseChild := make(chan struct{}), make(chan struct{})
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		w.Header().Set("Content-Type", "text/event-stream")
		switch parentCalls.Add(1) {
		case 1:
			subagentCall(w, `{"description":"Background lookup","prompt":"task","subagent_type":"worker","run_in_background":true}`)
		case 2:
			subagentText(w, model.Chat, "PARENT FIRST TURN")
		default:
			data, _ := json.Marshal(body)
			if !strings.Contains(string(data), "BACKGROUND FINISHED") {
				t.Error("new parent turn did not receive the completion")
			}
			subagentText(w, model.Chat, "NOTIFICATION RECEIVED")
		}
	}))
	defer parent.Close()
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(childStarted)
		select {
		case <-releaseChild:
			subagentText(w, model.Responses, "BACKGROUND FINISHED")
		case <-r.Context().Done():
		}
	}))
	defer child.Close()
	core, _ := subagentCore(t, parent.URL, child.URL, model.Responses)
	state, _ := core.Snapshot(context.Background())
	state.Settings.Subagents = []contracts.SubagentProfile{{
		ID: "worker", Name: "Worker", Description: "Lookup",
		Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	startSubagentParent(t, core)
	waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return len(s.Runs) == 2 && s.Runs[0].Status == "completed"
	})
	state, _ = core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "session.delete", ID: state.Runs[0].SessionID,
	}); err == nil {
		t.Fatal("deleting the parent orphaned its running background task")
	}
	select {
	case <-childStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("background child never started")
	}
	close(releaseChild)
	state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return len(s.Runs) == 3 && runsFinished(s)
	})
	if parentCalls.Load() != 3 || len(state.Queue) != 0 {
		t.Fatalf("background completion was lost or duplicated: calls=%d queue=%+v", parentCalls.Load(), state.Queue)
	}
	for _, session := range state.Sessions {
		if session.SubagentType != "" {
			continue
		}
		users := 0
		for _, message := range state.Messages[session.ID] {
			if message.Role == "user" {
				users++
			}
		}
		if users != 1 || session.RunningSubagents != 0 {
			t.Fatalf("completion was displayed as user input or activity leaked: users=%d %+v", users, session)
		}
	}
}

func TestSynchronousSubagentInheritsAndCancelsWithParent(t *testing.T) {
	childStarted := make(chan struct{})
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if body["reasoning_effort"] != "high" {
			t.Error("parent or inherited child lost reasoning selection")
		}
		encoded, _ := json.Marshal(body["tools"])
		if strings.Contains(string(encoded), `"name":"Agent"`) {
			subagentCall(w, `{"description":"Investigate","prompt":"task","subagent_type":"Explore"}`)
			return
		}
		for _, name := range []string{"Write", "Edit", "Bash"} {
			if strings.Contains(string(encoded), `"name":"`+name+`"`) {
				t.Errorf("read-only explorer has %s", name)
			}
		}
		close(childStarted)
		<-r.Context().Done()
	}))
	defer parent.Close()
	core, _ := subagentCore(t, parent.URL, parent.URL, model.Chat)
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
			ID: "parent", Name: "Parent", Protocol: model.Chat, BaseURL: parent.URL,
			Models: []model.Model{{ID: "parent-model", ReasoningLevels: []string{"low", "high"}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{ModelID: "parent/parent-model", ReasoningLevel: "High", Text: "Start"},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-childStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("inherited child did not start")
	}
	state, _ := core.Snapshot(context.Background())
	for _, session := range state.Sessions {
		if session.SubagentType == "" {
			if err := core.Execute(context.Background(), contracts.Command{Kind: "message.stop", SessionID: session.ID}); err != nil {
				t.Fatal(err)
			}
		} else if session.ModelID != "parent/parent-model" || session.ReasoningLevel != "High" {
			t.Fatalf("inherit did not keep both model and reasoning: %+v", session)
		}
	}
	state = waitSubagentState(t, core, runsFinished)
	for _, run := range state.Runs {
		if run.Status != "interrupted" {
			t.Fatalf("accepted child was not drained on cancellation: %+v", state.Runs)
		}
	}
}

func TestBackgroundSubagentRetainsResultWhenParentModelIsRemoved(t *testing.T) {
	releaseChild := make(chan struct{})
	var parentCalls atomic.Int32
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		switch parentCalls.Add(1) {
		case 1:
			subagentCall(w, `{"description":"Lookup","prompt":"task","subagent_type":"worker","run_in_background":true}`)
		case 2:
			subagentText(w, model.Chat, "WAITING")
		default:
			data, _ := json.Marshal(body)
			if !strings.Contains(string(data), "SAVED RESULT") {
				t.Error("retry did not receive the saved completion")
			}
			subagentText(w, model.Chat, "RECOVERED")
		}
	}))
	defer parent.Close()
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-releaseChild:
			subagentText(w, model.Chat, "SAVED RESULT")
		case <-r.Context().Done():
		}
	}))
	defer child.Close()
	core, _ := subagentCore(t, parent.URL, child.URL, model.Chat)
	state, _ := core.Snapshot(context.Background())
	state.Settings.Subagents = []contracts.SubagentProfile{{
		ID: "worker", Name: "Worker", Description: "Lookup",
		Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	startSubagentParent(t, core)
	state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return len(s.Runs) == 2 && s.Runs[0].Status == "completed"
	})
	parentID := state.Runs[0].SessionID
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.delete", ID: "parent"}); err != nil {
		t.Fatal(err)
	}
	close(releaseChild)
	state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return len(s.Runs) == 2 && runsFinished(s)
	})
	for _, session := range state.Sessions {
		if session.ID == parentID && (session.Status != contracts.Errored || session.LastOutcome != "failed") {
			t.Fatalf("failed automatic continuation was hidden: %+v", session)
		}
	}
	if len(state.Queue) != 1 || !strings.Contains(state.Queue[0].Text, "SAVED RESULT") || parentCalls.Load() != 2 {
		t.Fatalf("missing parent model lost the result or selected another model: %+v", state.Queue)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
			ID: "parent", Name: "Parent", Protocol: model.Chat, BaseURL: parent.URL,
			Models: []model.Model{{ID: "parent-model"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.retry", SessionID: parentID}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return len(s.Runs) == 3 && runsFinished(s)
	})
	if parentCalls.Load() != 3 || len(state.Queue) != 0 {
		t.Fatal("restored parent did not consume exactly one saved completion")
	}
}

func TestSubagentRestartAndForkRebuildExecutionState(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	parent := newTestSession(t, core, "", "Parent")
	childID, _, err := core.reserveSessionLocked(parent.StorageRootID)
	if err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	child := parent
	child.ID, child.SubagentType, child.ParentSessionID = childID, "Explore", parent.ID
	child.Status = contracts.Working
	core.state.Sessions = append(core.state.Sessions, child)
	core.loadedSessions[child.ID] = true
	core.history[child.ID], core.context[child.ID] = []model.Message{}, []model.Message{}
	core.state.Messages[child.ID] = []contracts.Message{}
	core.state.Sessions[0].RunningSubagents = 1
	core.state.Settings.Subagents[0].Model = &contracts.ModelSelection{ModelID: "configured/model", Reasoning: "Max"}
	core.state.Messages[parent.ID] = []contracts.Message{{
		ID: "parent-message", SessionID: parent.ID, Role: "assistant",
		Blocks: []contracts.Block{{
			ID: "saved-call", Kind: "subagent",
			Subagent: &contracts.Subagent{ID: child.ID, SessionID: child.ID, Status: "running"},
		}},
	}}
	core.state.Runs = append(core.state.Runs, contracts.Run{ID: "saved-run", SessionID: child.ID, Status: "running"})
	if err = core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	core.mu.Unlock()
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	state, _ := core.Snapshot(context.Background())
	for _, session := range state.Sessions {
		if session.RunningSubagents != 0 || session.Status == contracts.Working {
			t.Fatalf("restart kept nonexistent child execution: %+v", session)
		}
	}
	if state.Messages[parent.ID][0].Blocks[0].Subagent.Status != "interrupted" ||
		state.Runs[0].Status != "interrupted" {
		t.Fatal("restart did not retain the interrupted task evidence")
	}
	if selection := state.Settings.Subagents[0].Model; selection == nil ||
		selection.ModelID != "configured/model" || selection.Reasoning != "Max" {
		t.Fatal("restart lost the explicit profile selection")
	}
	for _, kind := range []string{"session.fork", "session.sidechat"} {
		if err = core.Execute(context.Background(), contracts.Command{Kind: kind, SessionID: child.ID}); err != nil {
			t.Fatal(err)
		}
		state, _ = core.Snapshot(context.Background())
		derived := state.Sessions[0]
		if derived.SubagentType != "" || derived.RunningSubagents != 0 {
			t.Fatalf("derived session inherited execution-only fields: %+v", derived)
		}
	}
}

func TestSubagentStartupFailureDoesNotLeaveAnOrphanRun(t *testing.T) {
	core, _ := subagentCore(t, "http://127.0.0.1:1", "http://127.0.0.1:1", model.Chat)
	core.mu.Lock()
	defer core.mu.Unlock()
	parent := newTestSession(t, core, "", "Parent")
	parent.ModelID = "parent/parent-model"
	core.state.Sessions[0] = parent
	run := &activeRun{
		id: "parent-run", selection: contracts.ModelSelection{ModelID: parent.ModelID},
		providers: core.providers,
	}
	core.runs[parent.ID] = run
	defer delete(core.runs, parent.ID)
	core.state.Messages[parent.ID] = []contracts.Message{{
		ID: "parent-message", TurnID: run.id, Role: "assistant",
		Blocks: []contracts.Block{{ID: "agent-call", Kind: "tool", Tool: &contracts.ToolCall{
			ID: "agent-call", Name: "Agent", Status: "running",
		}}},
	}}
	path := filepath.Join(core.storage.Directory(), "engine.json")
	saved := path + ".saved"
	if err := os.Rename(path, saved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove(path)
		_ = os.Rename(saved, path)
	}()
	profile := contracts.SubagentProfile{ID: "worker", Name: "Worker", Description: "Task"}
	_, _, err := core.spawnSubagentLocked(context.Background(), parent.ID, run, profile, "Task", "Do work", false)
	if err == nil {
		t.Fatal("unwritable execution history was accepted")
	}
	if len(core.state.Runs) != 0 || len(core.state.Sessions) != 1 || len(core.runs) != 1 {
		t.Fatalf("failed startup left an orphan execution: runs=%+v sessions=%+v", core.state.Runs, core.state.Sessions)
	}
	if len(core.loadedSessions) != 1 || !core.loadedSessions[parent.ID] {
		t.Fatal("failed startup left an orphan loaded-session identity")
	}
	entries, err := os.ReadDir(filepath.Join(core.storage.Directory(), "sessions"))
	if err != nil || len(entries) != 1 || entries[0].Name() != parent.ID {
		t.Fatal("failed startup left an empty child reservation", err)
	}
}

func TestSubagentUsesFrozenProfileAndProviderConfiguration(t *testing.T) {
	requestStarted, releaseParent := make(chan struct{}), make(chan struct{})
	var parentCalls, childCalls atomic.Int32
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if parentCalls.Add(1) == 1 {
			close(requestStarted)
			select {
			case <-releaseParent:
				subagentCall(w, `{"description":"Frozen task","prompt":"task","subagent_type":"worker"}`)
			case <-r.Context().Done():
			}
			return
		}
		subagentText(w, model.Chat, "DONE")
	}))
	defer parent.Close()
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		childCalls.Add(1)
		body := subagentRequest(t, r)
		if body["model"] != "child-model" || body["reasoning_effort"] != "max" ||
			r.Header.Get("Authorization") != "Bearer child-secret" {
			t.Error("in-flight subagent configuration was replaced by a settings edit")
		}
		subagentText(w, model.Chat, "SNAPSHOT")
	}))
	defer child.Close()
	core, _ := subagentCore(t, parent.URL, child.URL, model.Chat)
	state, _ := core.Snapshot(context.Background())
	state.Settings.Subagents = []contracts.SubagentProfile{{
		ID: "worker", Name: "Worker", Description: "Task",
		Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Max"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	startSubagentParent(t, core)
	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("parent did not start")
	}
	state.Settings.Subagents = nil
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
			ID: "child", Name: "Edited", Protocol: model.Chat, BaseURL: "http://127.0.0.1:1",
			Models: []model.Model{{ID: "changed-model"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	close(releaseParent)
	waitSubagentState(t, core, func(s contracts.Snapshot) bool { return len(s.Runs) == 2 && runsFinished(s) })
	if childCalls.Load() != 1 {
		t.Fatal("accepted profile was lost after its settings were edited")
	}
}

func subagentCore(
	t *testing.T,
	parentURL string,
	childURL string,
	protocol model.Protocol,
) (*Engine, string) {
	t.Helper()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	for _, provider := range []model.Provider{
		{ID: "parent", Name: "Parent", Protocol: model.Chat, BaseURL: parentURL, Models: []model.Model{{ID: "parent-model"}}},
		{
			ID: "child", Name: "Child", Protocol: protocol, BaseURL: childURL, APIKey: "child-secret",
			Models: []model.Model{{ID: "child-model", ReasoningLevels: []string{"low", "max"}}},
		},
	} {
		if err = core.Execute(context.Background(), contracts.Command{
			Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
		}); err != nil {
			t.Fatal(err)
		}
	}
	workspace := t.TempDir()
	if err = core.Execute(context.Background(), contracts.Command{
		Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Project"},
	}); err != nil {
		t.Fatal(err)
	}
	return core, workspace
}

func startSubagentParent(t *testing.T, core *Engine) {
	t.Helper()
	state, _ := core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: state.Projects[0].ID, ModelID: "parent/parent-model", Text: "Start",
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func subagentRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Error(err)
	}
	return body
}

func delegationSchema(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	for _, raw := range body["tools"].([]any) {
		tool := raw.(map[string]any)["function"].(map[string]any)
		if tool["name"] == "Agent" {
			return tool["parameters"].(map[string]any)
		}
	}
	t.Error("parent has no delegation tool")
	return map[string]any{"properties": map[string]any{}}
}

func subagentCall(w http.ResponseWriter, arguments string) {
	subagentWireCall(w, model.Chat, "Agent", arguments)
}

func subagentWireCall(w http.ResponseWriter, protocol model.Protocol, name, arguments string) {
	w.Header().Set("Content-Type", "text/event-stream")
	switch protocol {
	case model.Chat:
		sendSSE(w, map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call-1", "function": map[string]any{"name": name, "arguments": arguments},
			}}}, "finish_reason": "tool_calls",
		}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	case model.Responses:
		sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{
			"status": "completed", "output": []any{map[string]any{
				"type": "function_call", "call_id": "call-1", "name": name, "arguments": arguments,
			}},
		}})
	case model.Messages:
		var input any
		_ = json.Unmarshal([]byte(arguments), &input)
		sendSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{
			"type": "tool_use", "id": "call-1", "name": name, "input": input,
		}})
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}

func subagentText(w http.ResponseWriter, protocol model.Protocol, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	switch protocol {
	case model.Chat:
		sendSSE(w, map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"content": text}, "finish_reason": "stop",
		}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	case model.Responses:
		sendSSE(w, map[string]any{"type": "response.output_text.delta", "delta": text})
		sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{
			"status": "completed", "output": []any{},
		}})
	case model.Messages:
		sendSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}})
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}

func runsFinished(state contracts.Snapshot) bool {
	if len(state.Runs) == 0 {
		return false
	}
	for _, run := range state.Runs {
		if run.Status == "running" {
			return false
		}
	}
	return true
}

func waitSubagentState(
	t *testing.T,
	core *Engine,
	ready func(contracts.Snapshot) bool,
) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if ready(state) {
			return state
		}
		time.Sleep(5 * time.Millisecond)
	}
	state, _ := core.Snapshot(context.Background())
	t.Fatalf("subagent execution did not settle: %+v", state.Runs)
	return state
}
