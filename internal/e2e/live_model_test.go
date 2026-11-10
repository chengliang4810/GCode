//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Real native controls and local core; requests use an isolated HTTP fixture.
func TestDesktopLocalLiveModelChanges(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, steering := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/steering=%t", appearance, steering), func(t *testing.T) {
				binary, artifacts := testPaths(t)
				started, release := make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				var mu sync.Mutex
				var requests []map[string]any
				var credentials []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					requests = append(requests, body)
					credentials = append(credentials, r.Header.Get("Authorization"))
					first := len(requests) == 1
					mu.Unlock()
					if first {
						close(started)
						select {
						case <-release:
						case <-r.Context().Done():
							t.Error("selection canceled the in-flight request")
							return
						}
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"live-write\",\"function\":{\"name\":\"Write\",\"arguments\":\"{\\\"file_path\\\":\\\"live.txt\\\",\\\"content\\\":\\\"preserved\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
						return
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"LIVE_CHANGE_COMPLETED\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}))
				defer server.Close()
				defer unblock()
				data := filepath.Join(artifacts, "data")
				store, err := storage.Open(storage.Options{Directory: data})
				if err != nil {
					t.Fatal(err)
				}
				core, err := openExecutionFixture(t, engine.Options{Storage: store})
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{"alpha", "beta"} {
					err = core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
						ID: id, Name: id, Protocol: model.Chat, BaseURL: server.URL, APIKey: id + "-fixture-key",
						Models: []model.Model{{ID: "test", ReasoningLevels: []string{"low", "high", "max"}}},
					}}})
					if err != nil {
						_ = core.Close()
						t.Fatal(err)
					}
				}
				if err = core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Live model changes"}); err != nil {
					t.Fatal(err)
				}
				state, _ := core.Snapshot(t.Context())
				sid, initialModel, initialEffort := state.Sessions[0].ID, "alpha/test", "Low"
				err = core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{
					ID: sid, ModelID: &initialModel, ReasoningLevel: &initialEffort,
				}})
				if err != nil {
					t.Fatal(err)
				}
				if err = core.Close(); err != nil {
					t.Fatal(err)
				}
				language := "en"
				if appearance == "dark" {
					language = "zh-CN"
				}
				app := startApp(t, binary, artifacts, "live-model", data, "--local", "--appearance", appearance, "--language", language, "--width", "900", "--height", "600")
				// Unblock before stopping the private app on every exit path.
				defer unblock()
				app.act("back-to-conversations", "press", "")
				app.act("sidebar-session:"+sid, "press", "")
				app.waitKey("titlebar-session-info")
				app.act("input", "focus", "")
				app.act("input", "setValue", "Write live.txt and finish")
				app.await("composer draft is ready", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					input, err := tree.findKey("input")
					if err != nil {
						return err
					}
					if input.Value != "Write live.txt and finish" {
						return fmt.Errorf("draft=%q", input.Value)
					}
					return nil
				})
				app.act("send", "press", "")
				select {
				case <-started:
				case <-time.After(stateTimeout):
					t.Fatal("first model request did not start")
				}
				waitSelection := func(modelID, effort string) {
					app.await("running selection persists silently", func() error {
						snapshot, err := readFixtureSnapshot(data)
						if err != nil {
							return err
						}
						for _, session := range snapshot.Sessions {
							if session.ID == sid && session.ModelID == modelID && session.ReasoningLevel == effort && session.Status == contracts.Working {
								return nil
							}
						}
						return fmt.Errorf("selection not applied: %+v", snapshot.Sessions)
					})
				}
				app.act("reasoning-trigger", "press", "")
				app.act("effort-slider", "increment", "")
				waitSelection("alpha/test", "High")
				app.act("model-trigger", "press", "")
				app.act("model-provider:beta", "press", "")
				app.act("model-choice:beta/test", "press", "")
				waitSelection("beta/test", "Max")
				app.act("reasoning-trigger", "press", "")
				app.act("effort-slider", "decrement", "")
				waitSelection("beta/test", "High")
				app.capture("running-selection-changed")
				waitTasks := func(count int) {
					app.await("user tasks complete independently", func() error {
						snapshot, err := readFixtureSnapshot(data)
						if err != nil {
							return err
						}
						runs := map[string]string{}
						for _, run := range snapshot.Runs {
							runs[run.ID] = run.Status
						}
						if len(runs) != count {
							return fmt.Errorf("runs=%+v", snapshot.Runs)
						}
						for _, status := range runs {
							if status != "completed" {
								return fmt.Errorf("runs=%+v", snapshot.Runs)
							}
						}
						return nil
					})
				}
				if !steering {
					unblock()
					waitTasks(1)
					mu.Lock()
					oldTaskRequests := append([]map[string]any(nil), requests...)
					mu.Unlock()
					if len(oldTaskRequests) != 2 || oldTaskRequests[1]["reasoning_effort"] != "low" {
						t.Fatalf("picker edit changed the current task: %+v", oldTaskRequests)
					}
				}
				app.act("model-trigger", "press", "")
				app.act("model-provider:beta", "press", "")
				app.act("model-choice:beta/test", "press", "")
				app.act("input", "focus", "")
				app.act("input", "setValue", "Next user task")
				app.await("next draft is ready", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					input, err := tree.findKey("input")
					if err != nil {
						return err
					}
					if input.Value != "Next user task" {
						return fmt.Errorf("draft=%q", input.Value)
					}
					send, err := tree.findKey("send")
					if err != nil {
						return err
					}
					if send.Label == "Stop response" || send.Label == "停止回复" {
						return fmt.Errorf("send action still has the previous stop handler")
					}
					if _, err := tree.findKey("effort-slider"); err == nil {
						return fmt.Errorf("reasoning popup has not closed")
					}
					return nil
				})
				app.act("send", "press", "")
				if steering {
					app.await("steering input is queued before the current request finishes", func() error {
						snapshot, err := readFixtureSnapshot(data)
						if err != nil {
							return err
						}
						for _, queued := range snapshot.Queue {
							if queued.Text == "Next user task" && queued.Delivery == "steer" {
								return nil
							}
						}
						return fmt.Errorf("steering input was not queued: %+v", snapshot.Queue)
					})
					unblock()
				}
				waitTasks(2)
				mu.Lock()
				captured := append([]map[string]any(nil), requests...)
				capturedCredentials := append([]string(nil), credentials...)
				mu.Unlock()
				boundary := 2
				if steering {
					boundary = 1
				}
				if len(captured) != boundary+1 || captured[0]["reasoning_effort"] != "low" ||
					captured[boundary]["reasoning_effort"] != "high" ||
					capturedCredentials[0] != "Bearer alpha-fixture-key" || capturedCredentials[boundary] != "Bearer beta-fixture-key" {
					t.Fatalf("request settings=%+v", captured)
				}
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					t.Fatal(err)
				}
				turnModels := make(map[string]string)
				for _, request := range snapshot.ModelRequests {
					selected := request.ProviderID + "/" + request.ModelID
					if previous := turnModels[request.TurnID]; previous != "" && previous != selected {
						t.Fatalf("task contains mixed model accounting: %+v", snapshot.ModelRequests)
					}
					turnModels[request.TurnID] = selected
				}
				if len(turnModels) != 2 {
					t.Fatalf("user messages did not own two task statistics: %+v", turnModels)
				}
				app.capture("completed-without-interruption")
			})
		}
	}
}
