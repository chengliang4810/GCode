//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// Read the actual checkout through the visible project picker. The app process,
// profile, socket and writes belong to this test; no model is configured.
func TestDesktopLocalProjectCreationWithIgnoredArtifacts(t *testing.T) {
	binary, artifacts := testPaths(t)
	checkout, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(artifacts, "profile")
	app := startApp(t, binary, artifacts, "create", data, "--local")
	app.act("back-to-conversations", "press", "")
	app.act("sidebar-add-project", "press", "")
	app.act("project-picker-row:local", "press", "")
	volume := app.awaitKeyPrefix("location:")
	app.act("project-picker-row:"+volume, "press", "")
	app.act("project-edit-path", "press", "")
	app.act("project-path-input", "setValue", checkout)
	app.act("project-path-input", "focus", "")
	if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Enter"}}}, nil); err != nil {
		t.Fatal(err)
	}
	app.waitKey("project-picker-row:" + filepath.ToSlash(filepath.Join(checkout, "internal")))
	app.act("project-add-confirm", "press", "")
	var loaded contracts.Snapshot
	readState := func() error {
		payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
		if err != nil {
			return err
		}
		var saved struct{ Snapshot contracts.Snapshot }
		if err := json.Unmarshal(payload, &saved); err != nil {
			return err
		}
		loaded = saved.Snapshot
		if len(loaded.Projects) != 1 || filepath.Clean(loaded.Projects[0].Path) != checkout {
			return fmt.Errorf("project registration is not committed")
		}
		return nil
	}
	app.await("checkout project committed", readState)
	app.await("project picker closes after registration", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			if node.Label == "New project picker" {
				return fmt.Errorf("project picker is still open")
			}
		}
		return nil
	})
	app.await("created project is selected in the conversation composer", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err := tree.findKey("composer-project-picker")
		if err != nil {
			return err
		}
		if node.Label != "Select project: "+loaded.Projects[0].Name {
			return fmt.Errorf("composer project = %q, want %q", node.Label, loaded.Projects[0].Name)
		}
		return nil
	})
	app.capture("project-created")
	app.stop()
	// The file tree is a view of the running core, not persisted state: the
	// restarted app reads it from the Files panel of a real conversation.
	store, err := storage.Open(storage.Options{Directory: data})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	projectID := loaded.Projects[0].ID
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "File tree", ProjectID: projectID}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sessionID := snapshot.Sessions[0].ID
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := startApp(t, binary, artifacts, "restart", data, "--local")
	restarted.act("back-to-conversations", "press", "")
	restarted.waitKey("sidebar-project:" + projectID)
	restarted.act("sidebar-session:"+sessionID, "press", "")
	restarted.act("titlebar-files", "press", "")
	restarted.await("checkout file tree loaded independently", func() error {
		tree, err := restarted.client.tree()
		if err != nil {
			return err
		}
		if _, err := tree.findKey("project-files-loading:" + projectID); err == nil {
			return fmt.Errorf("file tree is still loading")
		}
		if _, err := tree.findKey("project-files-error:" + projectID); err == nil {
			return fmt.Errorf("file tree reports an error")
		}
		if _, err := tree.findKey(projectID + ":go.mod"); err != nil {
			return fmt.Errorf("checked out file missing from the tree: %w", err)
		}
		return nil
	})
	// Ignored output artifacts never reach the tree: only the checked-in
	// output/go.mod survives the ignore rules inside the output directory.
	restarted.act(projectID+":output", "expand", "")
	restarted.await("ignored output artifacts stay out of the file tree", func() error {
		tree, err := restarted.client.tree()
		if err != nil {
			return err
		}
		children := 0
		for _, node := range tree.Nodes {
			var key string
			if json.Unmarshal(node.Key, &key) != nil {
				continue
			}
			for _, artifact := range []string{"output/plugin-acceptance", "output/project-create-async-native"} {
				if strings.Contains(key, artifact) {
					return fmt.Errorf("ignored output artifact entered the file tree: %s", key)
				}
			}
			if !strings.HasPrefix(key, projectID+":output/") {
				continue
			}
			children++
			if key != projectID+":output/go.mod" {
				return fmt.Errorf("ignored entry entered the output directory: %s", key)
			}
		}
		if children == 0 {
			return fmt.Errorf("output directory did not expand")
		}
		return nil
	})
	restarted.capture("project-restored")
}
