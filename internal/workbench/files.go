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
	size := float32(a.CodeFontSize) * 13 / 12.5
	if size <= 0 {
		size = 13
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
	var files []contracts.File
	for _, file := range s.snapshot.Files {
		if file.ParentID == parent && s.visibleFile(file) {
			files = append(files, file)
		}
	}
	slices.SortStableFunc(files, func(a, b contracts.File) int {
		if a.Kind == "directory" && b.Kind != "directory" {
			return -1
		}
		if a.Kind != "directory" && b.Kind == "directory" {
			return 1
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return files
}
