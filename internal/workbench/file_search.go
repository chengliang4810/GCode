package workbench

import (
	"context"
	"strings"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type fileSearchState struct {
	result     contracts.WorkspaceSearchResult
	selected   int
	rows       []contracts.File
	collapsed  map[string]bool
	input      ui.Handle
	list       ui.ListState
	loading    bool
	error      string
	generation uint64
	cancel     context.CancelFunc
}

func (s *State) cancelFileSearch() {
	if s.search.cancel != nil {
		s.search.cancel()
		s.search.cancel = nil
	}
	s.search.generation++
	s.search.loading = false
}
func (s *State) clearFileSearch() {
	s.cancelFileSearch()
	s.fileSearch = ""
	s.search.result = contracts.WorkspaceSearchResult{}
	s.search.rows = nil
	s.search.selected = 0
}

// refreshFileSearch is an input event handler, never called while building View.
func (s *State) refreshFileSearch() {
	s.cancelFileSearch()
	s.search.result = contracts.WorkspaceSearchResult{}
	s.search.rows = nil
	s.search.collapsed = map[string]bool{}
	s.search.selected = 0
	s.search.error = ""
	if strings.TrimSpace(s.fileSearch) == "" {
		return
	}
	service, ok := s.backend.(contracts.WorkspaceSearchService)
	if !ok {
		s.search.error = "Workspace search is unavailable"
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.search.cancel = cancel
	s.search.loading = true
	generation := s.search.generation
	projectID := s.ProjectID
	request := contracts.WorkspaceSearchRequest{ProjectID: projectID, Query: s.fileSearch, FileLimit: 200, NamesOnly: true, IncludeHidden: s.snapshot.Settings.Files.ShowHidden}
	finish := func(result contracts.WorkspaceSearchResult, err error) {
		if s.closed.Load() || ctx.Err() != nil || s.search.generation != generation || s.ProjectID != projectID {
			return
		}
		s.search.loading = false
		if err != nil {
			s.search.error = err.Error()
		} else {
			s.search.result = result
			s.rebuildSearchRows()
		}
	}
	if s.win == nil {
		result, err := service.SearchWorkspace(ctx, request)
		finish(result, err)
		return
	}
	win := s.win
	go func() {
		timer := time.NewTimer(200 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		result, err := service.SearchWorkspace(ctx, request)
		if ctx.Err() != nil {
			return
		}
		win.Update(func() { finish(result, err) })
	}()
}
func (s *State) rebuildSearchRows() {
	byID := map[string]contracts.File{}
	for _, file := range s.snapshot.Files {
		if s.visibleFile(file) {
			byID[file.ID] = file
		}
	}
	included := map[string]bool{}
	for _, found := range s.search.result.Files {
		for file, ok := byID[found.FileID]; ok && !included[file.ID]; file, ok = byID[file.ParentID] {
			included[file.ID] = true
			if file.ParentID == "" {
				break
			}
		}
	}
	s.search.rows = nil
	var walk func(string)
	walk = func(parent string) {
		for _, file := range s.filesIn(parent) {
			if !included[file.ID] {
				continue
			}
			s.search.rows = append(s.search.rows, file)
			if file.Kind == "directory" && !s.search.collapsed[file.ID] {
				walk(file.ID)
			}
		}
	}
	walk("")
	s.search.selected = max(0, min(max(0, len(s.search.rows)-1), s.search.selected))
}
func (s *State) moveFileSearch(delta int) {
	if len(s.search.rows) > 0 {
		s.search.selected = max(0, min(len(s.search.rows)-1, s.search.selected+delta))
		s.search.list.ScrollTo(s.search.selected, ui.Center)
	}
}
func (s *State) activateFileSearch() {
	if s.search.selected >= len(s.search.rows) {
		return
	}
	file := s.search.rows[s.search.selected]
	if file.Kind == "directory" {
		s.search.collapsed[file.ID] = !s.search.collapsed[file.ID]
		s.rebuildSearchRows()
		return
	}
	s.OpenFile(file.ID)
}
func (s *State) fileSearchResults(c *ui.Context) {
	p := s.palette()
	q := &s.search
	if len(q.rows) == 0 {
		label := "No files found."
		if q.loading {
			label = "Searching…"
		} else if q.error != "" {
			label = q.error
		}
		ui.Text(c, tr(c, label)).Padding(20, 10).TextColor(p.Faint).FontSize(design.UISize(c, 11.5))
		return
	}
	if len(q.result.Files) >= 200 {
		ui.Text(c, tr(c, "Showing the first 200 matches")).Padding(5, 10).FontSize(design.UISize(c, 10)).TextColor(p.Faint)
	}
	q.list.Key = func(i int) any { return q.rows[i].ID }
	q.list.Label = func(i int) string { return q.rows[i].Name }
	ui.List(c, &q.list, len(q.rows), func(i int) {
		file := q.rows[i]
		depth := strings.Count(strings.Trim(file.Path, "/"), "/")
		button := ui.ButtonBase(c).Label(file.Name).Height(27).FillWidth().Padding(0, 8, 0, 8+14*float32(depth)).Gap(4).AlignItems(ui.Center).Drag(FileDrag{ID: file.ID, ProjectID: file.ProjectID, Path: file.Path, Directory: file.Kind == "directory"}).OnClick(func() { q.selected = i; s.activateFileSearch() }).ContextMenu(func(menu *ui.Menu) { s.fileContext(c, file, menu) })
		if i == q.selected {
			button.Background(sourceInk(p.Dark, .1))
		} else if button.Hovered() {
			button.Background(sourceInk(p.Dark, .055))
		}
		button.Children(func() {
			if file.Kind == "directory" {
				glyph := "alt-arrow-down"
				if q.collapsed[file.ID] {
					glyph = "alt-arrow-right"
				}
				ui.Icon(c, assets.Icon(glyph)).Size(14, 14)
			} else {
				ui.Box(c).Size(14, 14)
			}
			ui.Image(c, sourceFileIcon(file, file.Kind == "directory" && !q.collapsed[file.ID], p.Dark)).Size(14, 14)
			ui.Text(c, file.Name).SingleLine().MinWidth(0).FontSize(design.UISize(c, 11.5)).TextColor(p.Muted)
		})
	}).Label(tr(c, "Fuzzy workspace file results")).Grow(1).FillWidth()
}
