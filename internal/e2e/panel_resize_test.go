//go:build e2e

package e2e

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalPanelDragToClose(t *testing.T) {
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
			core, err := engine.Open(engine.Options{Storage: store})
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
				{Kind: "session.create", Name: "Drag to close"},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil || len(snapshot.Sessions) != 1 {
				t.Fatalf("fixture sessions=%d: %v", len(snapshot.Sessions), err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "panels", data, "--local",
				"--appearance", appearance, "--width", "1800", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+snapshot.Sessions[0].ID, "press", "")
			app.act("titlebar-right-pane", "press", "")
			app.act("titlebar-files", "press", "")
			pointer := func(kind string, x float64) {
				t.Helper()
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{
					"type": kind, "x": x, "y": 200,
				}}}, nil); err != nil {
					t.Fatal(err)
				}
			}
			waitWidth := func(label string, want float64) accessNode {
				t.Helper()
				var node accessNode
				app.await(label+" width", func() error {
					pointer("pointerMove", 600)
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					var found bool
					node, found = panelMotionNode(tree, label)
					if !found || math.Abs(node.Bounds.Width-want) > 1 {
						return fmt.Errorf("%s bounds=%+v, want width=%g", label, node.Bounds, want)
					}
					edge := float64(1800)
					switch label {
					case "Session sidebar":
						neighbor, ok := panelMotionNode(tree, "Conversation pane")
						if !ok {
							return fmt.Errorf("conversation seam missing")
						}
						edge = neighbor.Bounds.X - 1
					case "Tools workspace":
						neighbor, ok := panelMotionNode(tree, "Files explorer")
						if !ok {
							return fmt.Errorf("files seam missing")
						}
						edge = neighbor.Bounds.X
					}
					// Accessibility exposes the unclipped content during
					// a tween; wait for its reveal to reach the seam too.
					if visible := min(node.Bounds.Width, edge-node.Bounds.X); math.Abs(visible-want) > 1 {
						return fmt.Errorf("%s visible width=%g, want %g", label, visible, want)
					}
					return nil
				})
				return node
			}
			waitWidth("Files explorer", settings.FilesWidth)
			waitWidth("Tools workspace", settings.RightPaneWidth-1)
			for _, panel := range []struct {
				name, label, openKey string
				minimum              float64
				sidebar              bool
			}{
				{"sidebar", "Session sidebar", "titlebar-sidebar", float64(design.SidebarMin), true},
				{"workspace", "Tools workspace", "titlebar-right-pane", float64(design.RightMin), false},
				{"files", "Files explorer", "titlebar-files", float64(design.FilesMin), false},
			} {
				tree, err := app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				pane, found := panelMotionNode(tree, panel.label)
				if !found {
					t.Fatalf("%s missing before drag", panel.name)
				}
				// Native content excludes the one-DIP divider from the
				// sidebar and workspace widths; the files dock has no
				// trailing divider.
				contentMin := panel.minimum
				if panel.name != "files" {
					contentMin--
				}
				seam, direction := pane.Bounds.X-1, float64(1)
				if panel.sidebar {
					seam, direction = pane.Bounds.X+pane.Bounds.Width, -1
				}
				target := seam + direction*(pane.Bounds.Width-contentMin)
				pointer("pointerMove", seam)
				pointer("pointerDown", seam)
				pointer("pointerMove", target)
				app.await(panel.name+" stays open at minimum", func() error {
					pointer("pointerMove", target)
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					atMin, present := panelMotionNode(tree, panel.label)
					if !present || math.Abs(atMin.Bounds.Width-contentMin) > 1 {
						return fmt.Errorf("%s minimum bounds=%+v, present=%t", panel.name, atMin.Bounds, present)
					}
					return nil
				})
				pointer("pointerMove", target+direction*12)
				// Complete the collapse before releasing: the close must
				// be triggered by the continued drag, not pointerUp.
				app.await(panel.name+" closed while dragging", func() error {
					pointer("pointerMove", target+direction*13)
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					if _, present := panelMotionNode(tree, panel.label); present {
						return fmt.Errorf("%s remains visible", panel.name)
					}
					for _, label := range []string{"Session sidebar", "Tools workspace", "Files explorer"} {
						if label != panel.label {
							if _, present := panelMotionNode(tree, label); !present {
								return fmt.Errorf("neighbor %s closed", label)
							}
						}
					}
					return nil
				})
				pointer("pointerUp", target+direction*13)
				app.capture(panel.name + "-drag-closed")
				app.act(panel.openKey, "press", "")
				waitWidth(panel.label, contentMin)
				app.capture(panel.name + "-reopened-at-minimum")
			}
		})
	}
}
