package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

type hookFixtureModel func(context.Context, model.Request, func(model.Delta) error) (model.Result, error)

func (f hookFixtureModel) Stream(ctx context.Context, request model.Request, emit func(model.Delta) error) (model.Result, error) {
	return f(ctx, request, emit)
}

type hookFixtureTool struct {
	name    string
	execute func(context.Context, string, json.RawMessage) (model.ToolOutput, error)
}

func (t hookFixtureTool) Definition() model.Tool { return model.Tool{Name: t.name} }
func (t hookFixtureTool) Concurrent() bool       { return false }
func (t hookFixtureTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	return t.execute(ctx, cwd, raw)
}

func TestHookRuntimeAgentUsesActualToolsAndInheritedModel(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "proof.txt"), []byte("real evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	modelFixture := hookFixtureModel(func(_ context.Context, request model.Request, _ func(model.Delta) error) (model.Result, error) {
		calls++
		names := []string{}
		for _, tool := range request.Tools {
			names = append(names, tool.Name)
		}
		if slices.Contains(names, "Agent") || slices.Contains(names, "AskUserQuestion") || !slices.Contains(names, "Read") || !slices.Contains(names, "Bash") {
			t.Errorf("invalid verifier tool boundary: %v", names)
		}
		if request.Effort != "high" {
			t.Errorf("lost inherited effort: %q", request.Effort)
		}
		if calls == 1 {
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "read", Name: "Read", Arguments: `{"file_path":"proof.txt"}`}}}}, nil
		}
		if last := request.History[len(request.History)-1]; last.Role != "tool" || !strings.Contains(last.Text, "real evidence") {
			t.Errorf("did not read real file: %+v", last)
		}
		return model.Result{Message: model.Message{Role: "assistant", Text: `{"ok":true}`}}, nil
	})
	tools := append(agent.CoreTools(), hookFixtureTool{name: "Agent"}, hookFixtureTool{name: "AskUserQuestion"})
	evaluate := hookRuntimeEvaluator(hookRuntimeOptions{Model: modelFixture, Tools: tools}, contracts.Session{ReasoningLevel: "high"})
	result, err := evaluate(context.Background(), contracts.HookRule{Type: "agent", Model: "haiku", Prompt: "Read the proof and verify."}, agent.HookInput{CWD: directory})
	if err != nil || result.Output != `{"ok":true}` || calls != 2 {
		t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
	}
}

func TestHookRuntimeAgentStopsAfterFiftyModelCalls(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "proof.txt"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	backend := hookFixtureModel(func(context.Context, model.Request, func(model.Delta) error) (model.Result, error) {
		calls++
		return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "read", Name: "Read", Arguments: `{"file_path":"proof.txt"}`}}}}, nil
	})
	_, err := hookRuntimeEvaluator(hookRuntimeOptions{Model: backend, Tools: agent.CoreTools()}, contracts.Session{})(context.Background(),
		contracts.HookRule{Type: "agent", Prompt: "Never finish"}, agent.HookInput{CWD: directory})
	if err == nil || !strings.Contains(err.Error(), "50") || calls != 50 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestHookRuntimeMCPUsesExactConnectedNamespace(t *testing.T) {
	var executed atomic.Int32
	tool := hookFixtureTool{name: "mcp__plugin_example_service__check", execute: func(_ context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
		executed.Add(1)
		if string(raw) != `{"path":"proof.txt"}` {
			t.Errorf("MCP input changed: %s", raw)
		}
		return model.ToolOutput{Text: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}`}, nil
	}}
	evaluate := hookRuntimeEvaluator(hookRuntimeOptions{Tools: []agent.Tool{tool}}, contracts.Session{})
	result, err := evaluate(context.Background(), contracts.HookRule{Type: "mcp_tool", MCPServer: "plugin_example_service", ToolName: "check", ToolInput: json.RawMessage(`{"path":"proof.txt"}`)}, agent.HookInput{})
	if err != nil || executed.Load() != 1 || !strings.Contains(result.Output, "deny") {
		t.Fatalf("%+v %v", result, err)
	}
	_, err = evaluate(context.Background(), contracts.HookRule{Type: "mcp_tool", MCPServer: "service", ToolName: "check"}, agent.HookInput{})
	if err == nil || executed.Load() != 1 {
		t.Fatal("missing server matched another plugin's tool")
	}
}

func TestHookRuntimePermissionUsesRealQuestionTool(t *testing.T) {
	for _, test := range []struct {
		name, answer string
		allowed      bool
	}{{"approved", "Allow", true}, {"denied", "Deny", false}, {"custom answer fails closed", "maybe", false}} {
		t.Run(test.name, func(t *testing.T) {
			asked := false
			question := agent.QuestionTool{Ask: func(_ context.Context, questions []contracts.QuestionPage) ([]contracts.QuestionAnswer, error) {
				asked = true
				q := questions[0]
				if !strings.Contains(q.Question, `"command":"pwd"`) {
					t.Errorf("permission omitted arguments: %+v", q)
				}
				return []contracts.QuestionAnswer{{Text: test.answer}}, nil
			}}
			allowed, err := hookRuntimePermission(hookRuntimeOptions{Tools: []agent.Tool{question}})(context.Background(), agent.HookInput{ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"pwd"}`)}, "guard")
			if err != nil || allowed != test.allowed || !asked {
				t.Fatalf("allowed=%v asked=%v err=%v", allowed, asked, err)
			}
		})
	}
	if allowed, err := hookRuntimePermission(hookRuntimeOptions{})(context.Background(), agent.HookInput{}, ""); err == nil || allowed {
		t.Fatal("unavailable question silently allowed operation")
	}
}

