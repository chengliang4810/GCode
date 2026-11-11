// Package workbench implements the native workspace surfaces of the desktop shell.
package workbench

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/linktarget"
	"gcode/internal/linkui"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type surface struct{ kind, id, title string }

// FileDrag is an app-local workspace path, never a claim that a real local file
// exists. A composer may receive it as a file mention with ui.Drop[FileDrag].
type FileDrag struct {
	ID, ProjectID, Path string
	Directory           bool
}

type document struct {
	file            contracts.File
	editor          *editor.State
	conflict        bool
	closeAfterSave  bool
	image           ui.ImageSource
	imageError      string
	imageGeneration uint64
	preview         bool
	autosave        *time.Timer
	deleted         bool
	focus           bool
	viewer          ImageViewport
	sourceSession   string
	absolutePath    string
	linked          bool
	markdown        markdown.Options
}

// State owns persistent widget state and native resources. Call Update for every
// committed backend snapshot, on the UI thread, before drawing. Dispatch may be
// supplied by the application to execute commands asynchronously.
type State struct {
	win               *mygo.Window
	backend           contracts.Service
	ctx               context.Context
	cancel            context.CancelFunc
	closed            atomic.Bool
	snapshot          contracts.Snapshot
	scopeID           string
	scopes            map[string]workspaceScope
	tabs              []surface
	active            int
	tabScroll         ui.ScrollState
	tabObserver       tabStripObserver
	tabTooltip        tabTooltipState
	revealTab         bool
	systemDark        bool
	documents         map[string]*document
	diffs             map[string]*editor.DiffState
	browsers          map[string]*browser
	terminals         map[string]*terminalView
	folders           map[string]*bool
	fileChildrenIndex map[string][]contracts.File
	fileIndexProject  string
	fileIndexHidden   bool
	fileSearch        string
	search            fileSearchState
	recentFiles       []string
	pendingClose      string
	Error             string
	ProjectID         string
	Dispatch          func(contracts.Command)
	OnOpenSession     func(string)
	OnSessionVisible  func(string)
	OnSessionHidden   func(string)
	OnCloseSession    func(string)
	RenderSession     func(*ui.Context, string)
	RenderTrajectory  func(*ui.Context, string)
	RenderGoal        func(*ui.Context, string)
	OnNewSideChat     func()
	CanNewSideChat    func() bool
	OnOpenSurface     func()
	OnCloseSurface    func()
	OnToggleExpand    func()
	OnAnnotation      func(contracts.Annotation)
	OnLinkConfig      func(sessionID, documentPath string) linkui.Config
	nextAnnotation    uint32
	nextBrowser       int
	pendingTerminal   bool
	pendingFile       string
	historySearch     string
	commitID          string
	commitOpen        bool
	fileActionOpen    bool
	fileAction        string
	fileActionID      string
	fileActionName    string
	fileActionHost    string
	imageOpen         bool
	imageID           string
	imageViewer       ImageViewport
	linkCancel        context.CancelFunc
	linkGeneration    uint64
	pendingNavigation *fileNavigation
}

func New(win *mygo.Window, backend contracts.Service) *State {
	ctx, cancel := context.WithCancel(context.Background())
	return &State{win: win, backend: backend, ctx: ctx, cancel: cancel,
		documents: map[string]*document{}, diffs: map[string]*editor.DiffState{},
		browsers: map[string]*browser{}, terminals: map[string]*terminalView{}, folders: map[string]*bool{}, scopes: map[string]workspaceScope{}}
}

// UpdateTranscript accepts a message-only projection without reconciling files,
// tabs and terminals that this event cannot change.
func (s *State) UpdateTranscript(snapshot contracts.Snapshot) {
	if !s.closed.Load() {
		s.snapshot = snapshot
	}
}

