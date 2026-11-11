//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// This saved-reply fixture checks native link recognition for an unindexed
// generated file in a conversation without a project. File menu actions are
// covered by ui.Tester; the automation plugin cannot operate OS context menus.
func TestDesktopLocalGeneratedFileLink(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("%s-%d", appearance, width), func(t *testing.T) {
				binary, artifacts := testPaths(t)
				data := filepath.Join(artifacts, "profile")
				store, err := storage.Open(storage.Options{Directory: data})
				if err != nil {
					t.Fatal(err)
				}
				core, err := openExecutionFixture(t, engine.Options{Storage: store})
				if err != nil {
					t.Fatal(err)
				}
				defer core.Close()
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Generated file link"}); err != nil {
					t.Fatal(err)
				}
				snapshot, err := core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				session := snapshot.Sessions[0]
				if session.ProjectID != "" {
					t.Fatal("fixture must not depend on a project index")
				}
				if err := core.Close(); err != nil {
					t.Fatal(err)
				}
				const name = "巴中市城乡供水一体化管理平台_平台与设备清单.xlsx"
				report := filepath.Join(session.CWD, name)
				if err := os.WriteFile(report, []byte("link recognition fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				editFixtureConversation(t, data, session.ID, func(_ *contracts.Session, messages *[]contracts.Message) {
					*messages = []contracts.Message{{ID: "generated", SessionID: session.ID, Role: "assistant",
						Blocks: []contracts.Block{{ID: "generated-path", Kind: "text", Text: "Excel 已生成：\n\n**`" + filepath.ToSlash(report) + "`**\n\n普通代码：`plain()`"}}}}
				})
				app := startApp(t, binary, artifacts, "generated-link", data, "--local", "--appearance", appearance,
					"--width", fmt.Sprint(width), "--height", "700")
				app.act("back-to-conversations", "press", "")
				app.act("sidebar-session:"+session.ID, "press", "")
				app.await("generated path is an actionable native link", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					input, err := tree.findKey("input")
					if err != nil {
						return err
					}
					if input.Bounds.Height > 50 {
						return fmt.Errorf("conversation composer is still docking: %+v", input.Bounds)
					}
					for _, node := range tree.Nodes {
						if node.Role == "link" && strings.Contains(node.Label, "plain()") {
							return fmt.Errorf("ordinary code became a file link")
						}
						if node.Role == "link" && node.Label == name && slices.Contains(node.Actions, "press") {
							if node.Bounds.Width <= 0 || node.Bounds.X < 0 || node.Bounds.X+node.Bounds.Width > float64(width)+1 ||
								node.Bounds.Y < 44 || node.Bounds.Y+node.Bounds.Height > input.Bounds.Y {
								return fmt.Errorf("generated file link escaped the visible pane: %+v", node.Bounds)
							}
							return nil
						}
					}
					return fmt.Errorf("generated absolute path has no link actions")
				})
				if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
					t.Fatal(err)
				}
				app.capture("generated-file-link")
			})
		}
	}
}
