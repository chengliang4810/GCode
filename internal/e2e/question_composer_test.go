//go:build e2e

package e2e

import (
	"context"
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

// The native question form talks directly to the real Go core. Only the model
// endpoint is a private HTTP fixture; no question or answer snapshot is injected.
func TestDesktopLocalQuestionComposer(t *testing.T) {
	for _, test := range []struct {
		appearance, language string
		width, height        int
	}{{"light", "en", 1320, 880}, {"dark", "zh-CN", 900, 600}} {
		t.Run(test.appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "data")
			release := make(chan struct{})
			var calls atomic.Int32
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				switch calls.Add(1) {
				case 1:
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					args := `{"questions":[{"question":"Which approach should we use?","header":"Approach","options":[{"label":"Approach A","description":"Use the first approach"},{"label":"Approach B","description":"Use the second approach"}]},{"question":"Which features should we include?","header":"Features","multiSelect":true,"options":[{"label":"Skills","description":"Reusable instructions"},{"label":"Hooks","description":"Lifecycle callbacks"}]}]}`
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"question-call\",\"function\":{\"name\":\"AskUserQuestion\",\"arguments\":%s}}]},\"finish_reason\":\"tool_calls\"}]}\n\n", strconv.Quote(args))
				case 2:
					if !strings.Contains(string(body), "Approach B") || !strings.Contains(string(body), "Custom features") {
						t.Errorf("model continuation lost the submitted choices or custom answer: %s", body)
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Question form completed.\"},\"finish_reason\":\"stop\"}]}\n\n")
				default:
					t.Error("question caused an unexpected extra model request")
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer fixture.Close()
			ctx := context.Background()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.Appearance.ReduceMotion = true
			provider := model.Provider{ID: "question", Name: "Question fixture", Protocol: model.Chat,
				BaseURL: fixture.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
				{Kind: "session.create", Name: "Question composer"},
			} {
				if err := core.Execute(ctx, command); err != nil {
					_ = core.Close()
					t.Fatal(err)
				}
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			sessionID := snapshot.Sessions[0].ID
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startAppReady(t, binary, artifacts, "question", data, "back-to-conversations", "--local",
				"--appearance", test.appearance, "--language", test.language,
				"--width", strconv.Itoa(test.width), "--height", strconv.Itoa(test.height))
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.act("input", "setValue", "Ask two questions.")
			app.act("send", "press", "")
			app.await("first model request has accepted the message", func() error {
				if calls.Load() != 1 {
					return fmt.Errorf("model calls=%d", calls.Load())
				}
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != "" {
					return fmt.Errorf("sent message draft has not cleared: %q (%v)", input.Value, err)
				}
				return nil
			})
			app.act("input", "setValue", "Keep the ordinary message draft")
			close(release)
			app.await("question replaces the bottom composer", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				panel, err := tree.findKey("question-panel")
				if err != nil {
					return err
				}
				if panel.Bounds.Y < 100 || panel.Bounds.Y+panel.Bounds.Height > float64(test.height) {
					return fmt.Errorf("question is not inside the bottom composer: %+v", panel.Bounds)
				}
				if _, err := tree.findKey("input"); err == nil {
					return fmt.Errorf("ordinary message editor remains mounted")
				}
				for _, node := range tree.Nodes {
					if strings.Contains(string(node.Key), "activity-toggle:") {
						return fmt.Errorf("question was duplicated into an ordinary tool activity")
					}
				}
				return nil
			})
			app.capture("question-first-page")
			submit := app.awaitKeyPrefix("question-submit-")
			requestID := strings.TrimPrefix(submit, "question-submit-")
			first := "question-option-" + requestID + "-0-0"
			second := "question-option-" + requestID + "-1-0"
			app.act(first, "press", "")
			app.waitKey(second)
			app.act(second, "press", "")
			app.act("question-option-"+requestID+"-1-1", "press", "")
			answer := "question-answer-" + requestID + "-1"
			app.act(answer, "setValue", "Custom features")
			app.act("question-back-"+requestID, "press", "")
			app.waitKey(first)
			app.act("question-option-"+requestID+"-0-1", "press", "")
			app.waitKey(answer)
			app.await("paged custom answer survives going back", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey(answer)
				if err != nil || node.Value != "Custom features" {
					return fmt.Errorf("custom answer was lost: %q (%v)", node.Value, err)
				}
				return nil
			})
			app.capture("question-second-page")
			app.act(submit, "press", "")
			app.await("answers persist and execution continues once", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range state.Sessions {
					if session.ID != sessionID || session.Status != contracts.Idle || session.LastOutcome != "completed" || calls.Load() != 2 {
						continue
					}
					for _, message := range state.Messages[sessionID] {
						for _, block := range message.Blocks {
							q := block.Question
							if q != nil && q.ID == requestID && q.Resolved && len(q.Pages) == 2 && len(q.Answers) == 2 &&
								len(q.Answers[0].Options) == 1 && q.Answers[0].Options[0] == "Approach B" &&
								q.Answers[1].Text == "Custom features" && len(q.Answers[1].Options) == 0 {
								return nil
							}
						}
					}
				}
				return fmt.Errorf("question request has not settled")
			})
			app.await("ordinary composer and its draft return", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != "Keep the ordinary message draft" {
					return fmt.Errorf("ordinary draft was not restored: %q (%v)", input.Value, err)
				}
				if _, err := tree.findKey("question-panel"); err == nil {
					return fmt.Errorf("resolved question form remains mounted")
				}
				for _, node := range tree.Nodes {
					if strings.Contains(string(node.Key), "question-option-") {
						return fmt.Errorf("resolved question retained interactive options in the transcript")
					}
				}
				return nil
			})
			app.capture("question-resolved")
		})
	}
}
