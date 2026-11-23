//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// TestDesktopLocalWorkflowHoleShowsOnTheCard 让实例真的提出一处留白：卡上出现
// 待补全芯片，点它打开运行详情，详情里写着这一处留白在等谁补。
// 模型是本地 HTTP 夹具，不代表真实供应商或远程宿主验收。
func TestDesktopLocalWorkflowHoleShowsOnTheCard(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			script := `{"phases":[{"id":"plan","name":"Plan","instances":[{"id":"reviewer","name":"Reviewer","prompt":"Review the change."}]}]}`
			first := true
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := make([]byte, 0)
				buffer := make([]byte, 4096)
				for {
					n, err := r.Body.Read(buffer)
					body = append(body, buffer[:n]...)
					if err != nil {
						break
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if first {
					first = false
					args := fmt.Sprintf(`{"script":%s,"name":"Native-hole"}`, mustJSONString(script))
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"workflow-call\",\"function\":{\"name\":\"CreateWorkflow\",\"arguments\":%s}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", mustJSONString(args))
					return
				}
				// 实例这一轮提出一处留白，等主代理补代码。实例会话的提示
				// 以这句开头，主代理的回合不会。
				if strings.Contains(string(body), "You are the workflow instance") {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<<workflow:hole>>Missing target<<workflow:hole>>\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Workflow native proof\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			data := filepath.Join(artifacts, theme, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			if err = core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
				ID: "native-hole", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL,
				Models: []model.Model{{ID: "model"}},
			}}}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.WorkflowMode = contracts.WorkflowModeAlwaysOn
			if err = core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			if err = core.Close(); err != nil {
				t.Fatal(err)
			}

			language := "en"
			if theme == "dark" {
				language = "zh-CN"
			}
			app := startApp(t, binary, artifacts, theme+"-native", data, "--local", "--appearance", theme, "--language", language)
			app.act("back-to-conversations", "press", "")
			app.act("input", "setValue", "Design a review workflow")
			app.act("send", "press", "")

			app.await("the hole is recorded and the run waits for it", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) != 1 {
					return fmt.Errorf("runs=%+v", state.WorkflowRuns)
				}
				if len(state.WorkflowRuns[0].Holes) != 1 {
					return fmt.Errorf("holes=%+v", state.WorkflowRuns[0].Holes)
				}
				if state.WorkflowRuns[0].Holes[0].Title != "Missing target" {
					return fmt.Errorf("hole=%+v", state.WorkflowRuns[0].Holes[0])
				}
				return nil
			})

			chipLabel := "1 holes"
			if theme == "dark" {
				chipLabel = "1 处留白"
			}
			app.await("the card shows the waiting hole", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == chipLabel {
						return nil
					}
				}
				return fmt.Errorf("the waiting hole chip %q is not on the card", chipLabel)
			})
			app.capture("workflow-hole-chip")

			chip := app.awaitKeyPrefix("workflow-card-holes:")
			app.act(chip, "press", "")
			app.await("the run detail names the waiting hole", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				labels := map[string]bool{}
				for _, node := range tree.Nodes {
					if node.Label != "" {
						labels[node.Label] = true
					}
				}
				if !labels["Missing target"] {
					return fmt.Errorf("the run detail does not name the hole: %v", labels)
				}
				return nil
			})
			app.capture("workflow-hole-detail")
		})
	}
}
