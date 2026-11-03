//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

// A real read executes through the local core before the native app loads its
// persisted transcript. Questions then execute from the native composer. Only
// the model is a local HTTP fixture; no snapshot or tool result is fabricated.
func TestDesktopLocalTranscriptActivity(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			var thought, output strings.Builder
			for i := range 80 {
				fmt.Fprintf(&thought, "THOUGHT %03d  \n", i)
				fmt.Fprintf(&output, "READ OUTPUT %03d\n", i)
			}
			const filename = "activity-fixture.txt"
			filePath := filepath.Join(workspace, filename)
			if err := os.WriteFile(filePath, []byte(output.String()), 0600); err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				requestNumber := requests.Add(1)
				t.Logf("local provider request %d: %s", requestNumber, r.URL.Path)
				if err := os.WriteFile(filepath.Join(artifacts, fmt.Sprintf("provider-request-%02d.json", requestNumber)), body, 0600); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(delta map[string]any, finish string) {
					payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{
						"delta": delta, "finish_reason": finish,
					}}})
					if err != nil {
						t.Error(err)
						return
					}
					fmt.Fprintf(w, "data: %s\n\n", payload)
				}
				call := func(id, name, arguments string) {
					send(map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": id, "function": map[string]any{"name": name, "arguments": arguments},
					}}}, "tool_calls")
				}
				switch requestNumber {
				case 1:
					send(map[string]any{"reasoning_content": thought.String()}, "")
					call("activity-read", "Read", `{"file_path":"activity-fixture.txt"}`)
				case 2:
					if !strings.Contains(string(body), "READ OUTPUT 079") {
						t.Error("provider continuation did not receive the real file's final line")
					}
					send(map[string]any{"content": "Activity fixture finished."}, "stop")
				case 3:
					call("activity-ask", "AskUserQuestion", `{"questions":[{"question":"Choose transcript activity option","header":"Activity","options":[{"label":"Activity answer A","description":"Choose the first activity answer"},{"label":"Activity answer B","description":"Choose the second activity answer"}],"multiSelect":false}]}`)
				case 4:
					if !strings.Contains(string(body), "Activity answer A") {
						t.Error("question answer did not reach the provider continuation")
					}
					send(map[string]any{"content": "Activity question answered."}, "stop")
				default:
					t.Error("unexpected extra provider request")
					send(map[string]any{"content": "Unexpected extra request."}, "stop")
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			ctx := context.Background()
			snapshot, err := core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.General.CompactMode = false
			settings.Appearance.ReduceMotion = true
			settings.Appearance.FontSize, settings.Appearance.CodeFontSize = 14, 12
			provider := model.Provider{ID: "activity", Name: "Activity fixture", Protocol: model.Chat,
				BaseURL: server.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Activity fixture"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: "activity-seed", ProjectID: snapshot.Projects[0].ID, ModelID: "activity/test", Text: "Read the activity fixture.",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			for {
				snapshot, err = core.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle && snapshot.Sessions[0].LastOutcome == "completed" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("real read fixture did not complete")
				}
				time.Sleep(20 * time.Millisecond)
			}
			sessionID := snapshot.Sessions[0].ID
			var groupID, thoughtID, readID string
			for _, message := range snapshot.Messages[sessionID] {
				for _, block := range message.Blocks {
					if block.Kind == "reasoning" {
						thoughtID = block.ID
						groupID = message.ID + ":activity:" + block.ID
					}
					if block.Tool != nil && block.Tool.Name == "Read" {
						readID = block.ID
						if block.Tool.Status != "completed" || block.Tool.IsError ||
							block.Tool.Path != filePath || block.Tool.Command != "" ||
							!strings.Contains(block.Tool.Output, "READ OUTPUT 079") {
							t.Fatalf("real read failed or lost its typed projection: %+v", block.Tool)
						}
					}
				}
			}
			if groupID == "" || thoughtID == "" || readID == "" || requests.Load() != 2 {
				t.Fatalf("missing real activity: group=%q thought=%q read=%q requests=%d", groupID, thoughtID, readID, requests.Load())
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}

			app := startApp(t, binary, artifacts, "activity", data, "--local", "--appearance", appearance,
				"--language", "en", "--width", "900", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.act("activity-toggle:"+groupID, "press", "")
			thoughtToggle := "activity-item-toggle:" + groupID + ":" + thoughtID
			readToggle := "activity-item-toggle:" + groupID + ":" + readID
			app.waitKey(thoughtToggle)
			app.await("completed thought shows its first line while details stay closed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, "THOUGHT 000") || activityTreeContains(tree, "THOUGHT 079") {
					return fmt.Errorf("completed thought preview is missing or exposes its tail")
				}
				for _, node := range tree.Nodes {
					if node.Label == "Activity detail" {
						return fmt.Errorf("thought details expanded without a click")
					}
				}
				read, err := tree.findKey(readToggle)
				if err != nil || read.Label != "Read: "+filePath {
					return fmt.Errorf("read title is not the typed path: %q (%v)", read.Label, err)
				}
				return nil
			})
			app.capture("collapsed-thought-and-read")
			app.act(thoughtToggle, "press", "")
			thoughtViewport := "activity-detail-viewport:" + groupID + ":" + thoughtID
			app.assertActivityScroll(thoughtViewport, thoughtToggle, "THOUGHT 079", 24*14*1.5)
			app.capture("thought-full-tail")
			app.act(thoughtToggle, "press", "")
			app.act(readToggle, "press", "")
			app.await("read invocation preserves structured arguments", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, `"path": "activity-fixture.txt"`) {
					return fmt.Errorf("original read arguments are missing")
				}
				return nil
			})
			readViewport := "activity-detail-viewport:" + groupID + ":" + readID
			app.assertActivityScroll(readViewport, readToggle, "READ OUTPUT 079", 24*12*1.5)
			app.capture("read-full-output-tail")
			app.act(readToggle, "press", "")

			app.act("input", "setValue", "Ask for an activity choice.")
			app.act("send", "press", "")
			app.await("AskUserQuestion creates one question card without an activity row", func() error {
				if requests.Load() != 3 {
					return fmt.Errorf("question fixture has received %d requests, want 3", requests.Load())
				}
				// input.requested is a live core event, not a persistence
				// boundary. Assert its native projection now; verify canonical
				// tool/question persistence after answering completes the run.
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				cards := 0
				for _, node := range tree.Nodes {
					if node.Label == "Choose transcript activity option" {
						cards++
					}
					if strings.Contains(node.Label, "AskUserQuestion") {
						return fmt.Errorf("AskUserQuestion leaked into activity: %q", node.Label)
					}
				}
				if cards != 1 {
					return fmt.Errorf("question title appears %d times after %d provider requests", cards, requests.Load())
				}
				return nil
			})
			app.capture("single-question-card")
			// Resolve the current native button immediately before activation.
			// Question options currently expose typed labels rather than keys.
			tree, err := app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			var answer accessNode
			for _, node := range tree.Nodes {
				if node.Label == "Activity answer A" && node.Role == "button" {
					answer = node
				}
			}
			if answer.ID == "" {
				t.Fatal("native question answer button is missing")
			}
			if err := app.client.call("act", map[string]any{"id": answer.ID, "action": "press"}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("question answer reaches real core and model continuation", func() error {
				current, err := activityDiskSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range current.Sessions {
					if session.ID == sessionID && session.Status == contracts.Idle && session.LastOutcome == "completed" && requests.Load() == 4 {
						var ask, answered bool
						for _, message := range current.Messages[sessionID] {
							for _, block := range message.Blocks {
								ask = ask || block.Tool != nil && block.Tool.Name == "AskUserQuestion" && block.Tool.Status == "completed"
								answered = answered || block.Question != nil && block.Question.Resolved && len(block.Question.Answers) == 1 && len(block.Question.Answers[0].Options) == 1 && block.Question.Answers[0].Options[0] == "Activity answer A"
							}
						}
						if ask && answered {
							return nil
						}
					}
				}
				return fmt.Errorf("question answer has not completed after %d provider requests", requests.Load())
			})
			app.capture("answered-question")
		})
	}
}

