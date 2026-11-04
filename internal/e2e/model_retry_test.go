//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Exercises the production retry timer and native UI using an isolated local
// HTTP fixture. Retry-After: 0 makes each observed attempt deterministic.
func TestDesktopLocalModelRetry(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var calls atomic.Int32
			started := make(chan int32, 8)
			advance := make(chan struct{}, 8)
			shutdown := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := calls.Add(1)
				started <- attempt
				select {
				case <-advance:
				case <-r.Context().Done():
					return
				case <-shutdown:
					return
				}
				if attempt <= 3 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprint(w, `{"error":{"type":"server_error","message":"fixture busy"}}`)
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"RETRY_RECOVERED\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			defer close(shutdown)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "retry", Name: "Local retry fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "test"}},
				}}},
				{Kind: "session.create", Name: "Retry fixture"},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := core.Snapshot(t.Context())
			sessionID := snapshot.Sessions[0].ID
			modelID := "retry/test"
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", SessionID: sessionID,
				Session: &contracts.SessionUpdate{ID: sessionID, ModelID: &modelID}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "retry", data, "--local", "--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "600")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.waitKey("titlebar-session-info")
			app.act("input", "focus", "")
			app.act("input", "setValue", "retry this request")
			app.await("composer draft is committed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if input.Value != "retry this request" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
			app.act("send", "press", "")
			for attempt := int32(1); attempt <= 4; attempt++ {
				select {
				case actual := <-started:
					if actual != attempt {
						t.Fatalf("unexpected attempt %d, want %d", actual, attempt)
					}
				case <-time.After(stateTimeout):
					t.Fatal("next physical request did not start")
				}
				if attempt < 4 {
					tree, err := app.client.tree()
					if err != nil {
						t.Fatal(err)
					}
					if _, err := tree.findKey("model-retry:" + sessionID); err == nil {
						t.Fatal("retry counter appeared before third retry")
					}
					advance <- struct{}{}
				}
			}
			status := app.waitKey("model-retry:" + sessionID)
			if status.Label != "重新连接中... 3/10" {
				t.Fatalf("incorrect retry caption: %+v", status)
			}
			// A real pointer hover checks the HTTP status tooltip.
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "pointerMove", "x": status.Bounds.X + status.Bounds.Width/2, "y": status.Bounds.Y + status.Bounds.Height/2,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("HTTP retry tooltip", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Role == "tooltip" && node.Label == "重新连接中... 3/10 · HTTP 503" {
						return nil
					}
				}
				return fmt.Errorf("retry tooltip missing")
			})
			app.capture("third-retry")
			cancelRun := appearance == "dark"
			if cancelRun {
				app.act("send", "press", "") // Empty composer exposes Stop response.
			} else {
				advance <- struct{}{}
			}
			app.await("retry clears when the run settles", func() error {
				state, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if state.Sessions[0].Status != contracts.Idle || len(state.ModelRequests) != 4 {
					return fmt.Errorf("run still active or accounting incomplete")
				}
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("model-retry:" + sessionID); err == nil {
					return fmt.Errorf("retry status remains visible")
				}
				outcome := "completed"
				if cancelRun {
					outcome = "interrupted"
				}
				if state.Runs[0].Status != outcome || calls.Load() != 4 {
					return fmt.Errorf("outcome=%s calls=%d", state.Runs[0].Status, calls.Load())
				}
				if !cancelRun && !activityTreeContains(tree, "RETRY_RECOVERED") {
					return fmt.Errorf("recovered response missing")
				}
				return nil
			})
			app.capture("settled")
		})
	}
}
