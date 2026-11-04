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
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The native plugin cannot select OS context menus. Component regressions
// cover both right-click entries; this starts the same tab with a normal route.
func TestDesktopLocalCallTrajectory(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
				if err != nil {
					t.Error(err)
					return
				}
				switch calls.Add(1) {
				case 1:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"trace-read\",\"function\":{\"name\":\"Read\",\"arguments\":\"{\\\"file_path\\\":\\\"trace.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
				case 2:
					if !strings.Contains(string(body), "TRACE_FILE_CONTENT") {
						t.Error("actual tool output did not reach the second request")
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"TRACE_REASONING\",\"content\":\"TRACE_FINAL_OUTPUT\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":5,\"prompt_tokens_details\":{\"cached_tokens\":8}}}\n\ndata: [DONE]\n\n")
				default:
					w.WriteHeader(400)
				}
			}))
			defer provider.Close()
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "trace.txt"), []byte("TRACE_FILE_CONTENT"), 0600); err != nil {
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
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "trace", Name: "Trace fixture", Protocol: model.Chat, BaseURL: provider.URL,
					APIKey: "trace-fixture-private-key", Models: []model.Model{{ID: "model"}},
				}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Trajectory"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := core.Snapshot(ctx)
			sessionID := ""
			for count, prompt := range []string{"TRACE_PROMPT", "FAILURE_PROMPT"} {
				if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					SessionID: sessionID, ProjectID: snapshot.Projects[0].ID, ModelID: "trace/model", Text: prompt,
				}}); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(stateTimeout)
				for {
					snapshot, err = core.Snapshot(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if len(snapshot.Runs) == count+1 && snapshot.Runs[count].Status != "running" {
						sessionID = snapshot.Sessions[0].ID
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("trajectory seed did not finish")
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			language, width := "en", "1320"
			if appearance == "dark" {
				language, width = "zh-CN", "900"
			}
			app := startAppReady(t, binary, artifacts, "trajectory", data, "trajectory-pane:"+sessionID, "--local", "--route", "trajectory/"+sessionID,
				"--appearance", appearance, "--language", language, "--width", width, "--height", "900")
			app.waitKey("trajectory-pane:" + sessionID)
			app.await("three persisted real HTTP attempts are displayed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				node, err := tree.findKey("trajectory-summary")
				if err != nil {
					return err
				}
				if !strings.Contains(node.Label, "3") {
					return fmt.Errorf("unexpected call count: %s", node.Label)
				}
				return nil
			})
			app.capture("initial")
			app.act("trajectory-search", "setValue", "TRACE_")
			app.await("search exposes recorded request, tool output and response", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, want := range []string{"TRACE_FINAL_OUTPUT", "TRACE_FILE_CONTENT", "TRACE_REASONING"} {
					if !activityTreeContains(tree, want) {
						return fmt.Errorf("trajectory missing %s", want)
					}
				}
				return nil
			})
			app.capture("request-response")
			app.act("trajectory-search", "setValue", "HTTP 400")
			app.await("failed HTTP attempt retains its error", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, "model request failed (HTTP 400)") {
					return fmt.Errorf("recorded model error missing")
				}
				if activityTreeContains(tree, "trace-fixture-private-key") {
					return fmt.Errorf("provider credential leaked into trajectory UI")
				}
				return nil
			})
			app.capture("failure")
			app.act("trajectory-refresh", "press", "")
			app.waitKey("trajectory-summary")
		})
	}
}
