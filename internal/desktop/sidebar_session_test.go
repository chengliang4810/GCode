package desktop

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarSessionMenuFlatCopyAndTrajectoryOrder(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, unread := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-unread%t", language, unread), func(t *testing.T) {
				a := testApp(t, "default")
				a.state.Settings.General.Language = language
				a.selectSession("chat-0")
				for i := range a.state.Sessions {
					if a.state.Sessions[i].ID == a.sessionID {
						a.state.Sessions[i].Unread = unread
					}
				}
				session := a.currentSession()
				tt := ui.NewTester(a.View, 1320, 880)
				open := func() {
					t.Helper()
					if err := rightClickAccessible(tt, ui.RoleButton, session.Title); err != nil {
						t.Fatal(err)
					}
				}
				open()
				mark := "Mark unread"
				if unread {
					mark = "Mark read"
				}
				menu := tt.Menu()
				index := slices.Index(menu, a.t(mark))
				if index < 0 || index+1 >= len(menu) || menu[index+1] != a.t("Call trajectory") {
					t.Fatalf("trajectory must follow the read action: %v", menu)
				}
				if slices.Contains(menu, a.t("Copy")) {
					t.Fatalf("copy submenu remains: %v", menu)
				}
				tt.CloseMenu()
				for _, copy := range []struct{ label, want string }{
					{"Copy title", session.Title},
					{"Copy session ID", session.ID},
					{"Copy branch", session.Branch},
				} {
					open()
					if err := tt.ChooseMenuItem(a.t(copy.label)); err != nil {
						t.Fatal(err)
					}
					if got := tt.Clipboard(); got != copy.want {
						t.Fatalf("%s copied %q, want %q", copy.label, got, copy.want)
					}
				}
			})
		}
	}
}
