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
	"regexp"
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
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
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
				requestID := "usage-" + strings.ReplaceAll(modelID, "/", "-")
				if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					RequestID: requestID, SessionID: sessionID, ProjectID: snapshot.Projects[0].ID, ModelID: modelID, Text: "Record usage",
				}}); err != nil {
					t.Fatal(err)
				}
				if sessionID == "" {
					snapshot, err = core.Snapshot(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if len(snapshot.Sessions) != 1 {
						t.Fatalf("usage fixture created %d sessions, want 1", len(snapshot.Sessions))
					}
					sessionID = snapshot.Sessions[0].ID
				}
				for {
					snapshot, err = core.Snapshot(ctx, sessionID)
					if err != nil {
						t.Fatal(err)
					}
					var session contracts.Session
					for _, candidate := range snapshot.Sessions {
						if candidate.ID == sessionID {
							session = candidate
							break
						}
					}
					turnCompleted := false
					for _, run := range snapshot.Runs {
						if run.SessionID == sessionID && run.RequestID == requestID {
							turnCompleted = run.Status == "completed"
							break
						}
					}
					if session.ID != "" && session.Status == contracts.Idle && session.LastOutcome == "completed" && turnCompleted {
						break
					}
					if ctx.Err() != nil {
						t.Fatal(ctx.Err())
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			snapshot, err = core.Snapshot(ctx, sessionID)
			if err != nil {
				t.Fatal(err)
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
			scope := "conversation-metrics:" + sessionID
			total := app.waitKey(scope + ":usage")
			if !strings.Contains(total.Label, "240 Token") {
				t.Fatalf("conversation totals/speed lost on restart: %s", total.Label)
			}
			if !strings.Contains(total.Label, "40.0%") {
				t.Fatalf("unreported cache diluted the hit rate after restart: %s", total.Label)
			}
			closeMetrics := func(panel string) {
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Escape"}}}, nil); err != nil {
					t.Fatal(err)
				}
				app.await("metrics panel closes with Escape", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					if _, err := tree.findKey(panel); err == nil {
						return fmt.Errorf("panel remains open")
					}
					return nil
				})
			}
			app.hoverMetric(scope + ":usage")
			app.waitKey(scope + ":usage-panel")
			if label := app.waitKey(scope + ":cache-hit").Label; !strings.Contains(label, "40.0%") {
				t.Fatal(label)
			}
			app.assertNoMetricCoverage()
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("conversation-usage-panel")
			closeMetrics(scope + ":usage-panel")
			timeLabel := app.waitKey(scope + ":time").Label
			if matched, err := regexp.MatchString(` (\d+d )?(\d+h )?(\d+m )?\d+s$`, timeLabel); err != nil || !matched {
				t.Fatalf("footer must show elapsed whole seconds: %s (%v)", timeLabel, err)
			}
			app.hoverMetric(scope + ":time")
			app.waitKey(scope + ":time-panel")
			if strings.Contains(app.waitKey(scope+":first-token").Label, "—") || strings.Contains(app.waitKey(scope+":model-time").Label, "—") {
				t.Fatal("actual model timing was not persisted")
			}
			app.assertNoMetricCoverage()
			app.capture("conversation-time-panel")
			closeMetrics(scope + ":time-panel")
			var lastReply string
			for _, message := range snapshot.Messages[sessionID] {
				if message.Role != "assistant" {
					continue
				}
				replyScope := "reply-metrics:" + message.ID
				node := app.waitKey(replyScope + ":usage")
				wantModel := strings.TrimPrefix(message.ModelID, "usage/")
				if !strings.Contains(node.Label, "120 Token") {
					t.Fatalf("reply model identity or usage is wrong: %s", node.Label)
				}
				expectedCache := "40.0%"
				if wantModel == "beta" {
					expectedCache = "—"
				}
				app.hoverMetric(replyScope + ":usage")
				app.waitKey(replyScope + ":usage-panel")
				if label := app.waitKey(replyScope + ":model").Label; !strings.Contains(label, "Local fixture · "+wantModel) {
					t.Fatalf("saved provider/model missing: %s", label)
				}
				if label := app.waitKey(replyScope + ":cache-hit").Label; !strings.Contains(label, expectedCache) {
					t.Fatalf("reply treated unreported cache as zero: %s", label)
				}
				if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
					t.Fatal(err)
				}
				app.capture("reply-" + wantModel + "-panel")
				closeMetrics(replyScope + ":usage-panel")
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
			if copy.Bounds.X+copy.Bounds.Width > node.Bounds.X+1 || node.Bounds.X+node.Bounds.Width > float64(windowWidth)+1 {
				t.Fatalf("reply metrics overlap copy or leave narrow window: copy=%+v metrics=%+v", copy.Bounds, node.Bounds)
			}
			app.capture("conversation-metrics")
			app.act("sidebar-usage", "press", "")
			if got := app.waitKey("usage-total").Label; got != "240" {
				t.Fatalf("persisted/forked usage total=%s, want 240", got)
			}
			for i, modelName := range []string{"Local fixture/alpha", "Local fixture/beta"} {
				label := app.waitKey(fmt.Sprintf("usage-model-%d", i)).Label
				if !strings.Contains(label, modelName) || !strings.Contains(label, "120 Tokens") || !strings.Contains(label, "50.0%") {
					t.Fatalf("actual request model split is wrong: %s", label)
				}
			}
			hint := app.waitKey("usage-value-" + time.Now().Format("2006-01-02")).Label
			if !strings.Contains(hint, "Local fixture/alpha · 120") || !strings.Contains(hint, "Local fixture/beta · 120") {
				t.Fatalf("usage trend tooltip lost provider/model names: %s", hint)
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