func activityTreeContains(tree accessTree, text string) bool {
	for _, node := range tree.Nodes {
		if strings.Contains(node.Label, text) {
			return true
		}
	}
	return false
}

func activityDiskSnapshot(directory string) (contracts.Snapshot, error) {
	return readFixtureSnapshot(directory)
}

func (app *nativeApp) assertActivityScroll(viewportKey, triggerKey, tail string, height float64) {
	app.t.Helper()
	viewport := app.waitKey(viewportKey)
	if math.Abs(viewport.Bounds.Height-height) > 1 {
		app.t.Fatalf("activity detail height=%g, want 24 lines=%g", viewport.Bounds.Height, height)
	}
	trigger := app.waitKey(triggerKey)
	input := app.waitKey("input")
	y := math.Max(viewport.Bounds.Y, 60) + 12
	if y >= math.Min(viewport.Bounds.Y+viewport.Bounds.Height, input.Bounds.Y-8) {
		app.t.Fatalf("activity detail has no visible wheel target: %+v", viewport.Bounds)
	}
	if err := app.client.call("input", map[string]any{"events": []map[string]any{{
		"type": "wheel", "x": viewport.Bounds.X + math.Min(viewport.Bounds.Width/2, 120), "y": y,
		"dy": 10000, "precise": true,
	}}}, nil); err != nil {
		app.t.Fatal(err)
	}
	app.await("complete activity tail scrolls independently of the conversation", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		if !activityTreeContains(tree, tail) {
			return fmt.Errorf("detail tail %q is inaccessible", tail)
		}
		after, err := tree.findKey(triggerKey)
		if err != nil {
			return err
		}
		if math.Abs(after.Bounds.Y-trigger.Bounds.Y) > 1 {
			return fmt.Errorf("detail wheel moved the outer conversation: %g -> %g", trigger.Bounds.Y, after.Bounds.Y)
		}
		return nil
	})
}
