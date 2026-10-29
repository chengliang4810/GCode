package desktop

import (
	"context"
	"fmt"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
)

type commandPaletteState struct {
	tab                     int
	selected                int
	activeKey               string
	list                    ui.ListState
	input                   ui.Handle
	result                  contracts.WorkspaceSearchResult
	query, projectID, error string
	loading                 bool
	generation              uint64
	cancel                  context.CancelFunc
}
type paletteRow struct {
	key, section, title, subtitle, glyph, command string
	session                                       *contracts.Session
	file                                          *contracts.File
	line, column                                  int
	preview                                       string
	ranges                                        []contracts.MatchRange
}

var paletteTabs = [...]string{"All", "Threads", "Commands", "Files"}

func paletteShortcut(combo string) string {
	if runtime.GOOS != "darwin" {
		return strings.ReplaceAll(strings.ReplaceAll(combo, "Mod+", "Ctrl+"), "Cmd+", "Ctrl+")
	}
	parts := strings.Split(combo, "+")
	if len(parts) == 0 {
		return combo
	}
	value := ""
	for _, modifier := range []struct{ name, glyph string }{{"Ctrl", "⌃"}, {"Alt", "⌥"}, {"Shift", "⇧"}, {"Mod", "⌘"}} {
		for _, part := range parts[:len(parts)-1] {
			if strings.EqualFold(part, modifier.name) {
				value += modifier.glyph
				break
			}
		}
	}
	return value + parts[len(parts)-1]
}
func (a *App) paletteKeyHint(c *ui.Context, key, label string) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Height(22).Padding(0, 5).Gap(4).Radius(5).AlignItems(ui.Center).Background(sourcePaletteInk(a.palette.Dark, .05)).Children(func() {
			switch key {
			case "↑ ↓":
				ui.Icon(c, assets.Icon("arrow-up")).Size(12.5, 12.5).TextColor(a.palette.Muted)
				ui.Icon(c, assets.Icon("arrow-down")).Size(12.5, 12.5).TextColor(a.palette.Muted)
			case "↵":
				ui.Icon(c, assets.Icon("return")).Size(12.5, 12.5).TextColor(a.palette.Muted)
			default:
				ui.Text(c, key).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
			}
		})
		ui.Text(c, tr(c, label)).FontSize(design.UISize(c, 10)).TextColor(a.palette.Muted)
	})
}
func sourcePaletteInk(dark bool, alpha float32) ui.Color {
	if dark {
		return ui.RGBA(255, 255, 255, alpha)
	}
	return ui.RGBA(0, 0, 0, alpha)
}
func sourcePaletteWash(dark bool, alpha float32) ui.Color {
	if dark {
		return ui.RGBA(235, 235, 235, alpha)
	}
	return ui.RGBA(26, 26, 26, alpha)
}
func paletteSelection(dark bool) ui.Color {
	alpha := float32(.06)
	if dark {
		alpha = .11
	}
	return sourcePaletteWash(dark, alpha)
}
func paletteScrim(dark bool) ui.Color {
	alpha := float32(.35)
	if !dark {
		alpha = .32 * (.35 / .60)
	}
	return ui.RGBA(0, 0, 0, alpha)
}

func paletteSessionHeight(session contracts.Session, settings contracts.Settings) float32 {
	return sidebarRowHeight(settings.SidebarCompact, settings.ShowProjectLabels, settings.SidebarShowBranch && session.Branch != "", settings.SidebarShowPullRequest && session.PullRequestNumber > 0)
}

func (a *App) toggleCommandPalette() {
	a.paletteOpen = !a.paletteOpen
	if !a.paletteOpen {
		a.stopPaletteSearch()
		return
	}
	a.paletteQuery = ""
	a.commands.tab, a.commands.selected, a.commands.activeKey = 0, 0, ""
	a.refreshPaletteSearch()
}
func (a *App) stopPaletteSearch() {
	if a.commands.cancel != nil {
		a.commands.cancel()
		a.commands.cancel = nil
	}
	a.commands.generation++
	a.commands.loading = false
}

