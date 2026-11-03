//go:build e2e

package e2e

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalProjectSelectionMenu(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "profile")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			settings := contracts.DefaultSettings()
			if appearance == "dark" {
				settings.Appearance.FontSize = 20
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			for i := range 18 {
				name := fmt.Sprintf("Project %02d", i)
				if i == 16 {
					name = "中文项目"
				}
				directory := filepath.Join(artifacts, fmt.Sprintf("project-%02d", i))
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create",
					ProjectCreate: &contracts.ProjectCreate{Path: directory, Name: name}}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var target contracts.Project
			for _, p := range snapshot.Projects {
				if p.Name == "中文项目" {
					target = p
				}
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "select", data, "--local", "--appearance", appearance,
				"--language", "zh-CN", "--width", "900", "--height", "800")
			idle := func() {
				t.Helper()
				if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
					t.Fatal(err)
				}
			}
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-new-conversation", "press", "")
			idle()
			app.act("input", "setValue", "切换项目时保留草稿")
			app.await("draft entered before project selection", func() error {
				if node := app.waitKey("input"); node.Value != "切换项目时保留草稿" {
					return fmt.Errorf("draft=%q", node.Value)
				}
				return nil
			})
			app.act("composer-project-picker", "press", "")
			footer := app.waitKey("composer-project-new")
			app.waitKey("search")
			idle()
			assertProjectMenu := func(hasProject bool) {
				t.Helper()
				group := app.waitKey("composer-project-picker-group")
				panel := app.waitKey("composer-project-menu")
				if math.Abs(panel.Bounds.X-group.Bounds.X) > 1 ||
					panel.Bounds.Y+panel.Bounds.Height > group.Bounds.Y-4 {
					t.Fatalf("project popup must align with the group and open above it: group=%+v popup=%+v", group.Bounds, panel.Bounds)
				}
				picker := app.waitKey("composer-project-picker")
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{
					"type": "pointerMove", "x": picker.Bounds.X + picker.Bounds.Width/2,
					"y": picker.Bounds.Y + picker.Bounds.Height/2,
				}}}, nil); err != nil {
					t.Fatal(err)
				}
				tree, err := app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tree.findKey("composer-project-clear"); err == nil {
					t.Fatal("open project popup exposed the hover clear shortcut")
				}
				_, noneError := tree.findKey("composer-project-none")
				if (noneError == nil) != hasProject {
					t.Fatalf("no-project action visibility does not match selected project: hasProject=%t", hasProject)
				}
			}
			assertProjectMenu(true)
			app.capture("project-menu")
			list := app.waitKey("composer-project-list")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "wheel", "x": list.Bounds.X + list.Bounds.Width/2,
				"y": list.Bounds.Y + list.Bounds.Height/2, "dy": 10000, "precise": true,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			lastProject := snapshot.Projects[len(snapshot.Projects)-1]
			app.waitKey("composer-project-option:" + lastProject.ID)
			if after := app.waitKey("composer-project-new"); after.Bounds != footer.Bounds {
				t.Fatal("scrolling projects moved the fixed footer")
			}
			idle()
			app.capture("project-menu-scrolled")
			// Search uses the visible native text input and preserves fixed actions.
			app.act("search", "setValue", "中文")
			app.waitKey("composer-project-option:" + target.ID)
			if after := app.waitKey("composer-project-new"); math.Abs(after.Bounds.Y-footer.Bounds.Y) > 1 {
				t.Fatal("filtering projects moved the footer away from the selector")
			}
			idle()
			app.capture("project-menu-search")
			app.act("composer-project-option:"+target.ID, "press", "")
			app.await("searched project becomes the composer target", func() error {
				node := app.waitKey("composer-project-picker")
				if node.Label != "选择项目: 中文项目" {
					return fmt.Errorf("project label=%q", node.Label)
				}
				return nil
			})
			app.act("composer-project-picker", "press", "")
			app.waitChecked("composer-project-option:"+target.ID, true)
			idle()
			assertProjectMenu(true)
			app.capture("project-menu-selected")
			app.act("composer-project-none", "press", "")
			app.await("project cleared without losing draft", func() error {
				if node := app.waitKey("composer-project-picker"); node.Label != "选择项目: 不使用项目" {
					return fmt.Errorf("project label=%q", node.Label)
				}
				if node := app.waitKey("input"); node.Value != "切换项目时保留草稿" {
					return fmt.Errorf("draft=%q", node.Value)
				}
				return nil
			})
			app.act("composer-project-picker", "press", "")
			idle()
			assertProjectMenu(false)
			app.capture("project-menu-without-project")
			app.act("composer-project-new", "press", "")
			app.waitKey("project-picker-row:local")
			idle()
			app.capture("new-project-opened")
		})
	}
}
