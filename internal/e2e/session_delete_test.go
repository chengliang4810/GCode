//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// OS context-menu selection is covered by ui.Tester. The archived page opens
// the same production dialog here, backed by the real local core.
func TestDesktopLocalSessionDeleteConfirmation(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "data")
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
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Deletion confirmation fixture"}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil || len(snapshot.Sessions) != 1 {
				t.Fatalf("session fixture: %v", err)
			}
			id := snapshot.Sessions[0].ID
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.archive", ID: id, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			language, width := "en", "1320"
			warning := "Once deleted, this conversation cannot be recovered. To hide it from the list instead, use Archive."
			if appearance == "dark" {
				language, width = "zh-CN", "900"
				warning = "删除后，该对话将无法找回。如果只是想从列表中隐藏，可以使用归档操作。"
			}
			app := startAppReady(t, binary, artifacts, "delete", data, "archived-delete-"+id,
				"--local", "--route", "settings/Archived sessions", "--appearance", appearance,
				"--language", language, "--width", width, "--height", "700")
			open := func() {
				app.act("archived-delete-"+id, "press", "")
				app.waitKey("session-delete-confirm")
				app.await("irreversible deletion and archive guidance", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					for _, node := range tree.Nodes {
						if node.Label == warning {
							if node.Bounds.Height <= 0 || node.Bounds.Width > 400 {
								return fmt.Errorf("warning outside dialog: %+v", node.Bounds)
							}
							return nil
						}
					}
					return fmt.Errorf("warning missing")
				})
			}
			open()
			app.capture("confirmation")
			app.act("session-delete-cancel", "press", "")
			app.waitKey("archived-delete-" + id)
			app.await("cancel preserves persisted session", func() error {
				current, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(current.Sessions) != 1 || current.Sessions[0].ID != id {
					return fmt.Errorf("cancel lost the conversation")
				}
				return nil
			})
			open()
			app.act("session-delete-confirm", "press", "")
			app.await("confirmed deletion persisted", func() error {
				current, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if len(current.Sessions) != 0 {
					return fmt.Errorf("deleted conversation remains")
				}
				return nil
			})
			app.capture("deleted")
		})
	}
}
