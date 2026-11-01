package desktop

import (
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type sidebarState struct {
	usageOpen             bool
	pullRequests          *contracts.RepoRefScope
	selection             int
	selectedSession       string
	initialized           bool
	focusList             bool
	revealID              string
	revealGroup           string
	revealProject         *contracts.ProjectCreate
	renameID, renameTitle string
	focusRename           bool
	deleteID              string
	deleteOpen            bool
	projectsClosed        bool
	conversationsClosed   bool
	limits                map[string]int
}

// sidebarViewMenu is the single radio choice between sidebar views, matching
// ZCode's project sidebar plus a device view.
func (a *App) sidebarViewMenu(menu *ui.Menu) {
	settings := &a.state.Settings
	for _, option := range []struct{ title, value string }{
		{"By project", "projects"}, {"By group", "grouped"},
		{"Recently updated", "timeline"}, {"By device", "device"},
	} {
		if menu.Item(a.t(option.title)).Checked(settings.SidebarGrouping == option.value).Chosen() {
			settings.SidebarGrouping = option.value
			a.saveSettings()
		}
	}
}
