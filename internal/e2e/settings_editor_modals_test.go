//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"

	"gcode/internal/i18n"
)

func TestDesktopLocalResourceEditorModals(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, mode := range []struct{ name, language, appearance, width, height string }{
		{"wide-light", "en", "light", "1320", "880"},
		{"narrow-dark", "zh-CN", "dark", "900", "600"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			app := startApp(t, binary, artifacts, mode.name, filepath.Join(artifacts, mode.name, "data"), "--local", "--language", mode.language, "--appearance", mode.appearance, "--width", mode.width, "--height", mode.height)
			tr := func(value string) string { return i18n.In(mode.language, value) }
			for _, page := range []struct{ section, add, edit, title, nameField, name, bodyField, body string }{
				{"MCP servers", "Add MCP server", "Edit MCP server", "modal-server", "mcp-name", "modal-server", "mcp-command", "gcode-modal-unused-fixture"},
				{"Commands", "Add command", "Edit command", "/modal-review", "command-name", "modal-review", "command-content", "Review the supplied files."},
				{"Hooks", "Add hook", "Edit hook", "Modal hook", "hook-name", "Modal hook", "hook-command", "echo modal"},
			} {
				app.act("settings-nav-"+page.section, "press", "")
				pluginActLabel(app, tr(page.add))
				app.await("native modal "+page.section, func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					found := false
					for _, node := range tree.Nodes {
						if node.Role == "dialog" && node.Label == tr(page.add) {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("modal dialog missing")
					}
					if _, err := tree.findKey("resource-search-" + page.section); err == nil {
						return fmt.Errorf("background search is still interactive")
					}
					return nil
				})
				app.act(page.nameField, "setValue", page.name)
				app.act(page.bodyField, "setValue", page.body)
				app.capture(page.section + "-create-modal")
				app.act("editor-save", "press", "")
				app.await("closed saved modal "+page.section, func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					if _, err := tree.findKey("editor-save"); err == nil {
						return fmt.Errorf("editor did not close")
					}
					_, err = tree.findKey("resource-search-" + page.section)
					return err
				})
				edit := tr("Edit") + " " + page.title
				pluginActLabel(app, edit)
				app.waitKey(page.nameField)
				app.act(page.nameField, "setValue", "Unsaved modal draft")
				app.act("editor-cancel", "press", "")
				pluginActLabel(app, edit)
				app.await("cancel preserved stored value", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					node, err := tree.findKey(page.nameField)
					if err != nil {
						return err
					}
					if node.Value != page.name {
						return fmt.Errorf("name=%q want %q", node.Value, page.name)
					}
					return nil
				})
				app.capture(page.section + "-edit-modal")
				app.act("dialog-close", "press", "")
				app.waitKey("resource-search-" + page.section)
			}
			app.stop()
		})
	}
}