// Update never overwrites an unsaved local edit with an unrelated server edit.
func (s *State) Update(snapshot contracts.Snapshot) {
	if s.closed.Load() {
		return
	}
	s.snapshot = snapshot
	s.fileChildrenIndex = nil
	for i := len(s.tabs) - 1; i >= 0; i-- {
		if s.tabs[i].kind != "trajectory" && s.tabs[i].kind != "goal" {
			continue
		}
		found := false
		for _, session := range snapshot.Sessions {
			if session.ID == s.tabs[i].id {
				s.tabs[i].title = session.Title
				found = true
				break
			}
		}
		if !found {
			s.removeTab(i)
		}
	}
	for id, d := range s.documents {
		found := false
		for _, f := range snapshot.Files {
			if f.ID != id {
				continue
			}
			found = true
			d.linked = false
			for i := range s.tabs {
				if s.tabs[i].kind == "file" && s.tabs[i].id == f.ID {
					s.tabs[i].title = f.Name
				}
			}
			d.deleted = false
			if f.Kind == "image" {
				changed := imageResourceChanged(d.file, f)
				d.file, d.conflict = f, false
				if changed {
					d.image, d.imageError = nil, ""
					s.loadDocumentImage(d)
				}
				continue
			}
			if f.Version == d.file.Version {
				// Version equality keeps the accepted edit/conflict baseline;
				// metadata can still change after deletion, policy, or loading.
				content, version := d.file.Content, d.file.Version
				d.file = f
				d.file.Content, d.file.Version = content, version
				d.editor.ReadOnly, d.editor.Label = f.ReadOnly, f.Name
				continue
			}
			if f.Content == d.editor.Buffer.String() {
				d.editor.Buffer.MarkSaved()
				d.conflict = false
				d.file = f
				if d.closeAfterSave {
					s.removeFile(id)
				}
			} else if f.Content == d.file.Content {
				d.file = f
			} else if d.editor.Buffer.Modified() {
				d.conflict = true
			} else {
				d.editor.Buffer.Reset(f.Content)
				d.file = f
				d.conflict = false
			}
		}
		if !found && !d.linked {
			d.deleted = true
			d.file.ReadOnly = true
		}
	}
	liveTerminals := make(map[string]bool, len(snapshot.Terminals))
	for _, t := range snapshot.Terminals {
		liveTerminals[t.ID] = true
		if v := s.terminals[t.ID]; v != nil {
			v.update(t)
			s.configureTerminal(v)
		}
	}
	for id, v := range s.terminals {
		if liveTerminals[id] {
			continue
		}
		v.close()
		delete(s.terminals, id)
		for i := len(s.tabs) - 1; i >= 0; i-- {
			if s.tabs[i].kind == "terminal" && s.tabs[i].id == id {
				s.removeTab(i)
			}
		}
	}
	if s.pendingTerminal {
		for _, t := range snapshot.Terminals {
			if s.terminals[t.ID] == nil && (s.ProjectID == "" || t.ProjectID == s.ProjectID) && (snapshot.Runtime == nil || t.Status == "running" && t.SessionID == s.scopeID) {
				s.pendingTerminal = false
				s.openTerminal(t)
				break
			}
		}
	}
	if s.pendingFile != "" {
		for _, file := range snapshot.Files {
			if file.ID == s.pendingFile && file.Loaded {
				id := s.pendingFile
				s.pendingFile = ""
				s.OpenFile(id)
				s.applyPendingNavigation(id)
				break
			}
		}
	}
}

func (s *State) command(command contracts.Command) {
	if s.closed.Load() {
		return
	}
	if s.Dispatch != nil {
		s.Dispatch(command)
		return
	}
	if s.backend == nil {
		s.Error = "The workspace is not connected."
		return
	}
	if err := s.backend.Execute(s.ctx, command); err != nil {
		s.Error = err.Error()
		return
	}
	if snap, err := s.backend.Snapshot(s.ctx); err == nil {
		s.Update(snap)
	} else {
		s.Error = err.Error()
	}
}

func (s *State) palette() design.Tokens {
	a := s.snapshot.Settings.Appearance
	dark := strings.EqualFold(a.Mode, "dark") || strings.EqualFold(a.Mode, "system") && s.systemDark
	id := a.ThemeID
	if id == "" {
		if dark {
			id = a.DarkThemeID
		} else {
			id = a.LightThemeID
		}
	}
	return design.ApplyAccent(design.Palette(id, dark), a.Accent)
}

func (s *State) choose(kind, id, title string) {
	if s.closed.Load() {
		return
	}
	if s.OnOpenSurface != nil {
		s.OnOpenSurface()
	}
	for i, t := range s.tabs {
		if t.kind == kind && t.id == id {
			s.selectTab(i)
			if (kind == "session" || kind == "trajectory" || kind == "goal") && s.OnSessionVisible != nil {
				s.OnSessionVisible(id)
			}
			return
		}
	}
	s.tabs = append(s.tabs, surface{kind, id, title})
	s.selectTab(len(s.tabs) - 1)
	if (kind == "session" || kind == "trajectory" || kind == "goal") && s.OnSessionVisible != nil {
		s.OnSessionVisible(id)
	}
}

func (s *State) selectTab(index int) {
	if index < 0 || index >= len(s.tabs) {
		return
	}
	s.cancelLink()
	s.active = index
	s.revealTab = true
	t := s.tabs[index]
	for id, b := range s.browsers {
		if t.kind != "browser" || t.id != id {
			b.stopAnnotation()
		} else {
			b.focusBody = true
		}
	}
	if t.kind == "terminal" {
		if v := s.terminals[t.id]; v != nil {
			v.focusBody = true
		}
	}
}

