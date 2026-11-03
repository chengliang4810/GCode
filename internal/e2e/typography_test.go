//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// Real native windows and the local core, with private settings and no provider.
func TestDesktopLocalTypography(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, size := range []float64{12, 14, 20} {
			t.Run(fmt.Sprintf("%s-%g", appearance, size), func(t *testing.T) {
				binary, artifacts := testPaths(t)
				data := filepath.Join(artifacts, "data")
				store, err := storage.Open(storage.Options{Directory: data})
				if err != nil {
					t.Fatal(err)
				}
				core, err := engine.Open(engine.Options{Storage: store})
				if err != nil {
					_ = store.Close()
					t.Fatal(err)
				}
				settings := contracts.DefaultSettings()
				settings.Appearance.FontSize = size
				settings.General.Language = "en"
				err = core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings})
				closeErr := core.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("private typography settings: %v %v", err, closeErr)
				}
				app := startApp(t, binary, artifacts, "typography", data, "--local",
					"--appearance", appearance, "--width", "900", "--height", "700")
				checkHeading := func(section string) {
					app.await("native typography for "+section, func() error {
						tree, err := app.client.tree()
						if err != nil {
							return err
						}
						for _, node := range tree.Nodes {
							if node.Label == "Settings page heading: "+section {
								want := (size + 10) * 4 / 3
								if math.Abs(node.Bounds.Height-want) > 1 {
									return fmt.Errorf("heading height %g, want %g at UI font %g", node.Bounds.Height, want, size)
								}
								return nil
							}
						}
						return fmt.Errorf("heading %s is not visible", section)
					})

				}
				app.waitKey("data-directory")
				checkHeading("General")
				app.capture("general")
				app.act("settings-nav-Model settings", "press", "")
				app.waitKey("add-provider")
				checkHeading("Model settings")
				app.capture("models")
				app.act("settings-nav-General", "press", "")
				app.waitKey("Compact mode")
				app.act("back-to-conversations", "press", "")
				app.waitKey("model-trigger")
				app.capture("conversation")
				app.stop()
			})
		}
	}
}
