//go:build e2e

package e2e

import (
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

// questionFixture 让实例提出一个问题，并在主代理的下一轮替它回答。
type questionFixture struct {
	mu    sync.Mutex
	data  string
	turns int
}

func (f *questionFixture) runID() string {
	state, err := readFixtureSnapshot(f.data)
	if err != nil || len(state.WorkflowRuns) == 0 {
		return ""
	}
	return state.WorkflowRuns[0].ID
}

func (f *questionFixture) questionID() string {
	state, err := readFixtureSnapshot(f.data)
	if err != nil || len(state.WorkflowRuns) == 0 || len(state.WorkflowRuns[0].Questions) == 0 {
		return ""
	}
	return state.WorkflowRuns[0].Questions[0].ID
}

func (f *questionFixture) handle(w http.ResponseWriter, r *http.Request) {
	body := readRequestBody(r)
	f.mu.Lock()
	turn := f.turns
	f.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	switch {
	case strings.Contains(body, "You are the workflow instance"):
		if strings.Contains(body, "主 Agent 的回答") {
			writeModelText(w, "The instance finished its part.")
			return
		}
		writeModelText(w, "<<workflow:question>>Which directory should I review?<<workflow:question>>")
		return
	case modelRequestIsToolContinuation(body):
		writeModelText(w, "Workflow native proof")
		return
	default:
		f.mu.Lock()
		f.turns++
		f.mu.Unlock()
		if turn == 0 {
			writeToolCall(w, "CreateWorkflow", map[string]any{
				"script": `{"phases":[{"id":"plan","name":"Plan","instances":[{"id":"reviewer","name":"Reviewer","prompt":"Review the change."}]}]}`,
				"name":   "Native-question",
			})
			return
		}
		writeToolCall(w, "ResolveWorkflowQuestion", map[string]any{
			"run_id": f.runID(), "question_id": f.questionID(), "answer": "Review src/",
		})
	}
}

// TestDesktopLocalWorkflowQuestionShowsAndIsAnswered 走一遍问题的来回：实例把问题
// 升级给主代理，卡上出现待答芯片，用户催一句之后主代理真的作答，运行继续。
func TestDesktopLocalWorkflowQuestionShowsAndIsAnswered(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			data := filepath.Join(artifacts, theme, "data")
			fixture := &questionFixture{data: data}
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
				ID: "native-question", Name: "Local fixture", Protocol: model.Chat, BaseURL: server.URL,
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

			app.await("the instance escalated a question", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) != 1 || len(state.WorkflowRuns[0].Questions) != 1 {
					return fmt.Errorf("questions=%+v", state.WorkflowRuns)
				}
				return nil
			})

			// 卡上是待答的问题数，不是让用户直接答题的表单。
			chipLabel := "1 questions"
			if theme == "dark" {
				chipLabel = "1 个待答问题"
			}
			app.await("the card shows the pending question", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == chipLabel {
						return nil
					}
				}
				return fmt.Errorf("the question chip %q is not on the card", chipLabel)
			})
			app.capture("workflow-question-chip")

			// 问题写在运行详情里，芯片点开的就是那处详情。
			app.act(app.awaitKeyPrefix("workflow-card-questions:"), "press", "")
			app.await("the run detail names the question", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, "Which directory should I review?") {
						return nil
					}
				}
				return fmt.Errorf("the question is not in the run detail")
			})
			app.capture("workflow-question-detail")

			// 主代理在下一轮作答。
			app.act("input", "setValue", "Answer the open question")
			app.act("send", "press", "")
			app.await("the answer reached the instance and the run finished", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(state.WorkflowRuns) != 1 {
					return fmt.Errorf("runs=%+v", state.WorkflowRuns)
				}
				run := state.WorkflowRuns[0]
				if len(run.Questions) != 1 {
					return fmt.Errorf("questions=%+v", run.Questions)
				}
				if run.Questions[0].Answer != "Review src/" {
					return fmt.Errorf("answer %q", run.Questions[0].Answer)
				}
				if run.Status != contracts.WorkflowStatusCompleted {
					return fmt.Errorf("run status %q (%s)", run.Status, run.Error)
				}
				return nil
			})
			app.capture("workflow-question-answered")
		})
	}
}
