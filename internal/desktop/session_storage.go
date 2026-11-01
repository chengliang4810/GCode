package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) sessionStorageNotice(c *ui.Context, session contracts.Session) {
	if session.StorageError == "" {
		return
	}
	ui.Column(c.Key("session-storage-error:"+session.ID)).FillWidth().Padding(12, 24).Gap(8).Children(func() {
		ui.Text(c, tr(c, "Conversation data is unavailable")).TextColor(a.palette.Danger)
		ui.Text(c, session.StorageError).FillWidth().Selectable().
			FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		ui.Text(c, tr(c, "Restore access to the original data directory, then retry.")).
			FillWidth().FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		textButton(c.Key("retry"), tr(c, "Retry loading conversation")).OnClick(func() {
			a.dispatch(contracts.Command{Kind: "session.storage.retry", SessionID: session.ID})
		})
	})
}
