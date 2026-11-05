//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
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

// Real native UI and in-process core, with a local provider fixture only.
func TestDesktopLocalPromptReferences(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				select {
				case requests <- string(body):
				default:
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Reference received.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
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
			provider := model.Provider{ID: "prompt", Name: "Prompt fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Prompt fixture"}},
				{Kind: "resource.skill.save", Resource: &contracts.ResourceInput{Skill: &contracts.UserSkill{ID: "docs", Name: "文档", Content: "SKILL INSTRUCTIONS E2E", Enabled: true}}},
				{Kind: "resource.command.save", Resource: &contracts.ResourceInput{Command: &contracts.UserCommand{ID: "review", Name: "review", Content: "REVIEW $ARGUMENTS", Enabled: true}}},
			} {
				if err := core.Execute(context.Background(), command); err != nil {
					_ = core.Close()
					t.Fatal(err)
				}
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "references", data, "--local", "--appearance", appearance, "--width", "900")
			app.act("back-to-conversations", "press", "")
			app.await("composer has no standalone goal shortcut", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == "Goal" || node.Label == "Start a goal" {
						return fmt.Errorf("unexpected composer shortcut: %q", node.Label)
					}
				}
				return nil
			})
			app.capture("composer")
			app.act("input", "setValue", "/rev")
			app.act("input", "focus", "")
			app.waitKey("prompt-choice:command:review")
			app.assertPromptAbove()
			app.capture("slash-picker")
			app.act("prompt-choice:command:review", "press", "")
			app.await("mouse selection inserts the command atom", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if node.Value != "\ufffc " {
					return fmt.Errorf("command reference not inserted: %q", node.Value)
				}
				return nil
			})
			app.act("input", "focus", "")
			// insertText is an explicit automation protocol action. Text events
			// require OS window focus, which the runner does not take from the user.
			if err := app.client.call("act", map[string]any{"key": "input", "action": "insertText", "text": "请检查 @doc"}, nil); err != nil {
				t.Fatal(err)
			}
			app.waitKey("prompt-choice:skill:docs")
			app.assertPromptAbove()
			app.capture("mention-picker")
			app.act("prompt-choice:skill:docs", "press", "")
			app.await("resource selection inserts the skill atom", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if strings.Count(node.Value, "\ufffc") != 2 {
					return fmt.Errorf("skill reference not inserted: %q", node.Value)
				}
				return nil
			})
			app.capture("selected-tokens")
			app.act("send", "press", "")
			app.await("provider receives expanded command and selected skill", func() error {
				select {
				case body := <-requests:
					if strings.Contains(body, "REVIEW") && strings.Contains(body, "SKILL INSTRUCTIONS E2E") && !strings.Contains(body, "gcode://") {
						return nil
					}
					return fmt.Errorf("unexpected provider request: %s", body)
				default:
					return fmt.Errorf("provider has not received the message")
				}
			})
			var acceptedID string
			app.await("canonical reference persisted in completed conversation", func() error {
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range snapshot.Sessions {
					if session.Status != contracts.Idle || session.LastOutcome != "completed" || strings.Contains(session.Title, "gcode://") {
						continue
					}
					for _, message := range snapshot.Messages[session.ID] {
						if message.Role == "user" && len(message.Blocks) > 0 && strings.Contains(message.Blocks[0].Text, "gcode://command/review") && strings.Contains(message.Blocks[0].Text, "gcode://skill/docs") {
							acceptedID = session.ID
							return nil
						}
					}
				}
				return fmt.Errorf("completed conversation with canonical references is missing")
			})
			app.capture("rendered-message")
			app.stop()
			restarted := startApp(t, binary, artifacts, "restart", data, "--local", "--appearance", appearance, "--width", "900")
			restarted.act("back-to-conversations", "press", "")
			restarted.act("sidebar-session:"+acceptedID, "press", "")
			restarted.await("reference labels restored from persisted canonical input", func() error {
				tree, err := restarted.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, "请检查") && strings.Contains(node.Label, "文档") {
						return nil
					}
				}
				return fmt.Errorf("restored reference message is not visible")
			})
			restarted.capture("restored-message")
			// Reproduce the user's mixed inline command example without sending it.
			restarted.act("input", "setValue", "你好 /in")
			restarted.act("input", "focus", "")
			restarted.act("prompt-choice:command:init", "press", "")
			restarted.await("inline Init inserted", func() error {
				tree, err := restarted.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if node.Value != "你好 \ufffc " {
					return fmt.Errorf("inline Init missing: %q", node.Value)
				}
				return nil
			})
			if err := restarted.client.call("act", map[string]any{"key": "input", "action": "insertText", "text": "/go"}, nil); err != nil {
				t.Fatal(err)
			}
			restarted.act("prompt-choice:command:goal", "press", "")
			restarted.await("mixed inline commands inserted", func() error {
				tree, err := restarted.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if node.Value != "你好 \ufffc \ufffc " {
					return fmt.Errorf("mixed command references missing: %q", node.Value)
				}
				return nil
			})
			restarted.capture("init-goal-inline")
		})
	}
}

