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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Seven real model rounds and six real local file reads; only the provider
// stream is synthetic. Missing cache fields in two rounds must remain missing.
func TestDesktopLocalConversationMetricsPanels(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				step := int(calls.Add(1))
				input, output := 11765, 0
				if step == 6 {
					input = 16487
				}
				if step == 7 {
					input, output = 16488, 1600
				}
				usage := map[string]any{"prompt_tokens": input, "completion_tokens": output}
				if step <= 5 {
					usage["prompt_tokens_details"] = map[string]any{"cached_tokens": 4000}
				}
				delta, finish := map[string]any{"content": "Detailed metrics answer"}, "stop"
				if step < 7 {
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("read-%d", step),
						"function": map[string]any{"name": "Read", "arguments": `{"file_path":"metrics.txt"}`}}}}
					finish = "tool_calls"
				}
				payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}, "usage": usage})
				w.Header().Set("Content-Type", "text/event-stream")
				time.Sleep(20 * time.Millisecond)
				fmt.Fprintf(w, "data: %s\n\n", payload)
				w.(http.Flusher).Flush()
				time.Sleep(15 * time.Millisecond)
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer provider.Close()
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "metrics.txt"), []byte("Metrics fixture\n"), 0600); err != nil {
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
			ctx := t.Context()
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "metrics", Name: "Metrics fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Detailed metrics"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := core.Snapshot(ctx)
			requestID := "metrics-full"
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{RequestID: requestID, ProjectID: snapshot.Projects[0].ID, ModelID: "metrics/model", Text: "Show full metrics"}}); err != nil {
				t.Fatal(err)
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Sessions) != 1 {
				t.Fatalf("metrics fixture created %d sessions, want 1", len(snapshot.Sessions))
			}
			sessionID := snapshot.Sessions[0].ID
			var session contracts.Session
			deadline := time.Now().Add(stateTimeout)
			for {
				snapshot, err = core.Snapshot(ctx, sessionID)
				if err != nil {
					t.Fatal(err)
				}
				session = contracts.Session{}
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
				if session.Status == contracts.Idle && session.LastOutcome == "completed" && turnCompleted {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("metrics run did not settle")
				}
				time.Sleep(15 * time.Millisecond)
			}
			if calls.Load() != 7 || session.LastOutcome != "completed" {
				t.Fatalf("tool loop failed: outcome=%s calls=%d runs=%+v", session.LastOutcome, calls.Load(), snapshot.Runs)
			}
			replyID := ""
			snapshot, err = core.Snapshot(ctx, sessionID)
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range snapshot.Messages[sessionID] {
				if message.Role == "assistant" {
					replyID = message.ID
					break
				}
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "panels", data, "--local", "--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			scope := "reply-metrics:" + replyID
			timing := app.waitKey(scope + ":time")
			usageTrigger := app.waitKey(scope + ":usage")
			var wallDuration, generationDuration time.Duration
			for _, run := range snapshot.Runs {
				if run.SessionID == sessionID && run.RequestID == requestID && run.FinishedAt != nil {
					wallDuration = run.FinishedAt.Sub(run.StartedAt)
				}
			}
			for _, request := range snapshot.ModelRequests {
				generationDuration += request.DecodeDuration
			}
			if want := fmt.Sprintf("用时 %.2fs", wallDuration.Seconds()); wallDuration <= 0 || !strings.Contains(timing.Label, want) {
				t.Fatalf("reply does not show the turn's wall time: %s; want %s", timing.Label, want)
			}
			copyButton := app.waitKey("copy-message:" + replyID)
			dateText := app.waitKey("message-date:" + replyID)
			tree, err := app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			var body, disclosure accessNode
			for _, node := range tree.Nodes {
				var key string
				_ = json.Unmarshal(node.Key, &key)
				switch {
				case node.Label == "Detailed metrics answer":
					body = node
				case strings.HasPrefix(key, "activity-toggle:"):
					disclosure = node
				}
			}
			if body.ID == "" || disclosure.ID == "" ||
				disclosure.Bounds.X+10 != body.Bounds.X || copyButton.Bounds.X != body.Bounds.X {
				t.Fatalf("tool disclosure/copy must align with body: body=%+v disclosure=%+v copy=%+v",
					body.Bounds, disclosure.Bounds, copyButton.Bounds)
			}
			if copyButton.Bounds.X+copyButton.Bounds.Width > usageTrigger.Bounds.X ||
				usageTrigger.Bounds.X+usageTrigger.Bounds.Width > timing.Bounds.X ||
				timing.Bounds.X+timing.Bounds.Width > dateText.Bounds.X {
				t.Fatalf("reply footer must show copy, usage, time, date: copy=%+v usage=%+v time=%+v date=%+v",
					copyButton.Bounds, usageTrigger.Bounds, timing.Bounds, dateText.Bounds)
			}
			app.hoverMetric(scope + ":usage")
			panel := app.waitKey(scope + ":usage-panel")
			trigger := app.waitKey(scope + ":usage")
			for key, want := range map[string]string{"input": "9.2万 Token", "cached": "2万 Token", "output": "1.6千 Token", "cache-hit": "34.0%", "model": "Metrics fixture · model"} {
				if got := app.waitKey(scope + ":" + key).Label; !strings.Contains(got, want) {
					t.Fatalf("%s: %s; want %s", key, got, want)
				}
			}
			app.assertNoMetricCoverage()
			if panel.Bounds.X < 0 || panel.Bounds.X+panel.Bounds.Width > 901 || panel.Bounds.Y < 0 || panel.Bounds.Y+panel.Bounds.Height > 881 {
				t.Fatalf("panel leaves native window: %+v", panel.Bounds)
			}
			if panel.Bounds.X < trigger.Bounds.X-1 {
				t.Fatalf("reply panel escaped into the sidebar: trigger=%+v panel=%+v", trigger.Bounds, panel.Bounds)
			}
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("reply-usage-panel")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "pointerMove", "x": panel.Bounds.X + 2, "y": panel.Bounds.Y + 2,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.waitKey(scope + ":usage-panel")
			app.hoverMetric(scope + ":time")
			app.waitKey(scope + ":time-panel")
			app.waitKey(scope + ":output-speed")
			if label := app.waitKey(scope + ":decode-time").Label; !strings.Contains(label, fmt.Sprintf("%.2fs", generationDuration.Seconds())) {
				t.Fatalf("reply time panel does not show the observed generation time: %s", label)
			}
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("reply-time-panel")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Escape"}}}, nil); err != nil {
				t.Fatal(err)
			}
			scope = "conversation-metrics:" + sessionID
			conversationTime := app.waitKey(scope + ":time")
			conversationUsage := app.waitKey(scope + ":usage")
			if conversationTime.Bounds.X+conversationTime.Bounds.Width > conversationUsage.Bounds.X {
				t.Fatalf("composer footer must show time before usage: time=%+v usage=%+v",
					conversationTime.Bounds, conversationUsage.Bounds)
			}
			if got := app.waitKey(scope + ":time").Label; !strings.Contains(got, "1 轮 7 步") || !strings.Contains(got, "用时") {
				t.Fatal(got)
			}
			app.hoverMetric(scope + ":time")
			app.waitKey(scope + ":time-panel")
			for _, key := range []string{"model-time", "tool-time", "first-token", "decode-time", "output-speed"} {
				if label := app.waitKey(scope + ":" + key).Label; strings.Contains(label, "—") {
					t.Fatalf("missing observed timing: %s", label)
				}
			}
			app.assertNoMetricCoverage()
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("conversation-time-panel")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "pointerMove", "x": 20, "y": 80,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("leaving metrics closes the hover panel", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey(scope + ":time-panel"); err == nil {
					return fmt.Errorf("time panel remains open without hover")
				}
				return nil
			})
		})
	}
}

