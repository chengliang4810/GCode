//go:build e2e

package e2e

import (
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
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestDesktopLocalTranscriptEditLineCounts(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			const filename = "conversation_metrics.go"
			if err := os.WriteFile(filepath.Join(workspace, filename), []byte("original\n"), 0600); err != nil {
				t.Fatal(err)
			}
			content := strings.Repeat("edited line\n", 12)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls := []model.Call{
					{ID: "edit-lines", Name: "Edit", Arguments: fmt.Sprintf(`{"file_path":%q,"old_string":"original\n","new_string":%q}`, filename, content)},
					{ID: "write-lines", Name: "Write", Arguments: `{"file_path":"new_file.go","content":"first\nsecond\n"}`},
				}
				index := int(requests.Add(1)) - 1
				delta, finish := map[string]any{"content": "Line count fixture complete."}, "stop"
				if index < len(calls) {
					call := calls[index]
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": call.ID,
						"function": map[string]any{"name": call.Name, "arguments": call.Arguments}}}}
					finish = "tool_calls"
				}
				wire, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", wire)
			}))
			defer server.Close()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = core.Close() })
			settings := contracts.DefaultSettings()
			settings.Appearance.ReduceMotion = true
			settings.General.ThreadNamingModel = "first-message"
			provider := model.Provider{ID: "fixture", Name: "Fixture", Protocol: model.Chat,
				BaseURL: server.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := core.Snapshot(t.Context())
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				ProjectID: snapshot.Projects[0].ID, ModelID: "fixture/test", Text: "Edit and write the line-count fixtures.",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			var sessionID string
			for {
				snapshot, err = core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle && snapshot.Sessions[0].LastOutcome == "completed" {
					sessionID = snapshot.Sessions[0].ID
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("edit fixture did not complete: %+v", snapshot.Runs)
				}
				time.Sleep(20 * time.Millisecond)
			}
			snapshot, err = core.Snapshot(t.Context(), sessionID)
			if err != nil {
				t.Fatal(err)
			}
			groupID := ""
			changes := map[string]*contracts.Change{}
			for _, message := range snapshot.Messages[sessionID] {
				for _, block := range message.Blocks {
					if block.Tool != nil && groupID == "" {
						groupID = message.ID + ":activity:" + block.ID
					}
					if block.Tool != nil {
						changes[block.ID] = block.Tool.Diff
					}
				}
			}
			if groupID == "" || requests.Load() != 3 {
				t.Fatalf("missing completed real tools: group=%q requests=%d", groupID, requests.Load())
			}
			if edit, write := changes["edit-lines"], changes["write-lines"]; edit == nil || write == nil ||
				edit.Additions != 12 || edit.Deletions != 1 || write.Additions != 2 || write.Deletions != 0 {
				t.Fatalf("real tools did not produce the expected line counts: %+v", changes)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			width := "1320"
			if appearance == "dark" {
				width = "900"
			}
			app := startAppReady(t, binary, artifacts, "counts", data, "activity-toggle:"+groupID,
				"--local", "--route", "chat/"+sessionID, "--appearance", appearance, "--language", "zh-CN",
				"--width", width, "--height", "700")
			app.act("activity-toggle:"+groupID, "press", "")
			for _, test := range []struct{ id, counts string }{{"edit-lines", "+12 -1"}, {"write-lines", "+2 -0"}} {
				key := "activity-item-toggle:" + groupID + ":" + test.id
				node := app.waitKey(key)
				if !strings.HasSuffix(node.Label, " "+test.counts) || node.Bounds.Width <= 0 {
					t.Fatalf("per-call line counts=%+v, want %s", node, test.counts)
				}
			}
			app.capture("collapsed-edit-line-counts")
			editRow := groupID + ":edit-lines"
			app.act("activity-item-toggle:"+editRow, "press", "")
			app.waitKey("activity-diff-viewport:" + editRow)
			app.capture("expanded-edit-diff")
			app.act("activity-item-toggle:"+editRow, "press", "")
		})
	}
}
