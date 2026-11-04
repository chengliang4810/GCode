package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"strings"
	"time"
)

func tr(c *ui.Context, message string) string { return localize.T(c, message) }

func date(c *ui.Context, at time.Time, layout string) string { return localize.Date(c, at, layout) }

func (a *App) workedFor(milliseconds int64) string {
	return a.t("Worked for ") + i18n.Duration(a.state.Settings.General.Language, milliseconds)
}

func (a *App) t(message string) string { return i18n.In(a.state.Settings.General.Language, message) }

func (a *App) settingOptionText(c *ui.Context, label, value, text string) string {
	switch label {
	case "Interface font", "Terminal font", "Code & diff font", "Math font", "Light theme", "Dark theme":
		return text
	case "Thread naming model":
		if value != "first-message" && value != "current-model" {
			return text
		}
	}
	return tr(c, text)
}

func (a *App) sidebarHeaderText(row sidebarRow) string {
	if row.ProjectID != "" || strings.HasPrefix(row.Key, "session-group:") || strings.HasPrefix(row.Key, "device:") {
		return row.Header
	}
	return a.t(row.Header)
}

func (a *App) paletteRowTitle(row paletteRow) string {
	if row.command == "" {
		return row.title
	}
	if row.command == "theme:light" {
		return a.t("Switch to light theme")
	}
	if row.command == "theme:dark" {
		return a.t("Switch to dark theme")
	}
	return a.t(row.title)
}

func (a *App) syncNativeLanguage() {
	a.workspace.SetLanguage(a.state.Settings.General.Language)
	a.bottom.SetLanguage(a.state.Settings.General.Language)
	if a.win != nil && mygo.App.Locale() != i18n.Native(a.state.Settings.General.Language) {
		mygo.App.SetLocale(i18n.Native(a.state.Settings.General.Language))
	}
}

func (a *App) activitySummary(summary contracts.ActivitySummary) string {
	if i18n.Normalize(a.state.Settings.General.Language) == i18n.English {
		return summary.Label()
	}
	var parts []string
	for _, item := range []struct {
		count int
		key   string
	}{
		{summary.Thoughts, "Thought %d times"}, {summary.Notes, "%d progress updates"},
		{summary.Commands, "Ran %d commands"}, {summary.EditedFiles, "Edited %d files"},
		{summary.Reads, "Read %d files"}, {summary.Searches, "Searched %d times"},
		{summary.Fetches, "Fetched %d pages"}, {summary.Todos, "Updated %d todos"},
		{summary.Other, "Called %d tools"}, {summary.Failed, "%d failed"},
	} {
		if item.count > 0 {
			parts = append(parts, i18n.In(a.state.Settings.General.Language, item.key, item.count))
		}
	}
	return strings.Join(parts, " · ")
}

func (a *App) annotationLabel(index uint32) string {
	return i18n.In(a.state.Settings.General.Language, "Annotation %d", index)
}

func (a *App) deviceDisplayName(device contracts.Device) string {
	if device.Current && device.Name == "This device" {
		return a.t("This device")
	}
	if device.Platform == "cloud" && device.Name == "Cloud" {
		return a.t("Cloud")
	}
	return device.Name
}
