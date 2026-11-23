//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// TestDesktopLocalWorkflowDirectRunCard 用真实本机核心验证模型调用 CreateWorkflow
// 后无需用户操作即启动，且对话中出现运行卡和阶段线。
// 模型是本地 HTTP 夹具，不代表真实供应商或远程宿主验收。
func TestDesktopLocalWorkflowDirectRunCard(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			var calls atomic.Int64
			// 六个阶段：卡上的阶段线一定超出卡片宽度，横向滚动因此出现。
			script := `{"phases":[` +
				`{"id":"plan","name":"Plan","instances":[{"id":"reviewer","name":"Reviewer","prompt":"Review the change."}]},` +
				`{"id":"build","name":"Build"},{"id":"check","name":"Check"},` +
				`{"id":"report","name":"Report"},{"id":"ship2","name":"Ship"},{"id":"close","name":"Close"}]}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if calls.Add(1) == 1 {
					args := fmt.Sprintf(`{"script":%s,"name":"Native-card"}`, mustJSONString(script))
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"workflow-call\",\"function\":{\"name\":\"CreateWorkflow\",\"arguments\":%s}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", mustJSONString(args))
					return
				}
				if strings.Contains(readRequestBody(r), "You are the workflow instance") {
					// 实例交一件产物：完成卡因此有东西可展示。
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<<workflow:artifact>>report|the native report<<workflow:artifact>>\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
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
				ID: "native-card", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL,
				Models: []model.Model{{ID: "model"}},
			}}}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			// 始终开启：工具目录里就有工作流工具，不必先敲命令激活。
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

			app.await("the workflow run card appears in the conversation", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				names := map[string]bool{}
				for _, node := range tree.Nodes {
					if node.Label != "" {
						names[node.Label] = true
					}
				}
				if !names["Native-card"] {
					return fmt.Errorf("the run card is not in the conversation")
				}
				return nil
			})
			app.capture("workflow-run-card")
			// 阶段线超出卡片时，卡上给一条细滚动轨，而不是让阶段跑出卡外。
			scrollbar := app.awaitKeyPrefix("workflow-timeline-scrollbar:")
			if scrollbar == "" {
				t.Fatal("the overflowing stage line offers a scrollbar")
			}

			app.await("the workflow run really finished", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) != 1 {
					return fmt.Errorf("runs=%+v", state.WorkflowRuns)
				}
				if state.WorkflowRuns[0].Status != contracts.WorkflowStatusCompleted {
					return fmt.Errorf("run status %q", state.WorkflowRuns[0].Status)
				}
				if state.WorkflowRuns[0].MessageID == "" {
					return fmt.Errorf("the run is not anchored to the turn that launched it")
				}
				return nil
			})
			// 跑完之后那一轮末尾先出现完成卡：主产物预览 + 四项统计。
			app.await("the completion card shows the delivery", func() error {
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
				want := []string{"Workflow completed", "report", "the native report", "Duration", "Tokens", "Instances", "Phases entered"}
				if theme == "dark" {
					want = []string{"工作流已完成", "report", "the native report", "耗时", "Token", "子代理", "进入阶段"}
				}
				for _, want := range want {
					if !labels[want] {
						return fmt.Errorf("completion card is missing %q", want)
					}
				}
				return nil
			})
			app.capture("workflow-completion-card")
		})
	}
}

// mustJSONString quotes a string as a JSON literal for the model fixture.
func mustJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
