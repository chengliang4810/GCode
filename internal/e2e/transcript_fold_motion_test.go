//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

const foldMotionTail = "FOLD TAIL POSITION"

// A real core persists long prior history, reasoning and an actual file read.
// The native driver samples every ~20 ms through the transition, rather than
// treating a matching final screenshot as evidence of stationary follow motion.
func TestDesktopLocalTranscriptFoldMotion(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			t.Setenv("MYGO_FRAME_STATS", "all")
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			var history, thought, output strings.Builder
			for i := range 60 {
				fmt.Fprintf(&history, "EARLIER HISTORY %03d occupies real conversation space before the current activity.\n\n", i)
			}
			for i := range 100 {
				fmt.Fprintf(&thought, "FOLD THOUGHT %03d  \n", i)
				fmt.Fprintf(&output, "FOLD FILE OUTPUT %03d\n", i)
			}
			if err := os.WriteFile(filepath.Join(workspace, "fold-motion.txt"), []byte(output.String()), 0600); err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				number := requests.Add(1)
				if err := os.WriteFile(filepath.Join(artifacts, fmt.Sprintf("provider-request-%02d.json", number)), body, 0600); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(delta map[string]any, finish string) {
					payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
					if err != nil {
						t.Error(err)
						return
					}
					fmt.Fprintf(w, "data: %s\n\n", payload)
				}
				switch number {
				case 1:
					send(map[string]any{"content": history.String()}, "stop")
				case 2:
					send(map[string]any{"reasoning_content": thought.String()}, "")
					send(map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "fold-read", "function": map[string]any{
							"name": "Read", "arguments": `{"file_path":"fold-motion.txt"}`,
						},
					}}}, "tool_calls")
				case 3:
					if !strings.Contains(string(body), "FOLD FILE OUTPUT 099") {
						t.Error("provider continuation did not receive the real file's final line")
					}
					send(map[string]any{"content": foldMotionTail}, "stop")
				default:
					t.Errorf("unexpected provider request %d", number)
					send(map[string]any{"content": "Unexpected extra request."}, "stop")
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer provider.Close()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.General.CompactMode = false
			settings.Appearance.MotionPreference = "off" // Full motion.
			settings.Appearance.ReduceMotion = false
			settings.Appearance.PauseInBackground = false
			settings.Appearance.FontSize, settings.Appearance.CodeFontSize = 14, 12
			config := model.Provider{ID: "fold", Name: "Local fold fixture", Protocol: model.Chat,
				BaseURL: provider.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Native fold fixture"}},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: "fold-history", ProjectID: snapshot.Projects[0].ID, ModelID: "fold/test", Text: "Produce the earlier history.",
			}}); err != nil {
				t.Fatal(err)
			}
			snapshot = awaitFoldSeed(t, core)
			sessionID := snapshot.Sessions[0].ID
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: "fold-current", SessionID: sessionID, ModelID: "fold/test", Text: "Read the fold fixture.",
			}}); err != nil {
				t.Fatal(err)
			}
			snapshot = awaitFoldSeed(t, core)
			messageCount := len(snapshot.Messages[sessionID])
			var groupID, thoughtID, readID string
			for _, message := range snapshot.Messages[sessionID] {
				for _, block := range message.Blocks {
					if block.Kind == "reasoning" && strings.Contains(block.Text, "FOLD THOUGHT 099") {
						groupID, thoughtID = message.ID+":activity:"+block.ID, block.ID
					}
					if block.Tool != nil && block.Tool.Name == "Read" && strings.Contains(block.Tool.Output, "FOLD FILE OUTPUT 099") {
						if block.Tool.Status != "completed" || block.Tool.IsError {
							t.Fatalf("real file read failed: %+v", block.Tool)
						}
						readID = block.ID
					}
				}
			}
			if groupID == "" || thoughtID == "" || readID == "" || requests.Load() != 3 {
				t.Fatalf("missing real fold fixture: group=%q thought=%q read=%q requests=%d", groupID, thoughtID, readID, requests.Load())
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}

			app := startApp(t, binary, artifacts, "full-motion", data, "--local", "--appearance", appearance,
				"--language", "en", "--width", "900", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.waitKey("titlebar-session-info")
			app.await("requested conversation's trailing body is mounted", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				_, err = foldTextNode(tree, foldMotionTail)
				return err
			})
			const draft = "KEEP FOLD MOTION DRAFT"
			app.act("input", "setValue", draft)
			app.await("central fold draft is business-bound before testing motion", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil || input.Value != draft {
					return fmt.Errorf("central draft has not committed: %q, %v", input.Value, err)
				}
				send, err := tree.findKey("send")
				if err != nil || !slices.Contains(send.Actions, "press") {
					return fmt.Errorf("business-bound send action is unavailable: %v", err)
				}
				return nil
			})
			app.proveFoldConversationOverflow()
			group := "activity-toggle:" + groupID
			thoughtToggle := "activity-item-toggle:" + groupID + ":" + thoughtID
			samples := map[string][]foldMotionSample{}
			samples["group-open"] = app.sampleFoldMotion(group, group, true, false, nil)
			samples["thought-open"] = app.sampleFoldMotion(thoughtToggle, group, true, false, nil)
			app.assertFoldInternalScroll("activity-detail-viewport:"+groupID+":"+thoughtID, "FOLD THOUGHT 099", 24*14*1.5)
			app.capture("thought-tail-full-motion")
			samples["thought-close"] = app.sampleFoldMotion(thoughtToggle, group, false, false, nil)
			samples["group-close"] = app.sampleFoldMotion(group, group, false, false, nil)
			samples["rapid-group-reversal"] = app.sampleFoldMotion(group, group, true, false, []time.Duration{45 * time.Millisecond, 90 * time.Millisecond})
			app.capture("rapid-reversal-settled")
			app.assertFoldDraft(draft)
			if err := storage.WriteJSON(filepath.Join(artifacts, "motion-samples.json"), samples); err != nil {
				t.Fatal(err)
			}
			app.stop()
			log, err := os.ReadFile(filepath.Join(app.artifacts, "app.log"))
			if err != nil || !bytesContainsFoldStats(log) {
				t.Fatalf("MYGO_FRAME_STATS native log is absent: %v", err)
			}

			// The same persisted real core content is reopened with reduced
			// motion. No transcript or activity state is injected.
			store, err = storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err = openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			persisted, err := core.Snapshot(t.Context(), sessionID)
			if err != nil || len(persisted.Sessions) != 1 || persisted.Sessions[0].ID != sessionID ||
				len(persisted.Messages[sessionID]) != messageCount || requests.Load() != 3 {
				t.Fatalf("fold actions changed canonical conversation state after reopening the real core: requests=%d err=%v", requests.Load(), err)
			}
			settings.Appearance.MotionPreference, settings.Appearance.ReduceMotion = "on", true
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			reduced := startApp(t, binary, artifacts, "reduced-motion", data, "--local", "--appearance", appearance,
				"--language", "en", "--width", "900", "--height", "880")
			reduced.act("back-to-conversations", "press", "")
			reduced.act("sidebar-session:"+sessionID, "press", "")
			reduced.waitKey("titlebar-session-info")
			reduced.waitKey(group)
			reducedSamples := reduced.sampleFoldMotion(group, group, true, true, nil)
			reduced.capture("group-expanded-reduced-motion")
			if err := storage.WriteJSON(filepath.Join(artifacts, "reduced-motion-samples.json"), reducedSamples); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func awaitFoldSeed(t *testing.T, core *engine.Engine) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(stateTimeout)
	for {
		state, err := core.Snapshot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Sessions) == 1 && state.Sessions[0].Status == contracts.Idle && state.Sessions[0].LastOutcome == "completed" {
			scoped, err := core.Snapshot(t.Context(), state.Sessions[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			return scoped
		}
		if time.Now().After(deadline) {
			t.Fatal("real core fold fixture did not complete")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type foldMotionSample struct {
	AtMS            float64 `json:"atMs"`
	TailY           float64 `json:"tailY"`
	TriggerY        float64 `json:"triggerY"`
	TriggerExpanded bool    `json:"triggerExpanded"`
}

func foldTextNode(tree accessTree, label string) (accessNode, error) {
	for _, node := range tree.Nodes {
		if node.Role == "text" && node.Label == label {
			return node, nil
		}
	}
	return accessNode{}, fmt.Errorf("text %q is not mounted", label)
}

func (app *nativeApp) proveFoldConversationOverflow() {
	app.t.Helper()
	tree, err := app.client.tree()
	if err != nil {
		app.t.Fatal(err)
	}
	var conversation accessNode
	for _, node := range tree.Nodes {
		if node.Label == "Conversation pane" {
			conversation = node
		}
	}
	if conversation.Bounds.Width <= 0 || conversation.Bounds.Height <= 0 {
		app.t.Fatal("conversation has no visible wheel target")
	}
	if err := app.client.call("input", map[string]any{"events": []map[string]any{{
		"type": "wheel", "x": conversation.Bounds.X + min(100, conversation.Bounds.Width/2),
		"y": conversation.Bounds.Y + 100, "dy": -4000, "precise": true,
	}}}, nil); err != nil {
		app.t.Fatal(err)
	}
	app.await("real earlier history overflows the conversation viewport", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			if node.Role == "button" && node.Label == "Back to latest" && slices.Contains(node.Actions, "press") {
				return app.client.call("act", map[string]any{"id": node.ID, "action": "press"}, nil)
			}
		}
		return fmt.Errorf("scrolling earlier history did not expose Back to latest")
	})
	app.await("conversation has returned to its overflowing end", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			if node.Label == "Back to latest" {
				return fmt.Errorf("conversation is still away from its end")
			}
		}
		tail, err := foldTextNode(tree, foldMotionTail)
		if err != nil || tail.Bounds.Y < 38 {
			return fmt.Errorf("tail is not visibly anchored: %+v %v", tail.Bounds, err)
		}
		return nil
	})
	app.capture("overflowing-conversation-at-end")
}

