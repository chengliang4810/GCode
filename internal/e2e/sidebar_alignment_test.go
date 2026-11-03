//go:build e2e

package e2e

import (
	"math"
	"path/filepath"
	"testing"
)

func TestDesktopScenarioSidebarAlignment(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			app := startApp(t, binary, artifacts, "sidebar-alignment", filepath.Join(artifacts, "data"), "--scenario", "default", "--appearance", appearance, "--width", "900")
			app.act("back-to-conversations", "press", "")
			navigation := app.waitKey("sidebar-new-conversation")
			for _, key := range []string{"group:pinned:toggle", "section:projects:toggle", "section:conversations:toggle", "sidebar-project:fieldnotes", "sidebar-session:chat-0"} {
				node := app.waitKey(key)
				if math.Abs(node.Bounds.X-navigation.Bounds.X) > 0.5 {
					t.Fatalf("%s left=%g, navigation left=%g", key, node.Bounds.X, navigation.Bounds.X)
				}
			}
			app.capture("shared-left-rail")
		})
	}
}
