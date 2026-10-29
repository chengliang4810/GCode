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

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

type surface struct{ kind, id, title string }

// FileDrag is an app-local workspace path, never a claim that a real local file
// exists. A composer may receive it as a file mention with ui.Drop[FileDrag].
type FileDrag struct {
	ID, ProjectID, Path string
	Directory           bool
}

type document struct {
	file           contracts.File
	editor         *editor.State
	conflict       bool
	closeAfterSave bool
	image          ui.ImageSource
	imageError     string
	preview        bool
	autosave       *time.Timer
	deleted        bool
	focus          bool
	viewer         ImageViewport
}

// State owns persistent widget state and native resources. Call Update for every
// committed backend snapshot, on the UI thread, before drawing. Dispatch may be
// supplied by the application to execute commands asynchronously.
type State struct {
	win             *mygo.Window
	backend         contracts.Service
	ctx             context.Context
	cancel          context.CancelFunc
	closed          atomic.Bool
	snapshot        contracts.Snapshot
	scopeID         string
	scopes          map[string]workspaceScope
	tabs            []surface
	active          int
	tabScroll       ui.ScrollState
	tabObserver     tabStripObserver
	tabTooltip      tabTooltipState
	revealTab       bool
	systemDark      bool
	documents       map[string]*document
	diffs           map[string]*editor.DiffState
	browsers        map[string]*browser
	terminals       map[string]*terminalView
	folders         map[string]*bool
	fileSearch      string
	search          fileSearchState
	tree            ui.Handle
	recentFiles     []string
	prSearch        string
	prTab           int
	checksExpanded  bool
	selectedPR      string
	comment         string
	pendingClose    string
	Error           string
	ProjectID       string
	Dispatch        func(contracts.Command)
	OnOpenSession   func(string)
	OnCloseSession  func(string)
	RenderSession   func(*ui.Context, string)
	OnOpenSurface   func()
	OnCloseSurface  func()
	OnToggleExpand  func()
	OnAnnotation    func(contracts.Annotation)
	nextAnnotation  uint32
	nextBrowser     int
	pendingTerminal bool
	historySearch   string
	commitID        string
	commitOpen      bool
	fileActionOpen  bool
	fileAction      string
	fileActionID    string
	fileActionName  string
	imageOpen       bool
	imageID         string
	imageViewer     ImageViewport
}

func New(win *mygo.Window, backend contracts.Service) *State {
	ctx, cancel := context.WithCancel(context.Background())
	return &State{win: win, backend: backend, ctx: ctx, cancel: cancel,
		documents: map[string]*document{}, diffs: map[string]*editor.DiffState{},
		browsers: map[string]*browser{}, terminals: map[string]*terminalView{}, folders: map[string]*bool{}, scopes: map[string]workspaceScope{}}
}