func (app *nativeApp) sampleFoldMotion(actionKey, referenceKey string, expanded, reduced bool, reverse []time.Duration) []foldMotionSample {
	app.t.Helper()
	if err := app.client.call("idle", map[string]any{"timeout": 1500}, nil); err != nil {
		app.t.Fatalf("native layout did not settle before folding: %v", err)
	}
	before, err := app.client.tree()
	if err != nil {
		app.t.Fatal(err)
	}
	tail, err := foldTextNode(before, foldMotionTail)
	if err != nil {
		app.t.Fatal(err)
	}
	reference := app.waitKey(referenceKey)
	app.waitKey(actionKey)
	start := time.Now()
	if err := app.client.call("act", map[string]any{"key": actionKey, "action": "press"}, nil); err != nil {
		app.t.Fatal(err)
	}
	var samples []foldMotionSample
	reversed := 0
	for time.Since(start) < 340*time.Millisecond {
		if reversed < len(reverse) && time.Since(start) >= reverse[reversed] {
			if err := app.client.call("act", map[string]any{"key": actionKey, "action": "press"}, nil); err != nil {
				app.t.Fatal(err)
			}
			reversed++
		}
		tree, err := app.client.tree()
		if err != nil {
			app.t.Fatal(err)
		}
		current, err := foldTextNode(tree, foldMotionTail)
		if err != nil {
			app.t.Fatal(err)
		}
		trigger, err := tree.findKey(referenceKey)
		if err != nil {
			app.t.Fatal(err)
		}
		action, err := tree.findKey(actionKey)
		if err != nil {
			app.t.Fatal(err)
		}
		samples = append(samples, foldMotionSample{
			AtMS: float64(time.Since(start).Microseconds()) / 1000, TailY: current.Bounds.Y,
			TriggerY: trigger.Bounds.Y, TriggerExpanded: slices.Contains(action.States, "expanded"),
		})
		time.Sleep(20 * time.Millisecond)
	}
	// Preserve samples even when a dynamic assertion fails.
	name := strings.ReplaceAll(actionKey, ":", "-")
	if err := storage.WriteJSON(filepath.Join(app.artifacts, name+fmt.Sprintf("-expanded-%t.json", expanded)), samples); err != nil {
		app.t.Fatal(err)
	}
	if len(samples) < 5 {
		app.t.Fatalf("only %d frames sampled during the fold", len(samples))
	}
	final := samples[len(samples)-1]
	intermediate := 0
	for _, sample := range samples {
		if math.Abs(sample.TailY-tail.Bounds.Y) > 2 {
			app.t.Fatalf("fold moved trailing body during animation: before=%g, sample=%+v", tail.Bounds.Y, sample)
		}
		if math.Abs(sample.TriggerY-reference.Bounds.Y) > 1 && math.Abs(sample.TriggerY-final.TriggerY) > 1 {
			intermediate++
		}
	}
	if final.TriggerExpanded != expanded {
		app.t.Fatalf("fold final expanded=%t, want %t", final.TriggerExpanded, expanded)
	}
	if reduced && intermediate != 0 {
		app.t.Fatalf("reduced-motion fold exposed %d intermediate positions", intermediate)
	}
	if !reduced && intermediate == 0 {
		app.t.Fatalf("no intermediate fold positions were observed; final state alone is insufficient evidence")
	}
	if err := app.client.call("idle", map[string]any{"timeout": 1500}, nil); err != nil {
		app.t.Fatalf("native layout did not settle after sampled folding: %v", err)
	}
	app.await("sampled fold has committed its final disclosure state", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		action, err := tree.findKey(actionKey)
		if err != nil {
			return err
		}
		if slices.Contains(action.States, "expanded") != expanded {
			return fmt.Errorf("native disclosure has not committed expanded=%t", expanded)
		}
		current, err := foldTextNode(tree, foldMotionTail)
		if err != nil || math.Abs(current.Bounds.Y-tail.Bounds.Y) > 2 {
			return fmt.Errorf("body moved after the sampled transition: %g -> %g, %v", tail.Bounds.Y, current.Bounds.Y, err)
		}
		return nil
	})
	return samples
}

