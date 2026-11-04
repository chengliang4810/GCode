//go:build e2e

package e2e

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// Sample the real native window during the width tween: settled screenshots
// alone miss the temporary vertical compression and jumping sidebar footer.
func TestDesktopLocalPanelToggleHeight(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "data")
			workspace := filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
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
			settings := contracts.DefaultSettings()
			settings.Appearance.MotionPreference = "off"
			settings.Appearance.ReduceMotion = false
			settings.Appearance.PauseInBackground = false
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}},
				{Kind: "session.create", Name: "Panel motion regression"},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil || len(snapshot.Sessions) != 1 {
				t.Fatalf("panel fixture sessions=%d: %v", len(snapshot.Sessions), err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "panels", data, "--local",
				"--appearance", appearance, "--width", "1600", "--height", "1000")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+snapshot.Sessions[0].ID, "press", "")
			app.act("titlebar-right-pane", "press", "")
			app.act("titlebar-files", "press", "")
			if err := app.client.call("idle", map[string]any{"timeout": 1500}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("initial")
			for _, panel := range []struct {
				name, closeKey, openKey, label string
			}{
				{"sidebar", "sidebar-toggle", "titlebar-sidebar", "Session sidebar"},
				{"workspace", "titlebar-right-pane", "titlebar-right-pane", "Tools workspace"},
				{"files", "titlebar-files", "titlebar-files", "Files explorer"},
			} {
				tree, err := app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				initial, found := panelMotionNode(tree, panel.label)
				if !found || initial.Bounds.Height < 900 {
					t.Fatalf("%s initial pane is missing or compressed: %+v", panel.name, initial)
				}
				footer := app.waitKey("sidebar-settings")
				for _, direction := range []struct{ name, key string }{
					{"close", panel.closeKey}, {"open", panel.openKey},
				} {
					start := time.Now()
					app.act(direction.key, "press", "")
					var samples []panelMotionSample
					captured := false
					motionSamples := 0
					for time.Since(start) < 340*time.Millisecond {
						// Keep sampling real input-driven frames if another
						// window obscures this owned test process. MyGo holds
						// timer-only frames while a native window is occluded.
						if err := app.client.call("input", map[string]any{"events": []map[string]any{{
							"type": "pointerMove", "x": 600 + len(samples)%2, "y": 300,
						}}}, nil); err != nil {
							t.Fatal(err)
						}
						tree, err := app.client.tree()
						if err != nil {
							t.Fatal(err)
						}
						node, present := panelMotionNode(tree, panel.label)
						button, _ := tree.findKey("sidebar-settings")
						// Native accessibility reports the full content bounds,
						// even where the animated wrapper clips them. Measure
						// the reveal at the adjacent column's seam instead.
						visibleWidth := float64(0)
						if present {
							edge := float64(1600)
							switch panel.name {
							case "sidebar":
								neighbor, ok := panelMotionNode(tree, "Conversation pane")
								if !ok {
									t.Fatal("conversation seam is missing")
								}
								edge = neighbor.Bounds.X - 1
							case "workspace":
								neighbor, ok := panelMotionNode(tree, "Files explorer")
								if !ok {
									t.Fatal("files seam is missing")
								}
								edge = neighbor.Bounds.X
							}
							visibleWidth = max(0, min(node.Bounds.Width, edge-node.Bounds.X))
						}
						samples = append(samples, panelMotionSample{
							AtMS:    float64(time.Since(start).Microseconds()) / 1000,
							Present: present, X: node.Bounds.X, Y: node.Bounds.Y,
							Width: node.Bounds.Width, Height: node.Bounds.Height,
							VisibleWidth: visibleWidth,
							FooterY:      button.Bounds.Y, FooterHeight: button.Bounds.Height,
						})
						if visibleWidth > 1 && visibleWidth < initial.Bounds.Width-1 {
							motionSamples++
						}
						if !captured && motionSamples >= 2 && visibleWidth > initial.Bounds.Width*0.2 && visibleWidth < initial.Bounds.Width*0.8 {
							app.capture(panel.name + "-" + direction.name + "-during")
							captured = true
						}
						time.Sleep(12 * time.Millisecond) // Sampling cadence, not a completion condition.
					}
					name := panel.name + "-" + direction.name
					if err := storage.WriteJSON(filepath.Join(app.artifacts, name+"-samples.json"), samples); err != nil {
						t.Fatal(err)
					}
					intermediate := 0
					for _, sample := range samples {
						if !sample.Present || sample.VisibleWidth <= 0.5 {
							continue
						}
						if math.Abs(sample.Y-initial.Bounds.Y) > 1 || math.Abs(sample.Height-initial.Bounds.Height) > 1 {
							t.Errorf("%s compressed vertically during the tween: initial=%+v sample=%+v", name, initial.Bounds, sample)
						}
						if panel.name == "sidebar" && sample.FooterHeight > 0 && math.Abs(sample.FooterY-footer.Bounds.Y) > 1 {
							t.Errorf("%s footer moved vertically: initial=%g sample=%+v", name, footer.Bounds.Y, sample)
						}
						if sample.VisibleWidth > 1 && sample.VisibleWidth < initial.Bounds.Width-1 {
							intermediate++
						}
					}
					if intermediate < 2 || !captured {
						t.Fatalf("%s did not observe the real width transition: %d intermediate samples, capture=%t", name, intermediate, captured)
					}
					if err := app.client.call("idle", map[string]any{"timeout": 1500}, nil); err != nil {
						t.Fatal(err)
					}
					app.await(name+" reached its final layout", func() error {
						if err := app.client.call("input", map[string]any{"events": []map[string]any{{
							"type": "pointerMove", "x": 600, "y": 300,
						}}}, nil); err != nil {
							return err
						}
						tree, err := app.client.tree()
						if err != nil {
							return err
						}
						node, present := panelMotionNode(tree, panel.label)
						if direction.name == "close" {
							if present && node.Bounds.Width > 0.5 {
								return fmt.Errorf("pane still visible: %+v", node.Bounds)
							}
							return nil
						}
						if !present || math.Abs(node.Bounds.Width-initial.Bounds.Width) > 1 ||
							math.Abs(node.Bounds.Height-initial.Bounds.Height) > 1 {
							return fmt.Errorf("pane has not returned to its original size: %+v", node.Bounds)
						}
						return nil
					})
					app.capture(name + "-settled")
				}
			}
		})
	}
}

type panelMotionSample struct {
	AtMS, X, Y, Width, Height, VisibleWidth, FooterY, FooterHeight float64
	Present                                                        bool
}

func panelMotionNode(tree accessTree, label string) (accessNode, bool) {
	for _, node := range tree.Nodes {
		if node.Label == label {
			return node, true
		}
	}
	return accessNode{}, false
}
