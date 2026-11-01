package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestSoftwareUpdatesUseDesktopCallback(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{900, 1320} {
				t.Run(fmt.Sprintf("%s/dark=%v/width=%d", language, dark, width), func(t *testing.T) {
					app, _ := settingsTestApp(t)
					app.section = "General"
					app.palette = design.Palette("rcode-light", false)
					if dark {
						app.palette = design.Palette("rcode-dark", true)
					}
					checks := 0
					app.checkForUpdates = func() { checks++ }
					tt := ui.NewTester(func(c *ui.Context) {
						localize.Set(c, language)
						c.SetTheme(app.palette.UI())
						app.settingsView(c)
					}, width, 800)
					tt.SetLocale(i18n.Native(language))
					tt.Frame()
					label := i18n.In(language, "Check for updates…")
					button, ok := tt.Find(label)
					if !ok || button.W <= 0 || button.X < 0 || button.X+button.W > float32(width) {
						t.Fatalf("missing or overflowing update button: %+v", button)
					}
					if checks != 0 {
						t.Fatal("rendering started an update check")
					}
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					tt.Frame()
					if checks != 1 {
						t.Fatalf("check called %d times for one click", checks)
					}
				})
			}
		}
	}
}

func TestSoftwareUpdatesAbsentWithoutDesktopUpdater(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.section = "General"
	app.palette = design.Palette("rcode-light", false)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.settingsView(c)
	}, 900, 800)
	if tt.HasText("Check for updates…") {
		t.Fatal("updater entry appeared without a native updater")
	}
	for _, link := range app.settingLinks() {
		if link.title == "Software updates" {
			t.Fatal("unavailable update entry appeared in settings search")
		}
	}
}