func (app *nativeApp) assertPromptAbove() {
	app.t.Helper()
	app.await("prompt picker remains above the composer", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		panel, err := tree.findKey("prompt-picker")
		if err != nil {
			return err
		}
		input, err := tree.findKey("input")
		if err != nil {
			return err
		}
		if panel.Bounds.Height <= 0 || panel.Bounds.Y < 0 || panel.Bounds.Y+panel.Bounds.Height > input.Bounds.Y {
			return fmt.Errorf("picker is not above input: picker=%+v input=%+v", panel.Bounds, input.Bounds)
		}
		return nil
	})
}

// The goal and todo display state is a persisted fixture. This verifies native
// overlay geometry and input routing, not goal execution or a provider.
func TestDesktopPromptPickerOverGoalTodos(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
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
			ctx := context.Background()
			if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Overlay fixture"}}); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := core.Snapshot(ctx)
			if err := core.Execute(ctx, contracts.Command{Kind: "session.create", ProjectID: snapshot.Projects[0].ID, Name: "Goal and todos"}); err != nil {
				t.Fatal(err)
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			// This case tests overlay geometry, not command editing. Seed its
			// display catalog once while the private host is stopped, avoiding
			// concurrent catalog reads during repeated Windows file replacement.
			catalog := struct {
				Commands []contracts.UserCommand `json:"commands"`
			}{}
			for i := range 20 {
				id := fmt.Sprintf("command-%02d", i)
				catalog.Commands = append(catalog.Commands, contracts.UserCommand{ID: id, Name: id, Content: "Unused overlay fixture", Enabled: true})
			}
			if err := storage.WriteJSON(filepath.Join(data, "user-resources.json"), catalog); err != nil {
				t.Fatal(err)
			}
			// Goal.set starts a model run. This geometry fixture instead edits
			// only its stopped host's display record and commits its checksum.
			session := snapshot.Sessions[0]
			editFixtureConversation(t, data, session.ID, func(saved *contracts.Session, messages *[]contracts.Message) {
				saved.Goal = &contracts.SessionGoal{
					ID: "overlay-goal", Status: contracts.GoalPaused, Iteration: 1,
					Objective: "Keep the command picker above the composer.\nCover the existing goal details.\nCover the existing todo list.",
					Reason:    "Goal details behind the picker.\nThis content must be occluded.\nThe picker must not move it.",
				}
				session.Goal = saved.Goal
				*messages = []contracts.Message{{
					ID: "overlay-todos", SessionID: session.ID, Role: "assistant",
					Blocks: []contracts.Block{{ID: "todo-block", Kind: "todos", Todos: []contracts.Todo{
						{Text: "Todo behind popup 1", Status: "pending"}, {Text: "Todo behind popup 2", Status: "pending"},
					}}},
				}}
			})
			app := startApp(t, binary, artifacts, "overlay", data, "--local", "--appearance", appearance, "--width", "900", "--height", "600")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+session.ID, "press", "")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("attached panels stay inside composer controls", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				attach, err := tree.findKey("attach")
				if err != nil {
					return err
				}
				send, err := tree.findKey("send")
				if err != nil {
					return err
				}
				for _, key := range []string{"composer-goal:" + session.ID, "todo-panel:main:" + session.ID} {
					panel, err := tree.findKey(key)
					if err != nil {
						return err
					}
					if panel.Bounds.X < attach.Bounds.X-2 || panel.Bounds.X+panel.Bounds.Width > send.Bounds.X+send.Bounds.Width+2 {
						return fmt.Errorf("attached panel exceeds composer controls: panel=%+v attach=%+v send=%+v", panel.Bounds, attach.Bounds, send.Bounds)
					}
				}
				return nil
			})
			app.capture("goal-and-todos")
			app.act("input", "focus", "")
			app.act("input", "setValue", "/")
			app.waitKey("prompt-choice:command:init")
			app.assertPromptAbove()
			app.await("popup overlaps goal details and todos", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				panel, err := tree.findKey("prompt-picker")
				if err != nil {
					return err
				}
				overlaps := func(node accessNode) bool {
					a, b := panel.Bounds, node.Bounds
					return a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
				}
				goalNode, goalErr := tree.findKey("composer-goal:" + session.ID)
				todoNode, todoErr := tree.findKey("todo-panel:main:" + session.ID)
				goal, todo := goalErr == nil && overlaps(goalNode), todoErr == nil && overlaps(todoNode)
				if !goal || !todo {
					return fmt.Errorf("popup does not overlap both regions: goal=%t todo=%t, panel=%+v", goal, todo, panel.Bounds)
				}
				return nil
			})
			app.capture("picker-over-goal-and-todos")
			app.act("prompt-choice:command:init", "press", "")
			app.await("occluding menu remains selectable", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if node.Value != "\ufffc " {
					return fmt.Errorf("candidate did not reach composer: %q", node.Value)
				}
				return nil
			})
		})
	}
}
