package workbench

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type workspaceScope struct {
	tabs                                []surface
	recentFiles                         []string
	search                              fileSearchState
	active                              int
	tabScroll                           ui.ScrollState
	documents                           map[string]*document
	diffs                               map[string]*editor.DiffState
	folders                             map[string]*bool
	fileSearch, historySearch, commitID string
	pendingTerminal                     bool
}

// SwitchSession keeps tabs, editor buffers/selections, tree state and scroll
// positions owned by their conversation. It never executes backend commands.
func (s *State) SwitchSession(id string) {
	if s.closed.Load() || s.scopeID == id {
		return
	}
	s.StopAnnotations()
	s.cancelLink()
	s.pendingFile, s.pendingNavigation = "", nil
	s.cancelFileSearch()
	for _, document := range s.documents {
		if document.autosave != nil {
			document.autosave.Stop()
			document.autosave = nil
		}
	}
	s.scopes[s.scopeID] = workspaceScope{
		tabs: slices.Clone(s.tabs), active: s.active, tabScroll: s.tabScroll,
		recentFiles: slices.Clone(s.recentFiles),
		search:      s.search,
		documents:   s.documents, diffs: s.diffs, folders: s.folders,
		fileSearch: s.fileSearch, historySearch: s.historySearch, commitID: s.commitID,
		pendingTerminal: s.pendingTerminal,
	}
	next := s.scopes[id]
	s.scopeID = id
	s.tabs, s.active, s.tabScroll = slices.Clone(next.tabs), next.active, next.tabScroll
	s.recentFiles = slices.Clone(next.recentFiles)
	s.search = next.search
	s.documents, s.diffs, s.folders = next.documents, next.diffs, next.folders
	if s.documents == nil {
		s.documents = map[string]*document{}
	}
	if s.diffs == nil {
		s.diffs = map[string]*editor.DiffState{}
	}
	if s.folders == nil {
		s.folders = map[string]*bool{}
	}
	s.fileSearch, s.historySearch, s.commitID = next.fileSearch, next.historySearch, next.commitID
	s.pendingTerminal = next.pendingTerminal
	s.pendingClose, s.Error = "", ""
	s.fileActionOpen, s.imageOpen, s.commitOpen = false, false, false
	for i, tab := range s.tabs {
		if browser := s.browsers[tab.id]; tab.kind == "browser" && browser != nil {
			s.tabs[i].title = browser.data.Title
		}
	}
	s.Update(s.snapshot)
	if len(s.tabs) > 0 {
		s.selectTab(min(s.active, len(s.tabs)-1))
	}
}
