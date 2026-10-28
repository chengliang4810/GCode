package mock

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
)

// syncFileChangeLocked keeps the editor's saved text and Changes view coherent.
// The mock uses one contiguous hunk rather than pretending to run Git.
func (s *Server) syncFileChangeLocked(file *contracts.File, previous string) {
	index := -1
	for i, c := range s.state.Changes {
		if c.ProjectID == file.ProjectID && c.Path == file.Path {
			index = i
			break
		}
	}
	change := contracts.Change{ProjectID: file.ProjectID, Path: file.Path, Status: "modified", OldContent: previous}
	if index >= 0 {
		change = s.state.Changes[index]
	}
	change.NewContent = file.Content
	if change.OldContent == change.NewContent && change.Status != "added" && change.OldPath == "" {
		file.Modified = false
		if index >= 0 {
			s.removeFileChangeLocked(index)
		}
		return
	}
	change.Additions, change.Deletions, change.Hunks = contiguousDiff(change.OldContent, change.NewContent)
	if index >= 0 {
		s.state.Changes[index] = change
	} else {
		change.ID = s.idLocked("change")
		s.state.Changes = append(s.state.Changes, change)
	}
}

func (s *Server) removeFileChangeLocked(index int) {
	id := s.state.Changes[index].ID
	s.state.Changes = append(s.state.Changes[:index], s.state.Changes[index+1:]...)
	for i := range s.state.PullRequests {
		ids := s.state.PullRequests[i].ChangeIDs[:0]
		for _, changeID := range s.state.PullRequests[i].ChangeIDs {
			if changeID != id {
				ids = append(ids, changeID)
			}
		}
		s.state.PullRequests[i].ChangeIDs = ids
	}
}

func (s *Server) createFileChangeLocked(file contracts.File) {
	if file.Kind == "directory" {
		return
	}
	added, removed, hunks := contiguousDiff("", file.Content)
	s.state.Changes = append(s.state.Changes, contracts.Change{ID: s.idLocked("change"), ProjectID: file.ProjectID, Path: file.Path, Status: "added", NewContent: file.Content, Additions: added, Deletions: removed, Hunks: hunks})
}

func (s *Server) renameFileChangeLocked(file *contracts.File, oldPath string) {
	if file.Kind == "directory" || file.Path == oldPath {
		return
	}
	for i := range s.state.Changes {
		change := &s.state.Changes[i]
		if change.ProjectID != file.ProjectID || change.Path != oldPath {
			continue
		}
		change.Path = file.Path
		if change.Status != "added" {
			if change.OldPath == "" {
				change.OldPath = oldPath
			}
			change.Status = "renamed"
			if change.OldPath == change.Path {
				change.OldPath = ""
				change.Status = "modified"
			}
		}
		s.syncFileChangeLocked(file, file.Content)
		return
	}
	s.state.Changes = append(s.state.Changes, contracts.Change{ID: s.idLocked("change"), ProjectID: file.ProjectID, Path: file.Path, OldPath: oldPath, Status: "renamed", OldContent: file.Content, NewContent: file.Content})
}

func (s *Server) deleteFileChangeLocked(file contracts.File) {
	if file.Kind == "directory" {
		return
	}
	for i := range s.state.Changes {
		change := &s.state.Changes[i]
		if change.ProjectID != file.ProjectID || change.Path != file.Path {
			continue
		}
		if change.Status == "added" {
			s.removeFileChangeLocked(i)
			return
		}
		if change.OldPath != "" {
			change.Path = change.OldPath
			change.OldPath = ""
		}
		change.Status, change.NewContent = "deleted", ""
		change.Additions, change.Deletions, change.Hunks = contiguousDiff(change.OldContent, "")
		return
	}
	added, removed, hunks := contiguousDiff(file.Content, "")
	s.state.Changes = append(s.state.Changes, contracts.Change{ID: s.idLocked("change"), ProjectID: file.ProjectID, Path: file.Path, Status: "deleted", OldContent: file.Content, Additions: added, Deletions: removed, Hunks: hunks})
}

func contiguousDiff(oldText, newText string) (int, int, []contracts.DiffHunk) {
	lines := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n"), "\n")
	}
	old, new := lines(oldText), lines(newText)
	prefix := 0
	for prefix < len(old) && prefix < len(new) && old[prefix] == new[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(new)-prefix && old[len(old)-1-suffix] == new[len(new)-1-suffix] {
		suffix++
	}
	removed, added := len(old)-prefix-suffix, len(new)-prefix-suffix
	if removed == 0 && added == 0 {
		return 0, 0, nil
	}
	from := max(0, prefix-3)
	tail := min(suffix, 3)
	body := []contracts.DiffLine{}
	for i := from; i < prefix; i++ {
		body = append(body, contracts.DiffLine{Kind: "context", Text: old[i], OldLine: i + 1, NewLine: i + 1})
	}
	for i := prefix; i < len(old)-suffix; i++ {
		body = append(body, contracts.DiffLine{Kind: "remove", Text: old[i], OldLine: i + 1})
	}
	for i := prefix; i < len(new)-suffix; i++ {
		body = append(body, contracts.DiffLine{Kind: "add", Text: new[i], NewLine: i + 1})
	}
	for i := 0; i < tail; i++ {
		oi, ni := len(old)-suffix+i, len(new)-suffix+i
		body = append(body, contracts.DiffLine{Kind: "context", Text: old[oi], OldLine: oi + 1, NewLine: ni + 1})
	}
	oldCount, newCount := prefix-from+removed+tail, prefix-from+added+tail
	oldStart, newStart := from+1, from+1
	if oldCount == 0 {
		oldStart = 0
	}
	if newCount == 0 {
		newStart = 0
	}
	return added, removed, []contracts.DiffHunk{{Header: fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldCount, newStart, newCount), OldStart: oldStart, NewStart: newStart, Lines: body}}
}
