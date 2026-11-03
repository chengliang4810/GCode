//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
				var request struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				cache := ""
				if request.Model == "alpha" {
					cache = `,"prompt_tokens_details":{"cached_tokens":40}`
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Usage fixture answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20%s}}\n\n", cache)
				w.(http.Flusher).Flush()
				time.Sleep(25 * time.Millisecond) // simulated decode, independently of native UI timing
				fmt.Fprint(w, "data: [DONE]\n\n")
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
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			total := app.waitKey("conversation-metrics:" + sessionID)
			if !strings.HasPrefix(total.Label, "240 tokens · ") || strings.Contains(total.Label, "— tokens/s") {
				t.Fatalf("conversation totals/speed lost on restart: %s", total.Label)
			}
			wantCache, noCache, coverage := "40.0% cache", "— cache", "Cache data available for 1/2 requests."
			if language == "zh-CN" {
				wantCache, noCache, coverage = "缓存 40.0%", "缓存 —", "1/2 次请求有可用的缓存数据。"
			}
			if !strings.Contains(total.Label, wantCache) || !strings.Contains(total.Label, coverage) {
				t.Fatalf("unreported cache diluted the hit rate after restart: %s", total.Label)
			}
			var lastReply string
			for _, message := range snapshot.Messages[sessionID] {
				if message.Role != "assistant" {
					continue
				}
				node := app.waitKey("reply-metrics:" + message.ID)
				wantModel := strings.TrimPrefix(message.ModelID, "usage/")
				if !strings.HasPrefix(node.Label, "120 tokens · ") || !strings.Contains(node.Label, "Local fixture / "+wantModel) {
					t.Fatalf("reply model identity or usage is wrong: %s", node.Label)
				}
				expectedCache := wantCache
				if wantModel == "beta" {
					expectedCache = noCache
				}
				if !strings.Contains(node.Label, expectedCache) {
					t.Fatalf("reply treated unreported cache as zero: %s", node.Label)
				}
				lastReply = message.ID
			}
			node := app.waitKey("reply-metrics:" + lastReply)
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "pointerMove", "x": node.Bounds.X + 5, "y": node.Bounds.Y + node.Bounds.Height/2,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			copy := app.waitKey("copy-message:" + lastReply)
			windowWidth, _ := strconv.Atoi(width)
			if node.Bounds.X < copy.Bounds.X+copy.Bounds.Width-1 || node.Bounds.X+node.Bounds.Width > float64(windowWidth)+1 {
				t.Fatalf("reply metrics overlap copy or leave narrow window: copy=%+v metrics=%+v", copy.Bounds, node.Bounds)
			}
			app.capture("conversation-metrics")
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
