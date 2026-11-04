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
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestDesktopLocalModelProviderMenuWidth(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("%s-%d", appearance, width), func(t *testing.T) {
				binary, artifacts := testPaths(t)
				data := filepath.Join(artifacts, "data")
				store, err := storage.Open(storage.Options{Directory: data})
				if err != nil {
					t.Fatal(err)
				}
				core, err := openExecutionFixture(t, engine.Options{Storage: store})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = core.Close() })
				provider := model.Provider{
					ID: "alpha", Name: "大碗饭", Protocol: model.Chat,
					BaseURL: "http://127.0.0.1:1/v1",
					Models:  []model.Model{{ID: "deepseek-v4.1-flash"}},
				}
				if err := core.Execute(context.Background(), contracts.Command{
					Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
				}); err != nil {
					t.Fatal(err)
				}
				if err := core.Close(); err != nil {
					t.Fatal(err)
				}
				app := startApp(t, binary, artifacts, "picker-width", data,
					"--local", "--appearance", appearance, "--language", "zh-CN",
					"--width", fmt.Sprint(width), "--height", "600")
				app.act("back-to-conversations", "press", "")
				app.act("model-trigger", "press", "")
				app.await("supplier card is at least as wide as the model trigger", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					trigger, err := tree.findKey("model-trigger")
					if err != nil {
						return err
					}
					row, err := tree.findKey("model-provider:alpha")
					if err != nil {
						return err
					}
					// The card has 4 DIP padding and a 1 DIP border on each side.
					if row.Bounds.Width+10 < trigger.Bounds.Width {
						return fmt.Errorf("supplier card is narrower than trigger: row=%+v trigger=%+v", row.Bounds, trigger.Bounds)
					}
					if math.Abs(row.Bounds.X+row.Bounds.Width/2-trigger.Bounds.X-trigger.Bounds.Width/2) > 1 {
						return fmt.Errorf("supplier card is not centered on trigger")
					}
					return nil
				})
				app.capture("provider-menu-width")
			})
		}
	}
}
