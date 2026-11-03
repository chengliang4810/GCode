//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The native app reopens a real failed run, then receives its first usage data
// through the real local core. Only the model endpoint is an HTTP fixture.
func TestDesktopLocalConversationMetricsVisibility(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var requests atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					return
				}
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"First useful metrics\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20}}\n\n")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			// Close the app and unblock any handler before closing this server.
			t.Cleanup(provider.Close)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx, cancel := context.WithTimeout(context.Background(), stateTimeout)
			defer cancel()
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "metrics", Name: "Local fixture", Protocol: model.Chat, BaseURL: provider.URL,
					Models: []model.Model{{ID: "model"}},
				}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Metrics visibility"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				ProjectID: snapshot.Projects[0].ID, ModelID: "metrics/model", Text: "Fail without usage",
			}}); err != nil {
				t.Fatal(err)
			}
			var sessionID string
			for {
				snapshot, err = core.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Runs) == 1 && snapshot.Runs[0].Status == "failed" {
					sessionID = snapshot.Runs[0].SessionID
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(20 * time.Millisecond):
				}
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "visibility", data, "--local",
				"--appearance", appearance, "--language", "en", "--width", "900", "--height", "700")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			assertHidden := func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == "Run failed" || node.Label == "运行失败" {
						return fmt.Errorf("redundant failure footer remains")
					}
				}
				if metric, err := tree.findKey("conversation-metrics:" + sessionID); err == nil {
					return fmt.Errorf("unknown conversation metrics are visible: %s", metric.Label)
				}
				if !activityTreeContains(tree, "model request failed (HTTP 405)") {
					return fmt.Errorf("transcript failure detail is missing")
				}
				return nil
			}
			app.await("failed run retains transcript detail with no redundant footer or unknown metrics", assertHidden)
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("failed-without-metrics")
			app.act("input", "setValue", "Return the first useful metrics")
			app.act("send", "press", "")
			select {
			case <-started:
			case <-time.After(stateTimeout):
				t.Fatal("native send did not reach the model fixture")
			}
			if err := assertHidden(); err != nil {
				t.Fatal(err)
			}
			app.capture("waiting-without-metrics")
			release <- struct{}{}
			app.await("first useful data reveals conversation metrics", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				metric, err := tree.findKey("conversation-metrics:" + sessionID)
				if err != nil {
					return err
				}
				if !strings.HasPrefix(metric.Label, "≥120 tokens · ") {
					return fmt.Errorf("first reported usage is missing: %s", metric.Label)
				}
				send, err := tree.findKey("send")
				if err != nil || send.Label != "Send message" {
					return fmt.Errorf("response has not completed: %s (%v)", send.Label, err)
				}
				return nil
			})
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("first-useful-metrics")
		})
	}
}