func TestHookRuntimePromptUsesActualConfiguredProvider(t *testing.T) {
	var requests atomic.Int32
	var verification atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		requests.Add(1)
		if body["model"] != "model" {
			t.Errorf("hook alias leaked to provider: %v", body["model"])
		}
		messages, _ := body["messages"].([]any)
		first, _ := messages[0].(map[string]any)
		system, _ := first["content"].(string)
		if strings.HasPrefix(system, "Evaluate the hook condition") {
			verification.Add(1)
			goalResponse(w, model.Chat, `{"ok":true}`, nil, true)
			return
		}
		goalResponse(w, model.Chat, "Completed.", nil, true)
	}))
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.Hooks = []contracts.HookRule{{ID: "prompt-check", Event: contracts.HookUserPromptSubmit, Type: "prompt", Prompt: "Check the submitted task. $ARGUMENTS", Model: "haiku", Enabled: true}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "Inspect the fixture")
	state = waitSubagentState(t, core, runsFinished)
	if state.Runs[0].Status != "completed" || verification.Load() != 1 || requests.Load() != 2 {
		t.Fatalf("requests=%d verification=%d runs=%+v", requests.Load(), verification.Load(), state.Runs)
	}
}

func TestHookRuntimeQueueIsInternalAndSurvivesObserverDisconnect(t *testing.T) {
	var requests atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		requests.Add(1)
		goalResponse(w, model.Chat, "Completed.", nil, true)
	}))
	sendGoal(t, core, "Initial task")
	state := waitSubagentState(t, core, runsFinished)
	sessionID := state.Sessions[0].ID
	observerCtx, cancel := context.WithCancel(context.Background())
	_, err := core.Subscribe(observerCtx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := core.queueHookResult(sessionID, agent.HookExecution{Rule: contracts.HookRule{ID: "async"}, Result: agent.HookResult{Context: "async facts"}}, false); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	pending := core.drainPendingLocked(sessionID, true)
	core.mu.Unlock()
	if len(pending) != 1 || pending[0].UserInput || len(pending[0].InputIDs) != 0 || !strings.Contains(pending[0].Text, "async facts") {
		t.Fatalf("hook result became human input: %+v", pending)
	}
	if err := core.queueHookResult(sessionID, agent.HookExecution{Rule: contracts.HookRule{ID: "rewake"}, ExitCode: 2, Stderr: "verify the background result"}, true); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	if requests.Load() != 2 || state.Sessions[0].LastOutcome != "completed" {
		t.Fatalf("observer cancellation stopped continuation: requests=%d state=%+v", requests.Load(), state.Sessions[0])
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	found := false
	for _, message := range core.history[sessionID] {
		if strings.Contains(message.Text, "verify the background result") {
			found = true
			if message.UserInput {
				t.Error("rewake message was marked human")
			}
		}
	}
	if !found {
		t.Fatal("rewake result did not reach actual model history")
	}
}

func TestHookRuntimeRedactsSensitiveOutputAndExportsBoundTranscript(t *testing.T) {
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "Completed.", nil, true)
	}))
	sendGoal(t, core, "Initial task")
	state := waitSubagentState(t, core, runsFinished)
	session := state.Sessions[0]
	secret := "private-hook-credential"
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	rule := contracts.HookRule{ID: "sensitive", Event: contracts.HookSessionStart, Type: "command", Command: program, Enabled: true,
		UserConfig: map[string]string{"api_token": secret}, Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper", filepath.Join(workspace, "hook.json"),
			`{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"` + secret + `"}}`}}
	run := &activeRun{id: "fixture-hook-run", pending: map[string]contracts.QueuedMessage{}}
	runner, invoke := core.runHookRunner(session, run, []contracts.HookRule{rule})
	result, err := invoke(context.Background(), agent.HookInput{Event: contracts.HookSessionStart, Source: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Wait(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Context, secret) || !strings.Contains(result.Context, "[REDACTED]") {
		t.Fatalf("model-facing secret leak: %+v", result)
	}
	state, _ = core.Snapshot(context.Background())
	public, _ := json.Marshal(state)
	if strings.Contains(string(public), secret) {
		t.Fatal("secret leaked into public diagnostics")
	}
	data, err := os.ReadFile(filepath.Join(workspace, "hook.json"))
	if err != nil {
		t.Fatal(err)
	}
	var input agent.HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	root, err := core.storage.Resolve(session.StorageRootID)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "sessions", session.ID, "hook-transcript.jsonl")
	if input.TranscriptPath != wantPath {
		t.Fatalf("transcript escaped session root: %q want %q", input.TranscriptPath, wantPath)
	}
	transcript, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(transcript)), "\n")
	if len(lines) < 2 {
		t.Fatalf("actual history missing: %s", transcript)
	}
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["message"] == nil {
			t.Fatalf("missing Claude message shape: %s", line)
		}
	}
}

