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
					navigation, ok := tt.Find(app.t("Settings navigation"))
					if !ok {
						t.Fatal("settings navigation missing")
					}
					tt.Scroll(navigation.X+navigation.W/2, navigation.Y+navigation.H/2, 0, 1000)
					tt.Frame()
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
					// Capabilities are one line each inside the introduction card:
					// the page states what the build is, not a marketing grid.
					capabilities := []string{
						"Three model protocols: OpenAI Chat Completions, OpenAI Responses and Anthropic Messages, with providers you configure yourself.",
						"Native desktop UI over the same Go core: chat, files, terminal, Git and subagents.",
						"Local conversations by default; the core also runs headless on a server you own.",
					}
					for _, line := range capabilities {
						bounds, ok := tt.Find(app.t(line))
						if !ok || bounds.X < introduction.X || bounds.X+bounds.W > introduction.X+introduction.W ||
							bounds.Y < introduction.Y || bounds.Y+bounds.H > introduction.Y+introduction.H {
							t.Fatalf("About capability missing or outside its card: %+v, card=%+v", bounds, introduction)
						}
					}
					if tt.HasText(app.t("Choose any provider")) {
						t.Fatal("About still renders the marketing feature grid")
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
