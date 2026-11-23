//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// workflowFixture 是一条按请求内容分流的模型夹具：它替主代理发起工作流、
// 替实例提出留白或问题，并在主代理被追问时替它补全留白、回答问题。
// 分流靠请求体本身，因此不依赖请求到达的先后。
type workflowFixture struct {
	mu       sync.Mutex
	data     string
	script   string
	name     string
	turns    int
	requests int
}

func (f *workflowFixture) scriptFor(turn int) string { return f.script }

// handle answers one model request.
func (f *workflowFixture) handle(w http.ResponseWriter, r *http.Request) {
	body := readRequestBody(r)
	f.mu.Lock()
	f.requests++
	turn := f.turns
	f.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	switch {
	case strings.Contains(body, "You are the workflow instance"):
		// 实例会话：拿到补全或回答之后的下一轮，收到的是那句交付文本。
		if strings.Contains(body, "留白补全") || strings.Contains(body, "主 Agent 的回答") {
			writeModelText(w, "The instance finished its part.")
			return
		}
		writeModelText(w, "<<workflow:hole>>Missing target<<workflow:hole>>")
		return
	case modelRequestIsToolContinuation(body):
		// 工具结果回来之后主代理继续说人话。
		writeModelText(w, "Workflow native proof")
		return
	default:
		f.mu.Lock()
		f.turns++
		f.mu.Unlock()
		switch turn {
		case 0:
			writeToolCall(w, "CreateWorkflow", map[string]any{
				"script": f.script, "name": f.name, "question": "Run the review workflow?",
			})
		default:
			writeToolCall(w, "FillWorkflowHole", map[string]any{
				"run_id": f.runID(), "hole_id": f.holeID(), "text": "src/app.go",
			})
		}
	}
}

// runID reads the running workflow's identity from the persisted state.
func (f *workflowFixture) runID() string {
	state, err := readFixtureSnapshot(f.data)
	if err != nil || len(state.WorkflowRuns) == 0 {
		return ""
	}
	return state.WorkflowRuns[0].ID
}

// holeID reads the waiting hole's identity from the persisted state.
func (f *workflowFixture) holeID() string {
	state, err := readFixtureSnapshot(f.data)
	if err != nil || len(state.WorkflowRuns) == 0 || len(state.WorkflowRuns[0].Holes) == 0 {
		return ""
	}
	return state.WorkflowRuns[0].Holes[0].ID
}

// modelRequestIsToolContinuation reports whether the request is the agent
// speaking again after a tool answered.
func modelRequestIsToolContinuation(body string) bool {
	var request struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if json.Unmarshal([]byte(body), &request) != nil || len(request.Messages) == 0 {
		return false
	}
	return request.Messages[len(request.Messages)-1].Role == "tool"
}

// writeModelText answers with plain content.
func writeModelText(w http.ResponseWriter, text string) {
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", mustJSONString(text))
}

// writeToolCall answers with one tool call.
func writeToolCall(w http.ResponseWriter, name string, arguments map[string]any) {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"workflow-call\",\"function\":{\"name\":%s,\"arguments\":%s}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", mustJSONString(name), mustJSONString(string(encoded)))
}

// readRequestBody reads a model request body.
func readRequestBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	data := make([]byte, 0)
	buffer := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buffer)
		data = append(data, buffer[:n]...)
		if err != nil {
			break
		}
	}
	return string(data)
}

// TestDesktopLocalWorkflowFillHoleContinuesTheRun 让实例提出留白之后，主代理在
// 下一轮真的把它补上：补全走同一个运行，运行继续跑到完成。
func TestDesktopLocalWorkflowFillHoleContinuesTheRun(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			data := filepath.Join(artifacts, theme, "data")
			fixture := &workflowFixture{
				data:   data,
				script: `{"phases":[{"id":"plan","name":"Plan","instances":[{"id":"reviewer","name":"Reviewer","prompt":"Review the change."}]}]}`,
				name:   "Native-fill",
			}
			server := httptest.NewServer(http.HandlerFunc(fixture.handle))
			defer server.Close()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			if err = core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
				ID: "native-fill", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL,
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
			approve := app.awaitKeyPrefix("workflow-confirm-once:")
			app.act(approve, "press", "")

			app.await("the hole is waiting for the main agent", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) != 1 || len(state.WorkflowRuns[0].Holes) != 1 {
					return fmt.Errorf("runs=%+v", state.WorkflowRuns)
				}
				return nil
			})
			app.capture("workflow-hole-waiting")

			// 主代理在下一轮补上这处留白。
			app.act("input", "setValue", "Fill the missing target")
			app.act("send", "press", "")
			app.await("the hole is filled and the run continues", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) != 1 {
					return fmt.Errorf("runs=%+v", state.WorkflowRuns)
				}
				run := state.WorkflowRuns[0]
				if len(run.Holes) != 1 {
					return fmt.Errorf("holes=%+v", run.Holes)
				}
				if run.Holes[0].Status != contracts.WorkflowHoleFilled {
					return fmt.Errorf("hole status %q", run.Holes[0].Status)
				}
				if run.Holes[0].Text != "src/app.go" {
					return fmt.Errorf("hole text %q", run.Holes[0].Text)
				}
				if run.Status != contracts.WorkflowStatusCompleted {
					return fmt.Errorf("run status %q", run.Status)
				}
				return nil
			})
			app.capture("workflow-hole-filled")
		})
	}
}
