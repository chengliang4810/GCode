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
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Real Go core, request parsing, model switching, forks and persisted records.
// Only the remote model endpoint is a local HTTP fixture.
func TestDesktopLocalApplicationUsage(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Usage fixture answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":40}}}\n\ndata: [DONE]\n\n")
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
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx, cancel := context.WithTimeout(context.Background(), stateTimeout)
			defer cancel()
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "usage", Name: "Local fixture", Protocol: model.Chat, BaseURL: provider.URL,
					Models: []model.Model{{ID: "alpha"}, {ID: "beta"}},
				}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Usage fixture"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := core.Snapshot(ctx)
			var sessionID string
			for _, modelID := range []string{"usage/alpha", "usage/beta"} {
				if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					SessionID: sessionID, ProjectID: snapshot.Projects[0].ID, ModelID: modelID, Text: "Record usage",
				}}); err != nil {
					t.Fatal(err)
				}
				for {
					snapshot, err = core.Snapshot(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if len(snapshot.Runs) > 0 && snapshot.Runs[len(snapshot.Runs)-1].Status == "completed" {
						sessionID = snapshot.Sessions[0].ID
						break
					}
					if ctx.Err() != nil {
						t.Fatal(ctx.Err())
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: sessionID}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			language, width := "en", "1320"
			if appearance == "dark" {
				language, width = "zh-CN", "900"
			}
			app := startApp(t, binary, artifacts, "usage", data, "--local", "--language", language,
				"--appearance", appearance, "--width", width, "--height", "1100")
			app.act("sidebar-usage", "press", "")
			if got := app.waitKey("usage-total").Label; got != "240" {
				t.Fatalf("persisted/forked usage total=%s, want 240", got)
			}
			for i, modelID := range []string{"usage/alpha", "usage/beta"} {
				label := app.waitKey(fmt.Sprintf("usage-model-%d", i)).Label
				if !strings.Contains(label, modelID) || !strings.Contains(label, "120 Tokens") || !strings.Contains(label, "50.0%") {
					t.Fatalf("actual request model split is wrong: %s", label)
				}
			}
			app.capture("usage-7d")
			app.act("usage-range-30", "press", "")
			app.act("usage-heatmap-weekly", "press", "")
			app.act("usage-heatmap-cumulative", "press", "")
			app.act("usage-heatmap-daily", "press", "")
			app.act("usage-refresh", "press", "")
			app.await("refresh retains the exact recorded total", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				total, err := tree.findKey("usage-total")
				if err != nil {
					return err
				}
				if total.Label != "240" {
					return fmt.Errorf("usage total=%s", total.Label)
				}
				refresh, err := tree.findKey("usage-refresh")
				if err != nil {
					return err
				}
				for _, state := range refresh.States {
					if state == "disabled" {
						return fmt.Errorf("usage refresh is still running")
					}
				}
				return nil
			})
			app.capture("usage-30d")
		})
	}
}
