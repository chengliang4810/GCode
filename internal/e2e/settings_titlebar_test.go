//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDesktopSettingsTitlebarSidebarMotion(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			app := startApp(t, binary, artifacts, mode, filepath.Join(artifacts, mode+"-data"),
				"--local", "--appearance", mode, "--width", "900", "--height", "600")
			app.act("settings-nav-Appearance", "press", "")
			app.waitKey("appearance-mode-dark")
			// The sidebar's first button already clears the platform's native controls.
			minimumX := app.waitKey("sidebar-toggle").Bounds.X
			openX := app.waitKey("settings-title").Bounds.X
			var samples []accessNode
			for step, open := range []bool{false, true, false} {
				key := "sidebar-toggle"
				if open {
					key = "titlebar-sidebar"
				}
				app.act(key, "press", "")
				deadline := time.Now().Add(3 * time.Second)
				for {
					tree, err := app.client.tree()
					if err != nil {
						t.Fatal(err)
					}
					title, err := tree.findKey("settings-title")
					if err != nil {
						t.Fatal(err)
					}
					samples = append(samples, title)
					if title.Bounds.X < minimumX-1 {
						t.Fatalf("open=%t: title x=%g entered native control area ending at %g", open, title.Bounds.X, minimumX)
					}
					_, collapsed := tree.findKey("titlebar-sidebar")
					if open && title.Bounds.X >= openX-1 || !open && collapsed == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("sidebar did not finish open=%t", open)
					}
				}
				app.capture(fmt.Sprintf("%d-sidebar-open-%t", step, open))
			}
			data, err := json.MarshalIndent(samples, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(app.artifacts, "title-motion.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
		})
	}
}
