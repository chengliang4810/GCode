package workbench

import (
	"cmp"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (s *State) codeFont() ui.Font {
	a := s.snapshot.Settings.Appearance
	size := float32(a.CodeFontSize)
	if size <= 0 {
		size = design.CodeFontSize
	}
	return ui.Font{Family: design.MonoFont(a.CodeFont), Size: size}
}

func (s *State) visibleFile(file contracts.File) bool {
	if s.ProjectID != "" && file.ProjectID != s.ProjectID {
		return false
	}
	if !s.snapshot.Settings.Files.ShowHidden {
		for _, part := range strings.Split(strings.ReplaceAll(file.Path, "\\", "/"), "/") {
			if strings.HasPrefix(part, ".") && part != "." && part != ".." {
				return false
			}
		}
	}
	return true
}

func (s *State) filesIn(parent string) []contracts.File {
	if s.fileChildrenIndex == nil || s.fileIndexProject != s.ProjectID || s.fileIndexHidden != s.snapshot.Settings.Files.ShowHidden {
		s.fileChildrenIndex = make(map[string][]contracts.File)
		s.fileIndexProject = s.ProjectID
		s.fileIndexHidden = s.snapshot.Settings.Files.ShowHidden
		for _, file := range s.snapshot.Files {
			if s.visibleFile(file) {
				s.fileChildrenIndex[file.ParentID] = append(s.fileChildrenIndex[file.ParentID], file)
			}
		}
		for _, files := range s.fileChildrenIndex {
			slices.SortStableFunc(files, func(a, b contracts.File) int {
				if a.Kind == "directory" && b.Kind != "directory" {
					return -1
				}
				if a.Kind != "directory" && b.Kind == "directory" {
					return 1
				}
				return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})
		}
	}
	return s.fileChildrenIndex[parent]
}

func (s *State) projectFilesStatus(c *ui.Context) {
	for _, project := range s.snapshot.Projects {
		if s.ProjectID != "" && project.ID != s.ProjectID {
			continue
		}
		switch project.FilesState {
		case contracts.LoadLoading:
			ui.Text(c.Key("project-files-loading:"+project.ID), tr(c, "Loading project files…")).
				Label(tr(c, "Loading project files…")).TextColor(s.palette().Muted).
				Padding(8).FillWidth().FontSize(design.TextSize(c, design.TextBase))
		case contracts.LoadError:
			ui.Row(c.Key("project-files-error:" + project.ID)).FillWidth().Padding(8).Gap(4).Children(func() {
				ui.Text(c, tr(c, "File loading incomplete")+": "+tr(c, project.FilesError)).
					Label(tr(c, "File loading incomplete") + ": " + tr(c, project.FilesError)).
					TextColor(s.palette().Muted).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase))
				s.iconButton(c, "refresh", "Retry", func() {
					s.command(contracts.Command{Kind: "project.files.refresh", ID: project.ID})
				})
			})
		}
	}
}
