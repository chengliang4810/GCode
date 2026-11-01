//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestDesktopLocalSidebarFooter(t *testing.T) {
	for _, test := range []struct {
		name, language, appearance, width string
	}{
		{"english-light", "en", "light", "1320"},
		{"chinese-dark-narrow", "zh-CN", "dark", "900"},
	} {
		t.Run(test.name, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			app := startApp(t, binary, artifacts, "local", filepath.Join(artifacts, "data"),
				"--local", "--language", test.language, "--appearance", test.appearance, "--width", test.width)
			for _, key := range []string{"sidebar-settings", "sidebar-pull-requests", "sidebar-usage"} {
				app.waitKey(key)
			}
			assertFooterUpdateHidden(t, app)
			app.act("back-to-conversations", "press", "")
			app.await("left settings action opens General", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("settings-search"); err == nil {
					return fmt.Errorf("settings has not closed")
				}
				return nil
			})
			app.capture("footer")
			app.act("sidebar-settings", "press", "")
			app.waitKey("Compact mode")
			app.act("sidebar-usage", "press", "")
			if node := app.waitKey("usage-total"); node.Label != "0" {
				t.Fatalf("empty profile usage total=%q, want 0", node.Label)
			}
			app.capture("usage")
			app.act("dialog-close", "press", "")
			app.await("usage dialog closed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("usage-total"); err == nil {
					return fmt.Errorf("usage dialog remains open")
				}
				return nil
			})
			app.act("check-for-updates", "press", "")
			app.waitKey("ok") // The raw E2E build cannot self-update or contact a feed.
			app.waitKey("title")
			app.capture("native-updater")
			app.act("ok", "press", "")
			app.waitKey("sidebar-settings")
			assertFooterUpdateHidden(t, app)
			app.capture("returned-settings")
		})
	}
}

func assertFooterUpdateHidden(t *testing.T, app *nativeApp) {
	t.Helper()
	tree, err := app.client.tree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.findKey("sidebar-updates"); err == nil {
		t.Fatal("the footer exposed an update before a new version was detected")
	}
}
