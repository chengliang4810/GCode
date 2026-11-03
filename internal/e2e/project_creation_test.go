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
	app.await("checkout file tree loaded independently", func() error {
		if err := readState(); err != nil {
			return err
		}
		if loaded.Projects[0].FilesState != contracts.LoadReady {
			return fmt.Errorf("files state %q: %s", loaded.Projects[0].FilesState, loaded.Projects[0].FilesError)
		}
		if len(loaded.Files) == 0 {
			return fmt.Errorf("file list is empty")
		}
		for _, file := range loaded.Files {
			if file.Path == "output/plugin-acceptance" || strings.HasPrefix(file.Path, "output/plugin-acceptance/") ||
				file.Path == "output/project-create-async-native" || strings.HasPrefix(file.Path, "output/project-create-async-native/") {
				return fmt.Errorf("ignored output artifact entered the file tree: %s", file.Path)
			}
		}
		return nil
	})
	app.capture("project-created")
	app.stop()
	restarted := startApp(t, binary, artifacts, "restart", data, "--local")
	restarted.act("back-to-conversations", "press", "")
	restarted.waitKey("sidebar-project:" + loaded.Projects[0].ID)
	restarted.capture("project-restored")
}
