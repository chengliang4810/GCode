//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalGlobalInstructionsPersistence(t *testing.T) {
	binary, artifacts := testPaths(t)
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			directory := filepath.Join(artifacts, appearance)
			data := filepath.Join(directory, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			err = core.WriteGlobalInstructions(t.Context(), "original host instructions")
			closeErr := core.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("seed host instructions: %v %v", err, closeErr)
			}
			path := filepath.Join(data, "AGENTS.md")
			app := startApp(t, binary, directory, "edit", data, "--local", "--appearance", appearance, "--width", "900")
			app.act("settings-nav-Personalization", "press", "")
			waitValue := func(app *nativeApp, value string) {
				t.Helper()
				app.await("global instructions loaded from host", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					node, err := tree.findKey("agents-draft")
					if err != nil || node.Value != value {
						return fmt.Errorf("instructions value is not %q: %v", value, err)
					}
					return nil
				})
			}
			waitValue(app, "original host instructions")
			app.act("agents-draft", "focus", "")
			want := "保存到宿主的全局指令 " + appearance
			app.act("agents-draft", "setValue", want)
			app.act("settings-nav-General", "press", "")
			app.await("global instructions persisted after leaving settings", func() error {
				contents, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if string(contents) != want {
					return fmt.Errorf("instructions were not saved: %q", contents)
				}
				return nil
			})
			app.stop()
			reopened := startApp(t, binary, directory, "restart", data, "--local", "--appearance", appearance, "--width", "900")
			reopened.act("settings-nav-Personalization", "press", "")
			waitValue(reopened, want)
			reopened.capture("host-instructions-restored")
		})
	}
}
