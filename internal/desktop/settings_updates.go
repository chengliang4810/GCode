package desktop

import "github.com/egoist/mygo/ui"

// SetUpdateAvailable receives the updater's status on the UI thread.
func (a *App) SetUpdateAvailable(available bool) {
	if a.closed || a.updateAvailable == available {
		return
	}
	a.updateAvailable = available
	if a.win != nil {
		a.win.Invalidate()
	}
}

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