func (s *State) OpenFile(id string) {
	if s.closed.Load() {
		return
	}
	s.cancelLink()
	for _, f := range s.snapshot.Files {
		if f.ID == id && f.Kind != "directory" {
			if s.snapshot.Runtime != nil && !f.Loaded {
				s.pendingFile = id
				s.command(contracts.Command{Kind: "file.open", ID: id})
				return
			}
			s.recentFiles = append([]string{id}, slices.DeleteFunc(s.recentFiles, func(previous string) bool { return previous == id })...)
			s.openDocument(f, s.scopeID, "", false)
			s.documents[id].focus = true
			s.choose("file", id, f.Name)
			return
		}
	}
}

func (s *State) OpenChange(id string) {
	if s.closed.Load() {
		return
	}
	for _, ch := range s.snapshot.Changes {
		if ch.ID == id {
			if s.diffs[id] == nil {
				old, newText := editor.NewBuffer(ch.OldContent), editor.NewBuffer(ch.NewContent)
				old.SetLanguage(editor.LanguageFor(ch.Path))
				newText.SetLanguage(editor.LanguageFor(ch.Path))
				s.diffs[id] = &editor.DiffState{Old: old, New: newText, Layout: editor.DiffUnified}
			}
			s.choose("diff", id, path.Base(ch.Path))
			return
		}
	}
}

// OpenSession opens a native side transcript when its renderer is available.
// Hosts without a side renderer keep their existing central navigation.
func (s *State) OpenSession(id string) {
	if s.closed.Load() || id == "" {
		return
	}
	if s.RenderSession == nil {
		if s.OnOpenSession != nil {
			s.OnOpenSession(id)
		}
		return
	}
	title := "Conversation"
	for _, session := range s.snapshot.Sessions {
		if session.ID == id {
			title = session.Title
			break
		}
	}
	s.choose("session", id, title)
}

func (s *State) OpenSurface(kind string) {
	if s.closed.Load() {
		return
	}
	s.Error = ""
	switch kind {
	case "sidechat":
		if s.canNewSideChat() {
			s.OnNewSideChat()
		}
	case "browser":
		s.openBrowser("")
	case "terminal":
		for _, t := range s.snapshot.Terminals {
			if (s.ProjectID == "" || t.ProjectID == s.ProjectID) && (s.snapshot.Runtime == nil || t.Status == "running" && t.SessionID == s.scopeID) {
				s.openTerminal(t)
				return
			}
		}
		s.NewTerminal()
	case "history":
		s.choose(kind, "history", "History")
	case "changes":
		s.choose(kind, "changes", "Changes")
	case "files":
		s.choose("files", "files", "Files")
	default:
		s.choose("picker", "picker", "Workspace")
	}
}

func (s *State) NewTerminal() {
	if s.closed.Load() {
		return
	}
	s.pendingTerminal = true
	s.command(contracts.Command{Kind: "terminal.create", ProjectID: s.ProjectID, SessionID: s.scopeID})
}

// VisibleSessionIDs returns the conversation IDs whose details may be rendered
// by this workspace. The current workspace scope is included even when no
// session tab is open, followed by session, trajectory, and goal tabs.
func (s *State) VisibleSessionIDs() []string {
	if s.closed.Load() {
		return nil
	}
	ids := make([]string, 0, 1+len(s.tabs))
	if s.scopeID != "" {
		ids = append(ids, s.scopeID)
	}
	for _, tab := range s.tabs {
		switch tab.kind {
		case "session", "trajectory", "goal":
			if tab.id != "" {
				ids = append(ids, tab.id)
			}
		}
	}
	return ids
}

func (s *State) ActiveKind() string {
	if len(s.tabs) == 0 {
		return "picker"
	}
	return s.tabs[s.active].kind
}
func (s *State) TabCount() int { return len(s.tabs) }

func (s *State) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.cancel()
	s.cancelLink()
	for _, b := range s.browsers {
		b.close()
	}
	for _, t := range s.terminals {
		t.close()
	}
	for _, d := range s.documents {
		if d.autosave != nil {
			d.autosave.Stop()
		}
	}
	for _, scope := range s.scopes {
		for _, document := range scope.documents {
			if document.autosave != nil {
				document.autosave.Stop()
			}
		}
	}
	return nil
}

func (s *State) removeFile(id string) {
	if d := s.documents[id]; d != nil && d.autosave != nil {
		d.autosave.Stop()
	}
	delete(s.documents, id)
	for i, t := range s.tabs {
		if t.kind == "file" && t.id == id {
			s.removeTab(i)
			return
		}
	}
}

