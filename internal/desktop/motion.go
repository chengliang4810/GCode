package desktop

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) configureMotion(c *ui.Context) {
	settings := a.state.Settings.Appearance
	motion := ui.MotionSystem
	switch strings.ToLower(settings.MotionPreference) {
	case "off", "full":
		motion = ui.MotionFull
	case "on", "reduced", "reduce":
		motion = ui.MotionReduced
	}
	if settings.ReduceMotion || settings.PauseInBackground && a.win != nil && !a.win.IsFocused() {
		motion = ui.MotionReduced
	}
	c.SetMotionPreferences(&ui.MotionPreferences{Motion: motion, SmoothScrolling: settings.SmoothScrolling})
}

func (a *App) reduceMotion(c *ui.Context) bool {
	settings := a.state.Settings.Appearance
	if settings.ReduceMotion {
		return true
	}
	if settings.PauseInBackground && a.win != nil && !a.win.IsFocused() {
		return true
	}
	switch strings.ToLower(settings.MotionPreference) {
	case "reduced", "reduce", "on":
		return true
	case "full", "off":
		return false
	default:
		return c.Preferences().ReduceMotion
	}
}
