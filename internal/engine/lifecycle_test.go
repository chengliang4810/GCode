package engine

import (
	"context"
	"encoding/json"
	"fmt"
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
	"gcode/internal/storage"
)

func TestGeneratedConversationIDsAreLowercase(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	parent := snapshot.Sessions[0].ID
	for _, kind := range []string{"session.sidechat", "session.fork"} {
		if err = core.Execute(ctx, contracts.Command{Kind: kind, SessionID: parent}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.Sessions) != 3 {
		t.Fatal("expected parent, side chat and fork")
	}
	seen := map[string]bool{}
	for _, session := range snapshot.Sessions {
		if len(session.ID) != 12 || strings.Trim(session.ID, "abcdefghijklmnopqrstuvwxyz0123456789") != "" || seen[session.ID] {
			t.Fatalf("invalid generated conversation ID: %s", session.ID)
		}
		seen[session.ID] = true
	}
}

func TestInterruptedReplyRetriesWithoutRepeatingCompletedTools(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				attempt := requests.Add(1)
				callID, path, content := "Call_KeepCase", "completed.txt", "completed-once"
				if attempt == 2 {
					callID, path, content = "Call_Incomplete", "must-not-exist.txt", "unsafe"
				}
				arguments := fmt.Sprintf(`{"file_path":%q,"content":%q}`, path, content)
				if attempt > 1 && (!strings.Contains(string(payload), "Call_KeepCase") || !strings.Contains(string(payload), "Wrote")) {
					t.Error("completed tool result was lost during recovery")
				}
				if attempt > 2 && strings.Contains(string(payload), "Call_Incomplete") {
					t.Error("incomplete response was committed into model history")
				}
				switch protocol {
				case model.Chat:
					if attempt < 3 {
						delta := map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": callID, "function": map[string]any{"name": "Write", "arguments": arguments}}}}
						choice := map[string]any{"delta": delta}
						if attempt == 1 {
							choice["finish_reason"] = "tool_calls"
						} else {
							delta["content"] = "discarded partial"
						}
						sendSSE(w, map[string]any{"choices": []any{choice}})
					} else {
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"RECOVERED_OK\"},\"finish_reason\":\"stop\"}]}\n\n")
					}
					if attempt != 2 {
						fmt.Fprint(w, "data: [DONE]\n\n")
					}
				case model.Responses:
					switch attempt {
					case 1:
						sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": callID, "name": "Write", "arguments": arguments}}}})
					case 2:
						fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"discarded partial\"}\n\n")
					default:
						fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"RECOVERED_OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
					}
				case model.Messages:
					switch attempt {
					case 1:
						sendSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": callID, "name": "Write", "input": map[string]any{"file_path": path, "content": content}}})
						fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
					case 2:
						fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"discarded partial\"}}\n\n")
					default:
						fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"RECOVERED_OK\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
					}
				}
			}))
			defer provider.Close()
			store, _ := storage.Open(storage.Options{Home: t.TempDir()})
			core, err := openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx := context.Background()
			config := model.Provider{ID: "p", Name: "P", Protocol: protocol, BaseURL: provider.URL, Models: []model.Model{{ID: "m"}}}
			if err = core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
				t.Fatal(err)
			}
			if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "Write once then confirm", ModelID: "p/m"}}); err != nil {
				t.Fatal(err)
			}
			summary, err := core.Snapshot(ctx)
			if err != nil || len(summary.Sessions) != 1 {
				t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
			}
			snapshot := waitIdle(t, core, summary.Sessions[0].ID)
			run := snapshot.Runs[0]
			snapshot, err = core.Snapshot(ctx, run.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != "completed" || requests.Load() != 3 {
				t.Fatalf("interrupted response was not recovered: %+v, requests=%d", run, requests.Load())
			}
			var directory string
			for _, session := range snapshot.Sessions {
				if session.ID == run.SessionID {
					directory = session.CWD
				}
			}
			proof, err := os.ReadFile(filepath.Join(directory, "completed.txt"))
			if err != nil || string(proof) != "completed-once" {
				t.Fatalf("completed effect lost: %s %v", proof, err)
			}
			if _, err = os.Stat(filepath.Join(directory, "must-not-exist.txt")); !os.IsNotExist(err) {
				t.Fatal("incomplete tool call was executed")
			}
			calls := 0
			for _, message := range snapshot.Messages[run.SessionID] {
				for _, block := range message.Blocks {
					if block.Tool != nil && block.Tool.Name == "Write" {
						calls++
					}
				}
			}
			if calls != 1 {
				t.Fatal("recovery repeated a completed tool call")
			}
			text := ""
			for _, message := range snapshot.Messages[run.SessionID] {
				for _, block := range message.Blocks {
					text += block.Text
				}
			}
			if strings.Contains(text, "discarded partial") || !strings.Contains(text, "RECOVERED_OK") {
				t.Fatalf("incomplete response leaked into the recovered display: %s", text)
			}
		})
	}
}