func (s *State) CloseActive() {
	if len(s.tabs) > 0 {
		s.closeTab(s.active)
	}
}
func (s *State) closeTab(index int) {
	if index < 0 || index >= len(s.tabs) {
		return
	}
	t := s.tabs[index]
	if t.kind == "session" && s.OnCloseSession != nil {
		s.OnCloseSession(t.id)
	}
	if t.kind == "file" {
		if d := s.documents[t.id]; d != nil && d.editor.Buffer.Modified() {
			s.pendingClose = t.id
			return
		}
		if d := s.documents[t.id]; d != nil && d.autosave != nil {
			d.autosave.Stop()
		}
		delete(s.documents, t.id)
	}
	if t.kind == "browser" {
		if b := s.browsers[t.id]; b != nil {
			b.close()
		}
		delete(s.browsers, t.id)
	}
	if t.kind == "terminal" {
		if v := s.terminals[t.id]; v != nil {
			v.close()
			delete(s.terminals, t.id)
		}
		s.command(contracts.Command{Kind: "terminal.close", ID: t.id})
	}
	s.removeTab(index)
	if (t.kind == "session" || t.kind == "trajectory" || t.kind == "goal") && s.OnSessionHidden != nil {
		s.OnSessionHidden(t.id)
	}
}
func (s *State) removeTab(index int) {
	if index < 0 || index >= len(s.tabs) {
		return
	}
	active := s.active
	if index < active {
		active--
	}
	s.tabs = append(s.tabs[:index], s.tabs[index+1:]...)
	if len(s.tabs) == 0 {
		s.active = 0
		if s.OnCloseSurface != nil {
			s.OnCloseSurface()
		}
	} else {
		s.selectTab(min(active, len(s.tabs)-1))
	}
}

func (s *State) View(c *ui.Context) { s.view(c, true, 0) }

// Body draws a workspace whose TabStrip is hosted by the shell titlebar.
func (s *State) Body(c *ui.Context) { s.view(c, false, 0) }

// BodyBelow draws Body under a shell titlebar band: the surface still fills
// the window height behind the band, while its own content starts below it.
func (s *State) BodyBelow(c *ui.Context, top float32) { s.view(c, false, top) }

func (s *State) view(c *ui.Context, ownTabs bool, top float32) {
	s.syncAppearance(c)
	p := s.palette()
	container := ui.Column(c).Key("workspace:"+s.scopeID).Label(tr(c, "Workspace surface")).Fill().MinWidth(0).Padding(top, 0, 0, 0).Background(design.PanelBackground(p, s.snapshot.Settings.Appearance.Surface)).TextColor(p.Text).Font(design.Font(s.snapshot.Settings.Appearance.Font))
	if value, ok := ui.Drop[FileDrag](container); ok {
		s.OpenFile(value.ID)
	}
	if _, over := ui.DragOver[FileDrag](container); over {
		container.Border(1, p.Accent)
	}
	container.Children(func() {
		if ownTabs && len(s.tabs) > 0 {
			s.TabStrip(c, false)
		}
		if s.Error != "" {
			ui.Text(c, s.Error).TextColor(p.Danger).Padding(8, 12)
		}
		if s.pendingClose != "" {
			s.closeConfirmation(c)
			return
		}
		if len(s.tabs) == 0 {
			s.picker(c)
			return
		}
		t := s.tabs[s.active]
		switch t.kind {
		case "file":
			s.fileView(c, t.id)
		case "files":
			s.FilesView(c)
		case "diff":
			s.diffView(c, t.id)
		case "changes":
			s.changesView(c)
		case "history":
			s.historyView(c)
		case "browser":
			s.browserView(c, t.id)
		case "terminal":
			s.terminalView(c, t.id)
		case "session":
			if s.RenderSession != nil {
				s.RenderSession(c, t.id)
			}
		case "trajectory":
			if s.RenderTrajectory != nil {
				s.RenderTrajectory(c, t.id)
			}
		case "goal":
			if s.RenderGoal != nil {
				s.RenderGoal(c, t.id)
			}
		default:
			s.picker(c)
		}
	})
	s.imageDialog(c)
}

func (s *State) iconButton(c *ui.Context, icon, label string, action func()) {
	ui.ButtonBase(c).Label(tr(c, label)).Size(24, 24).Radius(6).Tooltip(tr(c, label)).OnClick(action).Children(func() { ui.Icon(c, assets.Icon(icon)).Size(16, 16) })
}

