package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

func TestAccountMenuStaysAboveFooterInsideWindow(t *testing.T) {
	for _, language := range []string{i18n.Chinese, i18n.English} {
		for _, size := range []struct{ width, height int }{{900, 600}, {1320, 900}} {
			t.Run(fmt.Sprintf("%s-%dx%d", language, size.width, size.height), func(t *testing.T) {
				a := testApp(t, "default")
				a.state.Settings.General.Language = language
				a.saveSettings()
				tt := ui.NewTester(a.View, size.width, size.height)
				trigger := a.t("Account menu: ") + a.state.User.Name
				if err := tt.Click(trigger); err != nil {
					t.Fatal(err)
				}
				check := func(width, height int) {
					t.Helper()
					panel, ok := tt.Find(a.t("Account menu"))
					if !ok || panel.W <= 0 || panel.H <= 0 {
						t.Fatal("account menu is not in the application window")
					}
					anchor, ok := tt.Find(trigger)
					if !ok {
						t.Fatal("account trigger disappeared")
					}
					if panel.X < 0 || panel.Y < 0 || panel.X+panel.W > float32(width) || panel.Y+panel.H > float32(height) {
						t.Fatalf("menu outside window: %+v", panel)
					}
					if panel.Y+panel.H > anchor.Y {
						t.Fatalf("menu did not open above footer: menu=%+v anchor=%+v", panel, anchor)
					}
					if _, ok := tt.Find(a.t("Sign out")); !ok {
						t.Fatal("sign-out item missing")
					}
				}
				check(size.width, size.height)
				tt.SetSize(900, 600)
				check(900, 600)
				tt.Key(0, ui.KeyEscape)
				if _, ok := tt.Find(a.t("Account menu")); ok {
					t.Fatal("Escape did not dismiss the menu")
				}
				if err := tt.Click(trigger); err != nil {
					t.Fatal(err)
				}
				tt.ClickAt(500, 50)
				if _, ok := tt.Find(a.t("Account menu")); ok {
					t.Fatal("outside click did not dismiss the menu")
				}
			})
		}
	}
}

func TestAccountMenuKeyboardAndActions(t *testing.T) {
	for _, signOut := range []bool{false, true} {
		t.Run(fmt.Sprintf("sign-out-%t", signOut), func(t *testing.T) {
			a := testApp(t, "default")
			tt := ui.NewTester(a.View, 900, 600)
			if err := tt.Click("Account menu: " + a.state.User.Name); err != nil {
				t.Fatal(err)
			}
			if signOut {
				tt.Key(0, ui.KeyDown)
			}
			tt.Key(0, ui.KeyEnter)
			if _, ok := tt.Find("Account menu"); ok {
				t.Fatal("selected menu action did not dismiss it")
			}
			if signOut {
				if a.state.User.SignedIn {
					t.Fatal("sign out did not reach the typed backend")
				}
			} else if a.route != "settings" {
				t.Fatal("settings action did not navigate")
			}
		})
	}
}
