package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestNamingUsesSelectedModeAcrossProtocols(t *testing.T) {
	for _, naming := range []struct{ mode, modelID string }{
		{"current-model", "unselected"},
		{"checks/selected/model", "selected/model"},
	} {
		for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
			t.Run(naming.mode+"/"+string(protocol), func(t *testing.T) {
				var namingCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body := subagentRequest(t, r)
					payload, _ := json.Marshal(body)
					if strings.Contains(string(payload), "Create a concise conversation title") {
						namingCalls.Add(1)
						if body["model"] != naming.modelID {
							t.Errorf("naming used model %v, want %s", body["model"], naming.modelID)
						}
						if tools, ok := body["tools"].([]any); ok && len(tools) != 0 {
							t.Error("naming request exposed tools")
						}
						limit := "max_completion_tokens"
						if protocol == model.Responses {
							limit = "max_output_tokens"
						} else if protocol == model.Messages {
							limit = "max_tokens"
						}
						wantLimit := any(float64(8192))
						if naming.mode == "current-model" && protocol != model.Messages {
							wantLimit = nil
						}
						if body[limit] != wantLimit {
							t.Errorf("naming changed the configured model output limit: got %v, want %v", body[limit], wantLimit)
						}
						goalResponse(w, protocol, "真实模型标题", nil, true)
					} else {
						goalResponse(w, protocol, "Done", nil, true)
					}
				}))
				t.Cleanup(server.Close)
				core := modelCheckCore(t, protocol, server.URL)
				state, _ := core.Snapshot(context.Background())
				settings := state.Settings
				settings.General.ThreadNamingModel = naming.mode
				if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
					t.Fatal(err)
				}
				project := createNamingProject(t, core)
				if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					ProjectID: project.ID, ModelID: "checks/unselected", Text: "请帮我分析项目结构",
				}}); err != nil {
					t.Fatal(err)
				}
				summary, err := core.Snapshot(context.Background())
				if err != nil || len(summary.Sessions) != 1 {
					t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
				}
				after := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
					return len(state.Sessions) == 1 && state.Sessions[0].Title == "真实模型标题"
				}, summary.Sessions[0].ID)
				if namingCalls.Load() != 1 || after.Sessions[0].ModelID != "checks/unselected" {
					t.Fatalf("naming changed execution model or repeated: %+v", after.Sessions[0])
				}
				core.mu.Lock()
				for _, message := range core.history[after.Sessions[0].ID] {
					if message.Text == "真实模型标题" {
						t.Error("naming contaminated Agent history")
					}
				}
				core.mu.Unlock()
			})
		}
	}
}

func TestNamingUsesUserInputBeforeMainReplyWithReasoning(t *testing.T) {
	mainStarted := make(chan struct{})
	releaseMain := make(chan struct{})
	defer close(releaseMain)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if body["model"] == "selected/model" {
			if body["max_completion_tokens"] != float64(8192) {
				t.Errorf("naming request imposed its own output cap: %v", body["max_completion_tokens"])
			}
			messages, _ := body["messages"].([]any)
			if len(messages) != 2 {
				t.Errorf("naming included conversation history: %+v", messages)
			} else if input, _ := messages[1].(map[string]any); input["role"] != "user" || input["content"] != "这两个文件的内容都有啥" {
				t.Errorf("naming did not use the user's original input: %+v", input)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			sendSSE(w, map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"reasoning_content": strings.Repeat("Consider the user's topic. ", 128)},
			}}})
			sendSSE(w, map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"content": "了解两个文件的内容"}, "finish_reason": "stop",
			}}, "usage": map[string]any{"prompt_tokens": 76, "completion_tokens": 512}})
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		close(mainStarted)
		select {
		case <-releaseMain:
			goalResponse(w, model.Chat, "This main reply arrives after the title.", nil, true)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	project := createNamingProject(t, core)
	state, _ := core.Snapshot(t.Context())
	settings := state.Settings
	settings.General.ThreadNamingModel = "checks/selected/model"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: project.ID, ModelID: "checks/unselected", Text: "这两个文件的内容都有啥",
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-mainStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("main request did not start")
	}
	summary, _ := core.Snapshot(t.Context())
	sessionID := summary.Sessions[0].ID
	named := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
		return state.Sessions[0].Title == "了解两个文件的内容"
	}, sessionID)
	if named.Sessions[0].LastOutcome != "" {
		t.Fatal("naming waited for the main conversation to finish")
	}
	trajectory, err := core.ReadModelTrajectory(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range trajectory.Records {
		if record.Source == "naming" && record.Response != nil {
			if record.Error != "" || record.Response.Usage.Output != 512 {
				t.Fatalf("reasoning naming request failed: %+v", record)
			}
			return
		}
	}
	t.Fatal("completed naming request was not recorded")
}

func TestNamingManualRenameCancelsEvenWhenTitleUnchanged(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if body["model"] == "selected/model" {
			close(started)
			<-r.Context().Done()
			close(canceled)
			return
		}
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	project := createNamingProject(t, core)
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.General.ThreadNamingModel = "checks/selected/model"
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: project.ID, ModelID: "checks/unselected", Text: "Initial title",
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("naming request did not start")
	}
	state, _ = core.Snapshot(context.Background())
	title := state.Sessions[0].Title
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: state.Sessions[0].ID, Title: &title}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("manual rename did not cancel naming HTTP request")
	}
	after, _ := core.Snapshot(context.Background())
	if after.Sessions[0].Title != title {
		t.Fatal("manual title was replaced")
	}
}

func TestNamingFirstMessageDoesNotCallNamingModel(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	project := createNamingProject(t, core)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: project.ID, ModelID: "checks/unselected", Text: "首条消息作为标题",
	}}); err != nil {
		t.Fatal(err)
	}
	summary, err := core.Snapshot(context.Background())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
	}
	state := waitIdle(t, core, summary.Sessions[0].ID)
	if state.Sessions[0].Title != "首条消息作为标题" || requests.Load() != 1 {
		t.Fatalf("default naming made extra requests: %d %+v", requests.Load(), state.Sessions[0])
	}
}

func TestNamingShutdownCancelsAndJoins(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if body["model"] == "selected/model" {
			close(started)
			<-r.Context().Done()
			close(canceled)
			return
		}
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	project := createNamingProject(t, core)
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.General.ThreadNamingModel = "checks/selected/model"
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: project.ID, ModelID: "checks/unselected", Text: "Title",
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("naming did not start")
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not cancel the naming request")
	}
}

func createNamingProject(t *testing.T, core *Engine) contracts.Project {
	t.Helper()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	return state.Projects[len(state.Projects)-1]
}