func (s *State) picker(c *ui.Context) {
	p := s.palette()
	items := []struct{ kind, title, icon string }{}
	if s.OnNewSideChat != nil && s.scopeID != "" {
		items = append(items, struct{ kind, title, icon string }{"sidechat", "Side chat", "chat-round-line"})
	}
	items = append(items, struct{ kind, title, icon string }{"browser", "Browser", "globe"}, struct{ kind, title, icon string }{"terminal", "Terminal", "terminal"})
	if s.hasGit() {
		items = append(items, struct{ kind, title, icon string }{"changes", "Changes", "list"}, struct{ kind, title, icon string }{"history", "History", "git-branch"})
	}
	ui.Column(c).Grow(1).FillWidth().Padding(16).Center().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(280).Gap(8).Children(func() {
			for _, item := range items {
				key := item.kind
				if item.kind == "sidechat" {
					key = "side-chat-launcher"
				}
				c.Key(key)
				b := ui.ButtonBase(c).Label(tr(c, item.title)).FillWidth().Height(44).Padding(0, 14).Radius(10).Border(1, p.Border).Gap(10).AlignItems(ui.Center).Background(sourceInk(p.Dark, .02)).OnClick(func() { s.OpenSurface(item.kind) })
				if item.kind == "sidechat" {
					b.Disabled(!s.canNewSideChat())
				}
				if b.Hovered() {
					b.Background(sourceInk(p.Dark, .05)).BorderColor(p.BorderStrong)
				}
				b.Children(func() {
					ui.Icon(c, assets.Icon(item.icon)).Size(15, 15).TextColor(p.Muted)
					ui.Text(c, tr(c, item.title)).FontWeight(500).TextColor(p.Text)
				})
			}
		})
	})
}

func (s *State) canNewSideChat() bool {
	return s.OnNewSideChat != nil && s.scopeID != "" && (s.CanNewSideChat == nil || s.CanNewSideChat())
}

func (s *State) hasGit() bool {
	for _, project := range s.snapshot.Projects {
		if project.ID == s.ProjectID && project.GitDetected {
			return true
		}
	}
	return false
}

func sourceInk(dark bool, alpha float32) ui.Color {
	if dark {
		return ui.RGBA(255, 255, 255, alpha)
	}
	return ui.RGBA(0, 0, 0, alpha)
}

func (s *State) FilesView(c *ui.Context) {
	s.filesView(c, "workspace", &s.search.workspace, 0)
}

// FilesDockView shares conversation data with the workspace explorer, while
// keeping each displayed control's identity, focus and scroll state separate.
func (s *State) FilesDockView(c *ui.Context) {
	s.filesView(c, "dock", &s.search.dock, 0)
}

// FilesDockBelow draws the dock under the shell titlebar band, its surface
// reaching the window's top edge.
func (s *State) FilesDockBelow(c *ui.Context, top float32) {
	s.filesView(c, "dock", &s.search.dock, top)
}

func (s *State) filesView(c *ui.Context, host string, controls *fileExplorerControls, top float32) {
	s.syncAppearance(c)
	p := s.palette()
	ui.Column(c.Key("files-explorer:"+s.scopeID+":"+host)).Label(tr(c, "Files explorer")).Fill().MinWidth(0).Padding(top, 0, 0, 0).Background(design.PanelBackground(p, s.snapshot.Settings.Appearance.Surface)).TextColor(p.Text).Children(func() {
		ui.Row(c).Height(38).Padding(0, 8).Gap(4).AlignItems(ui.Center).BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Background(p.Shell).Children(func() {
			ui.Row(c).Grow(1).Height(24).Padding(0, 8).Gap(6).AlignItems(ui.Center).Radius(6).Background(sourceInk(p.Dark, .035)).Children(func() {
				ui.Icon(c, assets.Icon("magnifer")).Size(12, 12).TextColor(p.Faint)
				input := ui.TextInputBase(c, &s.fileSearch).Bind(&controls.input).Placeholder(tr(c, "Search files")).Label(tr(c, "Search files")).Grow(1).FontSize(design.TextSize(c, design.TextBase)).OnChange(s.refreshFileSearch).OnShortcut(0, ui.KeyEscape, func() { s.clearFileSearch(); controls.tree.Focus() })
				input.OnShortcut(0, ui.KeyDown, func() { s.moveFileSearch(1, controls) }).OnShortcut(0, ui.KeyUp, func() { s.moveFileSearch(-1, controls) }).OnSubmit(s.activateFileSearch)
			})
			icon, label := "eye-closed", "Show all files (even hidden)"
			if s.snapshot.Settings.Files.ShowHidden {
				icon = "eye"
				label = "Hide hidden and ignored files"
			}
			s.iconButton(c, icon, label, func() {
				settings := s.snapshot.Settings
				settings.Files.ShowHidden = !settings.Files.ShowHidden
				s.command(contracts.Command{Kind: "settings.update", Settings: &settings})
				if s.fileSearch != "" {
					s.refreshFileSearch()
				}
			})
		})
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(4, 0).Children(func() {
			s.projectFilesStatus(c)
			if s.fileSearch != "" {
				s.fileSearchResults(c, host, controls)
				return
			}
			ui.Scroll(c).Grow(1).FillWidth().Children(func() { ui.Tree(c, func() { s.fileChildren(c, "", host) }).Bind(&controls.tree).FillWidth() })
		}).ContextMenu(func(m *ui.Menu) {
			if m.Item(tr(c, "New file…")).Chosen() {
				s.requestFileAction("create", "", host)
			}
			if m.Item(tr(c, "New folder…")).Chosen() {
				s.requestFileAction("mkdir", "", host)
			}
		})
	})
	if s.fileActionHost == host {
		s.fileActionDialog(c)
	}
}

