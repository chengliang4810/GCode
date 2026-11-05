//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
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

// The model endpoint is private; goal admission, TodoWrite, editing, stopping,
// resuming and completion all run through the real in-process core.
func TestDesktopLocalGoalTodoStack(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var calls atomic.Int32
			release := make(chan struct{}, 1)
			shutdown := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				n := calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if strings.Contains(string(body), "Review whether the objective below") {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"passed\\\":true,\\\"reason\\\":\\\"The edited objective was checked.\\\"}\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				if n == 2 || n == 3 {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					case <-shutdown:
						return
					}
				}
				if n == 1 || n == 3 {
					var items []map[string]string
					for i := range 8 {
						status := "completed"
						if n == 1 {
							status = "pending"
							if i < 2 {
								status = "completed"
							} else if i == 2 {
								status = "in_progress"
							}
						}
						items = append(items, map[string]string{"text": fmt.Sprintf("Goal task %d", i+1), "status": status})
					}
					if n == 3 && !strings.Contains(string(body), "Edited native goal") {
						t.Error("resume lost the saved objective")
					}
					args, _ := json.Marshal(map[string]any{"todos": items})
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"goal-todo-%d\",\"function\":{\"name\":\"TodoWrite\",\"arguments\":%s}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", n, strconv.Quote(string(args)))
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"GOAL_WORK_READY\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
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
				t.Fatal(err)
			}
			snapshot, _ := core.Snapshot(t.Context())
			settings := snapshot.Settings
			settings.Appearance.ReduceMotion = true
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "goal", Name: "Goal fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "test"}},
				}}},
				{Kind: "session.create", Name: "Goal and todos"},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			sessionID := snapshot.Sessions[0].ID
			modelID := "goal/test"
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
			app := startApp(t, binary, artifacts, "stack", data, "--local", "--appearance", appearance, "--width", width, "--height", height)
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.act("input", "setValue", "/goal Original native goal")
			app.act("send", "press", "")
			key := "todo-panel:main:" + sessionID
			app.waitKey(key + ":toggle")
			app.waitKey("composer-goal:" + sessionID)
			app.await("todo and goal attach above the composer", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				todo, err := tree.findKey(key)
				if err != nil {
					return err
				}
				goal, err := tree.findKey("composer-goal:" + sessionID)
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if todo.Bounds.Y+todo.Bounds.Height > goal.Bounds.Y+.5 || goal.Bounds.Y+goal.Bounds.Height > input.Bounds.Y ||
					todo.Bounds.X-goal.Bounds.X != 12 || goal.Bounds.X+goal.Bounds.Width-todo.Bounds.X-todo.Bounds.Width != 12 {
					return fmt.Errorf("incorrect composer stack: todo=%+v goal=%+v input=%+v", todo.Bounds, goal.Bounds, input.Bounds)
				}
				return nil
			})
			app.capture("collapsed-stack")
			groupKey := app.awaitKeyPrefix("activity-toggle:")
			app.act(groupKey, "press", "")
			itemKey := app.awaitKeyPrefix("activity-item-toggle:")
			app.act(itemKey, "press", "")
			app.await("todo tool detail exposes every status", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, text := range []string{"[x] Goal task 1", "[~] Goal task 3", "[ ] Goal task 4"} {
					if !activityTreeContains(tree, text) {
						return fmt.Errorf("missing formatted todo detail %q", text)
					}
				}
				return nil
			})
			app.capture("todo-tool-detail")
			app.act(groupKey, "press", "")
			app.act(key+":toggle", "press", "")
			app.waitKey(key + ":text:2")
			app.await("expanded rows are inside the tray", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				panel, err := tree.findKey(key)
				if err != nil {
					return err
				}
				row, err := tree.findKey(key + ":text:3")
				if err != nil {
					return err
				}
				if row.Bounds.Y+row.Bounds.Height > panel.Bounds.Y+panel.Bounds.Height {
					return fmt.Errorf("todo rows are still clipped")
				}
				return nil
			})
			app.capture("expanded-stack")
			app.act("input", "setValue", "Keep the message draft")
			app.act("goal-edit:"+sessionID, "press", "")
			app.act("goal-editor-input:"+sessionID, "setValue", "Keep the unsaved goal draft")
			app.act("workspace-close:goal:"+sessionID, "press", "")
			app.act("goal-edit:"+sessionID, "press", "")
			editorValue := func(expected string) {
				t.Helper()
				app.await("goal editor value is "+expected, func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					node, err := tree.findKey("goal-editor-input:" + sessionID)
					if err != nil {
						return err
					}
					if node.Value != expected {
						return fmt.Errorf("goal draft=%q, want %q", node.Value, expected)
					}
					return nil
				})
			}
			editorValue("Keep the unsaved goal draft")
			app.act("goal-editor-reload:"+sessionID, "press", "")
			editorValue("Original native goal")
			app.act("goal-editor-input:"+sessionID, "setValue", "Edited native goal")
			app.act("goal-editor-save:"+sessionID, "press", "")
			app.await("real core saves the goal edit", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range state.Sessions {
					if session.ID == sessionID && session.Goal != nil && session.Goal.Objective == "Edited native goal" && session.Goal.Revision == 1 {
						return nil
					}
				}
				return fmt.Errorf("edited objective has not been persisted")
			})
			app.capture("goal-editor")
			app.act("titlebar-right-pane", "press", "")
			app.act("goal-toggle:"+sessionID, "press", "")
			app.await("goal pauses its owned run", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range state.Sessions {
					if session.ID == sessionID && session.Goal != nil && session.Goal.Status == contracts.GoalPaused && session.Status == contracts.Idle {
						return nil
					}
				}
				return fmt.Errorf("goal has not paused")
			})
			app.waitKey(key + ":dismiss")
			app.capture("paused-stack")
			paused, err := readFixtureSnapshot(data)
			if err != nil {
				t.Fatal(err)
			}
			var pausedGoalID string
			for _, session := range paused.Sessions {
				if session.ID == sessionID {
					pausedGoalID = session.Goal.ID
				}
			}
			app.act("goal-toggle:"+sessionID, "press", "")
			release <- struct{}{}
			app.await("goal completes after resume", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range state.Sessions {
					if session.ID == sessionID && session.Goal != nil && session.Goal.Status == contracts.GoalComplete && session.Status == contracts.Idle {
						if session.Goal.ID == pausedGoalID || session.Goal.Revision != 1 {
							return fmt.Errorf("resume did not isolate execution while retaining the saved revision")
						}
						return nil
					}
				}
				return fmt.Errorf("goal has not completed")
			})
			app.await("finished todo tray hides while goal remains", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey(key + ":toggle"); err == nil {
					return fmt.Errorf("completed todos still visible")
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != "Keep the message draft" {
					return fmt.Errorf("goal controls lost composer draft: %q (%v)", input.Value, err)
				}
				return nil
			})
			app.capture("complete-goal")
			app.act("goal-clear:"+sessionID, "press", "")
			app.await("clear removes goal header", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("composer-goal:" + sessionID); err == nil {
					return fmt.Errorf("cleared goal remains visible")
				}
				return nil
			})
		})
	}
}
