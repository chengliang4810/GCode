package workbench

import (
	"gcode/internal/design"
	"path"
	"strings"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

func (s *State) fileContext(c *ui.Context, f contracts.File, m *ui.Menu, host string) {
	if f.Kind != "directory" && m.Item(tr(c, "Open")).Chosen() {
		s.OpenFile(f.ID)
	}
	if m.Item(tr(c, "Copy path")).Chosen() {
		c.Services().WriteClipboard(f.Path)
	}
	m.Separator()
	if f.Kind == "directory" {
		if m.Item(tr(c, "New file…")).Chosen() {
			s.requestFileAction("create", f.ID, host)
		}
		if m.Item(tr(c, "New folder…")).Chosen() {
			s.requestFileAction("mkdir", f.ID, host)
		}
	}
	if m.Item(tr(c, "Rename…")).Disabled(f.ReadOnly).Chosen() {
		s.requestFileAction("rename", f.ID, host)
	}
	if m.Item(tr(c, "Delete…")).Disabled(f.ReadOnly).Chosen() {
		s.requestFileAction("delete", f.ID, host)
	}
}

func (s *State) requestFileAction(kind, id, host string) {
	s.fileActionHost = host
	s.fileAction = kind
	s.fileActionID = id
	s.fileActionName = ""
	s.fileActionOpen = true
	if kind == "rename" {
		for _, f := range s.snapshot.Files {
			if f.ID == id {
				s.fileActionName = f.Name
				break
			}
		}
	}
}

func (s *State) performFileAction() {
	command := contracts.Command{ID: s.fileActionID, ProjectID: s.ProjectID}
	switch s.fileAction {
	case "rename":
		command.Kind = "file.rename"
		command.Name = strings.TrimSpace(s.fileActionName)
	case "delete":
		command.Kind = "file.delete"
	case "create", "mkdir":
		base := ""
		for _, f := range s.snapshot.Files {
			if f.ID == s.fileActionID {
				base = f.Path
				command.ProjectID = f.ProjectID
				break
			}
		}
		command.Kind = "file.create"
		command.Path = path.Join(base, strings.TrimSpace(s.fileActionName))
		command.Enabled = s.fileAction == "mkdir"
	default:
		return
	}
	if s.fileAction != "delete" && strings.TrimSpace(s.fileActionName) == "" {
		return
	}
	s.fileActionOpen = false
	s.command(command)
}

func (s *State) fileActionDialog(c *ui.Context) {
	p := s.palette()
	ui.DialogBase(c, &s.fileActionOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .4))
		panel.Width(400).Padding(24).Radius(12).Background(p.Dialog).Border(1, p.Border)
		title, button := "New file", "Create"
		switch s.fileAction {
		case "rename":
			title = "Rename"
			button = "Rename"
		case "delete":
			title = "Delete"
			button = "Delete"
		case "mkdir":
			title = "New folder"
		}
		ui.Text(c, tr(c, title)).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600).Margin(0, 0, 16, 0)
		if s.fileAction == "delete" {
			name := s.fileActionID
			for _, f := range s.snapshot.Files {
				if f.ID == s.fileActionID {
					name = f.Name
					break
				}
			}
			ui.Text(c, tr(c, "Delete ")+name+"?").Margin(0, 0, 8, 0)
			ui.Text(c, tr(c, "This action cannot be undone.")).TextColor(p.Muted).Margin(0, 0, 16, 0)
		} else {
			ui.TextInputBase(c, &s.fileActionName).Label(tr(c, "Name")).Placeholder(tr(c, "Name")).AutoFocus().Padding(10, 12).Radius(6).Background(p.Input).OnSubmit(s.performFileAction)
		}
		ui.Row(c).Gap(8).Margin(16, 0, 0, 0).Justify(ui.End).Children(func() {
			ui.Button(c, tr(c, "Cancel")).OnClick(func() { s.fileActionOpen = false })
			ui.PrimaryButton(c, tr(c, button)).Disabled(s.fileAction != "delete" && strings.TrimSpace(s.fileActionName) == "").OnClick(s.performFileAction)
		})
	})
}
