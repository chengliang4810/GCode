package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type toastKind uint8

const (
	toastInfo toastKind = iota
	toastSuccess
	toastWarning
	toastError
)

func (a *App) notification(kind toastKind, message string) {
	a.toastKind, a.toast = kind, message
	a.toastUntil = time.Now().Add(3 * time.Second)
}

func (a *App) toastView(c *ui.Context) {
	if a.toast == "" || !time.Now().Before(a.toastUntil) {
		return
	}
	width, _ := c.Size()
	ui.Overlay(c, func() {
		ui.Row(c).Absolute().Top(48).Left(0).Right(0).Justify(ui.Center).Children(func() {
			controlToast(c.Key("app-toast"), a.toastKind, tr(c, a.toast)).MaxWidth(min(float32(560), max(float32(0), width-32)))
		})
	})
	c.After(time.Until(a.toastUntil))
}
