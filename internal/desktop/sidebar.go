package desktop

import (
	"gcode/internal/i18n"
	"time"
)

func (a *App) projectNameFor(id string) string {
	for _, p := range a.state.Projects {
		if p.ID == id {
			return p.Name
		}
	}
	return "No project"
}
func (a *App) deviceNameFor(id string) string {
	for _, d := range a.state.Devices {
		if d.ID == id {
			return d.Name
		}
	}
	return "This device"
}

func relativeAge(at, now time.Time, language ...string) string {
	lang := "en"
	if len(language) > 0 {
		lang = language[0]
	}
	d := now.Sub(at)
	if d < time.Minute {
		return i18n.In(lang, "now")
	}
	if d < time.Hour {
		return i18n.In(lang, "%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return i18n.In(lang, "%dh", int(d.Hours()))
	}
	return i18n.In(lang, "%dd", int(d.Hours()/24))
}
