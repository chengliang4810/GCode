package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
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

func sidebarGrouping(value string) string {
	switch value {
	case "project", "By project":
		return "project"
	case "device", "By device":
		return "device"
	default:
		return "list"
	}
}

func relativeAge(at time.Time, sessions []contracts.Session) string {
	now := at
	for _, s := range sessions {
		if s.UpdatedAt.After(now) {
			now = s.UpdatedAt
		}
	}
	d := now.Sub(at)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (a *App) projectPopover(c *ui.Context, b ui.Element) {
	ui.Popover(c, b, &a.projectMenu, func() {
		ui.Column(c).Width(280).Gap(2).Children(func() {
			for _, p := range a.state.Projects {
				p := p
				menuChoice(c, p.Name, a.projectID == p.ID, func() { a.projectID = p.ID; a.branch = p.Branch; a.projectMenu = false })
			}
			menuChoice(c, "Don't work in a project", a.projectID == "", func() { a.projectID = ""; a.projectMenu = false })
			line(c)
			menuChoice(c, "New project…", false, func() { a.openNewProject(); a.projectMenu = false })
		})
	})
}
