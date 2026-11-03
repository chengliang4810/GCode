package desktop

import (
	"fmt"
	"math"
	"testing"

	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

func TestSidebarFooterSettingsAndUpdates(t *testing.T) {
	for _, language := range []string{i18n.Chinese, i18n.English} {
		for _, size := range []struct {
			width, height int
			dark          bool
		}{{900, 600, false}, {1320, 900, false}, {900, 600, true}, {1320, 900, true}} {
			t.Run(fmt.Sprintf("%s-%dx%d-dark=%v", language, size.width, size.height, size.dark), func(t *testing.T) {
				a := testApp(t, "default")
				a.state.Settings.General.Language = language
				a.state.Settings.Appearance.Mode = "light"
				a.state.Settings.Appearance.ThemeID = "rcode-light"
				if size.dark {
					a.state.Settings.Appearance.Mode = "dark"
					a.state.Settings.Appearance.ThemeID = "rcode-dark"
				}
				a.saveSettings()
				checks := 0
				a.checkForUpdates = func() { checks++ }
				tt := ui.NewTester(a.View, size.width, size.height)
				for _, label := range []string{a.t("Account menu: ") + a.state.User.Name, a.t("Sign out")} {
					if _, ok := tt.Find(label); ok {
						t.Fatalf("removed sidebar control remains: %q", label)
					}
				}
				labels := []string{"Check for updates…", "Mobile", "Usage statistics", "Settings"}
				var previous ui.Rect
				var gap float32
				for i, label := range labels {
					button, ok := tt.Find(a.t(label))
					if !ok || button.W != design.Controls().IconButtonSize || button.H != design.Controls().IconButtonSize || button.X < 0 || button.X+button.W > a.visibleSidebarWidth || button.Y+button.H > float32(size.height) {
						t.Fatalf("missing or overflowing footer action %q: %+v", label, button)
					}
					if i > 0 {
						currentGap := button.X - previous.X - previous.W
						if currentGap <= 0 || button.Y != previous.Y || (i > 1 && math.Abs(float64(currentGap-gap)) > 1) {
							t.Fatalf("footer spacing/order differs at %q: previous=%+v current=%+v gap=%v", label, previous, button, gap)
						}
						gap = currentGap
					}
					previous = button
				}
				if _, ok := tt.Find(a.t("Pull requests")); ok {
					t.Fatal("removed Git menu remains")
				}
				if checks != 0 {
					t.Fatal("rendering checked for updates")
				}
				if err := tt.Click(a.t("Check for updates…")); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if checks != 1 {
					t.Fatalf("one update click called the updater %d times", checks)
				}
				if err := tt.Click(a.t("Settings")); err != nil {
					t.Fatal(err)
				}
				if a.route != "settings" {
					t.Fatal("footer settings did not navigate")
				}
			})
		}
	}
}

func TestSidebarFooterMobileDialog(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 900, 600)
	for range 2 {
		if err := tt.Click(a.t("Mobile")); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !a.sidebarUI.mobileOpen || !tt.HasText(a.t("Mobile")) {
			t.Fatal("mobile action did not open its dialog")
		}
		if err := tt.Click(a.t("Close dialog")); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.sidebarUI.mobileOpen {
			t.Fatal("mobile dialog did not close")
		}
	}
}

func TestSidebarFooterWithoutUpdater(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 900, 600)
	if _, ok := tt.Find(a.t("Check for updates…")); !ok {
		t.Fatal("footer update action must always be visible")
	}
	if a.checkForUpdates != nil {
		t.Fatal("fixture unexpectedly has an updater")
	}
}
