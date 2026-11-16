package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSettingsReturnPreservesConversation(t *testing.T) {
	for _, entry := range []string{"Settings", "Usage statistics"} {
		for _, selected := range []bool{false, true} {
			for _, dark := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/selected=%v/dark=%v", entry, selected, dark), func(t *testing.T) {
					a := testApp(t, "default")
					a.state.Settings.Appearance.Mode = "light"
					if dark {
						a.state.Settings.Appearance.Mode = "dark"
					}
					id := ""
					if selected {
						id = "chat-0"
						a.navigate("chat/" + id)
					}
					a.draft = "未发送的草稿"
					tt := ui.NewTester(a.View, 900, 700)
					if err := tt.Click(a.t(entry)); err != nil {
						t.Fatal(err)
					}
					if a.route != "settings" {
						t.Fatal("settings entry did not open settings")
					}
					if err := tt.Click(a.t("Back to conversations")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if a.route != "chat" || a.sessionID != id || a.draft != "未发送的草稿" {
						t.Fatalf("return lost conversation or draft: route=%q id=%q draft=%q", a.route, a.sessionID, a.draft)
					}
					a.back(-1)
					a.back(1)
					if a.route != "chat" || a.sessionID != id {
						t.Fatal("history lost the returned conversation")
					}
					a.OpenRoute("settings")
					a.newConversation()
					if a.route != "chat" || a.sessionID != "" || a.draft != "" {
						t.Fatal("explicit new conversation did not open an empty canvas")
					}
				})
			}
		}
	}
}

func TestSettingsReturnAfterConversationDeleted(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.OpenRoute("settings")
	a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
	tt := ui.NewTester(a.View, 900, 700)
	if err := tt.Click(a.t("Back to conversations")); err != nil {
		t.Fatal(err)
	}
	if a.route != "chat" || a.sessionID != "" {
		t.Fatal("return retained deleted conversation")
	}
}
