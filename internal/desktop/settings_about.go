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
			// Capabilities stay one line each: the about page states what the
			// build is, and the settings pages themselves explain how it works.
			ui.Column(c.Key("about-features")).FillWidth().Label(tr(c, "GCode capabilities")).Gap(4).Children(func() {
				for _, key := range []string{
					"Three model protocols: OpenAI Chat Completions, OpenAI Responses and Anthropic Messages, with providers you configure yourself.",
					"Native desktop UI over the same Go core: chat, files, terminal, Git and subagents.",
					"Local conversations by default; the core also runs headless on a server you own.",
				} {
					ui.Text(c.Key("about-capability-"+key[:12]), tr(c, key)).
						FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted).LineHeight(1.5)
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
