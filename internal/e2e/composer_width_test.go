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

func TestDesktopLocalComposerConversationWidth(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, windowWidth := range []int{900, 1680} {
			t.Run(fmt.Sprintf("%s-%d", appearance, windowWidth), func(t *testing.T) {
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
				settings := contracts.DefaultSettings()
				settings.Appearance.TranscriptWidth = 1200
				if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
					t.Fatal(err)
				}
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Conversation width"}); err != nil {
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
				editFixtureConversation(t, data, sessionID, func(_ *contracts.Session, messages *[]contracts.Message) {
					*messages = []contracts.Message{
						{ID: "width-user", SessionID: sessionID, Role: "user",
							Blocks: []contracts.Block{{ID: "width-prompt", Kind: "text", Text: "调整内容宽度后，输入框应同步调整。"}}},
						{ID: "width-assistant", SessionID: sessionID, Role: "assistant",
							Blocks: []contracts.Block{{ID: "width-reply", Kind: "text", Text: "这是一条用于检查对话布局的本地展示消息。请比较内容区域与底部输入框在不同宽度下的位置。"}}},
					}
				})
				app := startApp(t, binary, artifacts, "width", data, "--local", "--appearance", appearance,
					"--width", fmt.Sprint(windowWidth), "--height", "880")
				app.act("back-to-conversations", "press", "")
				app.act("sidebar-session:"+sessionID, "press", "")
				waitInfo := func(visible bool) {
					t.Helper()
					app.await("info card visibility", func() error {
						tree, err := app.client.tree()
						if err != nil {
							return err
						}
						found := false
						for _, node := range tree.Nodes {
							found = found || node.Role == "button" && node.Label == "Fork chat"
						}
						if found != visible {
							return fmt.Errorf("info card visible=%t, want %t", found, visible)
						}
						return nil
					})
				}
				// Pin the info card closed so this test measures the conversation
				// width independently of the card's automatic docking threshold.
				app.act("titlebar-session-info", "press", "")
				waitInfo(true)
				app.act("titlebar-session-info", "press", "")
				waitInfo(false)
				waitWidth := func(width int) {
					t.Helper()
					app.await("conversation width is persisted", func() error {
						saved, err := readFixtureSnapshot(data)
						if err != nil {
							return err
						}
						if got := saved.Settings.Appearance.TranscriptWidth; got != float64(width) {
							return fmt.Errorf("width=%v, want %d", got, width)
						}
						return nil
					})
				}
				for _, width := range []int{560, 1200, 736} {
					app.act("sidebar-settings", "press", "")
					app.act("settings-nav-Appearance", "press", "")
					app.act("conversation-width", "focus", "")
					key, currentWidth := "Home", 560
					if width == 1200 {
						key, currentWidth = "End", 1200
					}
					if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": key}}}, nil); err != nil {
						t.Fatal(err)
					}
					waitWidth(currentWidth)
					for currentWidth < width {
						app.act("conversation-width", "increment", "")
						currentWidth += 16
						waitWidth(currentWidth)
					}
					app.act("back-to-conversations", "press", "")
					app.act("sidebar-session:"+sessionID, "press", "")
					app.await("composer follows conversation width", func() error {
						tree, err := app.client.tree()
						if err != nil {
							return err
						}
						attach, err := tree.findKey("attach")
						if err != nil {
							return err
						}
						send, err := tree.findKey("send")
						if err != nil {
							return err
						}
						for _, pane := range tree.Nodes {
							if pane.Label != "Conversation pane" {
								continue
							}
							// Compact attach/send targets sit 9 DIP inside either
							// pill edge. Their geometry also proves both edges move.
							left, right := attach.Bounds.X-9, send.Bounds.X+send.Bounds.Width+9
							want := min(pane.Bounds.Width, float64(width)+32) - 32
							if math.Abs(right-left-want) > 1 ||
								math.Abs((left+right)/2-pane.Bounds.X-pane.Bounds.Width/2) > 1 {
								return fmt.Errorf("width %d: composer edges=%v,%v pane=%+v, want width=%v", width, left, right, pane.Bounds, want)
							}
							return nil
						}
						return fmt.Errorf("conversation pane missing")
					})
					app.capture(fmt.Sprintf("conversation-width-%d", width))
				}
			})
		}
	}
}
