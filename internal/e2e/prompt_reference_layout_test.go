//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// Display-only history in a private real core; no model request is made.
// Pixel alignment is asserted by the component test, and these native captures
// cover the mixed Latin/CJK message in both themes and a wrapped paragraph.
func TestDesktopLocalPromptReferenceAlignment(t *testing.T) {
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
			if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Reference alignment"}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			sessionID := snapshot.Sessions[0].ID
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			review := (contracts.PromptReference{Kind: "command", ID: "review", Label: "review"}).Markdown()
			docs := (contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}).Markdown()
			source := review + " 请检查 " + docs
			editFixtureConversation(t, data, sessionID, func(_ *contracts.Session, messages *[]contracts.Message) {
				*messages = []contracts.Message{
					{ID: "alignment-short", SessionID: sessionID, Role: "user",
						Blocks: []contracts.Block{{ID: "short", Kind: "text", Text: source}}},
					{ID: "alignment-wrapped", SessionID: sessionID, Role: "user",
						Blocks: []contracts.Block{{ID: "wrapped", Kind: "text", Text: strings.Repeat("中英文 mixed text ", 8) + source}}},
				}
			})
			app := startApp(t, binary, artifacts, "reference-layout", data,
				"--local", "--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "600")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			app.await("mixed reference labels are visible", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, "review 请检查 文档") {
						return nil
					}
				}
				return fmt.Errorf("mixed reference message is missing")
			})
			app.capture("reference-alignment")
		})
	}
}
