//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Native rendering and the real local core. The private model endpoint holds
// its response so the user bubble can be checked independently of model output.
// The pre-acceptance frame is covered separately by ui.Tester with dispatch held.
func TestDesktopLocalMessageSendFeedback(t *testing.T) {
	for _, test := range []struct {
		appearance, width string
		existing          bool
	}{{"light", "1320", false}, {"dark", "900", true}} {
		t.Run(test.appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "profile")
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"MESSAGE ACCEPTED\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer provider.Close()
			defer unblock()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{
				Provider: model.Provider{ID: "feedback", Name: "Send fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "test"}}},
			}}); err != nil {
				t.Fatal(err)
			}
			sessionID := ""
			if test.existing {
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Send feedback"}); err != nil {
					t.Fatal(err)
				}
				snapshot, err := core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				sessionID = snapshot.Sessions[0].ID
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startAppReady(t, binary, artifacts, "send-feedback", data, "back-to-conversations",
				"--local", "--appearance", test.appearance, "--language", "zh-CN", "--width", test.width, "--height", "880")
			app.act("back-to-conversations", "press", "")
			if test.existing {
				app.act("sidebar-session:"+sessionID, "press", "")
				app.await("existing conversation is selected", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					title, err := tree.findKey("conversation-title")
					if err != nil || title.Label != "Send feedback" {
						return fmt.Errorf("conversation selection has not settled: %q (%v)", title.Label, err)
					}
					return nil
				})
			} else {
				app.act("sidebar-new-conversation", "press", "")
			}
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			const prompt = "立即显示本条用户消息"
			app.act("input", "setValue", prompt)
			app.await("draft is committed to the selected conversation", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != prompt {
					return fmt.Errorf("draft = %q (%v)", input.Value, err)
				}
				return nil
			})
			app.act("send", "press", "")
			app.await("one user bubble while the model is blocked", func() error {
				if calls.Load() != 1 {
					return fmt.Errorf("model calls = %d", calls.Load())
				}
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != "" {
					return fmt.Errorf("composer has not cleared: %q (%v)", input.Value, err)
				}
				count := 0
				for _, node := range tree.Nodes {
					if node.Role == "listItem" && node.Label == prompt && !strings.Contains(string(node.Key), "sidebar-") {
						count++
					}
				}
				if count != 1 {
					return fmt.Errorf("user bubble count = %d", count)
				}
				return nil
			})
			app.capture("user-message-before-model-response")
			unblock()
			app.await("accepted message is persisted once with its request identity", func() error {
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != contracts.Idle {
					return fmt.Errorf("conversation has not finished")
				}
				sessionID = snapshot.Sessions[0].ID
				count := 0
				for _, message := range snapshot.Messages[sessionID] {
					if message.Role == "user" {
						count++
						if !strings.HasPrefix(message.RequestID, "primary:") {
							return fmt.Errorf("user message lost request identity")
						}
					}
				}
				if count != 1 {
					return fmt.Errorf("persisted user messages = %d", count)
				}
				return nil
			})
			// With no active goal, this command fails before any model call.
			app.await("composer reflects the completed run", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				send, err := tree.findKey("send")
				if err != nil || send.Label != "发送消息" {
					return fmt.Errorf("composer still reflects the active run: %q (%v)", send.Label, err)
				}
				return nil
			})
			app.act("input", "setValue", "/goal pause")
			app.await("control command is committed before sending", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != "/goal pause" {
					return fmt.Errorf("command draft is not committed: %q (%v)", input.Value, err)
				}
				return nil
			})
			app.act("send", "press", "")
			retry := app.awaitKeyPrefix("retry-send:")
			app.act("input", "setValue", "保留发送后新输入的草稿")
			app.act(retry, "press", "")
			app.await("failed retry keeps the new draft and the same failure row", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey(retry); err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != "保留发送后新输入的草稿" {
					return fmt.Errorf("retry replaced the newer draft: %q (%v)", input.Value, err)
				}
				return nil
			})
			app.capture("failed-send-retry-preserves-draft")
			if calls.Load() != 1 {
				t.Fatal("failed control command unexpectedly reached the model")
			}
		})
	}
}