func TestHookRuntimeSubagentStartPrecedesActualChildModel(t *testing.T) {
	var parentCalls, childCalls atomic.Int32
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), "Subagent task instructions:") {
			childCalls.Add(1)
			if strings.Count(string(raw), "startup facts") != 1 {
				t.Errorf("child context missing or duplicated: %s", raw)
			}
			goalResponse(w, model.Chat, "Child completed.", nil, true)
			return
		}
		if parentCalls.Add(1) == 1 {
			goalResponse(w, model.Chat, "", &model.Call{ID: "delegate", Name: "Agent", Arguments: `{"description":"Inspect fixture","prompt":"Inspect the code","subagent_type":"Explore"}`}, true)
			return
		}
		goalResponse(w, model.Chat, "Parent completed.", nil, true)
	}))
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	for _, event := range []contracts.HookEvent{contracts.HookSubagentStart, contracts.HookSubagentStop} {
		output := `{}`
		if event == contracts.HookSubagentStart {
			output = `{"decision":"block","reason":"Start cannot block creation","hookSpecificOutput":{"hookEventName":"SubagentStart","additionalContext":"startup facts"}}`
		}
		settings.Hooks = append(settings.Hooks, contracts.HookRule{ID: string(event), Event: event, Matcher: "Explore", Type: "command", Command: program, Enabled: true,
			Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper", filepath.Join(workspace, string(event)+".json"), output}})
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "Delegate the inspection")
	state = waitSubagentState(t, core, runsFinished)
	if parentCalls.Load() != 2 || childCalls.Load() != 1 {
		t.Fatalf("parent=%d child=%d runs=%+v", parentCalls.Load(), childCalls.Load(), state.Runs)
	}
	var parent, child contracts.Session
	for _, session := range state.Sessions {
		if session.SubagentType == "Explore" {
			child = session
		} else {
			parent = session
		}
	}
	for _, event := range []contracts.HookEvent{contracts.HookSubagentStart, contracts.HookSubagentStop} {
		data, err := os.ReadFile(filepath.Join(workspace, string(event)+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var input agent.HookInput
		if err := json.Unmarshal(data, &input); err != nil {
			t.Fatal(err)
		}
		if input.SessionID != parent.ID || input.AgentID != child.ID || input.SubagentType != "Explore" {
			t.Fatalf("%s lost public parent/child identity: %+v", event, input)
		}
		if event == contracts.HookSubagentStop && input.AgentTranscriptPath == input.TranscriptPath {
			t.Fatalf("child transcript reused parent transcript: %+v", input)
		}
	}
}

func TestHookRuntimeManualCompactionTriggersActualLifecycle(t *testing.T) {
	var summaries atomic.Int32
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), "Summarize this coding conversation") {
			summaries.Add(1)
			goalResponse(w, model.Chat, "Actual provider summary.", nil, true)
			return
		}
		goalResponse(w, model.Chat, "Completed.", nil, true)
	}))
	sendGoal(t, core, "Initial task")
	state := waitSubagentState(t, core, runsFinished)
	sessionID := state.Sessions[0].ID
	core.mu.Lock()
	for i := 0; i < 4; i++ {
		message := model.Message{Role: "user", Text: "fixture history"}
		core.history[sessionID] = append(core.history[sessionID], message)
		core.context[sessionID] = append(core.context[sessionID], message)
	}
	core.mu.Unlock()
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	for _, event := range []contracts.HookEvent{contracts.HookPreCompact, contracts.HookPostCompact, contracts.HookSessionStart} {
		settings.Hooks = append(settings.Hooks, contracts.HookRule{ID: string(event), Event: event, Type: "command", Command: program, Enabled: true,
			Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper", filepath.Join(workspace, string(event)+".json"), `{}`}})
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: "/compact"}}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished)
	if summaries.Load() != 1 {
		t.Fatalf("compaction did not call actual configured provider: %d runs=%+v", summaries.Load(), state.Runs)
	}
	for _, event := range []contracts.HookEvent{contracts.HookPreCompact, contracts.HookPostCompact, contracts.HookSessionStart} {
		data, err := os.ReadFile(filepath.Join(workspace, string(event)+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var input agent.HookInput
		if err := json.Unmarshal(data, &input); err != nil {
			t.Fatal(err)
		}
		if event == contracts.HookSessionStart && input.Source != "compact" {
			t.Fatalf("%+v", input)
		}
		if event != contracts.HookSessionStart && input.Trigger != "manual" {
			t.Fatalf("%+v", input)
		}
		if event == contracts.HookPostCompact && !strings.Contains(input.CompactSummary, "Actual provider summary.") {
			t.Fatalf("%+v", input)
		}
	}
}

func TestHookRuntimeEventsExposeActualProcessResultAndIdentity(t *testing.T) {
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "Completed.", nil, true)
	}))
	sendGoal(t, core, "Initial task")
	state := waitSubagentState(t, core, runsFinished)
	session := state.Sessions[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := core.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	rule := contracts.HookRule{ID: "plugin@market/Stop/0/0", PluginID: "plugin@market", Name: "nonunique-name", Event: contracts.HookStop, Type: "command", Command: program, Enabled: true,
		Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper", filepath.Join(workspace, "event-input.json"), `{}`}}
	runner, invoke := core.runHookRunner(session, &activeRun{id: "hook-event", pending: map[string]contracts.QueuedMessage{}}, []contracts.HookRule{rule})
	if _, err := invoke(context.Background(), agent.HookInput{Event: contracts.HookStop}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Wait(); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case event := <-events:
			if event.Kind != "hook.finished" {
				continue
			}
			if event.ResourceID != rule.ID || event.ComponentID != rule.ID || event.PluginID != rule.PluginID || event.ExitCode == nil || *event.ExitCode != 0 || event.Error != "" || event.Text != `{}` {
				t.Fatalf("lost actual process evidence: %+v", event)
			}
			return
		default:
			t.Fatal("hook.finished event missing")
		}
	}
}