func (app *nativeApp) hoverMetric(key string) {
	app.t.Helper()
	node := app.waitKey(key)
	if err := app.client.call("input", map[string]any{"events": []map[string]any{{
		"type": "pointerMove", "x": node.Bounds.X + node.Bounds.Width/2, "y": node.Bounds.Y + node.Bounds.Height/2,
	}}}, nil); err != nil {
		app.t.Fatal(err)
	}
}

func (app *nativeApp) assertNoMetricCoverage() {
	app.t.Helper()
	tree, err := app.client.tree()
	if err != nil {
		app.t.Fatal(err)
	}
	for _, node := range tree.Nodes {
		for _, removed := range []string{
			"缓存计数覆盖", "速度计数覆盖", "首字计数覆盖", "速度不包含首字等待和工具执行时间。",
			"Cache count coverage", "Speed count coverage", "First-token count coverage",
			"Speed excludes first-token wait and tool execution.",
			"Only reported usage is counted; totals may be incomplete.", "仅统计已上报用量，总计可能不完整。",
		} {
			if strings.Contains(node.Label, removed) {
				app.t.Fatalf("metrics still show removed detail: %s", node.Label)
			}
		}
	}
}

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
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx, cancel := context.WithTimeout(context.Background(), stateTimeout)
			defer cancel()
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "metrics", Name: "Local fixture", Protocol: model.Chat, BaseURL: provider.URL,
					Models: []model.Model{{ID: "model", ContextWindow: 200000}},
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
			requestID := "metrics-failure"
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: requestID, ProjectID: snapshot.Projects[0].ID, ModelID: "metrics/model", Text: "Fail without usage",
			}}); err != nil {
				t.Fatal(err)
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Sessions) != 1 {
				t.Fatalf("metrics visibility fixture created %d sessions, want 1", len(snapshot.Sessions))
			}
			sessionID := snapshot.Sessions[0].ID
			var session contracts.Session
			for {
				snapshot, err = core.Snapshot(ctx, sessionID)
				if err != nil {
					t.Fatal(err)
				}
				session = contracts.Session{}
				for _, candidate := range snapshot.Sessions {
					if candidate.ID == sessionID {
						session = candidate
						break
					}
				}
				turnFailed := false
				for _, run := range snapshot.Runs {
					if run.SessionID == sessionID && run.RequestID == requestID {
						turnFailed = run.Status == "failed"
						break
					}
				}
				if session.Status == contracts.Errored && session.LastOutcome == "failed" && turnFailed {
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
			contextKey := "context-usage:" + sessionID
			unknownContext := app.waitKey(contextKey)
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
			if waitingContext := app.waitKey(contextKey); waitingContext.Bounds.X != unknownContext.Bounds.X || waitingContext.Bounds.Width != unknownContext.Bounds.Width {
				t.Fatalf("waiting request shifted context usage: before=%+v waiting=%+v", unknownContext.Bounds, waitingContext.Bounds)
			}
			app.capture("waiting-without-metrics")
			release <- struct{}{}
			app.await("first useful data reveals conversation metrics", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				metric, err := tree.findKey("conversation-metrics:" + sessionID + ":usage")
				if err != nil {
					return err
				}
				if !strings.Contains(metric.Label, "120 Token") || strings.Contains(metric.Label, "≥") {
					return fmt.Errorf("first reported usage is missing: %s", metric.Label)
				}
				send, err := tree.findKey("send")
				if err != nil || send.Label != "Send message" {
					return fmt.Errorf("response has not completed: %s (%v)", send.Label, err)
				}
				return nil
			})
			if reportedContext := app.waitKey(contextKey); reportedContext.Bounds.X != unknownContext.Bounds.X || reportedContext.Bounds.Width != unknownContext.Bounds.Width {
				t.Fatalf("reported metrics shifted context usage: before=%+v reported=%+v", unknownContext.Bounds, reportedContext.Bounds)
			}
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("first-useful-metrics")
			scope := "conversation-metrics:" + sessionID
			app.hoverMetric(scope + ":usage")
			app.waitKey(scope + ":usage-panel")
			app.assertNoMetricCoverage()
			app.capture("partial-usage-panel")
			if contextRing := app.waitKey(contextKey); contextRing.Bounds.Width != 28 {
				t.Fatalf("context entry still reserves visible percentage text: %+v", contextRing.Bounds)
			}
			app.hoverMetric(contextKey)
			var contextPanel accessNode
			contextPanelVisible := func(want bool) error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				visible := false
				for _, node := range tree.Nodes {
					if node.Label == "Context window menu" {
						contextPanel, visible = node, true
					}
					if node.Role == "tooltip" && strings.HasPrefix(node.Label, "Context usage:") {
						return fmt.Errorf("old percentage tooltip remains")
					}
				}
				if visible != want {
					return fmt.Errorf("context panel visible=%v want=%v", visible, want)
				}
				if want && !activityTreeContains(tree, "100/200,000 (0.1%)") {
					return fmt.Errorf("full context details missing")
				}
				return nil
			}
			app.await("ring hover opens full context details", func() error { return contextPanelVisible(true) })
			app.await("previous usage card closes after leaving its trigger", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey(scope + ":usage-panel"); err == nil {
					return fmt.Errorf("previous usage card still open")
				}
				return nil
			})
			app.capture("context-details-on-hover")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "pointerMove", "x": contextPanel.Bounds.X + 8, "y": contextPanel.Bounds.Y + 8,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("pointer can enter the context card", func() error { return contextPanelVisible(true) })
			app.hoverMetric(contextKey)
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "key", "key": "Escape",
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("Escape dismisses the hovered context card", func() error { return contextPanelVisible(false) })
		})
	}
}