func TestFailedSendDoesNotCreateConversation(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	err = core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "no provider"}})
	state, _ := core.Snapshot(context.Background())
	if err == nil || len(state.Sessions) != 0 || len(state.Messages) != 0 {
		t.Fatal("failed send changed authoritative state")
	}
}

func TestRemoveQueuedMessagePreventsExecutionAndStopCancels(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if requests.Add(1) == 1 {
			close(started)
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer provider.Close()
	defer close(release)
	store, _ := storage.Open(storage.Options{Home: t.TempDir()})
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	config := model.Provider{ID: "p", Name: "P", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "m"}}}
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "first", ModelID: "p/m"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("model did not start")
	}
	state, _ := core.Snapshot(ctx)
	sid := state.Sessions[0].ID
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "must not execute", SessionID: sid}}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx, sid)
	if len(state.Queue) != 1 {
		t.Fatal("missing queued message")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "queue.remove", ID: state.Queue[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.stop", SessionID: sid}); err != nil {
		t.Fatal(err)
	}
	state = waitIdle(t, core, sid)
	if requests.Load() != 1 || state.Runs[0].Status != "interrupted" || len(state.Queue) != 0 {
		t.Fatalf("stop/queue: %+v", state.Runs)
	}
}

func TestRealProjectFileReadConflictAndSearch(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	path := filepath.Join(workspace, "example.go")
	if err := os.WriteFile(path, []byte("package example\n// 查找内容\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := storage.Open(storage.Options{Home: home})
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
		t.Fatal(err)
	}
	state := waitForProjectFiles(t, core)
	file := state.Files[0]
	if file.Loaded || file.Content != "" {
		t.Fatal("file content was eagerly loaded into every snapshot")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "file.open", ID: file.ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx)
	file = state.Files[0]
	if !file.Loaded || !strings.Contains(file.Content, "查找内容") {
		t.Fatal("lazy file load failed")
	}
	result, err := core.SearchWorkspace(ctx, contracts.WorkspaceSearchRequest{ProjectID: file.ProjectID, Query: "查找内容"})
	if err != nil || len(result.Content) != 1 || result.Content[0].Line != 2 {
		t.Fatalf("%+v %v", result, err)
	}
	if err := os.WriteFile(path, []byte("external change"), 0600); err != nil {
		t.Fatal(err)
	}
	err = core.Execute(ctx, contracts.Command{Kind: "file.save", ID: file.ID, Version: file.Version, Text: "must not overwrite"})
	data, _ := os.ReadFile(path)
	if err == nil || string(data) != "external change" {
		t.Fatal("external edit was overwritten")
	}
}

func TestCompactionDoesNotMutateInputHistory(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"summary\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer fixture.Close()
	history := []model.Message{}
	for i := 0; i < 16; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history = append(history, model.Message{Role: role, Text: fmt.Sprintf("original-%d", i)})
	}
	provider := model.Provider{
		ID: "p", Name: "P", Protocol: model.Chat, BaseURL: fixture.URL,
		Models: []model.Model{{ID: "m"}},
	}
	gateway, err := model.NewClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	compacted, err := agent.Compact(context.Background(), gateway, model.Request{History: history})
	if err != nil || len(compacted) >= len(history) || history[0].Text != "original-0" {
		t.Fatalf("compaction: %+v %v", compacted, err)
	}
}