func (app *nativeApp) assertFoldInternalScroll(viewportKey, lastLine string, height float64) {
	app.t.Helper()
	viewport := app.waitKey(viewportKey)
	if math.Abs(viewport.Bounds.Height-height) > 1 {
		app.t.Fatalf("detail viewport height=%g, want 24 lines=%g", viewport.Bounds.Height, height)
	}
	before, err := app.client.tree()
	if err != nil {
		app.t.Fatal(err)
	}
	tail, err := foldTextNode(before, foldMotionTail)
	if err != nil {
		app.t.Fatal(err)
	}
	input := app.waitKey("input")
	y := max(viewport.Bounds.Y, 60) + 12
	if y >= min(viewport.Bounds.Y+viewport.Bounds.Height, input.Bounds.Y-8) {
		app.t.Fatalf("detail has no visible scroll target: %+v", viewport.Bounds)
	}
	if err := app.client.call("input", map[string]any{"events": []map[string]any{{
		"type": "wheel", "x": viewport.Bounds.X + min(viewport.Bounds.Width/2, 120), "y": y, "dy": 10000, "precise": true,
	}}}, nil); err != nil {
		app.t.Fatal(err)
	}
	app.await("full detail tail scrolls while the outer body remains fixed", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		if !activityTreeContains(tree, lastLine) {
			return fmt.Errorf("full detail tail %q is not mounted", lastLine)
		}
		current, err := foldTextNode(tree, foldMotionTail)
		if err != nil || math.Abs(current.Bounds.Y-tail.Bounds.Y) > 2 {
			return fmt.Errorf("inner scrolling moved outer body: %g -> %g, %v", tail.Bounds.Y, current.Bounds.Y, err)
		}
		return nil
	})
}

func (app *nativeApp) assertFoldDraft(draft string) {
	app.t.Helper()
	tree, err := app.client.tree()
	if err != nil {
		app.t.Fatal(err)
	}
	input, err := tree.findKey("input")
	if err != nil || input.Value != draft {
		app.t.Fatalf("fold actions lost the central draft: %q %v", input.Value, err)
	}
}

func bytesContainsFoldStats(log []byte) bool {
	return strings.Contains(string(log), "mygo: frame ") && strings.Contains(string(log), "allocations")
}
