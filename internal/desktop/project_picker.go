package desktop

import (
	"context"
	"fmt"
	"path"
	"strings"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type newProjectState struct {
	step                              int // devices, locations, folders
	deviceID, query, typedPath, error string
	pathEdit, loading                 bool
	listing                           contracts.DirectoryListing
	selected                          int
	list                              ui.ListState
	input                             ui.Handle
	generation                        uint64
	cancel                            context.CancelFunc
	submission                        *contracts.ProjectCreate
	submissionID                      string
}
type projectPickerRow struct {
	key, title, glyph string
	device            *contracts.Device
	directory         *contracts.DirectoryEntry
}

func (a *App) projectCrumbs(c *ui.Context) {
	p := &a.newProject
	current := p.listing.Path
	if p.loading || p.error != "" {
		current = p.typedPath
	}
	home := p.listing.Home
	if home == "" || !strings.HasPrefix(current, home) {
		ui.Text(c, current).Label("Project folder trail").SingleLine().Padding(4, 6).FontSize(design.UISize(c, 13))
		return
	}
	crumb := func(title, folder string) {
		if folder == current {
			ui.Text(c, title).Label("Project folder trail").SingleLine().Padding(4, 6).FontSize(design.UISize(c, 13))
		} else {
			textButton(c, title).Padding(4, 6).OnClick(func() { a.projectBrowse(folder) })
		}
	}
	crumb("Home", home)
	folder := home
	for _, part := range strings.Split(strings.TrimPrefix(current, strings.TrimSuffix(home, "/")+"/"), "/") {
		if part == "" || current == home {
			break
		}
		icon(c, "alt-arrow-right", 12)
		folder = path.Join(folder, part)
		crumb(part, folder)
	}
}

func (a *App) openNewProject() {
	a.stopProjectBrowse()
	a.newProject = newProjectState{generation: a.newProject.generation + 1}
	a.projectOpen = true
	a.paletteOpen = false
	a.stopPaletteSearch()
}
func (a *App) stopProjectBrowse() {
	if a.newProject.cancel != nil {
		a.newProject.cancel()
		a.newProject.cancel = nil
	}
	a.newProject.generation++
	a.newProject.loading = false
}
func (a *App) projectBrowse(folder string) {
	a.stopProjectBrowse()
	p := &a.newProject
	p.step = 2
	p.query = ""
	p.selected = 0
	p.error = ""
	p.pathEdit = false
	if folder != "" {
		p.typedPath = folder
	}
	service, ok := a.service.(contracts.ProjectDirectoryService)
	if !ok {
		p.error = "Folder browsing is unavailable"
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	p.loading = true
	generation := p.generation
	request := contracts.DirectoryRequest{DeviceID: p.deviceID, Path: folder}
	finish := func(result contracts.DirectoryListing, err error) {
		if a.closed || ctx.Err() != nil || !a.projectOpen || a.newProject.generation != generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
		} else {
			p.listing = result
			p.typedPath = result.Path
		}
	}
	if a.win == nil {
		result, err := service.ListDirectories(ctx, request)
		finish(result, err)
		return
	}
	win := a.win
	go func() {
		result, err := service.ListDirectories(ctx, request)
		if ctx.Err() != nil {
			return
		}
		win.Update(func() { finish(result, err) })
	}()
}
func (a *App) projectPickerRows() []projectPickerRow {
	p := &a.newProject
	var rows []projectPickerRow
	switch p.step {
	case 0:
		for _, device := range a.state.Devices {
			if queryWordsMatch(p.query, device.Name) {
				rows = append(rows, projectPickerRow{key: device.ID, title: device.Name, glyph: deviceGlyph(device), device: &device})
			}
		}
	case 1:
		if queryWordsMatch(p.query, "Home") {
			rows = append(rows, projectPickerRow{key: "home", title: "Home", glyph: "home"})
		}
	case 2:
		for _, directory := range p.listing.Entries {
			if directory.Hidden && !strings.HasPrefix(p.query, ".") {
				continue
			}
			if queryWordsMatch(p.query, directory.Name) {
				rows = append(rows, projectPickerRow{key: directory.Path, title: directory.Name, glyph: "folder", directory: &directory})
			}
		}
	}
	return rows
}
func deviceGlyph(device contracts.Device) string {
	if device.Platform == "darwin" {
		return "laptop"
	}
	if device.Platform == "linux" {
		return "remote-server"
	}
	return "monitor"
}
func deviceOnline(device contracts.Device) bool {
	return device.Current || device.Status == "online" || device.Status == "connected"
}
func (a *App) chooseProjectPicker(row projectPickerRow) {
	p := &a.newProject
	p.input.Focus()
	if row.device != nil {
		a.stopProjectBrowse()
		p.deviceID = row.device.ID
		p.step = 1
		p.query = ""
		p.selected = 0
		p.error = ""
		p.listing = contracts.DirectoryListing{}
		return
	}
	if row.directory != nil {
		a.projectBrowse(row.directory.Path)
		return
	}
	a.projectBrowse("")
}
func (a *App) addPickedProject() {
	p := &a.newProject
	if p.step != 2 || p.loading || p.error != "" || p.listing.Path == "" {
		return
	}
	request := contracts.ProjectCreate{DeviceID: p.deviceID, Name: path.Base(p.listing.Path), Path: p.listing.Path}
	if p.submission != nil {
		return
	}
	p.submission = &request
	p.generation++
	p.submissionID = fmt.Sprintf("project-picker-%d", p.generation)
	a.dispatch(contracts.Command{Kind: "project.create", ID: p.submissionID, ProjectCreate: &request})
}
func (a *App) projectCommandFinished(command contracts.Command, err error) bool {
	if command.Kind != "project.create" || command.ProjectCreate == nil {
		return false
	}
	p := &a.newProject
	if p.submission == nil || p.submissionID != command.ID || *p.submission != *command.ProjectCreate {
		return true
	}
	p.submission = nil
	if err != nil {
		p.error = err.Error()
	} else {
		a.projectOpen = false
		a.stopProjectBrowse()
	}
	return true
}
func (a *App) projectPickerBack() {
	p := &a.newProject
	if p.pathEdit {
		p.pathEdit = false
		return
	}
	switch p.step {
	case 2:
		if p.listing.Path != p.listing.Home && p.listing.Parent != "" {
			a.projectBrowse(p.listing.Parent)
			return
		}
		a.stopProjectBrowse()
		p.step = 1
	case 1:
		p.step = 0
	}
	p.query = ""
	p.selected = 0
	p.error = ""
}
func (a *App) newProjectDialog(c *ui.Context) {
	if !a.projectOpen {
		a.stopProjectBrowse()
		return
	}
	p := &a.newProject
	ui.DialogBase(c, &a.projectOpen, func(back, panel ui.Element) {
		width, height := c.Size()
		maxResults := max(float32(100), min(float32(360), height-180))
		top := max(float32(54), (height-maxResults-120)/2)
		back.Background(paletteScrim(a.palette.Dark)).Justify(ui.Start).Padding(top, 0, 0)
		panel.Label("New project picker").Width(min(560, width-32)).Radius(16).Border(1, a.palette.Border).Background(a.palette.Raised).Clip()
		ui.Column(c).FillWidth().Children(func() {
			placeholder := "Search devices…"
			switch p.step {
			case 1:
				placeholder = "Search locations…"
			case 2:
				placeholder = "Search folders…"
			}
			var input ui.Element
			ui.Row(c).Height(44).Padding(8, 16).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
				ui.Icon(c, assets.Icon("palette-search")).Size(16, 16).TextColor(a.palette.Muted)
				input = ui.TextInputBase(c, &p.query).Bind(&p.input).Label("New project search").Placeholder(placeholder).AutoFocus().Grow(1).MinWidth(0).FontSize(design.UISize(c, 14)).OnChange(func() {
					p.selected = 0
					if strings.HasSuffix(p.query, "/") {
						for _, directory := range p.listing.Entries {
							if strings.EqualFold(directory.Name, strings.TrimSuffix(p.query, "/")) {
								a.projectBrowse(directory.Path)
								break
							}
						}
					}
				})
			})
			ui.Row(c).Height(38).Padding(4, 10).Gap(2).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
				textButton(c, "New project").Padding(4, 6).OnClick(func() {
					a.stopProjectBrowse()
					p.step = 0
					p.deviceID = ""
					p.query = ""
					p.selected = 0
					p.error = ""
				})
				if p.deviceID != "" {
					icon(c, "alt-arrow-right", 12)
					textButton(c, a.deviceNameFor(p.deviceID)).Padding(4, 6).OnClick(func() { a.stopProjectBrowse(); p.step = 1; p.query = ""; p.error = "" })
				}
				if p.step == 2 {
					icon(c, "alt-arrow-right", 12)
					if p.pathEdit {
						ui.TextInputBase(c, &p.typedPath).Label("Project path").AutoFocus().MinWidth(0).Grow(1).OnSubmit(func() { a.projectBrowse(p.typedPath) }).OnShortcut(0, ui.KeyEscape, func() { p.pathEdit = false })
					} else {
						ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Children(func() {
							ui.Row(c).Gap(2).Children(func() { a.projectCrumbs(c) })
						})
					}
					iconButton(c, "pen", "Edit path").Size(24, 24).OnClick(func() { p.pathEdit = !p.pathEdit; p.typedPath = p.listing.Path })
				}
			})
			rows := a.projectPickerRows()
			p.selected = max(0, min(max(0, len(rows)-1), p.selected))
			choose := func() {
				if p.selected < len(rows) {
					a.chooseProjectPicker(rows[p.selected])
				}
			}
			move := func(delta int) {
				if len(rows) > 0 {
					p.selected = (p.selected + delta + len(rows)) % len(rows)
					p.list.ScrollTo(p.selected, ui.Center)
				}
			}
			input.OnShortcut(0, ui.KeyDown, func() { move(1) }).OnShortcut(0, ui.KeyUp, func() { move(-1) }).OnShortcut(0, ui.KeyLeft, a.projectPickerBack).OnShortcut(0, ui.KeyBackspace, func() {
				if p.query == "" {
					a.projectPickerBack()
				}
			}).OnShortcut(ui.Ctrl, ui.KeyL, func() {
				if p.step > 0 {
					p.pathEdit = true
				}
			}).OnShortcut(ui.Cmd, ui.KeyEnter, a.addPickedProject).OnSubmit(choose).OnShortcut(0, ui.KeyTab, func() {
				if len(rows) > 0 {
					p.query = rows[p.selected].title
				}
			})
			p.list.Key = func(i int) any { return rows[i].key }
			p.list.Label = func(i int) string { return rows[i].title }
			if len(rows) > 0 && !p.loading && p.error == "" {
				ui.List(c, &p.list, len(rows), func(i int) {
					row := rows[i]
					ui.Column(c).FillWidth().Padding(0, 8).Children(func() {
						button := ui.ButtonBase(c).Label("Project folder: "+row.title).Height(30).FillWidth().Padding(4, 8).Gap(8).Radius(10).AlignItems(ui.Center).OnClick(func() { a.chooseProjectPicker(row) }).HandleInput(func(event ui.InputEvent) bool {
							if event.Kind == ui.InputPointerMove && event.Button < 0 {
								p.selected = i
								return true
							}
							return false
						})
						if i == p.selected {
							button.Background(paletteSelection(a.palette.Dark))
						} else if button.Hovered() {
							button.Background(a.palette.Hover)
						}
						button.Children(func() {
							ui.Icon(c, assets.Icon(row.glyph)).Size(16, 16).TextColor(a.palette.Muted)
							paletteHighlight(c, row.title, p.query, a.palette.Accent).SingleLine().MinWidth(0).Grow(1).FontSize(design.UISize(c, 13))
							if row.device != nil {
								color := a.palette.Faint
								if deviceOnline(*row.device) {
									color = a.palette.Success
								}
								ui.Box(c).Size(5, 5).Radius(3).Background(color)
							}
							if row.directory != nil && row.directory.Project {
								icon(c, "git-branch", 14)
							}
						})
					})
				}).Label("Project folders").Height(min(maxResults, float32(len(rows))*32+16)).Padding(8, 0).Gap(2).FillWidth()
			} else {
				ui.Column(c).Padding(24, 16).Gap(6).Children(func() {
					if p.loading {
						ui.Text(c, "Loading folders…")
					} else if p.error != "" {
						ui.Text(c, p.error).TextColor(a.palette.Danger)
						textButton(c, "Retry").OnClick(func() { a.projectBrowse(p.typedPath) })
					} else {
						title, hint := "No folders here", "Add this folder with "+paletteShortcut("Mod+Enter")+", or go back with ←."
						switch p.step {
						case 0:
							title, hint = "No devices found", "Try another device name."
						case 1:
							title, hint = "No locations found", "Try Home or a drive name."
						case 2:
							if p.query != "" {
								title, hint = "No folders match", "Type a path like ~/code or /mnt to jump there."
							}
						}
						ui.Text(c, title)
						ui.Text(c, hint).TextColor(a.palette.Muted)
					}
				})
			}
			ui.Row(c).Padding(7, 16).Gap(12).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(a.palette.Border).Children(func() {
				a.paletteKeyHint(c, "↑ ↓", "Navigate")
				a.paletteKeyHint(c, "↵", "Open")
				ui.Box(c).Grow(1)
				if p.step == 2 {
					textButton(c, "Add project").Disabled(p.loading || p.error != "" || p.submission != nil || p.listing.Path == "").OnClick(a.addPickedProject)
				}
			})
		})
	})
}