func TestQuestionAnswerIsRecordedOnceBeforeContinuing(t *testing.T) {
	var calls atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			arguments := `{"questions":[{"question":"Choose","header":"Choice","options":[{"label":"A","description":"First choice"},{"label":"B","description":"Second choice"}],"multiSelect":false},{"question":"Features","header":"Features","options":[{"label":"X","description":"Feature X"},{"label":"Y","description":"Feature Y"}],"multiSelect":true}]}`
			sendSSE(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "ask", "function": map[string]any{"name": "AskUserQuestion", "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
			fmt.Fprint(w, "data: [DONE]\n\n")
		} else {
			var request struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Error(err)
			}
			recorded := 0
			for _, message := range request.Messages {
				if message.Role != "tool" {
					continue
				}
				var result struct {
					Answers map[string]string `json:"answers"`
				}
				if json.Unmarshal([]byte(message.Content), &result) == nil && result.Answers["Choose"] == "A" && result.Answers["Features"] == `["X","Y"]` {
					recorded++
				}
			}
			if recorded != 1 {
				t.Error("answer missing from model history")
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Answered\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer fixture.Close()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	config := model.Provider{ID: "p", Name: "P", Protocol: model.Chat, BaseURL: fixture.URL, Models: []model.Model{{ID: "m"}}}
	if err = core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "Ask", ModelID: "p/m"}}); err != nil {
		t.Fatal(err)
	}
	var questionID, sessionID string
	deadline := time.Now().Add(5 * time.Second)
	for questionID == "" && time.Now().Before(deadline) {
		summary, _ := core.Snapshot(ctx)
		if len(summary.Sessions) > 0 {
			sessionID = summary.Sessions[0].ID
		}
		if sessionID == "" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		state, _ := core.Snapshot(ctx, sessionID)
		for _, messages := range state.Messages {
			for _, message := range messages {
				for _, block := range message.Blocks {
					if block.Question != nil {
						questionID = block.Question.ID
						if len(block.Question.Pages) != 2 {
							t.Fatal("questions were published as separate waits instead of a single paged request")
						}
					}
				}
			}
		}
		if questionID == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if questionID == "" {
		t.Fatal("no native question was published")
	}
	partial := contracts.Command{Kind: "request.answer", ID: questionID, QuestionAnswers: []contracts.QuestionAnswer{{Options: []string{"A"}}}}
	if err = core.Execute(ctx, partial); err == nil {
		t.Fatal("a partial answer resolved the whole question request")
	}
	if calls.Load() != 1 {
		t.Fatal("invalid partial answer resumed model execution")
	}
	answer := contracts.Command{Kind: "request.answer", ID: questionID, QuestionAnswers: []contracts.QuestionAnswer{{Options: []string{"A"}}, {Options: []string{"X", "Y"}}}}
	if err = core.Execute(ctx, answer); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, answer); err == nil {
		t.Fatal("question accepted two answers")
	}
	state := waitIdle(t, core, sessionID)
	if state.Runs[0].Status != "completed" || calls.Load() != 2 {
		t.Fatalf("%+v", state.Runs)
	}
}

func TestSubscriberSnapshotsHaveIndependentOwnership(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, _ := core.Subscribe(ctx)
	second, _ := core.Subscribe(ctx)
	<-first
	<-second
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	one, two := <-first, <-second
	one.Snapshot.Sessions[0].Title = "subscriber mutation"
	snapshot, _ := core.Snapshot(ctx)
	if two.Snapshot.Sessions[0].Title == "subscriber mutation" || snapshot.Sessions[0].Title == "subscriber mutation" {
		t.Fatal("subscriber mutated another owner's state")
	}
}

func TestDataDirectoryChangeKeepsExistingWorkspacesAtTheirRoots(t *testing.T) {
	home, external := t.TempDir(), t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	original := core.state.Sessions[0]
	if err = core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: external}}); err != nil {
		t.Fatal(err)
	}
	source := store.Directory()
	target := filepath.Join(home, "chosen")
	if err = os.WriteFile(filepath.Join(original.CWD, "proof.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "storage.change", Path: target}); err != nil {
		t.Fatal(err)
	}
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	state, _ := core.Snapshot(ctx)
	old := state.Sessions[core.sessionIndex(original.ID)]
	if old.CWD != original.CWD || old.StorageRootID != original.StorageRootID || state.Projects[0].Path != external {
		t.Fatal("directory change moved an existing workspace, storage identity, or user project")
	}
	if store.Directory() != target || store.ConfigDirectory() != source {
		t.Fatal("new runtime root did not retain the fixed configuration entrance")
	}
	data, err := os.ReadFile(filepath.Join(old.CWD, "proof.txt"))
	if err != nil || string(data) != "keep" {
		t.Fatal("original managed workspace was changed", err)
	}
	for _, name := range []string{"sessions", "workspaces"} {
		if _, err := os.Stat(filepath.Join(target, name, old.ID)); !os.IsNotExist(err) {
			t.Fatal("old data was copied to the new root", err)
		}
	}
	storeTestMessage(t, core, old.ID, "continued-at-original-root")
	entries, err := os.ReadDir(filepath.Join(source, "sessions", old.ID))
	if err != nil {
		t.Fatal(err)
	}
	savedAtOrigin := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(source, "sessions", old.ID, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		savedAtOrigin = savedAtOrigin || strings.Contains(string(content), "continued-at-original-root")
	}
	if !savedAtOrigin {
		t.Fatal("continuing an old conversation did not save at its original root")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx)
	fresh := state.Sessions[0]
	if fresh.StorageRootID == old.StorageRootID || fresh.CWD != filepath.Join(target, "workspaces", fresh.ID) {
		t.Fatalf("new conversation did not bind the new runtime root: %+v", fresh)
	}
}
