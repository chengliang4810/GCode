package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// workflowCommandCore 建一个带假供应商的核心，用来观察真实回合里的提示词
// 与工具目录。供应商只是 fixture，不代表真实模型。
func workflowCommandCore(t *testing.T, handler http.Handler) *Engine {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	if err = core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
			ID: "workflow-fixture", Name: "Workflow fixture", Protocol: model.Chat, BaseURL: server.URL,
			Models: []model.Model{{ID: "model"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return core
}

// modelRequest 记录假供应商收到的那一次请求。
type modelRequest struct {
	mu   sync.Mutex
	body map[string]any
}

func (r *modelRequest) handler(text string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body := map[string]any{}
		_ = json.NewDecoder(req.Body).Decode(&body)
		r.mu.Lock()
		r.body = body
		r.mu.Unlock()
		goalResponse(w, model.Chat, text, nil, true)
	})
}

func (r *modelRequest) last() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body
}

// promptText 取出请求里的系统提示与用户正文，用来判断模型看到了什么。
func promptText(body map[string]any) string {
	var out strings.Builder
	messages, _ := body["messages"].([]any)
	for _, item := range messages {
		message, _ := item.(map[string]any)
		text, _ := message["content"].(string)
		if text != "" {
			out.WriteString(text)
			out.WriteString("\n")
		}
	}
	return out.String()
}

func requestToolNames(body map[string]any) []string {
	var names []string
	tools, _ := body["tools"].([]any)
	for _, item := range tools {
		tool, _ := item.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func TestWorkflowCommandExpandsThePromptAndActivatesTheSession(t *testing.T) {
	record := &modelRequest{}
	core := workflowCommandCore(t, record.handler("Planned the workflow"))
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	sessionID := core.state.Sessions[0].ID
	activatedBefore := core.state.Sessions[0].WorkflowActivated
	core.mu.Unlock()
	if activatedBefore {
		t.Fatal("a fresh session starts without workflow tools")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: sessionID, ModelID: "workflow-fixture/model", Text: "/workflow review the login change",
	}}); err != nil {
		t.Fatal(err)
	}
	state := waitWorkflowState(t, core, func(state contracts.Snapshot) bool {
		for _, run := range state.Runs {
			if run.SessionID == sessionID && run.Status == contracts.RunStatusCompleted {
				return true
			}
		}
		return false
	})
	if !state.Sessions[0].WorkflowActivated {
		t.Fatal("the command activates the workflow capability for the session")
	}
	body := record.last()
	if body == nil {
		t.Fatal("the model was never called")
	}
	prompt := promptText(body)
	if !strings.Contains(prompt, "review the login change") {
		t.Fatalf("the request reaches the model: %q", prompt)
	}
	if !strings.Contains(prompt, "CreateWorkflow") {
		t.Fatalf("the prompt points at the workflow tool: %q", prompt)
	}
	if strings.Contains(prompt, "/workflow") {
		t.Fatalf("the command itself is replaced by the prompt: %q", prompt)
	}
	names := requestToolNames(body)
	for _, name := range workflowToolNames {
		if !containsString(names, name) {
			t.Fatalf("the activated turn offers %s: %v", name, names)
		}
	}
	// 用户气泡保留命令，运行卡挂在发起它的那一轮。
	view, err := core.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	messages := view.Messages[sessionID]
	if len(messages) == 0 || !strings.Contains(messages[0].Blocks[0].Text, "/workflow") {
		t.Fatalf("the conversation keeps the command: %+v", messages)
	}
}

func TestWorkflowCommandIsRejectedWhenWorkflowsAreOff(t *testing.T) {
	record := &modelRequest{}
	core := workflowCommandCore(t, record.handler("never called"))
	ctx := context.Background()
	snapshot, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshot.Settings
	settings.WorkflowMode = contracts.WorkflowModeDisabled
	if err = core.Execute(ctx, contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	sessionID := core.state.Sessions[0].ID
	core.mu.Unlock()
	err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: sessionID, ModelID: "workflow-fixture/model", Text: "/workflow review the login change",
	}})
	if err == nil {
		t.Fatal("a disabled mode refuses the command instead of running it")
	}
	if !strings.Contains(err.Error(), "turned off") {
		t.Fatalf("the refusal says why: %v", err)
	}
	if record.last() != nil {
		t.Fatal("nothing reached the model")
	}
}

// 中枢直接启动和恢复运行也是用户动作：按需模式这时就打开会话能力。
func TestWorkflowRunAndResumeActivateTheSession(t *testing.T) {
	record := &modelRequest{}
	core := workflowCommandCore(t, record.handler("Planned the workflow"))
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	sessionID := core.state.Sessions[0].ID
	core.mu.Unlock()
	if err := core.Execute(ctx, contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			SessionID: sessionID, Name: "Fixture", Program: workflowTestProgram(), ModelID: "workflow-fixture/model",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	activated := core.state.Sessions[0].WorkflowActivated
	core.mu.Unlock()
	if !activated {
		t.Fatal("starting a workflow activates the session capability")
	}
}

func TestWorkflowCommandText(t *testing.T) {
	for _, test := range []struct {
		name, text, body string
		requested        bool
	}{
		{"reference at the start", "[Workflow](gcode://command/workflow) review the change", "review the change", true},
		{"reference in the middle", "Please [Workflow](gcode://command/workflow) review the change", "Please  review the change", true},
		{"bare reference", "[Workflow](gcode://command/workflow)", "", true},
		{"typed command", "/workflow review the change", "review the change", true},
		{"typed command alone", "  /workflow  ", "", true},
		{"different command", "/goal review the change", "", false},
		{"absolute path stays prose", "/Users/me/workflow notes", "", false},
		{"ordinary prose", "explain the workflow feature", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, requested := workflowCommandText(test.text)
			if requested != test.requested || body != test.body {
				t.Fatalf("workflowCommandText(%q) = %q, %v", test.text, body, requested)
			}
		})
	}
}

func TestWorkflowCommandPromptKeepsTheRequestAndTheTopologyRule(t *testing.T) {
	prompt := workflowCommandPrompt("review the change")
	if !strings.Contains(prompt, "review the change") || !strings.Contains(prompt, "CreateWorkflow") {
		t.Fatalf("the prompt keeps the request and the tool: %q", prompt)
	}
	if !strings.Contains(prompt, "Decide the topology") {
		t.Fatalf("the prompt asks for the topology first: %q", prompt)
	}
	if strings.TrimSpace(workflowCommandPrompt("")) == "" {
		t.Fatal("an empty request still produces the guidance")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
