//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalProjectConversationReorder(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
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
				_ = store.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = core.Close() })
			if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := core.Snapshot(t.Context())
			project := snapshot.Projects[0].ID
			for _, name := range []string{"Conversation A", "Conversation B", "Conversation C"} {
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: project, Name: name}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			ids := map[string]string{}
			for _, session := range snapshot.Sessions {
				ids[session.Title] = session.ID
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			width := "1320"
			if appearance == "dark" {
				width = "900"
			}
			args := []string{"--local", "--route", "chat/" + ids["Conversation C"], "--appearance", appearance,
				"--language", "zh-CN", "--width", width, "--height", "700"}
			key := func(title string) string { return "sidebar-session:" + ids[title] }
			app := startAppReady(t, binary, artifacts, "drag", data, key("Conversation A"), args...)
			assertOrder := func(want []string) {
				t.Helper()
				app.await("project conversation order in UI and saved core", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					last := float64(-1)
					for _, title := range want {
						node, err := tree.findKey(key(title))
						if err != nil {
							return err
						}
						if node.Bounds.Y <= last {
							return fmt.Errorf("conversation %s at %g, preceding row at %g", title, node.Bounds.Y, last)
						}
						last = node.Bounds.Y
					}
					state, err := readFixtureSnapshot(data)
					if err != nil {
						return err
					}
					slices.SortStableFunc(state.Sessions, contracts.CompareProjectSessions)
					var titles []string
					for _, session := range state.Sessions {
						if session.ProjectID == project {
							if session.Pinned {
								return fmt.Errorf("drag pinned %s", session.Title)
							}
							titles = append(titles, session.Title)
						}
					}
					if !slices.Equal(titles, want) {
						return fmt.Errorf("saved order=%v, want %v", titles, want)
					}
					return nil
				})
			}
			drag := func(source, target string, after bool) {
				t.Helper()
				from, to := app.waitKey(key(source)), app.waitKey(key(target))
				x, y := from.Bounds.X+80, from.Bounds.Y+from.Bounds.Height/2
				targetY := to.Bounds.Y + 3
				if after {
					targetY = to.Bounds.Y + to.Bounds.Height - 3
				}
				for _, event := range []map[string]any{
					{"type": "pointerMove", "x": x, "y": y},
					{"type": "pointerDown", "x": x, "y": y},
					{"type": "pointerMove", "x": x + 12, "y": y},
					{"type": "pointerMove", "x": x, "y": targetY},
					{"type": "pointerUp", "x": x, "y": targetY},
				} {
					if err := app.client.call("input", map[string]any{"events": []map[string]any{event}}, nil); err != nil {
						t.Fatal(err)
					}
				}
			}
			assertOrder([]string{"Conversation C", "Conversation B", "Conversation A"})
			drag("Conversation A", "Conversation C", false)
			assertOrder([]string{"Conversation A", "Conversation C", "Conversation B"})
			app.capture("moved-to-top")
			drag("Conversation A", "Conversation B", true)
			assertOrder([]string{"Conversation C", "Conversation B", "Conversation A"})
			app.capture("moved-to-bottom")
			app.stop()
			app = startAppReady(t, binary, artifacts, "restart", data, key("Conversation A"), args...)
			assertOrder([]string{"Conversation C", "Conversation B", "Conversation A"})
			app.capture("saved-order-after-restart")
		})
	}
}
