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
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// TestDesktopLocalWorkflowHub 用真实本机核心走一遍中枢：已保存的定义列在中枢里，
// 进详情能看见脚本与参数，从中枢启动会在新会话里真的跑起来。
// 模型是本地 HTTP 夹具，不代表真实供应商或远程宿主验收。
func TestDesktopLocalWorkflowHub(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Workflow native proof\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			data := filepath.Join(artifacts, theme, "data")
			workspace := filepath.Join(artifacts, theme, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			commands := []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "native-workflow", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "model"}}}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Workflow fixture"}},
			}
			for _, command := range commands {
				if err = core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			projectID := ""
			if len(snapshot.Projects) > 0 {
				projectID = snapshot.Projects[0].ID
			}
			if err = core.Execute(t.Context(), contracts.Command{Kind: "workflow.save", Workflow: &contracts.WorkflowCommand{Definition: &contracts.WorkflowDefinitionSave{
				Name:        "Native-review",
				Description: "Reviews a change and reports back",
				Scope:       "project",
				ProjectID:   projectID,
				Script:      `{"phases":[{"id":"plan","name":"Plan"}]}`,
				Program: contracts.WorkflowProgram{Phases: []contracts.WorkflowPhaseSpec{{
					ID: "plan", Name: "Plan",
					Instances: []contracts.WorkflowActorSpec{{ID: "reviewer", Name: "Reviewer", Prompt: "Review the change."}},
				}}},
			}}}); err != nil {
				t.Fatal(err)
			}
			snapshot, err = core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.WorkflowDefinitions) != 1 {
				t.Fatalf("the definition is saved: %+v", snapshot.WorkflowDefinitions)
			}
			definition := snapshot.WorkflowDefinitions[0]
			if err = core.Close(); err != nil {
				t.Fatal(err)
			}

			language := "en"
			if theme == "dark" {
				language = "zh-CN"
			}
			app := startApp(t, binary, artifacts, theme+"-native", data, "--local", "--appearance", theme, "--language", language)
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-automations", "press", "")
			app.act("automation-module-workflows", "press", "")
			app.waitKey("workflow-hub-open:" + definition.ID)
			app.await("the saved definition is listed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, definition.Name) {
						return nil
					}
				}
				return fmt.Errorf("no node mentions %q", definition.Name)
			})
			app.capture("hub-list")

			app.act("workflow-hub-open:"+definition.ID, "press", "")
			app.waitKey("workflow-detail-run")
			app.await("the definition page shows the script", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, `"phases"`) {
						return nil
					}
				}
				return fmt.Errorf("the script is not on the definition page")
			})
			app.capture("hub-detail")

			app.act("workflow-detail-run", "press", "")
			app.await("the hub launch really ran", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, run := range state.WorkflowRuns {
					if run.DefinitionID != definition.ID {
						continue
					}
					if run.Status != contracts.WorkflowStatusCompleted {
						return fmt.Errorf("run status %q", run.Status)
					}
					if run.SessionID == "" {
						return fmt.Errorf("the run has no conversation")
					}
					return nil
				}
				return fmt.Errorf("no run of %q yet", definition.ID)
			})
			app.capture("hub-run")

			// 运行详情里的步骤面板读的是核心写的日志：真的运行留下了真的步骤。
			steps := "Steps"
			started := "Run started"
			if theme == "dark" {
				steps, started = "步骤", "运行开始"
			}
			app.await("the run detail shows the steps the core wrote", func() error {
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
				if !labels[steps] {
					return fmt.Errorf("the steps panel is not in the run detail")
				}
				if !labels[started] {
					return fmt.Errorf("the steps panel has no run step yet")
				}
				return nil
			})
			app.capture("hub-steps")

			// 从这一次运行能看到同类：这条对话的运行目录，再点回详情。
			state, err := readFixtureSnapshot(data)
			if err != nil {
				t.Fatal(err)
			}
			runID := state.WorkflowRuns[0].ID
			app.act("workflow-run-open-directory:"+runID, "press", "")
			app.waitKey("workflow-runs-row:" + runID)
			app.capture("hub-runs-directory")
			app.act("workflow-runs-row:"+runID, "press", "")
			app.waitKey("workflow-run:" + runID)

			// 这一次运行本来就是那份定义跑出来的，所以详情里直接给「再次运行」。
			app.await("the saved run offers to run again", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					var value string
					_ = json.Unmarshal(node.Key, &value)
					if strings.Contains(value, "workflow-again") {
						return nil
					}
				}
				return fmt.Errorf("the saved run offers to run again")
			})
			app.act(app.awaitKeyPrefix("workflow-again"), "press", "")
			app.await("the second run really started", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) < 2 {
					return fmt.Errorf("runs=%+v", state.WorkflowRuns)
				}
				return nil
			})
			app.capture("hub-run-again")
		})
	}
}
