//go:build e2e

package e2e

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDesktopLocalSidebarEmpty(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			app := startApp(t, binary, artifacts, "empty-sidebar", filepath.Join(artifacts, "data"),
				"--local", "--language", "en", "--appearance", appearance, "--width", "900")
			app.act("back-to-conversations", "press", "")
			empty := app.waitKey("sidebar-empty:projects")
			if empty.Label != "No projects added" {
				t.Fatalf("empty projects label=%q", empty.Label)
			}
			app.await("empty conversations section hidden", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("section:conversations"); err == nil {
					return fmt.Errorf("empty conversations section remains visible")
				}
				return nil
			})
			toggle := app.waitKey("sidebar-toggle")
			back := app.waitKey("sidebar-back")
			if math.Abs(toggle.Bounds.Y+toggle.Bounds.Height/2-back.Bounds.Y-back.Bounds.Height/2) > 0.5 {
				t.Fatal("toolbar controls do not share a center line")
			}
			if runtime.GOOS == "darwin" {
				// Window traffic lights are positioned at (14,14), in the native 16 DIP frame.
				if math.Abs(toggle.Bounds.Y+toggle.Bounds.Height/2-22) > 1 {
					t.Fatalf("toolbar center does not match traffic lights: %+v", toggle.Bounds)
				}
				newChat := app.waitKey("sidebar-new-conversation")
				if math.Abs(newChat.Bounds.X+10+8-22) > 0.5 {
					t.Fatalf("new conversation icon does not match close button column: %+v", newChat.Bounds)
				}
				if toggle.Bounds.X < 80 {
					t.Fatalf("sidebar toggle crowds traffic lights: %+v", toggle.Bounds)
				}
			}
			app.capture("empty-sidebar")
		})
	}
}
