//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestDesktopLocalProjectNewConversation(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct{ Role string }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if len(request.Messages) > 0 && request.Messages[len(request.Messages)-1].Role == "tool" {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Project file read.\"},\"finish_reason\":\"stop\"}]}\n\n")
				} else {
					fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"project-read","function":{"name":"Read","arguments":"{\"file_path\":\"marker.txt\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer provider.Close()
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			settings := contracts.DefaultSettings()
			settings.SidebarWidth = 350
			if appearance == "dark" {
				settings.SidebarWidth, settings.Appearance.FontSize = 264, 20
			}
			ctx := t.Context()
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "project", Name: "Project fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"first-project", "target-project"} {
				directory := filepath.Join(artifacts, name)
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "marker.txt"), []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
				if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: directory, Name: name}}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			projects := make(map[string]contracts.Project)
			for _, project := range snapshot.Projects {
				projects[project.Name] = project
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "session.create", ProjectID: projects["first-project"].ID, Name: "Existing conversation"}); err != nil {
				t.Fatal(err)
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			existingID := snapshot.Sessions[0].ID
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "project-new", data, "--local", "--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "1000")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+existingID, "press", "")
			app.await("existing conversation opens before entering its draft", func() error {
				title := app.waitKey("conversation-title")
				if title.Label != "Existing conversation" {
					return fmt.Errorf("current title=%q", title.Label)
				}
				return nil
			})
			app.act("input", "setValue", "保留旧对话草稿")
			app.await("old conversation draft is committed before navigation", func() error {
				if input := app.waitKey("input"); input.Value != "保留旧对话草稿" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
			target := projects["target-project"]
			rowKey := "sidebar-project-row:" + target.ID
			actionKey := "sidebar-project-new-conversation:" + target.ID
			assertAbsent := func() {
				t.Helper()
				tree, err := app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tree.findKey(actionKey); err == nil {
					t.Fatal("new-conversation action visible without project hover")
				}
			}
			assertAbsent()
			hover := func() accessNode {
				t.Helper()
				row := app.waitKey(rowKey)
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "pointerMove", "x": row.Bounds.X + 40, "y": row.Bounds.Y + row.Bounds.Height/2}}}, nil); err != nil {
					t.Fatal(err)
				}
				button := app.waitKey(actionKey)
				if button.Bounds.X+button.Bounds.Width > row.Bounds.X+row.Bounds.Width || button.Bounds.X < row.Bounds.X+row.Bounds.Width/2 {
					t.Fatalf("new button is not at project row's end: row=%+v button=%+v", row.Bounds, button.Bounds)
				}
				return button
			}
			button := hover()
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("project-hover")
			// Real pointer press/release must activate only the nested button.
			if err := app.client.call("input", map[string]any{"events": []map[string]any{
				{"type": "pointerMove", "x": button.Bounds.X + button.Bounds.Width/2, "y": button.Bounds.Y + button.Bounds.Height/2},
				{"type": "pointerDown", "x": button.Bounds.X + button.Bounds.Width/2, "y": button.Bounds.Y + button.Bounds.Height/2},
				{"type": "pointerUp", "x": button.Bounds.X + button.Bounds.Width/2, "y": button.Bounds.Y + button.Bounds.Height/2},
			}}, nil); err != nil {
				t.Fatal(err)
			}
			if label := app.waitKey("composer-project-picker").Label; !strings.Contains(label, target.Name) {
				t.Fatalf("new conversation did not select the clicked project: %s", label)
			}
			if input := app.waitKey("input"); input.Value != "" {
				t.Fatalf("new conversation reused the old draft: %s", input.Value)
			}
			app.waitKey("sidebar-empty:project:" + target.ID) // the button must not collapse the project.
			app.capture("project-targeted-canvas")
			app.act("input", "setValue", "清除项目时保留草稿")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			var clear accessNode
			app.await("hover exposes the project clear action after composer motion", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if clear, err = tree.findKey("composer-project-clear"); err == nil {
					return nil
				}
				picker, err := tree.findKey("composer-project-picker")
				if err != nil {
					return err
				}
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{
					"type": "pointerMove", "x": picker.Bounds.X + picker.Bounds.Width/2, "y": picker.Bounds.Y + picker.Bounds.Height/2,
				}}}, nil); err != nil {
					return err
				}
				return fmt.Errorf("project clear action not exposed yet")
			})
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("composer-project-hover-clear")
			clear = app.waitKey("composer-project-clear")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{
				{"type": "pointerMove", "x": clear.Bounds.X + clear.Bounds.Width/2, "y": clear.Bounds.Y + clear.Bounds.Height/2},
				{"type": "pointerDown", "x": clear.Bounds.X + clear.Bounds.Width/2, "y": clear.Bounds.Y + clear.Bounds.Height/2},
				{"type": "pointerUp", "x": clear.Bounds.X + clear.Bounds.Width/2, "y": clear.Bounds.Y + clear.Bounds.Height/2},
			}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("project clear keeps draft and shows no-project selection", func() error {
				if label := app.waitKey("composer-project-picker").Label; !strings.Contains(label, "不使用项目") {
					return fmt.Errorf("project selection=%s", label)
				}
				if input := app.waitKey("input"); input.Value != "清除项目时保留草稿" {
					return fmt.Errorf("draft changed on clear: %s", input.Value)
				}
				return nil
			})
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("composer-project-cleared")
			// Re-enter the same project through the existing sidebar action.
			hover()
			app.act(actionKey, "press", "")
			app.await("project can be explicitly selected again after clearing", func() error {
				if !strings.Contains(app.waitKey("composer-project-picker").Label, target.Name) {
					return fmt.Errorf("project was not reselected")
				}
				return nil
			})
			app.act("input", "setValue", "Read the project marker")
			app.act("send", "press", "")
			var createdID string
			app.await("new conversation reads the target project's real file", func() error {
				current, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range current.Sessions {
					if session.ID == existingID || session.ProjectID != target.ID || session.LastOutcome != "completed" {
						continue
					}
					if filepath.Clean(session.CWD) != filepath.Clean(target.Path) || session.DeviceID != target.DeviceID {
						return fmt.Errorf("wrong execution target: %+v", session)
					}
					for _, message := range current.Messages[session.ID] {
						for _, block := range message.Blocks {
							if block.Tool != nil && block.Tool.Name == "Read" && !block.Tool.IsError && strings.Contains(block.Tool.Output, "target-project") {
								createdID = session.ID
								return nil
							}
						}
					}
				}
				return fmt.Errorf("completed target-project read is missing")
			})
			app.waitKey("sidebar-session:" + createdID)
			app.capture("created-project-conversation")
			app.act("sidebar-session:"+existingID, "press", "")
			app.await("existing conversation draft is restored", func() error {
				if input := app.waitKey("input"); input.Value != "保留旧对话草稿" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
		})
	}
}

