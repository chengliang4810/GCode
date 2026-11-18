package workbench

import "github.com/egoist/mygo/ui/editor"

type fileChange struct {
	ID, ProjectID, Path    string
	OldContent, NewContent string
	Additions, Deletions   int
	counted                bool
	synced                 *editor.DiffState
}

func (ch *fileChange) diff() *editor.DiffState {
	old, next := editor.NewBuffer(ch.OldContent), editor.NewBuffer(ch.NewContent)
	language := editor.LanguageFor(ch.Path)
	old.SetLanguage(language)
	next.SetLanguage(language)
	d := &editor.DiffState{Old: old, New: next, Layout: editor.DiffUnified}
	ch.synced = d
	return d
}

func (ch *fileChange) refreshDiff(d *editor.DiffState) {
	if ch.synced == d {
		return
	}
	d.Old.Reset(ch.OldContent)
	d.New.Reset(ch.NewContent)
	ch.synced = d
}

func (ch *fileChange) stats() (int, int) {
	if !ch.counted {
		d := editor.DiffState{Old: editor.NewBuffer(ch.OldContent), New: editor.NewBuffer(ch.NewContent)}
		ch.Additions, ch.Deletions = d.Stats()
		ch.counted = true
	}
	return ch.Additions, ch.Deletions
}

func (s *State) groupedChanges() []fileChange {
	if s.fileChanges != nil {
		return s.fileChanges
	}
	type fileKey struct{ project, path string }
	index := make(map[fileKey]int)
	s.fileChanges = make([]fileChange, 0)
	for _, ch := range s.snapshot.Changes {
		key := fileKey{ch.ProjectID, ch.Path}
		if i, ok := index[key]; ok {
			s.fileChanges[i].NewContent = ch.NewContent
			continue
		}
		index[key] = len(s.fileChanges)
		s.fileChanges = append(s.fileChanges, fileChange{
			ID: ch.ID, ProjectID: ch.ProjectID, Path: ch.Path,
			OldContent: ch.OldContent, NewContent: ch.NewContent,
		})
	}
	return s.fileChanges
}

func (s *State) fileChange(id string) *fileChange {
	changes := s.groupedChanges()
	for i := range changes {
		if changes[i].ID == id {
			return &changes[i]
		}
	}
	return nil
}
