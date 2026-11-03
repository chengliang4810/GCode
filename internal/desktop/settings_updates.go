package desktop

import "github.com/egoist/mygo/ui"

func (a *App) softwareUpdateSettings(c *ui.Context) {
	if a.checkForUpdates == nil {
		return
	}
	a.settingsCard(c, func() {
		a.settingsRow(c, "Software updates", "", func() {
			textButton(c.Key("check-for-updates"), tr(c, "Check for updates…")).
				OnClick(a.checkForUpdates)
		})
	})
}