// Update never overwrites an unsaved local edit with an unrelated server edit.
func (s *State) Update(snapshot contracts.Snapshot) {
	if s.closed.Load() {
		return
	}
	s.snapshot = snapshot
	for id, d := range s.documents {
		found := false
		for _, f := range snapshot.Files {
			if f.ID != id {
				continue
			}
			found = true
			for i := range s.tabs {
				if s.tabs[i].kind == "file" && s.tabs[i].id == f.ID {
					s.tabs[i].title = f.Name
				}
			}
			d.deleted = false
			if f.Version == d.file.Version {
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
		if !found {
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
			if s.terminals[t.ID] == nil && (s.ProjectID == "" || t.ProjectID == s.ProjectID) {
				s.pendingTerminal = false
				s.openTerminal(t)
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
			return
		}
	}
	s.tabs = append(s.tabs, surface{kind, id, title})
	s.selectTab(len(s.tabs) - 1)
}

func (s *State) selectTab(index int) {
	if index < 0 || index >= len(s.tabs) {
		return
	}
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
			v.focusSearch = v.showSearch
			v.focusBody = !v.showSearch
		}
	}
}

func (s *State) OpenFile(id string) {
	if s.closed.Load() {
		return
	}
	for _, f := range s.snapshot.Files {
		if f.ID == id && f.Kind != "directory" {
			s.recentFiles = append([]string{id}, slices.DeleteFunc(s.recentFiles, func(previous string) bool { return previous == id })...)
			if s.documents[id] == nil {
				buffer := editor.NewBuffer(f.Content)
				buffer.SetLanguage(editor.LanguageFor(f.Path))
				s.documents[id] = &document{file: f, editor: &editor.State{Buffer: buffer, ReadOnly: f.ReadOnly, Label: f.Name, NoScrollPastEnd: true}}
				if f.Kind == "image" {
					d := s.documents[id]
					data, err := assets.Read(f.AssetRef)
					if err == nil {
						if strings.HasSuffix(strings.ToLower(f.AssetRef), ".svg") {
							d.image, err = ui.ParseSVG(data)
						} else {
							d.image, err = ui.DecodeBitmap(data)
						}
					}
					if err != nil {
						d.imageError = err.Error()
					}
				}
			}
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

func (s *State) OpenPR(id string) { s.selectedPR = id; s.prTab = 0; s.choose("pr", id, "Pull request") }

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
	case "browser":
		s.openBrowser("https://go.dev")
	case "terminal":
		for _, t := range s.snapshot.Terminals {
			if s.ProjectID == "" || t.ProjectID == s.ProjectID {
				s.openTerminal(t)
				return
			}
		}
		s.pendingTerminal = true
		s.command(contracts.Command{Kind: "terminal.create", ProjectID: s.ProjectID})
	case "history":
		s.choose(kind, "history", "History")
	case "changes":
		s.choose(kind, "changes", "Changes")
	case "pullrequests", "pull-requests", "pr":
		s.choose("pullrequests", "pullrequests", "Pull requests")
	case "files":
		s.choose("files", "files", "Files")
	default:
		s.choose("picker", "picker", "Workspace")
	}
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

func (s *State) View(c *ui.Context) { s.view(c, true) }

// Body draws a workspace whose TabStrip is hosted by the shell titlebar.
func (s *State) Body(c *ui.Context) { s.view(c, false) }

func (s *State) view(c *ui.Context, ownTabs bool) {
	s.syncAppearance(c)
	p := s.palette()
	container := ui.Column(c).Key("workspace:" + s.scopeID).Label(tr(c, "Workspace surface")).Fill().MinWidth(0).Background(design.PanelBackground(p, s.snapshot.Settings.Appearance.Surface)).TextColor(p.Text).Font(design.Font(s.snapshot.Settings.Appearance.Font))
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
		case "pullrequests":
			s.pullRequestsView(c)
		case "pr":
			s.pullRequestView(c, t.id)
		case "browser":
			s.browserView(c, t.id)
		case "terminal":
			s.terminalView(c, t.id)
		case "session":
			if s.RenderSession != nil {
				s.RenderSession(c, t.id)
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
	items := []struct{ kind, title, icon string }{{"browser", "Browser", "globe"}, {"terminal", "Terminal", "terminal"}}
	if s.hasGit() {
		items = append(items, struct{ kind, title, icon string }{"changes", "Diffs", "list"}, struct{ kind, title, icon string }{"history", "History", "git-branch"})
	}
	ui.Column(c).Grow(1).FillWidth().Padding(16).Center().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(280).Gap(8).Children(func() {
			for _, item := range items {
				c.Key(item.kind)
				b := ui.ButtonBase(c).Label(tr(c, item.title)).FillWidth().Height(44).Padding(0, 14).Radius(10).Border(1, p.Border).Gap(10).AlignItems(ui.Center).Background(sourceInk(p.Dark, .02)).OnClick(func() { s.OpenSurface(item.kind) })
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
	s.syncAppearance(c)
	p := s.palette()
	ui.Column(c).Key("files-explorer:" + s.scopeID).Label(tr(c, "Files explorer")).Fill().MinWidth(0).Background(design.PanelBackground(p, s.snapshot.Settings.Appearance.Surface)).TextColor(p.Text).Children(func() {
		ui.Row(c).Height(38).Padding(0, 8).Gap(4).AlignItems(ui.Center).BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Background(p.Shell).Children(func() {
			ui.Row(c).Grow(1).Height(24).Padding(0, 8).Gap(6).AlignItems(ui.Center).Radius(6).Background(sourceInk(p.Dark, .035)).Children(func() {
				ui.Icon(c, assets.Icon("magnifer")).Size(12, 12).TextColor(p.Faint)
				input := ui.TextInputBase(c, &s.fileSearch).Bind(&s.search.input).Placeholder(tr(c, "Search files")).Label(tr(c, "Search files")).Grow(1).FontSize(design.UISize(c, 11.5)).OnChange(s.refreshFileSearch).OnShortcut(0, ui.KeyEscape, func() { s.clearFileSearch(); s.tree.Focus() })
				input.OnShortcut(0, ui.KeyDown, func() { s.moveFileSearch(1) }).OnShortcut(0, ui.KeyUp, func() { s.moveFileSearch(-1) }).OnSubmit(s.activateFileSearch)
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
			if s.fileSearch != "" {
				s.fileSearchResults(c)
				return
			}
			ui.Scroll(c).Grow(1).FillWidth().Children(func() { ui.Tree(c, func() { s.fileChildren(c, "") }).Bind(&s.tree).FillWidth() })
		}).ContextMenu(func(m *ui.Menu) {
			if m.Item(tr(c, "New file…")).Chosen() {
				s.requestFileAction("create", "")
			}
			if m.Item(tr(c, "New folder…")).Chosen() {
				s.requestFileAction("mkdir", "")
			}
		})
	})
	s.fileActionDialog(c)
}

func (s *State) fileChildren(c *ui.Context, parent string) {
	s.fileChildrenLevel(c, parent, 0)
}

func (s *State) fileChildrenLevel(c *ui.Context, parent string, depth int) {
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
			row := ui.TreeItem(c, "", open, func() { s.fileChildrenLevel(c, f.ID, depth+1) }).Label(f.Name).Drag(FileDrag{ID: f.ID, ProjectID: f.ProjectID, Path: f.Path, Directory: true}).OnClick(func() { *open = !*open }).ContextMenu(func(m *ui.Menu) { s.fileContext(c, f, m) })
			s.styleFileRow(c, row, f, depth, *open, p)
		} else {
			row := ui.TreeItem(c, "", nil, nil).Label(f.Name).Selected(s.activeFile() == f.ID).Drag(FileDrag{ID: f.ID, ProjectID: f.ProjectID, Path: f.Path}).OnClick(func() { s.OpenFile(f.ID) }).ContextMenu(func(m *ui.Menu) { s.fileContext(c, f, m) })
			s.styleFileRow(c, row, f, depth, false, p)
		}
	}
}

func (s *State) styleFileRow(c *ui.Context, row ui.Element, f contracts.File, depth int, expanded bool, p design.Tokens) {
	// The official Tree's disclosure slot is 16px. Its two extra pixels
	// come out of the inset, preserving the source icon/text starting x.
	row.Height(27).FillWidth().Gap(0).Padding(0, 8, 0, 6+14*float32(depth)).FontSize(design.UISize(c, 11.5)).TextColor(p.Muted).Radius(0).Draw(func(painter *ui.Painter, r ui.Rect) {
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
		if strings.HasSuffix(d.file.Path, ".md") {
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
		ui.Scroll(c).Grow(1).FillWidth().Padding(20).Children(func() { markdown.View(c, d.editor.Buffer.String(), nil) })
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
		ui.Text(c, fmt.Sprintf(tr(c, "Save changes to %s?"), d.file.Name)).FontSize(design.UISize(c, 16)).FontWeight(500)
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
		ui.Text(c, tr(c, "Changes")).FontSize(design.UISize(c, 18)).FontWeight(600).Margin(0, 0, 16, 0)
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

func (s *State) pullRequestsView(c *ui.Context) {
	p := s.palette()
	ui.Row(c).Height(48).Padding(8, 16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tr(c, "Pull requests")).FontSize(design.UISize(c, 18)).FontWeight(600)
		ui.Spacer(c)
	})
	ui.TextInputBase(c, &s.prSearch).Label(tr(c, "Search pull requests")).Placeholder(tr(c, "Search pull requests")).Padding(8, 12).Margin(4, 16).Radius(6).Background(p.Input)
	ui.Scroll(c).Grow(1).FillWidth().Padding(8, 16).Children(func() {
		for _, pr := range s.snapshot.PullRequests {
			if !strings.Contains(strings.ToLower(pr.Title+pr.Repository), strings.ToLower(s.prSearch)) {
				continue
			}
			c.Key(pr.ID)
			ui.ButtonBase(c).Label(pr.Title).FillWidth().Padding(14, 12).Radius(10).Margin(0, 0, 8, 0).Background(p.Card).OnClick(func() { s.OpenPR(pr.ID) }).Children(func() {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, pr.Title).FontWeight(500)
					ui.Text(c, fmt.Sprintf(tr(c, "%s #%d · %s · %s"), pr.Repository, pr.Number, pr.Author, tr(c, pr.Status))).TextColor(p.Muted)
				})
			})
		}
	})
}

func (s *State) pullRequestView(c *ui.Context, id string) {
	var pr *contracts.PullRequest
	for i := range s.snapshot.PullRequests {
		if s.snapshot.PullRequests[i].ID == id {
			pr = &s.snapshot.PullRequests[i]
			break
		}
	}
	if pr == nil {
		return
	}
	p := s.palette()
	ui.Column(c).Padding(16, 20).Gap(8).Children(func() {
		ui.Text(c, pr.Title).FontSize(design.UISize(c, 20)).FontWeight(600)
		ui.Text(c, fmt.Sprintf(tr(c, "#%d · %s → %s"), pr.Number, pr.HeadBranch, pr.BaseBranch)).TextColor(p.Muted)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, tr(c, pr.Status)).Padding(3, 8).Radius(10).Background(p.Card).TextColor(p.Success)
			ui.Spacer(c)
			ui.Button(c, tr(c, "Merge pull request")).Disabled(pr.Status != "open").OnClick(func() { s.command(contracts.Command{Kind: "pr.merge", ID: id}) })
		})
	})
	ui.Tabs(c, &s.prTab, tr(c, "Summary"), tr(c, "Code"), tr(c, "Activity"))
	ui.Scroll(c).Grow(1).FillWidth().Padding(16, 20).Children(func() {
		switch s.prTab {
		case 0:
			markdown.View(c, pr.Body, nil)
			ui.ButtonBase(c).Label(tr(c, "Checks")).FillWidth().Padding(12, 0).Margin(16, 0, 0, 0).OnClick(func() { s.checksExpanded = !s.checksExpanded }).Children(func() {
				ui.Text(c, fmt.Sprintf(tr(c, "Checks · %d"), len(pr.Checks))).FontWeight(500)
				ui.Spacer(c)
				icon := "alt-arrow-right"
				if s.checksExpanded {
					icon = "alt-arrow-down"
				}
				ui.Icon(c, assets.Icon(icon)).Size(12, 12)
			})
			if s.checksExpanded {
				for _, check := range pr.Checks {
					c.Key(check.Name)
					ui.Column(c).Padding(10, 12).Gap(5).Children(func() {
						ui.Row(c).Gap(8).Children(func() {
							icon, color := "check", p.Success
							if check.Status == "failure" || check.Status == "failed" {
								icon, color = "danger-triangle", p.Danger
							}
							ui.Icon(c, assets.Icon(icon)).Size(14, 14).TextColor(color)
							ui.Text(c, check.Name).FontWeight(500)
							ui.Spacer(c)
							ui.Text(c, tr(c, check.Status)).TextColor(color)
						})
						ui.Text(c, check.Detail).TextColor(p.Muted)
					})
				}
			}
		case 1:
			for _, changeID := range pr.ChangeIDs {
				for _, change := range s.snapshot.Changes {
					if change.ID == changeID {
						ui.Button(c, change.Path).OnClick(func() { s.OpenChange(change.ID) })
					}
				}
			}
		case 2:
			comments := slices.Clone(pr.Comments)
			slices.SortFunc(comments, func(a, b contracts.Comment) int { return a.CreatedAt.Compare(b.CreatedAt) })
			if len(comments) == 0 {
				ui.Column(c).MinHeight(160).FillWidth().Center().Gap(16).Children(func() {
					ui.Icon(c, assets.Icon("chat-round-line")).Size(24, 24).TextColor(p.Muted)
					ui.Text(c, tr(c, "No activity yet")).FontSize(design.UISize(c, 15)).FontWeight(600)
				})
			}
			for _, comment := range comments {
				c.Key(comment.ID)
				own := comment.Author == s.snapshot.User.Name || comment.Author == s.snapshot.User.Email
				ui.Row(c).FillWidth().Margin(0, 24, 0, 0).Children(func() {
					if own {
						ui.Spacer(c)
					}
					bubble := ui.Column(c).MaxWidth(600).Grow(1).Gap(8)
					if own {
						bubble.Padding(10, 16).Radius(16).Background(p.Card)
					}
					bubble.Children(func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							initial := []rune(comment.Author)
							label := "?"
							if len(initial) > 0 {
								label = strings.ToUpper(string(initial[0]))
							}
							ui.Text(c, label).Size(24, 24).TextAlign(ui.Center).Radius(12).Background(p.Hover)
							ui.Text(c, comment.Author).FontWeight(500)
							ui.Text(c, date(c, comment.CreatedAt, "Jan 2, 15:04")).FontSize(design.UISize(c, 11)).TextColor(p.Muted)
						})
						if comment.Path != "" {
							location := comment.Path
							if comment.Line > 0 {
								location += fmt.Sprintf(":%d", comment.Line)
							}
							ui.Text(c, location).FontSize(design.UISize(c, 11)).TextColor(p.Muted)
						}
						markdown.View(c, comment.Body, nil)
					})
				})
			}
		}
	})
	if s.prTab == 2 {
		ui.Column(c).Padding(12, 20).Gap(8).BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
			ui.TextAreaBase(c, &s.comment).Label(tr(c, "Write a comment")).Placeholder(tr(c, "Write a comment… Type @ to mention someone")).MinHeight(72).Padding(12).Radius(16).Background(p.Input)
			ui.Row(c).Justify(ui.End).Children(func() {
				ui.PrimaryButton(c, tr(c, "Comment")).Disabled(strings.TrimSpace(s.comment) == "").OnClick(func() {
					text := s.comment
					s.comment = ""
					s.command(contracts.Command{Kind: "pr.comment", ID: id, Comment: &contracts.CommentInput{Body: text}})
				})
			})
		})
	}
}
