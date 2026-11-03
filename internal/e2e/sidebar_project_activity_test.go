//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopScenarioCollapsedProjectActivity(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			app := startApp(t, binary, artifacts, "project-activity", filepath.Join(artifacts, "data"), "--scenario", "loading", "--appearance", appearance, "--width", "900")
			app.act("back-to-conversations", "press", "")
			app.waitKey("sidebar-project:fieldnotes")
			app.act("sidebar-project:fieldnotes", "press", "")
			app.await("collapsed project reports running", func() error {
				node := app.waitKey("sidebar-project:fieldnotes")
				if !strings.Contains(node.Label, "Running") {
					return fmt.Errorf("project label=%q", node.Label)
				}
				return nil
			})
			app.capture("collapsed-project-running")
			app.act("sidebar-project:fieldnotes", "press", "")
			app.await("expanded project hides aggregate activity", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if node, err := tree.findKey("sidebar-project:fieldnotes"); err == nil && strings.Contains(node.Label, "Running") {
					return fmt.Errorf("aggregate indicator remains after expansion")
				}
				return nil
			})
		})
	}
}