func (s *State) fileChildren(c *ui.Context, parent, host string) {
	s.fileChildrenLevel(c, parent, host, 0)
}

func (s *State) fileChildrenLevel(c *ui.Context, parent, host string, depth int) {
	p := s.palette()
	for _, f := range s.filesIn(parent) {
		c.Key(f.ID)
		if f.Kind == "directory" {
			open := s.folders[f.ID]
			if open == nil {
				v := true
				open = &v
				s.folders[f.ID] = open
			}
			row := ui.TreeItem(c, "", open, func() { s.fileChildrenLevel(c, f.ID, host, depth+1) }).Label(f.Name).Drag(FileDrag{ID: f.ID, ProjectID: f.ProjectID, Path: f.Path, Directory: true}).OnClick(func() { *open = !*open }).ContextMenu(func(m *ui.Menu) { s.fileContext(c, f, m, host) })
			s.styleFileRow(c, row, f, depth, *open, p)
		} else {
			row := ui.TreeItem(c, "", nil, nil).Label(f.Name).Selected(s.activeFile() == f.ID).Drag(FileDrag{ID: f.ID, ProjectID: f.ProjectID, Path: f.Path}).OnClick(func() { s.OpenFile(f.ID) }).ContextMenu(func(m *ui.Menu) { s.fileContext(c, f, m, host) })
			s.styleFileRow(c, row, f, depth, false, p)
		}
	}
}

func (s *State) styleFileRow(c *ui.Context, row ui.Element, f contracts.File, depth int, expanded bool, p design.Tokens) {
	// The official Tree's disclosure slot is 16px. Its two extra pixels
	// come out of the inset, preserving the source icon/text starting x.
	row.Height(27).FillWidth().Gap(0).Padding(0, 8, 0, 6+14*float32(depth)).FontSize(design.TextSize(c, design.TextBase)).TextColor(p.Muted).Radius(0).Draw(func(painter *ui.Painter, r ui.Rect) {
		for level := 0; level < depth; level++ {
			x := r.X + 15 + 14*float32(level)
			painter.Line(x, r.Y, x, r.Y+r.H, 1, p.Border)
		}
	})
	if s.activeFile() == f.ID {
		row.Background(sourceInk(p.Dark, .08)).TextColor(p.Text)
	}
	row.Children(func() {
		ui.Image(c, sourceFileIcon(f, expanded, p.Dark)).Size(14, 14).Margin(0, 4)
		text := ui.Text(c, f.Name).SingleLine()
		if f.Modified {
			text.TextColor(p.Warning)
		}
	})
}

func (s *State) activeFile() string {
	if len(s.tabs) > 0 && s.tabs[s.active].kind == "file" {
		return s.tabs[s.active].id
	}
	return ""
}

