//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalSidebarLayout(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
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
			t.Cleanup(func() { _ = core.Close() })
			settings := contracts.DefaultSettings()
			settings.SidebarWidth = 350
			settings.Appearance.FontSize = 14
			if appearance == "dark" {
				settings.Appearance.FontSize = 20
				settings.SidebarWidth = 264
			}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"first-project", "second-project", "third-project"} {
				path := filepath.Join(artifacts, name)
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := core.Execute(context.Background(), contracts.Command{
					Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: path},
				}); err != nil {
					t.Fatal(err)
				}
			}
			projectSnapshot, err := core.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			firstID, secondID, emptyID := projectSnapshot.Projects[0].ID, projectSnapshot.Projects[1].ID, projectSnapshot.Projects[2].ID
			for i := range 10 {
				projectID, title := firstID, fmt.Sprintf("第一项目对话 %d", i)
				if i >= 7 {
					projectID, title = secondID, fmt.Sprintf("第二项目对话 %d", i)
				}
				if i == 9 {
					projectID, title = "", "独立对话"
				}
				if err := core.Execute(context.Background(), contracts.Command{
					Kind: "session.create", Name: title, ProjectID: projectID,
				}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(context.Background())
			if err != nil || len(snapshot.Projects) != 3 || len(snapshot.Sessions) != 10 {
				t.Fatalf("sidebar fixture: projects=%d sessions=%d err=%v", len(snapshot.Projects), len(snapshot.Sessions), err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "sidebar", data, "--local",
				"--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "1000")
			app.act("back-to-conversations", "press", "")
			app.waitKey("sidebar-new-conversation")
			app.waitKey("sidebar-search")
			app.waitKey("sidebar-project:" + snapshot.Projects[0].ID)
			count := func(want int) {
				app.await("visible conversation count", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					n := 0
					for _, node := range tree.Nodes {
						var key string
						_ = json.Unmarshal(node.Key, &key)
						if strings.HasPrefix(key, "sidebar-session:") {
							n++
						}
					}
					if n != want {
						return fmt.Errorf("visible conversations=%d, want %d", n, want)
					}
					return nil
				})
			}
			count(8)
			first := app.waitKey("sidebar-project:" + firstID)
			second := app.waitKey("sidebar-project:" + secondID)
			for _, session := range snapshot.Sessions {
				if session.ProjectID == firstID && session.Title != "第一项目对话 0" && session.Title != "第一项目对话 1" {
					node := app.waitKey("sidebar-session:" + session.ID)
					if node.Bounds.Y <= first.Bounds.Y || node.Bounds.Y >= second.Bounds.Y || node.Bounds.X <= first.Bounds.X {
						t.Fatalf("project conversation is outside its accordion: %+v", node.Bounds)
					}
				}
			}
			app.capture("initial")
			app.act("project:"+firstID+":more", "press", "")
			count(10)
			app.capture("show-more")
			app.act("input", "setValue", "保留当前输入")
			app.act("sidebar-project:"+firstID, "press", "")
			count(3)
			heading := app.waitKey("sidebar-project:" + emptyID)
			empty := app.waitKey("sidebar-empty:project:" + emptyID)
			gap := empty.Bounds.Y - (heading.Bounds.Y + heading.Bounds.Height)
			if gap < 0 || gap > 16 {
				t.Fatalf("empty state is detached from its heading: gap=%g heading=%+v empty=%+v",
					gap, heading.Bounds, empty.Bounds)
			}
			if input := app.waitKey("input"); input.Value != "保留当前输入" {
				t.Fatalf("project accordion changed the draft: %q", input.Value)
			}
			app.capture("collapsed-project")
			app.act("sidebar-project:"+firstID, "press", "")
			count(10)
			app.act("sidebar-toggle-projects", "press", "")
			count(1)
			app.capture("all-collapsed")
			app.act("sidebar-toggle-projects", "press", "")
			count(10)
			app.act("sidebar-project:"+firstID, "press", "")
			app.act("sidebar-search", "press", "")
			app.waitKey("command-search")
			app.act("command-search", "setValue", "第一项目对话 0")
			app.capture("search")
			var target contracts.Session
			for _, session := range snapshot.Sessions {
				if session.Title == "第一项目对话 0" {
					target = session
					break
				}
			}
			app.act("thread:"+target.ID, "press", "")
			selected := app.waitKey("sidebar-session:" + target.ID)
			if selected.Bounds.X >= settings.SidebarWidth || selected.Bounds.Y+selected.Bounds.Height > 1000-54 {
				t.Fatalf("search did not reveal the collapsed project's conversation: %+v", selected.Bounds)
			}
			app.capture("search-revealed-project")
			// Returning to the new-conversation canvas restores its untouched draft.
			app.act("sidebar-back", "press", "")
			if input := app.waitKey("input"); input.Value != "保留当前输入" {
				t.Fatalf("conversation navigation discarded the canvas draft: %q", input.Value)
			}
		})
	}
}
