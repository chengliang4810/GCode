package desktop

import "github.com/egoist/mygo/ui"

type sidebarState struct {
	accountOpen           bool
	accountSelection      int
	accountList           ui.ListState
	selection             int
	selectedSession       string
	initialized           bool
	focusList             bool
	projectQuery          string
	projectSelection      int
	projectList           ui.ListState
	projectInput          ui.Handle
	renameID, renameTitle string
	focusRename           bool
	archiveLimit          int
	archivedOpen          bool
	deleteID              string
	deleteOpen            bool
}

func (a *App) sidebarViewMenu(menu *ui.Menu) {
	settings := &a.state.Settings
	menu.Submenu(a.t("Organize"), func(menu *ui.Menu) {
		for _, option := range []struct{ title, value string }{{"By device", "device"}, {"By project", "project"}, {"None", "list"}} {
			if menu.Item(a.t(option.title)).Checked(sidebarGrouping(settings.SidebarGrouping) == option.value).Chosen() {
				settings.SidebarGrouping = option.value
				a.saveSettings()
			}
		}
	})
	menu.Submenu(a.t("Sort"), func(menu *ui.Menu) {
		for _, option := range []struct{ title, value string }{{"Last updated", "updated"}, {"Created", "created"}} {
			if menu.Item(a.t(option.title)).Checked(settings.SidebarSort == option.value).Chosen() {
				settings.SidebarSort = option.value
				a.saveSettings()
			}
		}
	})
	menu.Separator()
	menu.Submenu(a.t("Show"), func(menu *ui.Menu) {
		for _, option := range []struct {
			title string
			value *bool
		}{{"Branch", &settings.SidebarShowBranch}, {"Pull request", &settings.SidebarShowPullRequest}, {"Harness", &settings.SidebarShowHarness}, {"Project icon", &settings.ShowIcons}, {"Location", &settings.ShowProjectLabels}, {"Project filter", &settings.SidebarShowProjectFilter}, {"Archived", &settings.SidebarShowArchived}} {
			if menu.Item(a.t(option.title)).Checked(*option.value).Chosen() {
				*option.value = !*option.value
				a.saveSettings()
			}
		}
	})
	menu.Separator()
	if menu.Item(a.t("Compact")).Checked(settings.SidebarCompact).Chosen() {
		settings.SidebarCompact = !settings.SidebarCompact
		a.saveSettings()
	}
}
