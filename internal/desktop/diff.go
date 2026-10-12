package desktop

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

type diffLineKind uint8

const (
	diffContext diffLineKind = iota
	diffAdded
	diffDeleted
	diffMeta
	diffHunk
)

type diffLine struct {
	kind    diffLineKind
	oldLine int
	newLine int
	text    string
}

// cachedDiff owns the parsed rows and the syntax spans for one workspace
// change. The source text is part of the key so a refreshed workspace replaces
// the entry instead of drawing stale rows. Span colors are cached per theme.
type cachedDiff struct {
	raw      string
	rows     []diffLine
	added    int
	deleted  int
	language string
	spans    map[string][]ui.Span
}

const diffVirtualizationThreshold = 200

var unifiedHunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?: ?(.*))?$`)

func (a *App) diffView(c *ui.Context, p palette, workspace WorkspaceSnapshot) {
	ui.Column(c).Grow(1).Padding(12).Children(func() {
		if !workspace.Git.IsGit {
			emptyPanel(c, p, "git", "不是 Git 项目", "当前工作区没有 Git 状态可展示。")
			return
		}
		ui.Scroll(c.Key("diff-files-scroll")).Grow(1).Gap(10).Children(func() {
			if len(workspace.Git.Changes) == 0 {
				emptyPanel(c, p, "check", "工作区干净", "当前目录没有待查看的变更。")
			}
			for _, change := range workspace.Git.Changes {
				a.diffFile(c, p, change)
			}
		})
	})
}

func (a *App) diffHeaderSummary(c *ui.Context, p palette, workspace WorkspaceSnapshot) {
	if !workspace.Git.IsGit {
		return
	}
	added, deleted := 0, 0
	for _, change := range workspace.Git.Changes {
		diff := a.cachedDiff(change)
		added += diff.added
		deleted += diff.deleted
	}

	ui.Row(c).Shrink(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, strconv.Itoa(len(workspace.Git.Changes))+" 个文件").FontSize(11).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
		if added > 0 {
			ui.Text(c, "+"+strconv.Itoa(added)).FontSize(11).TextColor(p.success).SingleLine().Shrink(0)
		}
		if deleted > 0 {
			ui.Text(c, "−"+strconv.Itoa(deleted)).FontSize(11).TextColor(p.danger).SingleLine().Shrink(0)
		}
	})
}

func (a *App) diffFile(c *ui.Context, p palette, change appmodel.Change) {
	diff := a.cachedDiff(change)
	rows := diff.rows
	adds, deletes := diff.added, diff.deleted
	open, remembered := a.diffExpandedValue(change.Path)
	if !remembered {
		open = len(rows) <= 200
	}
	rowWidth := a.diffViewportWidth(c)
	parts := ui.CollapsibleBase(c.Key("diff-file-"+change.Path), &open)
	trigger := parts.Trigger.WidthPercent(100).Height(32).Padding(0, 8).Radius(8).Gap(8).AlignItems(ui.Center).
		Label("Diff 文件 " + change.Path).TextColor(p.text)
	if trigger.Hovered() {
		trigger = trigger.Background(p.surfaceAlt)
	}
	trigger.Children(func() {
		if open {
			ui.Icon(c, icon("chevron-down")).FontSize(13).TextColor(p.muted)
		} else {
			ui.Icon(c, icon("chevron-right")).FontSize(13).TextColor(p.muted)
		}
		ui.Icon(c, icon("file")).FontSize(15).TextColor(p.muted)
		ui.Text(c, change.Path).FontSize(12.5).FontWeight(550).SingleLine().Grow(1)
		if adds > 0 {
			ui.Text(c, "+"+strconv.Itoa(adds)).FontSize(11).TextColor(p.success).SingleLine()
		}
		if deletes > 0 {
			ui.Text(c, "−"+strconv.Itoa(deletes)).FontSize(11).TextColor(p.danger).SingleLine()
		}
		ui.Text(c, change.Status).FontSize(10.5).TextColor(p.muted).SingleLine()
	})
	if trigger.Changed() {
		a.rememberDiffExpanded(change.Path, open)
	}
	if panel := parts.Panel(func() {
		if strings.TrimSpace(change.Diff) == "" {
			ui.Text(c, "没有可显示的 Diff").FontSize(11.5).TextColor(p.muted).Padding(8, 12)
			return
		}
		renderedRows := len(rows)
		viewport := ui.ScrollHorizontal(c.Key("diff-horizontal-" + change.Path)).WidthPercent(100).MinWidth(0).Background(p.code).Radius(8)
		if renderedRows <= diffVirtualizationThreshold {
			viewport.Children(func() {
				ui.Column(c).Width(rowWidth).Children(func() {
					for index := range rows {
						a.renderDiffLine(c, p, diff, index, rowWidth)
					}
				})
			})
			return
		}
		height := float32(renderedRows*22 + 4)
		if height < 70 {
			height = 70
		}
		if height > 430 {
			height = 430
		}
		viewport.Height(height).Children(func() {
			ui.List(c.Key("diff-lines-"+change.Path), a.diffListState(change.Path), renderedRows, func(index int) {
				a.renderDiffLine(c, p, diff, index, rowWidth)
			}).Width(rowWidth).RowWidth(rowWidth).Height(height)
		})
	}); panel.Valid() {
		panel.WidthPercent(100).Padding(0, 0, 0, 0)
	}
}

func (a *App) renderDiffLine(c *ui.Context, p palette, diff *cachedDiff, index int, rowWidth float32) {
	line := diff.rows[index]
	if line.kind == diffHunk || line.kind == diffMeta {
		ui.Row(c).Width(rowWidth).Height(24).AlignItems(ui.Center).Padding(0, 8).Children(func() {
			color := p.muted
			if line.kind == diffHunk {
				color = p.accent
			}
			ui.Text(c, line.text).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11.5).TextColor(color).MinWidth(0).Grow(1).SingleLine().Tooltip(line.text)
		})
		return
	}
	background := p.code
	tone := p.codeText
	marker := " "
	switch line.kind {
	case diffAdded:
		background = p.diffAdd
		tone = p.success
		marker = "+"
	case diffDeleted:
		background = p.diffDel
		tone = p.danger
		marker = "−"
	}
	ui.Row(c).Width(rowWidth).MinHeight(22).AlignItems(ui.Start).Background(background).Children(func() {
		ui.Text(c, diffLineNumber(line.oldLine)).Width(30).TextAlign(ui.End).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(10.5).TextColor(p.muted).SingleLine()
		ui.Text(c, diffLineNumber(line.newLine)).Width(30).TextAlign(ui.End).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(10.5).TextColor(p.muted).SingleLine()
		ui.Text(c, marker).Width(18).TextAlign(ui.Center).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11.5).TextColor(tone).SingleLine()
		spans := a.diffLineSpans(diff, p, index)
		spans = append([]ui.Span(nil), spans...)
		ui.RichText(c, spans...).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(13).TextColor(p.codeText).LineHeight(1.3).MinWidth(0).Grow(1)
	})
}

func (a *App) diffViewportWidth(c *ui.Context) float32 {
	windowWidth, _ := c.Size()
	a.mu.RLock()
	leftWidth := a.leftWidth
	centerWidth := a.centerWidth
	a.mu.RUnlock()
	if leftWidth <= 0 {
		leftWidth = 240
	}
	if centerWidth <= 0 {
		centerWidth = 590
	}
	centerWidth = clampCenterWidth(windowWidth, leftWidth, centerWidth)
	rightWidth := windowWidth - leftWidth - centerWidth
	if rightWidth < rightPanelMinimumWidth {
		rightWidth = rightPanelMinimumWidth
	}
	width := rightWidth - 24
	if width < 220 {
		width = 220
	}
	return width
}

func (a *App) cachedDiff(change appmodel.Change) *cachedDiff {
	a.mu.RLock()
	cached := a.diffCache[change.Path]
	a.mu.RUnlock()
	if cached != nil && cached.raw == change.Diff {
		return cached
	}

	rows := parseDiffLines(change.Diff)
	added, deleted := diffCounts(rows)
	next := &cachedDiff{
		raw:      change.Diff,
		rows:     rows,
		added:    added,
		deleted:  deleted,
		language: diffLanguage(change.Path),
		spans:    make(map[string][]ui.Span),
	}
	a.mu.Lock()
	if a.diffCache == nil {
		a.diffCache = make(map[string]*cachedDiff)
	}
	if current := a.diffCache[change.Path]; current != nil && current.raw == change.Diff {
		a.mu.Unlock()
		return current
	}
	a.diffCache[change.Path] = next
	a.mu.Unlock()
	return next
}

func (a *App) diffLineSpans(diff *cachedDiff, p palette, index int) []ui.Span {
	key := boolString(p.dark) + ":" + strconv.Itoa(index)
	a.mu.RLock()
	spans, ok := diff.spans[key]
	a.mu.RUnlock()
	if ok {
		return spans
	}
	spans, _ = highlightCodeSpans(p, diff.language, diff.rows[index].text)
	a.mu.Lock()
	if existing, exists := diff.spans[key]; exists {
		a.mu.Unlock()
		return existing
	}
	diff.spans[key] = spans
	a.mu.Unlock()
	return spans
}

func parseDiffLines(raw string) []diffLine {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []diffLine
	oldLine, newLine := 1, 1
	hasHunk := false
	for _, source := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if source == "" && len(rows) > 0 {
			continue
		}
		if matches := unifiedHunkPattern.FindStringSubmatch(source); matches != nil {
			oldLine = parseDiffNumber(matches[1], 1)
			newLine = parseDiffNumber(matches[3], 1)
			rows = append(rows, diffLine{kind: diffHunk, text: source})
			hasHunk = true
			continue
		}
		if strings.HasPrefix(source, "--- ") || strings.HasPrefix(source, "+++ ") {
			rows = append(rows, diffLine{kind: diffMeta, text: source})
			continue
		}
		if !hasHunk && (strings.HasPrefix(source, "diff ") || strings.HasPrefix(source, "index ") || strings.HasPrefix(source, "new file") || strings.HasPrefix(source, "deleted file")) {
			rows = append(rows, diffLine{kind: diffMeta, text: source})
			continue
		}
		if strings.HasPrefix(source, "\\ No newline") {
			rows = append(rows, diffLine{kind: diffMeta, text: source})
			continue
		}
		if strings.HasPrefix(source, "+") {
			rows = append(rows, diffLine{kind: diffAdded, newLine: newLine, text: strings.TrimPrefix(source, "+")})
			newLine++
			continue
		}
		if strings.HasPrefix(source, "-") {
			rows = append(rows, diffLine{kind: diffDeleted, oldLine: oldLine, text: strings.TrimPrefix(source, "-")})
			oldLine++
			continue
		}
		if strings.HasPrefix(source, " ") {
			rows = append(rows, diffLine{kind: diffContext, oldLine: oldLine, newLine: newLine, text: strings.TrimPrefix(source, " ")})
			oldLine++
			newLine++
			continue
		}
		if source != "" {
			rows = append(rows, diffLine{kind: diffMeta, text: source})
		}
	}
	return rows
}

func parseDiffNumber(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func diffLineNumber(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}

func diffCounts(rows []diffLine) (added, deleted int) {
	for _, row := range rows {
		switch row.kind {
		case diffAdded:
			added++
		case diffDeleted:
			deleted++
		}
	}
	return added, deleted
}

func diffLanguage(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "" {
		return "text"
	}
	return normalizeCodeLanguage(ext)
}

func (a *App) diffExpandedValue(path string) (bool, bool) {
	a.mu.RLock()
	open, remembered := a.diffExpanded[path]
	a.mu.RUnlock()
	return open, remembered
}

func (a *App) rememberDiffExpanded(path string, open bool) {
	a.mu.Lock()
	if a.diffExpanded == nil {
		a.diffExpanded = make(map[string]bool)
	}
	a.diffExpanded[path] = open
	a.mu.Unlock()
}
