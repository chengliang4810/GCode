//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The local core executes real reads while an HTTP model fixture keeps the
// same turn active. Scrolling must not replay its already completed narration.
func TestDesktopLocalHistoryTextDoesNotReplay(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte("fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			paragraph := func(i int) string {
				return fmt.Sprintf("HISTORY %02d: 已完成的历史文字应该立即完整显示。滚动查看旧消息时，不应重新逐字渐显，也不应因为任务仍在运行而再次播放动画。", i)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(delta map[string]any, finish string) {
					payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
					fmt.Fprintf(w, "data: %s\n\n", payload)
				}
				n := int(calls.Add(1))
				switch {
				case n == 1:
					send(map[string]any{"content": "Seed ready."}, "stop")
				case n <= 17:
					send(map[string]any{"content": paragraph(n - 2)}, "")
					send(map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": fmt.Sprintf("read-%d", n),
						"function": map[string]any{"name": "Read", "arguments": `{"file_path":"fixture.txt"}`},
					}}}, "tool_calls")
				default:
					send(map[string]any{"reasoning_content": "The native fixture is waiting while history is inspected."}, "")
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = core.Close() })
			snapshot, _ := core.Snapshot(t.Context())
			settings := snapshot.Settings
			settings.General.CompactMode = false
			settings.Appearance.ReduceMotion = false
			settings.Appearance.MotionPreference = "full"
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "fixture", Name: "History reveal fixture", Protocol: model.Chat, BaseURL: server.URL,
					Models: []model.Model{{ID: "test"}},
				}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "History reveal"}},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				ProjectID: snapshot.Projects[0].ID, ModelID: "fixture/test", Text: "Seed the history fixture.",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			for {
				snapshot, err = core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("seed did not finish")
				}
				time.Sleep(20 * time.Millisecond)
			}
			sessionID := snapshot.Sessions[0].ID
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "history", data, "--local", "--appearance", appearance, "--width", "1100", "--height", "780")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.await("selected conversation body is loaded", func() error {
				tree, err := app.client.tree()
				if err != nil || !activityTreeContains(tree, "Seed ready.") {
					return fmt.Errorf("seed body is not ready: %v", err)
				}
				return nil
			})
			if err := app.client.call("idle", map[string]any{"timeout": 5000}, nil); err != nil {
				t.Fatal(err)
			}
			app.act("input", "setValue", "Run the history reveal fixture.")
			app.act("send", "press", "")
			app.await("all historical model rounds completed while the turn stays active", func() error {
				if calls.Load() < 18 {
					return fmt.Errorf("model requests=%d", calls.Load())
				}
				return nil
			})
			wheel := func(dy float64) {
				t.Helper()
				if err := app.client.call("input", map[string]any{"events": []map[string]any{
					{"type": "wheel", "x": 650, "y": 250, "dy": dy, "precise": true},
				}}, nil); err != nil {
					t.Fatal(err)
				}
			}
			for pass := range 3 {
				wheel(-100000)
				// Wait for the scroll position, not the full text: waiting for
				// the complete paragraph would conceal a replaying animation.
				app.await("history row enters the viewport", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					if !activityTreeContains(tree, "HISTORY 00") {
						return fmt.Errorf("first history row is not mounted")
					}
					return nil
				})
				tree, err := app.client.tree()
				if err != nil || !activityTreeContains(tree, paragraph(0)) {
					t.Fatalf("scroll pass %d replayed historical text: %v", pass, err)
				}
				app.capture(fmt.Sprintf("history-pass-%d", pass))
				wheel(100000)
			}
		})
	}
}
