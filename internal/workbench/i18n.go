package workbench

import (
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
	"time"
)

func tr(c *ui.Context, message string) string { return localize.T(c, message) }

func date(c *ui.Context, at time.Time, layout string) string { return localize.Date(c, at, layout) }

func (s *State) t(message string) string {
	return i18n.In(s.snapshot.Settings.General.Language, message)
}

func (s *State) tabTitle(tab surface) string {
	switch tab.kind {
	case "trajectory":
		return s.t("Call trajectory") + ": " + tab.title
	case "files", "changes", "history", "picker", "terminal":
		return s.t(tab.title)
	}
	return tab.title
}

// SetLanguage follows optimistic settings updates before backend ACKs.
func (s *State) SetLanguage(language string) {
	s.snapshot.Settings.General.Language = i18n.Normalize(language)
}
