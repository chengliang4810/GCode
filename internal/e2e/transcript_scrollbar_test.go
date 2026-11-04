//go:build e2e

package e2e

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalTranscriptScrollbarEdge(t *testing.T) {
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
				t.Fatal(err)
			}
			defer core.Close()
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Scrollbar history"}); err != nil {
				t.Fatal(err)
			}
			state, _ := core.Snapshot(t.Context())
			settings := state.Settings
			settings.Appearance.ReduceMotion = true
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
				t.Fatal(err)
			}
			sessionID := state.Sessions[0].ID
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			editFixtureConversation(t, data, sessionID, func(_ *contracts.Session, messages *[]contracts.Message) {
				*messages = nil
				for turn := range 3 {
					userID := fmt.Sprintf("user-%d", turn)
					*messages = append(*messages, contracts.Message{ID: userID, SessionID: sessionID, Role: "user",
						Blocks: []contracts.Block{{ID: userID + "-text", Kind: "text", Text: fmt.Sprintf("Scrollbar prompt %d", turn)}}})
					reply := contracts.Message{ID: fmt.Sprintf("reply-%d", turn), SessionID: sessionID, Role: "assistant"}
					for paragraph := range 30 {
						reply.Blocks = append(reply.Blocks, contracts.Block{ID: fmt.Sprintf("text-%d-%d", turn, paragraph),
							Kind: "text", Text: fmt.Sprintf("Scrollbar reply %d paragraph %d", turn, paragraph)})
					}
					*messages = append(*messages, reply)
				}
			})
			app := startApp(t, binary, artifacts, "scrollbar", data, "--local", "--appearance", appearance,
				"--language", "en", "--width", "1500", "--height", "800")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.waitKey("chat-transcript-list")
			app.await("information card is docked beside the reading column", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, "SIDE CHATS") {
					return fmt.Errorf("information card missing")
				}
				return nil
			})
			var viewport accessNode
			checkEdge := func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				viewport, err = tree.findKey("chat-transcript-list")
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Label == "Conversation pane" {
						if math.Abs(viewport.Bounds.X-node.Bounds.X) > 1 ||
							math.Abs(viewport.Bounds.X+viewport.Bounds.Width-node.Bounds.X-node.Bounds.Width) > 1 {
							return fmt.Errorf("viewport=%+v pane=%+v", viewport.Bounds, node.Bounds)
						}
						return nil
					}
				}
				return fmt.Errorf("conversation pane missing")
			}
			app.await("scroll viewport reaches the pane edge with information open", checkEdge)
			app.act("input", "focus", "")
			app.act("input", "setValue", "KEEP SCROLLBAR DRAFT")
			app.await("draft committed", func() error {
				if input := app.waitKey("input"); input.Value != "KEEP SCROLLBAR DRAFT" {
					return fmt.Errorf("draft=%q", input.Value)
				}
				return nil
			})
			x, y := viewport.Bounds.X+viewport.Bounds.Width-5, viewport.Bounds.Y+viewport.Bounds.Height/2
			if err := app.client.call("input", map[string]any{"events": []map[string]any{
				{"type": "pointerMove", "x": x, "y": y},
				{"type": "wheel", "x": x, "y": y, "dy": -1000, "precise": true},
			}}, nil); err != nil {
				t.Fatal(err)
			}
			app.waitKey("transcript-latest")
			app.capture("info-open-scrollbar-at-edge")
			app.act("transcript-latest", "press", "")
			app.act("titlebar-session-info", "press", "")
			app.await("scroll viewport keeps the same edge with information hidden", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if activityTreeContains(tree, "SIDE CHATS") {
					return fmt.Errorf("information card still visible")
				}
				return checkEdge()
			})
			app.capture("info-hidden")
			app.act("titlebar-right-pane", "press", "")
			app.await("scroll viewport follows the conversation pane beside the workspace", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, "Tools workspace") {
					return fmt.Errorf("workspace is not visible")
				}
				return checkEdge()
			})
			// The split divider has a 20 DIP drag target across its line.
			// Exercise scrolling on the conversation side of that target.
			x, y = viewport.Bounds.X+viewport.Bounds.Width-12, viewport.Bounds.Y+viewport.Bounds.Height/2
			if err := app.client.call("input", map[string]any{"events": []map[string]any{
				{"type": "pointerMove", "x": x, "y": y},
				{"type": "wheel", "x": x, "y": y, "dy": -1000, "precise": true},
			}}, nil); err != nil {
				t.Fatal(err)
			}
			app.waitKey("transcript-latest")
			if input := app.waitKey("input"); input.Value != "KEEP SCROLLBAR DRAFT" {
				t.Fatal("panel toggling discarded the draft")
			}
			app.capture("workspace-open")
		})
	}
}
