package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"strings"
)

func (a *App) sidebarProjects() []contracts.Project {
	var projects []contracts.Project
	query := strings.ToLower(strings.TrimSpace(a.sidebarUI.projectQuery))
	for _, project := range a.state.Projects {
		if queryWordsMatch(query, project.Name) {
			projects = append(projects, project)
		}
	}
	return projects
}

func (a *App) sidebarProjectPopover(c *ui.Context, trigger ui.Element) {
	ui.Popover(c, trigger, &a.sidebarProjectMenu, func() {
		ui.Column(c).Width(max(200, a.sidebarContentWidth-16)).MaxHeight(420).Padding(6).Gap(2).Children(func() {
			input := ui.TextInputBase(c, &a.sidebarUI.projectQuery).Bind(&a.sidebarUI.projectInput).Label(tr(c, "Search projects")).Placeholder(tr(c, "Search projects")).AutoFocus().Padding(6, 8).FontSize(design.UISize(c, 12)).OnChange(func() { a.sidebarUI.projectSelection = 0 })
			projects := a.sidebarProjects()
			showAll := strings.TrimSpace(a.sidebarUI.projectQuery) == ""
			offset := 0
			if showAll {
				offset = 1
			}
			count := len(projects) + offset
			choose := func() {
				i := a.sidebarUI.projectSelection
				if i == count {
					a.sidebarProjectMenu = false
					a.sidebarUI.projectQuery = ""
					a.openNewProject()
					return
				}
				if showAll && i == 0 {
					a.sidebarProjectID = ""
				} else if i >= offset && i < count {
					a.sidebarProjectID = projects[i-offset].ID
				}
				a.sidebarProjectMenu = false
				a.sidebarUI.projectQuery = ""
			}
			move := func(delta int) {
				a.sidebarUI.projectSelection = max(0, min(count, a.sidebarUI.projectSelection+delta))
				a.sidebarUI.projectList.ScrollTo(a.sidebarUI.projectSelection, ui.Center)
			}
			input.OnShortcut(0, ui.KeyDown, func() { move(1) }).OnShortcut(0, ui.KeyUp, func() { move(-1) }).OnSubmit(choose).OnShortcut(0, ui.KeyEscape, func() { a.sidebarProjectMenu = false; a.sidebarUI.projectQuery = "" })
			a.sidebarUI.projectList.Selected = nil
			a.sidebarUI.projectList.Key = func(i int) any {
				if showAll && i == 0 {
					return "all"
				}
				return projects[i-offset].ID
			}
			a.sidebarUI.projectList.Label = func(i int) string {
				if showAll && i == 0 {
					return a.t("All projects")
				}
				return projects[i-offset].Name
			}
			ui.List(c, &a.sidebarUI.projectList, count, func(i int) {
				label, detail, projectID, offline := tr(c, "All projects"), "", "", false
				if !showAll || i > 0 {
					project := projects[i-offset]
					label, detail, projectID = project.Name, project.Path+" @ "+a.deviceNameFor(project.DeviceID), project.ID
					for _, device := range a.state.Devices {
						if device.ID == project.DeviceID {
							offline = !deviceOnline(device)
							break
						}
					}
				}
				row := ui.ButtonBase(c).Label(label).Tooltip(detail).Height(30).FillWidth().MinWidth(0).Padding(4, 8).Gap(8).Radius(6).AlignItems(ui.Center).OnClick(func() { a.sidebarUI.projectSelection = i; choose() }).HandleInput(func(event ui.InputEvent) bool {
					if event.Kind == ui.InputPointerMove && event.Button < 0 {
						a.sidebarUI.projectSelection = i
						return true
					}
					return false
				})
				if a.sidebarProjectID == projectID {
					row.Background(paletteSelection(a.palette.Dark)).InsetShadow(0, 0, 0, 1, sourcePaletteInk(a.palette.Dark, .07))
				} else if i == a.sidebarUI.projectSelection {
					row.Background(a.palette.Hover)
				}
				row.Children(func() {
					icon(c, "folder", 14)
					paletteHighlight(c, label, a.sidebarUI.projectQuery, a.palette.Accent).SingleLine().MinWidth(0).Grow(1).FontSize(design.UISize(c, 13))
					if offline {
						ui.Box(c).Label(tr(c, "Device offline")).Children(func() { icon(c, "wifi-off", 12) }).TextColor(a.palette.Warning)
					}
				})
			}).Height(min(float32(count)*32, 224)).FillWidth().OnSubmit(choose)
			line(c)
			footer := textButton(c, tr(c, "New project")).Height(30).FillWidth().OnClick(func() { a.sidebarProjectMenu = false; a.sidebarUI.projectQuery = ""; a.openNewProject() })
			if a.sidebarUI.projectSelection == count {
				footer.Background(a.palette.Hover)
			}
		})
	})
}