// refreshPaletteSearch is called by input/open events, never by View.
func (a *App) refreshPaletteSearch() {
	a.stopPaletteSearch()
	p := &a.commands
	p.query, p.projectID, p.error = strings.TrimSpace(a.paletteQuery), a.projectID, ""
	p.result = contracts.WorkspaceSearchResult{}
	if a.sessionID == "" || a.projectID == "" {
		return
	}
	if p.query != "" && len([]rune(p.query)) < 2 {
		return
	}
	service, ok := a.service.(contracts.WorkspaceSearchService)
	if !ok {
		p.error = "Workspace search is unavailable"
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	generation := p.generation
	request := contracts.WorkspaceSearchRequest{ProjectID: p.projectID, Query: p.query, FileLimit: 50, ContentLimit: 100, MatchesPerFile: 3, Recent: p.query == ""}
	p.loading = true
	finish := func(result contracts.WorkspaceSearchResult, err error) {
		if a.closed || ctx.Err() != nil || !a.paletteOpen || a.commands.generation != generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
		} else {
			p.result = result
		}
	}
	if a.win == nil {
		result, err := service.SearchWorkspace(ctx, request)
		finish(result, err)
		return
	}
	win := a.win
	go func() {
		if request.Query != "" {
			timer := time.NewTimer(80 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
		result, err := service.SearchWorkspace(ctx, request)
		if ctx.Err() != nil {
			return
		}
		win.Update(func() { finish(result, err) })
	}()
}
func queryWordsMatch(query, text string) bool {
	text = strings.ToLower(text)
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
func scorePaletteField(text, term string, weight int) int {
	text = strings.ToLower(text)
	score := 0
	for offset := 0; offset < len(text); {
		at := strings.Index(text[offset:], term)
		if at < 0 {
			break
		}
		at += offset
		bonus := at == 0
		if !bonus {
			previous := []rune(text[:at])
			bonus = !unicode.IsLetter(previous[len(previous)-1]) && !unicode.IsNumber(previous[len(previous)-1])
		}
		if bonus {
			return weight + weight/2
		}
		score = weight
		offset = at + len(term)
	}
	return score
}
func (a *App) paletteThreadScore(session contracts.Session, query string) int {
	pr := ""
	for _, request := range a.state.PullRequests {
		if request.Number == session.PullRequestNumber {
			pr = fmt.Sprintf("#%d %s %s %s", request.Number, request.Title, request.HeadBranch, request.BaseBranch)
			break
		}
	}
	fields := []struct {
		text   string
		weight int
	}{{session.Title, 100}, {a.projectNameFor(session.ProjectID), 40}, {session.Branch, 30}, {a.deviceNameFor(session.DeviceID), 20}, {pr, 20}}
	score := 0
	for _, term := range strings.Fields(strings.ToLower(query)) {
		best := 0
		for _, field := range fields {
			best = max(best, scorePaletteField(field.text, term, field.weight))
		}
		if best == 0 {
			return 0
		}
		score += best
	}
	if session.Archived {
		score /= 2
	}
	return score
}
func (a *App) paletteRows() []paletteRow {
	query := strings.ToLower(strings.TrimSpace(a.paletteQuery))
	tab := a.commands.tab
	var rows []paletteRow
	addCommands := func(section string) {
		commands := []paletteRow{{key: "new", title: "New chat", glyph: "pen-new-square", command: "new"}, {key: "project", title: "New project", glyph: "folder", command: "project"}, {key: "settings", title: "Open settings", glyph: "settings", command: "settings"}}
		mode := "dark"
		if a.palette.Dark {
			mode = "light"
		}
		glyph := "moon"
		if mode == "light" {
			glyph = "sun"
		}
		commands = append(commands, paletteRow{key: "theme", title: fmt.Sprintf("Switch to %s theme", mode), glyph: glyph, command: "theme:" + mode})
		if selected := a.currentSession(); selected.ID != "" && !selected.Archived {
			commands = append(commands, paletteRow{key: "archive", title: "Archive thread", glyph: "archive-minimalistic", command: "archive"})
		}
		for _, row := range commands {
			if queryWordsMatch(query, row.title+" "+a.t(row.title)) {
				row.section = section
				rows = append(rows, row)
			}
		}
	}
	addThreads := func(section string, limit int) {
		var sessions []contracts.Session
		for _, session := range a.state.Sessions {
			if session.ParentSessionID != "" || session.SideChat {
				continue
			}
			if query == "" {
				if session.Archived || session.ID == a.sessionID {
					continue
				}
			} else if a.paletteThreadScore(session, query) == 0 {
				continue
			}
			sessions = append(sessions, session)
		}
		slices.SortStableFunc(sessions, func(x, y contracts.Session) int {
			if query != "" {
				if n := a.paletteThreadScore(y, query) - a.paletteThreadScore(x, query); n != 0 {
					return n
				}
			}
			return y.UpdatedAt.Compare(x.UpdatedAt)
		})
		for _, session := range sessions[:min(limit, len(sessions))] {
			rows = append(rows, paletteRow{key: "thread:" + session.ID, section: section, title: session.Title, session: &session})
		}
	}
	addFiles := func(section string, limit int) {
		if a.sessionID == "" {
			return
		}
		seen := map[string]bool{}
		matches := slices.Clone(a.commands.result.Files)
		if query == "" {
			matches = append(a.workspace.RecentFiles(), matches...)
		}
		count := 0
		for _, found := range matches {
			if seen[found.FileID] {
				continue
			}
			seen[found.FileID] = true
			for _, file := range a.state.Files {
				if file.ID != found.FileID || file.ProjectID != a.projectID {
					continue
				}
				if query == "" && file.Kind == "directory" {
					continue
				}
				rows = append(rows, paletteRow{key: "file:" + file.ID, section: section, title: path.Base(file.Path), subtitle: path.Dir(file.Path), file: &file})
				count++
				break
			}
			if count >= limit {
				break
			}
		}
	}
	addContent := func(limit int) {
		for _, found := range a.commands.result.Content[:min(limit, len(a.commands.result.Content))] {
			for _, file := range a.state.Files {
				if file.ID == found.FileID && file.ProjectID == a.projectID {
					rows = append(rows, paletteRow{key: fmt.Sprintf("content:%s:%d", found.FileID, found.Line), section: "In Files", title: fmt.Sprintf("%s:%d", found.Path, found.Line), file: &file, line: found.Line, column: found.Column, preview: found.Text, ranges: found.Ranges})
					break
				}
			}
		}
	}
	switch tab {
	case 0:
		if query == "" {
			addFiles("Recent Files", 5)
			addCommands("Quick Actions")
			addThreads("Recent Threads", 5)
		} else {
			addThreads("Threads", 5)
			addCommands("Commands")
			addFiles("Files", 5)
			addContent(5)
		}
	case 1:
		if query == "" {
			addThreads("Recent Threads", 50)
		} else {
			addThreads("Threads", 50)
		}
	case 2:
		if query == "" {
			addCommands("Quick Actions")
		} else {
			addCommands("Commands")
		}
	case 3:
		if query == "" {
			addFiles("Recent Files", 50)
		} else {
			addFiles("Files", 50)
			addContent(100)
		}
	}
	return rows
}
func (a *App) activatePalette(row paletteRow) {
	if strings.HasPrefix(row.command, "theme:") {
		a.state.Settings.Appearance.Mode = strings.TrimPrefix(row.command, "theme:")
		a.saveSettings()
		return
	}
	a.paletteOpen = false
	a.stopPaletteSearch()
	switch row.command {
	case "new":
		a.newConversation()
	case "project":
		a.openNewProject()
	case "settings":
		a.navigate("settings")
	case "archive":
		a.archiveSidebarSession(a.sessionID, true)
	}
	if row.session != nil {
		a.navigate("chat/" + row.session.ID)
	}
	if row.file != nil {
		if row.file.Kind == "directory" {
			a.setPanel(2, true)
			a.workspace.RevealFile(row.file.ID)
		} else {
			a.workspace.OpenFileAt(row.file.ID, row.line, row.column)
		}
	}
}
func (a *App) commandPalette(c *ui.Context) {
	if !a.paletteOpen {
		a.stopPaletteSearch()
		return
	}
	p := &a.commands
	ui.DialogBase(c, &a.paletteOpen, func(back, panel ui.Element) {
		width, height := c.Size()
		maxResults := max(float32(100), min(float32(360), height-180))
		top := max(float32(54), (height-maxResults-120)/2)
		back.Background(paletteScrim(a.palette.Dark)).Justify(ui.Start).Padding(top, 0, 0)
		panel.Label(tr(c, "Command palette")).Width(min(560, width-32)).Radius(16).Border(1, a.palette.Border).Background(a.palette.Raised).Clip()
		ui.Column(c).FillWidth().Children(func() {
			var input ui.Element
			ui.Row(c).Height(44).Padding(8, 16).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
				ui.Icon(c, assets.Icon("palette-search")).Size(16, 16).TextColor(a.palette.Muted)
				input = ui.TextInputBase(c, &a.paletteQuery).Bind(&p.input).Label(tr(c, "Command search")).Placeholder(tr(c, "Search anything…")).AutoFocus().Grow(1).MinWidth(0).FontSize(design.UISize(c, 14)).OnChange(func() { p.selected = 0; p.activeKey = ""; a.refreshPaletteSearch() })
				ui.Text(c, paletteShortcut("Mod+K")).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
			})
			changeTab := func(index int) {
				p.tab = index
				p.selected = 0
				p.activeKey = ""
				p.list.ScrollTo(0, ui.Start)
				p.input.Focus()
			}
			ui.Row(c).Padding(6, 12).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
				for i, title := range paletteTabs {
					row := ui.ButtonBase(c).Label(tr(c, "Command tab: ")+tr(c, title)).Padding(4, 8).Height(26).Radius(6).Gap(6).AlignItems(ui.Center).OnClick(func() { changeTab(i) })
					if p.tab == i {
						row.Background(sourcePaletteWash(a.palette.Dark, .1))
					}
					row.Children(func() {
						ui.Text(c, tr(c, title)).FontSize(design.UISize(c, 12))
						ui.Text(c, paletteShortcut(fmt.Sprintf("Mod+%d", i+1))).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
					})
				}
			})
			rows := a.paletteRows()
			if p.activeKey != "" {
				for i, row := range rows {
					if row.key == p.activeKey {
						p.selected = i
						break
					}
				}
			}
			p.selected = max(0, min(max(0, len(rows)-1), p.selected))
			execute := func() {
				if p.selected < len(rows) {
					a.activatePalette(rows[p.selected])
				}
			}
			move := func(delta int) {
				if len(rows) > 0 {
					p.selected = (p.selected + delta + len(rows)) % len(rows)
					p.activeKey = rows[p.selected].key
					p.list.ScrollTo(p.selected, ui.Center)
				}
			}
			input.OnShortcut(0, ui.KeyDown, func() { move(1) }).OnShortcut(0, ui.KeyUp, func() { move(-1) }).OnSubmit(execute).OnShortcut(0, ui.KeyTab, func() { changeTab((p.tab + 1) % 4) }).OnShortcut(ui.Shift, ui.KeyTab, func() { changeTab((p.tab + 3) % 4) })
			input.HandleInput(func(event ui.InputEvent) bool {
				return event.Kind == ui.InputKeyDown && event.Key == ui.KeyEnter && event.Repeat
			})
			for i, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4} {
				input.OnShortcut(ui.Ctrl, key, func() { changeTab(i) }).OnShortcut(ui.Super, key, func() { changeTab(i) })
			}
			p.list.Key = func(i int) any { return rows[i].key }
			p.list.Label = func(i int) string { return a.paletteRowTitle(rows[i]) }
			listHeight := float32(0)
			for i, row := range rows {
				if i == 0 || rows[i-1].section != row.section {
					listHeight += 20
					if i > 0 {
						listHeight += 16
					}
				}
				height := float32(32)
				if row.session != nil {
					height = paletteSessionHeight(*row.session, a.state.Settings)
				}
				listHeight += height
			}
			if len(rows) > 0 {
				ui.List(c, &p.list, len(rows), func(i int) {
					row := rows[i]
					ui.Column(c).FillWidth().Children(func() {
						if i == 0 || rows[i-1].section != row.section {
							if i > 0 {
								ui.Box(c).Height(1).FillWidth().Background(a.palette.Border).Margin(8, 0)
							}
							ui.Text(c, tr(c, row.section)).Height(20).Padding(2, 16, 4).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
						}
						a.paletteResult(c, row, i, p.selected == i, execute)
					})
				}).Label(tr(c, "Command results")).Height(min(maxResults, listHeight+16)).Padding(8, 0).Gap(2).FillWidth()
			} else {
				ui.Column(c).Padding(24, 16).Gap(6).AlignItems(ui.Center).Children(func() {
					title, hint := "No results", "Try a command, chat title, project, or device."
					if p.tab == 3 && a.paletteQuery == "" {
						title, hint = "No recent files", "Files you open or change in this workspace show up here."
					}
					ui.Text(c, tr(c, title))
					ui.Text(c, tr(c, hint)).TextColor(a.palette.Muted)
				})
			}
			if p.loading || p.error != "" {
				status := "Searching…"
				if p.error != "" {
					status = p.error
				}
				ui.Text(c, tr(c, status)).Padding(4, 16).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
			}
			ui.Row(c).Padding(7, 16).Gap(12).BorderWidth(1, 0, 0, 0).BorderColor(a.palette.Border).Children(func() {
				a.paletteKeyHint(c, "↑ ↓", "Navigate")
				a.paletteKeyHint(c, "↵", "Open")
				a.paletteKeyHint(c, "Esc", "Close")
			})
		})
	})
}
func (a *App) paletteResult(c *ui.Context, row paletteRow, index int, selected bool, execute func()) {
	ui.Column(c).FillWidth().Padding(0, 8).Children(func() { a.paletteResultRow(c, row, index, selected, execute) })
}
func (a *App) paletteResultRow(c *ui.Context, row paletteRow, index int, selected bool, execute func()) {
	height := float32(30)
	if row.session != nil {
		height = paletteSessionHeight(*row.session, a.state.Settings)
	}
	button := ui.ButtonBase(c).Key(row.key).Label(tr(c, "Command: ")+a.paletteRowTitle(row)).FillWidth().MinWidth(0).Height(height).Padding(4, 8).Gap(8).Radius(10).AlignItems(ui.Center).Justify(ui.Start).OnClick(func() { a.commands.selected = index; execute() })
	button.HandleInput(func(event ui.InputEvent) bool {
		if event.Kind == ui.InputPointerMove && event.Button < 0 {
			a.commands.selected = index
			a.commands.activeKey = row.key
			return true
		}
		return false
	})
	if selected {
		button.Background(paletteSelection(a.palette.Dark))
	} else if button.Hovered() {
		button.Background(a.palette.Hover)
	}
	button.Children(func() {
		if row.session != nil {
			a.paletteThread(c, *row.session)
			return
		}
		if row.file != nil {
			ui.Image(c, workbench.FileIcon(*row.file, a.palette.Dark)).Size(16, 16)
		} else {
			ui.Icon(c, assets.Icon(row.glyph)).Size(16, 16).TextColor(a.palette.Muted)
		}
		title := paletteHighlight(c, a.paletteRowTitle(row), a.paletteQuery, a.palette.Accent).SingleLine().MinWidth(0).MaxWidth(240).FontSize(design.UISize(c, 13))
		if row.command != "" {
			title.Grow(1).MaxWidth(10000)
			bindingID := ""
			switch row.command {
			case "new":
				bindingID = "newSession"
			case "project":
				bindingID = "newProject"
			case "archive":
				bindingID = "archiveSession"
			}
			combo := ""
			if row.command == "settings" {
				combo = "Mod+,"
			}
			for _, binding := range a.state.Settings.Shortcuts {
				if binding.ID == bindingID {
					combo = binding.Keys
					break
				}
			}
			if combo != "" {
				ui.Text(c, paletteShortcut(combo)).FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted)
			}
		}
		if row.preview != "" {
			paletteHighlightRanges(c, row.preview, row.ranges, a.palette.Accent).SingleLine().MinWidth(0).Grow(1).Font("Geist Mono").FontSize(design.UISize(c, 13)).TextColor(a.palette.Text)
		} else if row.subtitle != "" && row.subtitle != "." {
			ui.Text(c, row.subtitle).SingleLine().MinWidth(0).Grow(1).TextColor(a.palette.Muted)
		}
	})
}

