package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) runtimeDeviceSettings(c *ui.Context) {
	for _, device := range a.state.Devices {
		a.settingsCard(c, func() {
			a.settingsRow(c, device.Name, devicePlatform(device.Platform), func() { ui.Text(c, tr(c, "Online")).TextColor(a.palette.Muted) })
		})
	}
	if a.connections == nil {
		return
	}
	state := &a.runtimeUI
	a.settingsCard(c, func() {
		ui.Column(c).Padding(16).Gap(10).Children(func() {
			controlTextInput(c.Key("remote-host"), &state.remoteURL).Label(tr(c, "Remote host")).Placeholder("https://host:27655")
			controlTextInput(c.Key("remote-token"), &state.remoteToken).Label(tr(c, "Remote API token")).Placeholder(tr(c, "Leave empty to keep the saved token")).Password()
			ui.Row(c).Gap(8).Children(func() {
				textButton(c, tr(c, "Connect after restart")).Disabled(state.connectionSaving).OnClick(func() { a.configureConnection(true) })
				textButton(c, tr(c, "Disconnect after restart")).Disabled(state.connectionSaving || !state.connection.Enabled && state.connection.ActiveURL == "").OnClick(func() { a.configureConnection(false) })
			})
			if state.connection.Pending {
				ui.Text(c, tr(c, "Restart the desktop app to apply the connection change. Host tasks continue running.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			}
			if state.connection.ActiveURL != "" {
				ui.Text(c, state.connection.ActiveURL).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			}
		})
	})
}
func (a *App) configureConnection(enabled bool) {
	if a.connections == nil || a.runtimeUI.connectionSaving {
		return
	}
	input := contracts.ConnectionInput{URL: a.runtimeUI.remoteURL, Token: a.runtimeUI.remoteToken, Enabled: enabled, PreserveToken: a.runtimeUI.remoteToken == ""}
	service, ctx := a.connections, a.ctx
	a.runtimeUI.connectionSaving = true
	a.backgroundTask(func() func() {
		err := service.ConfigureConnection(ctx, input)
		info, infoErr := service.ConnectionInfo(ctx)
		return func() {
			a.runtimeUI.connectionSaving = false
			if err != nil {
				a.errorText = err.Error()
				return
			}
			if infoErr != nil {
				a.errorText = infoErr.Error()
				return
			}
			a.runtimeUI.connection = info
			a.runtimeUI.remoteToken = ""
		}
	})
}
