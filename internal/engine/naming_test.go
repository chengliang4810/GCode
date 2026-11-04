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
						if body[limit] != float64(96) {
							t.Errorf("unbounded naming output: %v", body[limit])
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
				after := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
					return len(state.Sessions) == 1 && state.Sessions[0].Title == "真实模型标题"
				})
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
	state := waitIdle(t, core)
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