func paletteHighlight(c *ui.Context, text, query string, color ui.Color) ui.Element {
	return paletteHighlightRanges(c, text, contracts.SearchRanges(text, query), color)
}
func paletteHighlightRanges(c *ui.Context, text string, ranges []contracts.MatchRange, color ui.Color) ui.Element {
	var spans []ui.Span
	offset := 0
	for _, match := range ranges {
		if match.Start < offset || match.End > len(text) || match.End < match.Start {
			continue
		}
		if match.Start > offset {
			spans = append(spans, ui.Span{Text: text[offset:match.Start]})
		}
		alpha := float32(.10)
		if c.Theme().Dark {
			alpha = .12
		}
		spans = append(spans, ui.Span{Text: text[match.Start:match.End], Color: color, Background: color.Alpha(alpha), Radius: 3})
		offset = match.End
	}
	spans = append(spans, ui.Span{Text: text[offset:]})
	return ui.RichText(c, spans...)
}
func (a *App) paletteThread(c *ui.Context, session contracts.Session) {
	settings := a.state.Settings
	compact := settings.SidebarCompact
	branch := settings.SidebarShowBranch && session.Branch != ""
	pr := settings.SidebarShowPullRequest && session.PullRequestNumber > 0
	ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
		if !compact && settings.ShowProjectLabels {
			ui.Row(c).Height(14).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
				paletteHighlight(c, a.projectNameFor(session.ProjectID)+" @ "+a.deviceNameFor(session.DeviceID), a.paletteQuery, a.palette.Accent).Grow(1).MinWidth(0).SingleLine().FontSize(design.UISize(c, 10)).TextColor(a.palette.Muted)
				a.sidebarSessionCorner(c, session, false, false)
			})
		}
		ui.Row(c).Height(17).Gap(4).MinWidth(0).AlignItems(ui.Center).Children(func() {
			if compact {
				a.sidebarStatusGlyph(c, session)
			}
			if settings.SidebarShowHarness {
				icon(c, a.sessionHarnessIcon(session), 13)
			}
			if settings.ShowIcons {
				icon(c, "folder", 13)
			}
			paletteHighlight(c, session.Title, a.paletteQuery, a.palette.Accent).Grow(1).MinWidth(0).SingleLine().FontSize(design.UISize(c, 13))
			if compact {
				if pr {
					a.sidebarPRBadge(c, session)
				}
				a.sidebarSessionCorner(c, session, false, true)
			}
		})
		if !compact && (branch || pr) {
			height := float32(14)
			if pr {
				height = 16
			}
			ui.Row(c).Height(height).Gap(6).MinWidth(0).AlignItems(ui.Center).Children(func() {
				if branch {
					paletteHighlight(c, session.Branch, a.paletteQuery, a.palette.Accent).Grow(1).MinWidth(0).SingleLine().FontSize(design.UISize(c, 10)).TextColor(a.palette.Faint)
				} else {
					ui.Spacer(c)
				}
				if pr {
					a.sidebarPRBadge(c, session)
				}
			})
		}
	})
}
