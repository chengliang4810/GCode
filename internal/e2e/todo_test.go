//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Only the provider endpoint is a fixture. TodoWrite runs in the real local
// core and reaches a native window through the production subscription path.
func TestDesktopLocalTodoWrite(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var calls atomic.Int32
			var requests atomic.Int32
			advance := make(chan struct{}, 4)
			shutdown := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				request := requests.Add(1)
				if request == 3 && !strings.Contains(string(body), "item status must be") {
					t.Error("invalid TodoWrite did not return its validation error to the model")
				}
				select {
				case <-advance:
				case <-r.Context().Done():
					return
				case <-shutdown:
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				n := calls.Add(1)
				args := ""
				switch n {
				case 1:
					var items []map[string]string
					for i := range 10 {
						status := "pending"
						if i < 4 {
							status = "completed"
						} else if i == 4 {
							status = "in_progress"
						}
						items = append(items, map[string]string{"text": fmt.Sprintf("Native task %d", i+1), "status": status})
					}
					raw, _ := json.Marshal(map[string]any{"todos": items})
					args = string(raw)
				case 2:
					args = `{"todos":[{"text":"REJECTED_TODO_MUST_NOT_RENDER","status":"done"}]}`
				case 3:
					args = `{"todos":[{"text":"Inspect native todo list","status":"completed"},{"text":"Verify todo persistence","status":"completed"}]}`
				case 5:
					args = `{"todos":[{"text":"Resume this unfinished task later","status":"pending"}],"resolution":{"kind":"blocked","reason":"Waiting for review"}}`
				default:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"TODOS_VERIFIED\"},\"finish_reason\":\"stop\"}]}\n\n")
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"todo-%d\",\"function\":{\"name\":\"TodoWrite\",\"arguments\":%s}}]},\"finish_reason\":\"tool_calls\"}]}\n\n", n, strconv.Quote(args))
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			defer close(shutdown)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.Appearance.ReduceMotion = true
			settings.General.CompactMode = appearance == "dark"
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "todos", Name: "Todo fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "test"}},
				}}},
				{Kind: "session.create", Name: "Native todos"},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			sessionID := snapshot.Sessions[0].ID
			modelID := "todos/test"
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", SessionID: sessionID,
				Session: &contracts.SessionUpdate{ID: sessionID, ModelID: &modelID}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			width, height := "1320", "880"
			if appearance == "dark" {
				width, height = "900", "600"
			}
			args := []string{"--local", "--appearance", appearance, "--width", width, "--height", height}
			app := startApp(t, binary, artifacts, "live", data, args...)
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.act("input", "focus", "")
			app.act("input", "setValue", "Track and verify these tasks")
			app.await("composer draft is committed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if input.Value != "Track and verify these tasks" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
			app.act("send", "press", "")
			advance <- struct{}{}
			key := "todo-panel:main:" + sessionID
			app.waitKey(key + ":toggle")
			tree, err := app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tree.findKey(key + ":text:4"); err == nil {
				t.Fatal("new todo panel starts expanded")
			}
			app.act(key+":toggle", "press", "")
			app.waitKey(key + ":text:3")
			app.waitKey(key + ":text:4")
			app.waitKey(key + ":text:5")
			app.act(key+":fold-earlier", "press", "")
			app.waitKey(key + ":text:0")
			app.await("expanded rows are inside the visible tray", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				panel, err := tree.findKey(key)
				if err != nil {
					return err
				}
				row, err := tree.findKey(key + ":text:0")
				if err != nil {
					return err
				}
				if row.Bounds.Y+row.Bounds.Height > panel.Bounds.Y+panel.Bounds.Height {
					return fmt.Errorf("row is still clipped")
				}
				return nil
			})
			app.capture("live-todos")
			advance <- struct{}{}
			app.await("failed TodoWrite returns to the model", func() error {
				if requests.Load() >= 3 {
					return nil
				}
				return fmt.Errorf("next request has not started")
			})
			tree, err = app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			count, err := tree.findKey(key + ":toggle")
			if err != nil || count.Value != "4/10" {
				t.Fatalf("failed call replaced current checklist: %+v %v", count, err)
			}
			advance <- struct{}{}
			app.await("completed todo list hides", func() error {
				if requests.Load() < 4 {
					return fmt.Errorf("completion has not been processed")
				}
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey(key + ":toggle"); err == nil {
					return fmt.Errorf("completed checklist remains visible")
				}
				return nil
			})
			app.capture("completed-todos")
			advance <- struct{}{}
			app.await("todo run finishes", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range state.Sessions {
					if session.ID == sessionID && session.Status == contracts.Idle && session.LastOutcome == "completed" {
						failed, completed := false, false
						for _, message := range state.Messages[sessionID] {
							for _, block := range message.Blocks {
								if block.Tool != nil {
									failed = failed || block.Tool.ID == "todo-2" && block.Tool.IsError
									completed = completed || block.Tool.ID == "todo-3" && block.Tool.Status == "completed"
								}
							}
						}
						if !failed || !completed {
							return fmt.Errorf("tool outcomes were not persisted: failed=%t completed=%t", failed, completed)
						}
						return nil
					}
				}
				return fmt.Errorf("todo run has not settled")
			})
			app.stop()
			restarted := startApp(t, binary, artifacts, "restart", data, args...)
			restarted.act("back-to-conversations", "press", "")
			restarted.act("sidebar-session:"+sessionID, "press", "")
			restarted.waitKey("input")
			tree, err = restarted.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tree.findKey(key + ":toggle"); err == nil {
				t.Fatal("restart revived the older unfinished list")
			}
			restarted.capture("completed-list-stays-hidden")
			if calls.Load() != 4 {
				t.Fatalf("reopening todos re-executed the model: calls=%d", calls.Load())
			}
			if err := restarted.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			restarted.act("input", "focus", "")
			restarted.act("input", "setValue", "Track new unfinished work")
			restarted.act("send", "press", "")
			advance <- struct{}{}
			toggle := restarted.waitKey(key + ":toggle")
			if toggle.Value != "0/1" {
				t.Fatalf("new work did not start a fresh list: %+v", toggle)
			}
			advance <- struct{}{}
			restarted.await("new unfinished run settles", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range state.Sessions {
					if session.ID == sessionID && session.Status == contracts.Idle && calls.Load() == 6 {
						return nil
					}
				}
				return fmt.Errorf("new run has not settled")
			})
			restarted.stop()
			reopened := startApp(t, binary, artifacts, "reopen-unfinished", data, args...)
			reopened.act("back-to-conversations", "press", "")
			reopened.act("sidebar-session:"+sessionID, "press", "")
			reopened.waitKey(key + ":toggle")
			reopened.capture("unfinished-list-restored")
			reopened.act(key+":dismiss", "press", "")
			reopened.act("sidebar-session:"+sessionID, "press", "")
			reopened.await("dismiss survives navigation", func() error {
				tree, err := reopened.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey(key + ":toggle"); err == nil {
					return fmt.Errorf("dismissed list returned")
				}
				return nil
			})
			if calls.Load() != 6 {
				t.Fatalf("reopening unfinished work executed model requests: %d", calls.Load())
			}
		})
	}
}
