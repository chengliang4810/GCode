//go:build e2e

package e2e

import (
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

// These are saved-history fixtures reopened through the real local core and a
// visible native window. They do not execute a model request.
func TestDesktopLocalMessageRail(t *testing.T) {
	for _, scene := range []struct {
		name, appearance, language string
		width                      int
	}{
		{"light-wide", "light", "en", 1320},
		{"dark-wide", "dark", "zh-CN", 1320},
		{"narrow", "light", "en", 900},
	} {
		t.Run(scene.name, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			for _, title := range []string{"Native rail history", "Single rail turn"} {
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: title}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.Appearance.ReduceMotion = true
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			var sessionID, singleID string
			for _, session := range snapshot.Sessions {
				id, turns := session.ID, 3
				if session.Title == "Single rail turn" {
					singleID, turns = id, 1
				} else {
					sessionID = id
				}
				editFixtureConversation(t, data, id, func(_ *contracts.Session, messages *[]contracts.Message) {
					*messages = nil
					for turn := range turns {
						userID := fmt.Sprintf("%s-user-%02d", id, turn)
						*messages = append(*messages, contracts.Message{ID: userID, SessionID: id, Role: "user",
							Blocks: []contracts.Block{{ID: userID + "-text", Kind: "text", Text: fmt.Sprintf("Native prompt %02d 中文🙂", turn)}}})
						reply := contracts.Message{ID: fmt.Sprintf("%s-reply-%02d", id, turn), SessionID: id, Role: "assistant"}
						for paragraph := range 24 {
							reply.Blocks = append(reply.Blocks, contracts.Block{
								ID: fmt.Sprintf("%s-%02d", reply.ID, paragraph), Kind: "text",
								Text: fmt.Sprintf("Native reply %02d paragraph %02d", turn, paragraph),
							})
						}
						*messages = append(*messages, reply)
					}
				})
			}
			app := startApp(t, binary, artifacts, "rail", data, "--local", "--appearance", scene.appearance,
				"--language", scene.language, "--width", fmt.Sprint(scene.width), "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			list := app.waitKey("chat-transcript-list")
			app.await("saved history is visible", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, "Native reply 02 paragraph 23") {
						return nil
					}
				}
				return fmt.Errorf("saved final reply has not appeared")
			})
			if scene.name == "narrow" {
				tree, err := app.client.tree()
				if err != nil {
					t.Fatal(err)
				}
				if list.Bounds.Width >= 768 {
					t.Fatalf("narrow fixture has a wide conversation: %+v", list.Bounds)
				}
				if _, err := tree.findKey("message-rail"); err == nil {
					t.Fatal("outline overlaps a narrow conversation")
				}
				app.capture("narrow-hidden")
				return
			}
			rail := app.waitKey("message-rail")
			first := app.waitKey("message-rail-tick:0")
			third := app.waitKey("message-rail-tick:2")
			if math.Abs(first.Bounds.X-list.Bounds.X-16) > 1 || first.Bounds.Width != 26 ||
				first.Bounds.Height != 10 || third.Bounds.Y-first.Bounds.Y != 26 {
				t.Fatalf("native outline geometry is wrong: first=%+v third=%+v list=%+v", first.Bounds, third.Bounds, list.Bounds)
			}
			if rail.Bounds.Y+rail.Bounds.Height > app.waitKey("input").Bounds.Y {
				t.Fatal("outline extends into the composer")
			}
			const draft = "KEEP NATIVE RAIL DRAFT"
			app.act("input", "setValue", draft)
			app.await("draft has committed before measuring hover geometry", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				input, err := tree.findKey("input")
				if err != nil {
					return err
				}
				if input.Value != draft {
					return fmt.Errorf("draft is still committing")
				}
				return nil
			})
			app.capture("compact-outline")
			app.await("outline preview appears beside its current mark", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if preview, err := tree.findKey("message-rail-tooltip"); err == nil {
					if strings.Contains(preview.Label, "Native prompt 00 中文🙂") &&
						strings.Contains(preview.Label, "Native reply 00 paragraph 00") {
						return nil
					}
					return fmt.Errorf("preview does not contain its own prompt and reply: %q", preview.Label)
				}
				mark, err := tree.findKey("message-rail-tick:0")
				if err != nil {
					return err
				}
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{
					"type": "pointerMove", "x": mark.Bounds.X + mark.Bounds.Width/2, "y": mark.Bounds.Y + mark.Bounds.Height/2,
				}}}, nil); err != nil {
					return err
				}
				return fmt.Errorf("hover preview has not appeared")
			})
			app.capture("hover-preview")
			first = app.waitKey("message-rail-tick:0")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "click", "x": first.Bounds.X + first.Bounds.Width/2, "y": first.Bounds.Y + first.Bounds.Height/2,
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			waitMessageRailSelection(app, 0)
			app.capture("first-prompt")
			app.act("message-rail-tick:1", "focus", "")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{
				"type": "key", "key": "Enter",
			}}}, nil); err != nil {
				t.Fatal(err)
			}
			waitMessageRailSelection(app, 1)
			app.capture("keyboard-second-prompt")
			app.act("transcript-latest", "press", "")
			waitMessageRailSelection(app, 2)
			app.capture("back-to-latest")
			app.act("sidebar-session:"+singleID, "press", "")
			app.await("single-turn conversation hides the outline", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("message-rail"); err == nil {
					return fmt.Errorf("outline is still mounted")
				}
				return nil
			})
			app.capture("single-turn-hidden")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.waitKey("message-rail")
			if input := app.waitKey("input"); input.Value != draft {
				t.Fatalf("outline navigation lost the session draft: %q", input.Value)
			}
			app.capture("restored-session")
		})
	}
}

func waitMessageRailSelection(app *nativeApp, index int) {
	app.t.Helper()
	app.await("outline follows its reading position", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err := tree.findKey(fmt.Sprintf("message-rail-tick:%d", index))
		if err != nil {
			return err
		}
		if !slices.Contains(node.States, "checked") {
			return fmt.Errorf("mark %d is not active: %v", index, node.States)
		}
		return nil
	})
}
