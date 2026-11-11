//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Use a private real local core. Selecting the current device needs no
// configured model, remote connection or provider request.
func TestDesktopLocalDeviceMenu(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			app := startApp(t, binary, artifacts, "device-menu", filepath.Join(artifacts, "data"),
				"--local", "--appearance", appearance, "--language", "en", "--width", "900", "--height", "600")
			app.act("back-to-conversations", "press", "")
			const draft = "KEEP DEVICE MENU DRAFT"
			app.act("input", "setValue", draft)
			app.assertLinkDraft(draft)
			menuItems := func(tree accessTree) []accessNode {
				var items []accessNode
				for _, node := range tree.Nodes {
					if strings.HasPrefix(strings.ToLower(node.Role), "menuitem") {
						items = append(items, node)
					}
				}
				return items
			}
			app.act("composer-device-picker", "focus", "")
			app.act("composer-device-picker", "press", "")
			app.await("framework device choices appear", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if len(menuItems(tree)) == 0 {
					return fmt.Errorf("device menu has no native menu items")
				}
				return nil
			})
			app.capture("device-menu-open")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{
				{"type": "key", "key": "Escape"},
			}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("Escape closes device menu and restores trigger focus", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				trigger, err := tree.findKey("composer-device-picker")
				if err != nil {
					return err
				}
				if len(menuItems(tree)) != 0 || tree.Focus != trigger.ID {
					return fmt.Errorf("items=%d focus=%s trigger=%s", len(menuItems(tree)), tree.Focus, trigger.ID)
				}
				return nil
			})
			app.act("composer-device-picker", "press", "")
			app.await("select current device from framework menu", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				items := menuItems(tree)
				if len(items) != 1 {
					return fmt.Errorf("private local host device choices=%d, want 1", len(items))
				}
				return app.client.call("act", map[string]any{"id": items[0].ID, "action": "press"}, nil)
			})
			app.await("selecting current device closes the menu", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if len(menuItems(tree)) != 0 {
					return fmt.Errorf("selected device menu remains visible")
				}
				return nil
			})
			app.assertLinkDraft(draft)
			app.capture("device-menu-selected")
		})
	}
}
