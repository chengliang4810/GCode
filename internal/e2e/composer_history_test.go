//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestDesktopLocalComposerHistory(t *testing.T) {
	for _, scene := range []struct {
		appearance, language, width string
	}{
		{"light", "en", "1320"},
		{"dark", "zh-CN", "900"},
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
			defer core.Close()
			const first, other = "Saved input history", "Other conversation"
			for _, title := range []string{first, other} {
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
			const oldest, newest = "First saved input", "最新输入🙂\nSecond line"
			const otherInput = "Only in the other conversation"
			ids := make(map[string]string)
			for _, session := range snapshot.Sessions {
				ids[session.Title] = session.ID
				editFixtureConversation(t, data, session.ID, func(_ *contracts.Session, messages *[]contracts.Message) {
					*messages = nil
					add := func(role, text string) {
						id := fmt.Sprintf("%s-%d", session.ID, len(*messages))
						*messages = append(*messages, contracts.Message{ID: id, SessionID: session.ID, Role: role,
							Blocks: []contracts.Block{{ID: id + "-text", Kind: "text", Text: text}}})
					}
					if session.Title == first {
						add("user", oldest)
						add("assistant", "Earlier answer")
						add("user", newest)
						add("assistant", "Latest answer is not an input")
					} else {
						add("user", otherInput)
					}
				})
			}
			app := startApp(t, binary, artifacts, "history", data, "--local", "--appearance", scene.appearance,
				"--language", scene.language, "--width", scene.width, "--height", "880")
			focusInput := func() {
				t.Helper()
				app.await("composer input focus", func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					input, err := tree.findKey("input")
					if err != nil || tree.Focus == input.ID {
						return err
					}
					if err := app.client.call("act", map[string]any{"key": "input", "action": "focus"}, nil); err != nil {
						return err
					}
					return fmt.Errorf("input focus not yet committed")
				})
			}
			waitValue := func(want string) {
				t.Helper()
				app.await(fmt.Sprintf("input equals %q", want), func() error {
					tree, err := app.client.tree()
					if err != nil {
						return err
					}
					input, err := tree.findKey("input")
					if err != nil {
						return err
					}
					if input.Value != want {
						return fmt.Errorf("input=%q, want %q", input.Value, want)
					}
					return nil
				})
			}
			key := func(key, want string) {
				t.Helper()
				focusInput()
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": key}}}, nil); err != nil {
					t.Fatal(err)
				}
				waitValue(want)
			}
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+ids[first], "press", "")
			app.waitKey("chat-transcript-list")
			waitValue("")
			key("Up", newest)
			key("Up", oldest)
			key("Down", newest)
			key("Down", "")
			const draft = "Keep this unsent draft"
			focusInput()
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "text", "text": draft}}}, nil); err != nil {
				t.Fatal(err)
			}
			waitValue(draft)
			key("Up", newest)
			key("Down", draft)
			key("Up", newest)
			app.act("sidebar-session:"+ids[other], "press", "")
			waitValue("")
			key("Up", otherInput)
			key("Down", "")
			// Capture after keyboard checks: native capture can change window focus.
			app.capture("other-conversation-draft")
		})
	}
}
