//go:build e2e

package e2e

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"gcode/internal/design"
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
			footer := app.waitKey("sidebar-footer")
			seam := footer.Bounds.X + footer.Bounds.Width
			target := float64(design.SidebarMax) - 1
			for _, event := range []map[string]any{
				{"type": "pointerMove", "x": seam, "y": 200},
				{"type": "pointerDown", "x": seam, "y": 200},
				{"type": "pointerMove", "x": target, "y": 200},
			} {
				if err := app.client.call("input", map[string]any{"events": []map[string]any{event}}, nil); err != nil {
					t.Fatal(err)
				}
			}
			app.await("widened sidebar retains fixed footer gaps and right alignment", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				footer, err := tree.findKey("sidebar-footer")
				if err != nil {
					return err
				}
				if math.Abs(footer.Bounds.Width-target) > 1 {
					return fmt.Errorf("sidebar footer width=%v, want %v", footer.Bounds.Width, target)
				}
				return nil
			})
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "pointerUp", "x": target, "y": 200,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			assertFooterActions(t, app)
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("wide-sidebar-footer")
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
	for i, key := range []string{"sidebar-updates", "sidebar-mobile", "sidebar-usage", "sidebar-settings"} {
		node, err := tree.findKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			currentGap := node.Bounds.X - previous.Bounds.X - previous.Bounds.Width
			if math.Abs(node.Bounds.Y-previous.Bounds.Y) > 1 || math.Abs(currentGap-float64(design.Sidebar().FooterGap)) > 1 {
				t.Fatalf("footer spacing/order differs: previous=%+v current=%+v gap=%v", previous.Bounds, node.Bounds, currentGap)
			}
		}
		previous = node
	}
	footer, err := tree.findKey("sidebar-footer")
	if err != nil {
		t.Fatal(err)
	}
	if margin := footer.Bounds.X + footer.Bounds.Width - previous.Bounds.X - previous.Bounds.Width; math.Abs(margin-12) > 1 {
		t.Fatalf("footer icons are not right aligned: right margin=%v", margin)
	}
}
