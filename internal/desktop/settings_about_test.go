package desktop

import (
	"fmt"
	"slices"
	"testing"

	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

func TestSettingsAboutNavigationLinksAndUpdates(t *testing.T) {
	for _, language := range []string{i18n.English, i18n.Chinese} {
		for _, appearance := range []string{"light", "dark"} {
			for _, width := range []int{900, 1320} {
				t.Run(fmt.Sprintf("%s-%s-%d", language, appearance, width), func(t *testing.T) {
					app, _ := settingsTestApp(t)
					app.route, app.section = "settings", "General"
					app.state.Settings.General.Language = language
					app.state.Settings.Appearance.Mode = appearance
					app.state.Settings.Appearance.ThemeID = "rcode-" + appearance
					app.saveSettings()
					checks := 0
					app.checkForUpdates = func() { checks++ }
					sections := app.sections()
					if sections[len(sections)-1] != "About" {
						t.Fatalf("last settings section = %q", sections[len(sections)-1])
					}
					tt := ui.NewTester(app.View, width, 800)
					if tt.HasText(app.t("Software updates")) {
						t.Fatal("software updates remain in General")
					}
					if err := tt.Click(app.t("About")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					for _, text := range []string{"GCode", gcodeGitHubURL, app.t("Support GCode"), app.t("Software updates")} {
						if !tt.HasText(text) {
							t.Fatalf("missing About content: %q", text)
						}
					}
					introduction, ok := tt.Find(app.t("About GCode"))
					if !ok || !tt.HasText(app.t("GCode means “Go Code” — an invitation to start coding — and also stands for Go, the language it is built with.")) {
						t.Fatal("grouped introduction or GCode name meaning is missing")
					}
					for _, title := range []string{"Choose any provider", "Native desktop, built for coding", "Goal-driven collaboration", "Your data, local or self-hosted", "Transparent usage", "Open source, in your hands"} {
						bounds, ok := tt.Find(app.t(title))
						if !ok || bounds.X < introduction.X || bounds.X+bounds.W > introduction.X+introduction.W ||
							bounds.Y < introduction.Y || bounds.Y+bounds.H > introduction.Y+introduction.H {
							t.Fatalf("About feature %q missing or outside its card: %+v, card=%+v", title, bounds, introduction)
						}
					}
					for _, label := range []string{"View on GitHub", "Star on GitHub", "Check for updates…"} {
						bounds, ok := tt.Find(app.t(label))
						if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width) || bounds.Y+bounds.H > 800 {
							t.Fatalf("About action %q missing or overflowing: %+v", label, bounds)
						}
					}
					if checks != 0 || len(tt.OpenedURLs()) != 0 {
						t.Fatal("rendering triggered an external action")
					}
					for _, label := range []string{"View on GitHub", "Star on GitHub"} {
						if err := tt.Click(app.t(label)); err != nil {
							t.Fatal(err)
						}
						tt.Frame()
					}
					if !slices.Equal(tt.OpenedURLs(), []string{gcodeGitHubURL, gcodeGitHubURL}) {
						t.Fatalf("GitHub actions opened %v", tt.OpenedURLs())
					}
					if err := tt.Click(app.t("Check for updates…")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if checks != 1 {
						t.Fatalf("update action called %d times", checks)
					}
				})
			}
		}
	}
}

func TestSettingsAboutWithoutUpdater(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "About"
	sections := app.sections()
	if sections[len(sections)-1] != "About" {
		t.Fatal("About must be last for the real runtime too")
	}
	tt := ui.NewTester(app.View, 900, 600)
	if tt.HasText("Software updates") {
		t.Fatal("unavailable updater action is visible")
	}
	if !tt.HasText("View on GitHub") || !tt.HasText("Star on GitHub") {
		t.Fatal("GitHub links must remain available without an updater")
	}
}
