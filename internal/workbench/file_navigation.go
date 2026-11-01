package workbench

import (
	"gcode/internal/contracts"
	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui/editor"
)

// OpenFileAt reveals a one-based line and zero-based UTF-8 byte column.
// A zero line retains its saved caret and scroll position.
func (s *State) OpenFileAt(id string, line, column int) {
	s.OpenFile(id)
	if document := s.documents[id]; document != nil && line > 0 {
		document.preview = false
		document.editor.GoTo(editor.Pos{Line: line - 1, Col: max(0, column)})
	} else if s.pendingFile == id && line > 0 {
		s.pendingNavigation = &fileNavigation{id: id, target: linktarget.Target{Line: line, Column: column}, byteColumn: true}
	}
}

// RevealFile expands the path's ancestors; directories remain in the explorer.
func (s *State) RevealFile(id string) {
	s.clearFileSearch()
	byID := map[string]contracts.File{}
	for _, file := range s.snapshot.Files {
		byID[file.ID] = file
	}
	for file, ok := byID[id]; ok; file, ok = byID[file.ParentID] {
		if file.Kind == "directory" {
			open := true
			s.folders[file.ID] = &open
		}
		if file.ParentID == "" {
			break
		}
	}
}

// RecentFiles returns conversation-owned access order without exposing UI state.
func (s *State) RecentFiles() []contracts.FileSearchMatch {
	var result []contracts.FileSearchMatch
	for _, id := range s.recentFiles {
		for _, file := range s.snapshot.Files {
			if file.ID == id && file.ProjectID == s.ProjectID {
				result = append(result, contracts.FileSearchMatch{FileID: id, Path: file.Path, Kind: file.Kind})
				break
			}
		}
	}
	return result
}
