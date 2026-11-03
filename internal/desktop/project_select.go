package desktop

import (
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type projectSelectState struct {
	query    string
	cursor   int
	keyboard bool
	list     ui.ListState
}

func (a *App) toggleProjectMenu() {
	a.projectMenu = !a.projectMenu
	if a.projectMenu {
		a.projectSelect = projectSelectState{}
		for i, p := range a.state.Projects {
			if p.ID == a.projectID {
				a.projectSelect.cursor = i
				a.projectSelect.list.ScrollIntoView(i)
				break
			}
		}
	}
}

func (a *App) projectSelectRows() []contracts.Project {
	query := strings.ToLower(strings.TrimSpace(a.projectSelect.query))
	var rows []contracts.Project
	for _, p := range a.state.Projects {
		if query == "" || strings.Contains(strings.ToLower(p.Name), query) {
			rows = append(rows, p)
		}
	}
	return rows
}

func (a *App) selectComposerProject(p contracts.Project) {
	a.projectID, a.deviceID, a.branch = p.ID, p.DeviceID, p.Branch
	a.projectMenu, a.branchMenu, a.deviceMenu = false, false, false
	a.bottom.ProjectID = p.ID
	a.syncChatWorkspace()
	a.syncComposerFooterGit()
	a.restoreModelSelection()
	a.normalizeReasoning()
}

func (a *App) projectSelectKey(key ui.Key) bool {
	p := &a.projectSelect
	rows := a.projectSelectRows()
	switch key {
	case ui.KeyUp, ui.KeyDown:
		if len(rows) > 0 {
			delta := 1
			if key == ui.KeyUp {
				delta = -1
			}
			p.cursor = (p.cursor + delta + len(rows)) % len(rows)
			p.keyboard = true
			p.list.ScrollIntoView(p.cursor)
		}
		return true
	case ui.KeyEnter:
		if len(rows) > 0 {
			a.selectComposerProject(rows[max(0, min(p.cursor, len(rows)-1))])
		}
		return true
	}
	return false
}

func (a *App) projectPopover(c *ui.Context, anchor ui.Element) {
	ui.PopoverBase(c.Key("composer-project-menu"), anchor, &a.projectMenu, func(panel ui.Element) {
		s := design.Controls()
		budget, below := composerMenuPlacement(c, panel, anchor, false, false)
		if below {
			panel.AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).Margin(6, 0, 0, 0)
		} else {
			panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0)
		}
		width, _ := c.Size()
		// Preserve the target group's left edge, shrinking the card near the
		// window's right edge instead of shifting it away from the selector.
		availableWidth := max(float32(0), width-16)
		if bounds := anchor.Bounds(); bounds.W > 0 {
			availableWidth = max(float32(0), width-bounds.X-8)
		}
		rowHeight := max(s.NavigationHeight, design.TextSize(c, s.FontSize)*1.5+s.ButtonPaddingY*2)
		searchHeight := max(s.InputHeight, design.TextSize(c, s.FontSize)*1.5+s.InputPaddingY*2)
		actions := 1
		if a.projectID != "" {
			actions++
		}
		fixedHeight := searchHeight + rowHeight*float32(actions) + s.PopupPadding*6 + s.BorderWidth*4
		rows := a.projectSelectRows()
		visibleRows := min(5, max(1, len(rows)), max(0, int((budget-fixedHeight)/rowHeight)))
		listHeight := float32(visibleRows) * rowHeight
		controlSelectPopup(c, panel).Width(min(float32(320), availableWidth)).
			MaxHeight(budget).SnapToWindow(8).Clip().Label(tr(c, "Select project"))
		a.floatingSurface(c, panel, s.PopupRadius)
		controlMenuSearch(c.Key("composer-project-search"), &a.projectSelect.query, tr(c, "Search projects")).
			AutoFocus().OnChange(func() {
			a.projectSelect.cursor, a.projectSelect.keyboard = 0, false
			a.projectSelect.list.ScrollIntoView(0)
		}).OnTextKey(func(k ui.TextKey) bool {
			a.projectSelect.query = k.Text()
			return a.projectSelectKey(k.Key)
		})
		controlMenuDivider(c)
		if len(rows) == 0 {
			ui.Box(c.Key("composer-project-empty")).FillWidth().Height(listHeight).Center().Children(func() {
				ui.Text(c, tr(c, "No results")).TextColor(c.Theme().TextMuted)
			})
		} else {
			p := &a.projectSelect
			p.cursor = max(0, min(p.cursor, len(rows)-1))
			p.list.Key = func(i int) any { return rows[i].ID }
			p.list.Label = func(i int) string { return rows[i].Name }
			ui.List(c.Key("composer-project-list"), &p.list, len(rows), func(i int) {
				project := rows[i]
				controlProjectMenuRow(c.Key("composer-project-option:"+project.ID), project.Name, "folder",
					a.projectID == project.ID, p.keyboard && p.cursor == i).
					OnClick(func() { a.selectComposerProject(project) })
			}).FillWidth().Height(listHeight).MinHeight(0).Focusable().
				OnShortcut(0, ui.KeyUp, func() { a.projectSelectKey(ui.KeyUp) }).
				OnShortcut(0, ui.KeyDown, func() { a.projectSelectKey(ui.KeyDown) }).
				OnShortcut(0, ui.KeyEnter, func() { a.projectSelectKey(ui.KeyEnter) })
		}
		controlMenuDivider(c)
		controlProjectMenuRow(c.Key("composer-project-new"), tr(c, "New project…"), "plus", false, false).
			Shrink(0).OnClick(func() { a.projectMenu = false; a.openNewProject() })
		if a.projectID != "" {
			controlProjectMenuRow(c.Key("composer-project-none"), tr(c, "Don't work in a project"), "close",
				false, false).Shrink(0).OnClick(a.clearComposerProject)
		}
	})
}
