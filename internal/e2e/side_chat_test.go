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
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Native controls and the real local core; model responses come from a local fixture.
func TestDesktopLocalSideChat(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			requests := make(chan string, 8)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				requests <- string(body)
				reply := "PARENT ANSWER"
				if strings.Contains(string(body), "SIDE QUESTION") {
					reply = "SIDE ANSWER"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", reply)
			}))
			defer provider.Close()
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
			defer core.Close()
			ctx := context.Background()
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "side", Name: "Local fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "test"}}}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Side chat fixture"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := core.Snapshot(ctx)
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: "parent-seed", ProjectID: snapshot.Projects[0].ID, ModelID: "side/test", Text: "PARENT CONTEXT",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			var parent contracts.Session
			for {
				snapshot, err = core.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle && snapshot.Sessions[0].LastOutcome == "completed" {
					parent = snapshot.Sessions[0]
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("parent fixture did not complete")
				}
				time.Sleep(20 * time.Millisecond)
			}
			snapshot, err = core.Snapshot(ctx, parent.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			parentCount := len(snapshot.Messages[parent.ID])
			<-requests
			language := "en"
			if appearance == "dark" {
				language = "zh-CN"
			}
			app := startApp(t, binary, artifacts, "side-chat", data, "--local", "--appearance", appearance, "--language", language, "--width", "900", "--height", "600")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+parent.ID, "press", "")
			app.waitKey("titlebar-session-info")
			app.act("input", "setValue", "Keep parent draft")
			app.await("parent draft is committed before opening another pane", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if input.Value != "Keep parent draft" {
					return fmt.Errorf("parent draft not committed: %q", input.Value)
				}
				return nil
			})
			app.act("titlebar-session-info", "press", "")
			app.await("session information has no side-chat creation action", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == "New side chat" || node.Label == "新建分支聊天" {
						return fmt.Errorf("removed action still visible")
					}
				}
				return nil
			})
			app.act("titlebar-right-pane", "press", "")
			app.waitKey("side-chat-launcher")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("launcher")
			app.act("side-chat-launcher", "press", "")
			var child string
			readSnapshot := func() (contracts.Snapshot, error) {
				return readFixtureSnapshot(data)
			}
			app.await("independent side chat persisted with empty visible history", func() error {
				current, err := readSnapshot()
				if err != nil {
					return err
				}
				for _, session := range current.Sessions {
					if session.ParentSessionID == parent.ID && session.SideChat {
						child = session.ID
						if len(current.Messages[child]) != 0 {
							return fmt.Errorf("inherited history leaked into the side transcript")
						}
						return nil
					}
				}
				return fmt.Errorf("no side session")
			})
			app.waitKey("side:" + child + ":input")
			app.capture("empty-side-chat")
			app.act("side:"+child+":input", "setValue", "SIDE QUESTION")
			app.act("side:"+child+":send", "press", "")
			var received string
			app.await("provider request includes the parent's context and the side question", func() error {
				if received == "" {
					select {
					case received = <-requests:
					default:
						return fmt.Errorf("provider has not received the request")
					}
				}
				for _, text := range []string{"PARENT CONTEXT", "PARENT ANSWER", "SIDE QUESTION"} {
					if !strings.Contains(received, text) {
						return fmt.Errorf("side request lacks %q", text)
					}
				}
				return nil
			})
			app.await("side response completes without changing parent history or draft", func() error {
				current, err := readSnapshot()
				if err != nil {
					return err
				}
				if len(current.Messages[parent.ID]) != parentCount {
					return fmt.Errorf("parent history changed")
				}
				done := false
				for _, session := range current.Sessions {
					done = done || session.ID == child && session.Status == contracts.Idle && session.LastOutcome == "completed"
				}
				if !done || len(current.Messages[child]) != 2 {
					return fmt.Errorf("side response has not completed")
				}
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if input.Value != "Keep parent draft" {
					return fmt.Errorf("parent draft changed: %q", input.Value)
				}
				return nil
			})
			app.capture("side-response")
			app.stop()
		})
	}
}
