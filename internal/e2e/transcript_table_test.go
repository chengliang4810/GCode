//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalMarkdownTables(t *testing.T) {
	for _, scene := range []struct {
		appearance, language string
		width, height        int
	}{
		{"light", "en", 900, 600}, {"dark", "zh-CN", 1320, 880},
	} {
		t.Run(scene.appearance, func(t *testing.T) {
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
			if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Native Markdown tables"}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			sessionID := snapshot.Sessions[0].ID
			settings := snapshot.Settings
			settings.Appearance.ReduceMotion = true
			if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			source := "A table with Chinese text and inline formatting.\n\n| 支柱 | 当前状态 |\n|---|---|\n| 修订感知 | `internal/storage/store.go` 已有 **revision + CAS** |\n| 事件日志 | [说明](https://example.com/docs) 与 `Runtime.Events()` 可读回 |\n\n"
			source += "| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 | Column 6 |\n|---|---|---|---|---|---|\n"
			for i := range 40 {
				source += fmt.Sprintf("| row %d long-value-abcdefghijklmnopqrstuvwxyz | second long-value-abcdefghijklmnopqrstuvwxyz | third | fourth | fifth | final long-value-abcdefghijklmnopqrstuvwxyz |\n", i)
			}
			editFixtureConversation(t, data, sessionID, func(_ *contracts.Session, messages *[]contracts.Message) {
				*messages = []contracts.Message{{ID: "native-table-message", SessionID: sessionID, Role: "assistant",
					Blocks: []contracts.Block{{ID: "native-table-block", Kind: "text", Text: source}}}}
			})
			app := startApp(t, binary, artifacts, "tables", data, "--local", "--appearance", scene.appearance, "--language", scene.language,
				"--width", fmt.Sprint(scene.width), "--height", fmt.Sprint(scene.height))
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			var previews []string
			app.await("both Markdown tables expose independent actions", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				previews = tableActionKeys(tree, "table-preview:")
				if len(previews) != 2 || len(tableActionKeys(tree, "table-copy:")) != 2 || len(tableActionKeys(tree, "table-export:")) != 2 {
					return fmt.Errorf("table controls missing: previews=%v", previews)
				}
				return nil
			})
			// Accessibility scroll actions are scoped to the test's own window.
			scrollTableActionIntoView(app, previews[0])
			app.capture("inline-table")
			app.act(previews[0], "press", "")
			firstID := strings.TrimPrefix(previews[0], "table-preview:")
			app.waitKey("table-preview-close:" + firstID)
			app.capture("table-preview")
			app.act("table-preview-close:"+firstID, "press", "")

			scrollTableActionIntoView(app, previews[1])
			secondID := strings.TrimPrefix(previews[1], "table-preview:")
			app.await("wide table scrollbar stays above the composer", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				bar, err := tree.findKey("table-horizontal-scroll:" + secondID)
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if bar.Bounds.Y+bar.Bounds.Height > input.Bounds.Y-4 {
					return fmt.Errorf("scrollbar overlaps input: bar=%+v input=%+v", bar.Bounds, input.Bounds)
				}
				return nil
			})
			app.act("table-horizontal-scroll:"+secondID, "increment", "")
			app.capture("wide-table-scroll")
			tree, err := app.client.tree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tree.findKey("table-expand-width:" + secondID); err == nil {
				before := app.waitKey("table-frame:" + secondID)
				app.act("table-expand-width:"+secondID, "press", "")
				app.await("wide table borrows available conversation width", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					frame, err := tree.findKey("table-frame:" + secondID)
					if err != nil {
						return err
					}
					if frame.Bounds.Width <= before.Bounds.Width+16 || frame.Bounds.X < 0 || frame.Bounds.X+frame.Bounds.Width > float64(scene.width)+1 {
						return fmt.Errorf("invalid expanded frame: before=%+v after=%+v", before.Bounds, frame.Bounds)
					}
					bar, err := tree.findKey("table-horizontal-scroll:" + secondID)
					if err != nil {
						return err
					}
					if math.Abs(bar.Bounds.X-frame.Bounds.X) > 1 || bar.Bounds.X+bar.Bounds.Width > float64(scene.width)+1 {
						return fmt.Errorf("expanded scrollbar escaped its table: bar=%+v frame=%+v", bar.Bounds, frame.Bounds)
					}
					return nil
				})
				app.capture("expanded-width")
			}
			app.act(previews[1], "press", "")
			header := app.waitKey("table-preview-header:" + secondID)
			content := app.waitKey("table-preview-content:" + secondID)
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "wheel", "x": content.Bounds.X + content.Bounds.Width/2, "y": content.Bounds.Y + content.Bounds.Height/2, "dy": 250, "precise": true,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("preview keeps its header fixed while body scrolls", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				after, err := tree.findKey("table-preview-header:" + secondID)
				if err != nil {
					return err
				}
				if math.Abs(after.Bounds.Y-header.Bounds.Y) > 1 {
					return fmt.Errorf("header moved")
				}
				return nil
			})
			app.capture("wide-table-preview")
			app.act("table-preview-close:"+secondID, "press", "")
		})
	}
}

func tableActionKeys(tree accessTree, prefix string) []string {
	var keys []string
	for _, node := range tree.Nodes {
		var key string
		if json.Unmarshal(node.Key, &key) == nil && strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys
}

func scrollTableActionIntoView(app *nativeApp, key string) {
	app.await("table action is in the visible conversation", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err := tree.findKey(key)
		if err != nil {
			return err
		}
		input, err := tree.findKey("input")
		if err != nil {
			return err
		}
		if node.Bounds.Y >= 48 && node.Bounds.Y+node.Bounds.Height <= input.Bounds.Y-8 {
			return nil
		}
		if slices.Contains(node.Actions, "scrollIntoView") {
			if err := app.client.call("act", map[string]any{"key": key, "action": "scrollIntoView"}, nil); err != nil {
				return err
			}
			return fmt.Errorf("waiting for table action to become visible")
		}
		delta := node.Bounds.Y - 72
		if node.Bounds.Y > input.Bounds.Y {
			delta = node.Bounds.Y - input.Bounds.Y + 64
		}
		if err := app.client.call("input", map[string]any{"events": []map[string]any{{
			"type": "wheel", "x": input.Bounds.X + input.Bounds.Width/2, "y": 100, "dy": delta, "precise": true,
		}}}, nil); err != nil {
			return err
		}
		return fmt.Errorf("scrolling table toolbar into view")
	})
}
