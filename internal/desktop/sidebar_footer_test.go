package desktop

import (
	"fmt"
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
				a.SetUpdateAvailable(true)
				tt := ui.NewTester(a.View, size.width, size.height)
				for _, label := range []string{a.t("Account menu: ") + a.state.User.Name, a.t("Sign out")} {
					if _, ok := tt.Find(label); ok {
						t.Fatalf("removed sidebar control remains: %q", label)
					}
				}
				settings, ok := tt.Find(a.t("Settings"))
				if !ok || settings.X > 40 {
					t.Fatalf("settings is not at the left of the footer: %+v", settings)
				}
				update, ok := tt.Find(a.t("Check for updates…"))
				if !ok || update.X <= settings.X+settings.W || update.X+update.W > a.visibleSidebarWidth+1 ||
					update.Y != settings.Y || update.W != design.Controls().IconButtonSize || update.H != settings.H {
					t.Fatalf("footer actions are not aligned at opposite ends: settings=%+v update=%+v sidebar=%v",
						settings, update, a.visibleSidebarWidth)
				}
				pr, prOK := tt.Find(a.t("Pull requests"))
				usage, usageOK := tt.Find(a.t("Usage statistics"))
				if !prOK || !usageOK || pr.X <= settings.X || usage.X <= pr.X ||
					usage.Y != settings.Y || pr.Y != settings.Y || usage.X+usage.W >= update.X {
					t.Fatalf("left actions are not ordered Settings/PR/Usage: %+v %+v %+v", settings, pr, usage)
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

func TestSidebarFooterHidesUpdateUntilNewVersionDetected(t *testing.T) {
	a := testApp(t, "default")
	checks := 0
	a.checkForUpdates = func() { checks++ }
	tt := ui.NewTester(a.View, 900, 600)
	label := a.t("Check for updates…")
	if _, ok := tt.Find(label); ok {
		t.Fatal("unknown update status exposed the footer action")
	}
	for range 2 {
		a.SetUpdateAvailable(true)
		tt.Frame()
		if _, ok := tt.Find(label); !ok {
			t.Fatal("detected new version did not expose the footer action")
		}
		a.SetUpdateAvailable(false)
		tt.Frame()
		if _, ok := tt.Find(label); ok {
			t.Fatal("footer action remained after the update became unavailable")
		}
	}
	if err := tt.Click(a.t("Settings")); err != nil {
		t.Fatal(err)
	}
	if button, ok := tt.Find(label); !ok || button.X < a.visibleSidebarWidth {
		t.Fatal("manual update check was lost from General settings")
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("manual check called updater %d times", checks)
	}
}

func TestSidebarFooterWithoutUpdater(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 900, 600)
	if _, ok := tt.Find(a.t("Check for updates…")); ok {
		t.Fatal("test/scenario app exposed an unavailable updater")
	}
	settings, ok := tt.Find(a.t("Settings"))
	if !ok || settings.X > 40 {
		t.Fatalf("settings moved when the updater is absent: %+v", settings)
	}
}