func TestDesktopLocalSidebarLayout(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
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
			first := app.waitKey("sidebar-project-row:" + firstID)
			second := app.waitKey("sidebar-project-row:" + secondID)
			for _, session := range snapshot.Sessions {
				if session.ProjectID == firstID && session.Title != "第一项目对话 0" && session.Title != "第一项目对话 1" {
					node := app.waitKey("sidebar-session:" + session.ID)
					if node.Bounds.Y <= first.Bounds.Y || node.Bounds.Y >= second.Bounds.Y || node.Bounds.X != first.Bounds.X {
						t.Fatalf("project conversation is outside its accordion: %+v", node.Bounds)
					}
				}
			}
			app.capture("initial")
			app.act("project:"+firstID+":more", "press", "")
			count(10)
			app.capture("show-more")
			app.act("input", "setValue", "保留当前输入")
			app.await("canvas draft is committed before project navigation", func() error {
				if input := app.waitKey("input"); input.Value != "保留当前输入" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
			app.act("sidebar-project:"+firstID, "press", "")
			count(3)
			heading := app.waitKey("sidebar-project-row:" + emptyID)
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
			app.act("section:projects:toggle", "press", "")
			count(1)
			app.capture("all-collapsed")
			app.act("section:projects:toggle", "press", "")
			count(1)
			app.act("sidebar-project:"+firstID, "press", "")
			app.act("sidebar-project:"+secondID, "press", "")
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
			app.await("back navigation restores the canvas draft", func() error {
				if input := app.waitKey("input"); input.Value != "保留当前输入" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
		})
	}
}