func (s *State) fileView(c *ui.Context, id string) {
	d := s.documents[id]
	if d == nil {
		return
	}
	p := s.palette()
	ui.Row(c).Height(34).Padding(4, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, d.file.Path).SingleLine().TextColor(p.Muted).Grow(1)
		if d.file.ReadOnly {
			ui.Text(c, tr(c, "Read only")).TextColor(p.Faint)
		} else if d.editor.Buffer.Modified() {
			ui.Text(c, tr(c, "Unsaved")).TextColor(p.Muted)
		}
		s.iconButton(c, "magnifer", "Find in file", func() { d.editor.Run(editor.Find) })
		if isMarkdown(d.file.Path) {
			label := "Preview"
			if d.preview {
				label = "Edit"
			}
			ui.ButtonBase(c).Label(tr(c, label)).Padding(4, 8).OnClick(func() { d.preview = !d.preview }).Children(func() { ui.Text(c, label) })
		}
		ui.ButtonBase(c).Label(tr(c, "Save file")).Disabled(d.file.ReadOnly || !d.editor.Buffer.Modified()).Padding(4, 8).OnClick(func() { s.save(id) }).Children(func() { ui.Text(c, tr(c, "Save")) })
	})
	if d.conflict {
		ui.Row(c).Padding(8, 12).Gap(8).Children(func() {
			ui.Text(c, tr(c, "This file changed since it was opened.")).TextColor(p.Warning)
			ui.Spacer(c)
			ui.Button(c, tr(c, "Reload")).OnClick(func() {
				for _, f := range s.snapshot.Files {
					if f.ID == id {
						d.file = f
						d.editor.Buffer.Reset(f.Content)
						d.conflict = false
					}
				}
			})
		})
	}
	if d.deleted {
		ui.Text(c, tr(c, "This file no longer exists.")).TextColor(p.Warning).Padding(8, 12)
	}
	if d.file.Kind == "image" {
		s.imagePreview(c, d.file)
		return
	}
	if d.preview {
		services := c.Services()
		preview := ui.Column(c).Grow(1).FillWidth().MinHeight(0).Selectable()
		selection := linkui.BeginSelection(preview)
		var cfg linkui.Config
		if s.OnLinkConfig != nil {
			cfg = s.OnLinkConfig(d.sourceSession, d.absolutePath)
			cfg.Selection = selection
			d.markdown.Block = func(c *ui.Context, document *parser.Document, index int) bool {
				return linkui.Block(c, document, index, cfg, d.markdown.Theme)
			}
		}
		d.markdown.ShareSelection = true
		d.markdown.CodeBlock = design.MarkdownCodeBlock
		d.markdown.LinkMenu = linkui.MarkdownMenu(c, cfg)
		d.markdown.OnLink = func(raw string) {
			if cfg.Open != nil {
				target, err := linktarget.ParsePreview(raw, d.absolutePath)
				if err != nil {
					s.Error = err.Error()
					return
				}
				if target.Kind == linktarget.Mail || target.Kind == linktarget.Web && !cfg.Internal {
					if cfg.OpenExternal != nil {
						cfg.OpenExternal(raw)
					}
				} else {
					cfg.Open(raw)
				}
				return
			}
			if err := s.openPreviewLink(d, raw, services.OpenURL); err != nil {
				s.Error = err.Error()
			}
		}
		preview.Children(func() {
			ui.Scroll(c).Grow(1).FillWidth().Padding(20).Children(func() {
				markdown.View(c, linktarget.NormalizeMarkdown(d.editor.Buffer.String()), &d.markdown)
			})
		})
		selection.Menu(c, preview)
		return
	}
	d.editor.Font = s.codeFont()
	d.editor.LineHeight = 20.0 / 13.0
	d.editor.ReadOnly = d.file.ReadOnly
	d.editor.LineNumbers = editor.LineNumbersOn
	if !s.snapshot.Settings.Files.LineNumbers {
		d.editor.LineNumbers = editor.LineNumbersOff
	}
	d.editor.SoftWrap = s.snapshot.Settings.Files.WordWrap
	d.editor.Theme = codeTheme(c.Theme(), p)
	d.editor.OnInput = func(ui.InputEvent) bool { s.scheduleAutosave(id); return false }
	view := editor.View(c, d.editor).Grow(1).FillWidth()
	if d.focus {
		view.Focus()
		d.focus = false
	}
	services := c.Services()
	c.OnShortcut(ui.Cmd, ui.KeyS, func() { s.save(id); services.Invalidate() })
}

func (s *State) scheduleAutosave(id string) {
	d := s.documents[id]
	if s.win == nil || d == nil || d.file.ReadOnly || !s.snapshot.Settings.Files.Autosave {
		return
	}
	if d.autosave != nil {
		d.autosave.Stop()
	}
	delay := time.Duration(max(100, s.snapshot.Settings.Files.AutosaveDelay)) * time.Millisecond
	d.autosave = time.AfterFunc(delay, func() {
		if s.closed.Load() {
			return
		}
		s.win.Update(func() {
			if !s.closed.Load() && s.documents[id] == d && s.snapshot.Settings.Files.Autosave && !d.conflict {
				s.save(id)
			}
		})
	})
}

func codeTheme(base *ui.Theme, p design.Tokens) *editor.Theme {
	t := editor.DefaultTheme(base)
	t.Background = p.Background
	t.Text = p.Text
	t.LineNumber = p.Faint
	t.ActiveLineNumber = p.Text
	t.Selection = p.Selection
	t.Cursor = p.Caret
	t.Added = p.DiffAdd
	t.Deleted = p.DiffDelete
	t.Modified = p.Warning
	for _, item := range []struct {
		kind editor.TokenKind
		role string
	}{
		{editor.TokenKeyword, "keyword"}, {editor.TokenType, "type"}, {editor.TokenFunction, "function"}, {editor.TokenVariable, "variable"}, {editor.TokenProperty, "property"}, {editor.TokenConstant, "constant"}, {editor.TokenNumber, "number"}, {editor.TokenString, "string"}, {editor.TokenEscape, "escape"}, {editor.TokenRegexp, "stringSpecial"}, {editor.TokenComment, "comment"}, {editor.TokenOperator, "operator"}, {editor.TokenPunctuation, "punctuation"}, {editor.TokenTag, "tag"}, {editor.TokenAttribute, "attribute"}, {editor.TokenMeta, "macro"}, {editor.TokenInvalid, "invalid"},
	} {
		if color, ok := p.Syntax[item.role]; ok {
			t.Tokens[item.kind].Color = color
		}
	}
	return t
}

