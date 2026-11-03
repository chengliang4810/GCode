package desktop

import (
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

const gcodeGitHubURL = "https://github.com/chengliang4810/GCode"

func (a *App) aboutSettings(c *ui.Context) {
	services := c.Services()
	a.settingsCard(c, func() {
		ui.Column(c.Key("about-introduction")).FillWidth().Label(tr(c, "About GCode")).Padding(12, 16).Gap(16).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, "GCode").FontSize(design.TextSize(c, design.TextTitle)).FontWeight(600)
				ui.Text(c, tr(c, "A native AI coding workbench.")).
					FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted)
				ui.Text(c, tr(c, "GCode means “Go Code” — an invitation to start coding — and also stands for Go, the language it is built with.")).
					FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted)
			})
			ui.Row(c.Key("about-features")).FillWidth().Label(tr(c, "GCode features")).Wrap().AlignItems(ui.Start).Gap(16).Children(func() {
				for _, feature := range []struct{ key, title, description string }{
					{"models", "Choose any provider", "Three model protocols. Connect cloud or local models."},
					{"native", "Native desktop, built for coding", "Native Go UI with chat, files, terminal, and Git."},
					{"agents", "Goal-driven collaboration", "Subagents work together on multi-step coding tasks."},
					{"data", "Your data, local or self-hosted", "Local conversations, no account required. Self-hosting supported."},
					{"usage", "Transparent usage", "See tokens, cache hits, and output speed."},
					{"source", "Open source, in your hands", "Read, build, and customize the source."},
				} {
					ui.Column(c.Key("about-feature-" + feature.key)).Width(240).Grow(1).Gap(4).Children(func() {
						ui.Text(c, tr(c, feature.title)).FontSize(design.TextSize(c, design.TextBase)).FontWeight(600)
						ui.Text(c, tr(c, feature.description)).FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted)
					})
				}
			})
		})
	})
	a.settingsCard(c, func() {
		a.settingsDataRow(c, "GitHub", gcodeGitHubURL, func() {
			textButton(c.Key("about-github"), tr(c, "View on GitHub")).
				OnClick(func() { services.OpenURL(gcodeGitHubURL) })
		})
		line(c)
		a.settingsRow(c, "Support GCode", "If GCode helps you, give it a Star on GitHub so more people can discover it.", func() {
			primaryButton(c.Key("about-star"), tr(c, "Star on GitHub")).
				OnClick(func() { services.OpenURL(gcodeGitHubURL) })
		})
	})
	a.softwareUpdateSettings(c)
}
