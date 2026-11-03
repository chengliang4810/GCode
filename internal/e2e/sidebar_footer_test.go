//go:build e2e

package e2e

import (
	"fmt"
	"math"
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
			for _, key := range []string{"sidebar-updates", "sidebar-mobile", "sidebar-usage", "sidebar-settings"} {
				app.waitKey(key)
			}
			assertFooterActions(t, app)
			app.act("back-to-conversations", "press", "")
			app.await("left settings action opens General", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("settings-nav-General"); err == nil {
					return fmt.Errorf("settings has not closed")
				}
				return nil
			})
			app.capture("footer")
			app.act("sidebar-mobile", "press", "")
			app.waitKey("dialog-close")
			app.capture("mobile-dialog")
			app.act("dialog-close", "press", "")
			app.await("mobile dialog closed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("dialog-close"); err == nil {
					return fmt.Errorf("mobile dialog remains open")
				}
				return nil
			})
			app.act("sidebar-updates", "press", "")
			app.waitKey("ok")
			app.capture("footer-update-check")
			app.act("ok", "press", "")
			app.act("sidebar-settings", "press", "")
			app.waitKey("Compact mode")
			app.act("sidebar-usage", "press", "")
			if node := app.waitKey("usage-total"); node.Label != "0" {
				t.Fatalf("empty profile usage total=%q, want 0", node.Label)
			}
			app.capture("usage")
			app.waitKey("usage-range-7")
			app.act("usage-range-30", "press", "")
			app.act("usage-heatmap-weekly", "press", "")
			app.act("usage-heatmap-cumulative", "press", "")
			app.act("usage-heatmap-daily", "press", "")
			app.act("back-to-conversations", "press", "")
			app.await("usage settings closed", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("usage-total"); err == nil {
					return fmt.Errorf("usage settings remain open")
				}
				return nil
			})
			app.act("sidebar-settings", "press", "")
			app.act("settings-nav-General", "press", "")
			app.waitKey("Compact mode")
			tree, err := app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tree.findKey("check-for-updates"); err == nil {
				t.Fatal("software updates remain in General")
			}
			app.act("settings-nav-About", "press", "")
			app.waitKey("about-github")
			app.waitKey("about-star")
			app.capture("about")
			app.act("check-for-updates", "press", "")
			app.waitKey("ok") // The raw E2E build cannot self-update or contact a feed.
			app.waitKey("title")
			app.capture("native-updater")
			app.act("ok", "press", "")
			app.waitKey("sidebar-settings")
			assertFooterActions(t, app)
			app.capture("returned-settings")
		})
	}
}

func assertFooterActions(t *testing.T, app *nativeApp) {
	t.Helper()
	tree, err := app.client.tree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.findKey("sidebar-pull-requests"); err == nil {
		t.Fatal("removed Git menu remains")
	}
	var previous accessNode
	var gap float64
	for i, key := range []string{"sidebar-updates", "sidebar-mobile", "sidebar-usage", "sidebar-settings"} {
		node, err := tree.findKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			currentGap := node.Bounds.X - previous.Bounds.X - previous.Bounds.Width
			if currentGap <= 0 || math.Abs(node.Bounds.Y-previous.Bounds.Y) > 1 || (i > 1 && math.Abs(currentGap-gap) > 1) {
				t.Fatalf("footer is not evenly ordered: previous=%+v current=%+v gap=%v", previous.Bounds, node.Bounds, gap)
			}
			gap = currentGap
		}
		previous = node
	}
}