func (s *State) imagePreview(c *ui.Context, f contracts.File) {
	d := s.documents[f.ID]
	if d == nil || d.image == nil {
		ui.Text(c, tr(c, "Unable to open image.")).Padding(20)
		return
	}
	d.viewer.View(c, d.image).ContextMenu(func(m *ui.Menu) {
		if m.Item(tr(c, "Fit to window")).Chosen() {
			d.viewer.geometry.fit()
		}
		if m.Item(tr(c, "Actual size")).Chosen() {
			g := &d.viewer.geometry
			g.zoom(1, g.width/2, g.height/2)
		}
		m.Separator()
		if m.Item(tr(c, "Open image")).Chosen() {
			s.imageID = f.ID
			s.imageOpen = true
			s.imageViewer = ImageViewport{}
		}
	})
}

func (s *State) save(id string) {
	d := s.documents[id]
	if d == nil || d.file.ReadOnly || !d.editor.Buffer.Modified() {
		return
	}
	s.command(contracts.Command{Kind: "file.save", ID: id, Text: d.editor.Buffer.String(), Version: d.file.Version})
}

func (s *State) closeConfirmation(c *ui.Context) {
	d := s.documents[s.pendingClose]
	if d == nil {
		s.pendingClose = ""
		return
	}
	ui.Column(c).Grow(1).FillWidth().Center().Gap(16).Children(func() {
		ui.Text(c, fmt.Sprintf(tr(c, "Save changes to %s?"), d.file.Name)).FontSize(design.TextSize(c, design.TextLarge)).FontWeight(500)
		ui.Text(c, tr(c, "Your changes will be lost if you close without saving.")).TextColor(s.palette().Muted)
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, tr(c, "Cancel")).OnClick(func() { s.pendingClose = "" })
			ui.Button(c, tr(c, "Don't save")).OnClick(func() { id := s.pendingClose; s.pendingClose = ""; s.removeFile(id) })
			ui.PrimaryButton(c, tr(c, "Save")).OnClick(func() { id := s.pendingClose; s.pendingClose = ""; d.closeAfterSave = true; s.save(id) })
		})
	})
}

func (s *State) diffView(c *ui.Context, id string) {
	d := s.diffs[id]
	if d == nil {
		return
	}
	ui.Row(c).Height(34).Padding(4, 12).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, tr(c, "Changes")).TextColor(s.palette().Muted)
		ui.Spacer(c)
		ui.Button(c, tr(c, "Unified")).OnClick(func() { d.Layout = editor.DiffUnified })
		ui.Button(c, tr(c, "Split")).OnClick(func() { d.Layout = editor.DiffSplit })
	})
	d.Font = s.codeFont()
	d.Theme = codeTheme(c.Theme(), s.palette())
	editor.DiffView(c, d).Grow(1).FillWidth()
}

func (s *State) changesView(c *ui.Context) {
	p := s.palette()
	ui.Scroll(c).Grow(1).FillWidth().Padding(16).Children(func() {
		ui.Text(c, tr(c, "Changes")).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600).Margin(0, 0, 16, 0)
		for _, ch := range s.snapshot.Changes {
			if s.ProjectID != "" && ch.ProjectID != s.ProjectID {
				continue
			}
			c.Key(ch.ID)
			ui.ButtonBase(c).Label(ch.Path).FillWidth().Padding(10, 8).Radius(6).OnClick(func() { s.OpenChange(ch.ID) }).Children(func() {
				ui.Icon(c, assets.Icon("file-code")).Size(16, 16)
				ui.Text(c, ch.Path).Grow(1).SingleLine()
				ui.Text(c, fmt.Sprintf(tr(c, "+%d"), ch.Additions)).TextColor(p.Success)
				ui.Text(c, fmt.Sprintf(tr(c, "−%d"), ch.Deletions)).TextColor(p.Danger)
			})
		}
		if len(s.snapshot.Changes) == 0 {
			ui.Text(c, tr(c, "No changes")).TextColor(p.Muted)
		}
	})
}

func shortSHA(sha string) string { return sha[:min(7, len(sha))] }
