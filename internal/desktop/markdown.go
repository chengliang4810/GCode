package desktop

import (
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui"
	goldast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	goldextast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// cachedMessageRender contains only values owned by this package. The parser's
// AST is never retained after the conversion, so a completed entry can be
// consumed by the UI thread without sharing parser state or source buffers.
type cachedMessageRender struct {
	text         string
	documentText string
	document     markdownDocument
	ready        bool
	generation   uint64
}

type markdownBlockKind uint8

const (
	markdownParagraph markdownBlockKind = iota + 1
	markdownHeading
	markdownBlockquote
	markdownBlockList
	markdownCodeBlock
	markdownBlockTable
	markdownThematicBreak
	markdownRawBlock
)

type markdownDocument struct {
	blocks []markdownBlock
}

type markdownBlock struct {
	kind     markdownBlockKind
	level    int
	inline   []markdownInline
	children []markdownBlock
	list     *markdownListModel
	code     string
	language string
	table    *markdownTableModel
}

type markdownListModel struct {
	ordered bool
	start   int
	items   []markdownListItem
}

type markdownListItem struct {
	task   string
	blocks []markdownBlock
}

type markdownTableModel struct {
	columns []markdownTableColumn
	rows    []markdownTableRow
}

type markdownTableColumn struct {
	title string
	align markdownTableAlign
}

type markdownTableRow struct {
	cells []markdownTableCell
}

type markdownTableCell struct {
	align  markdownTableAlign
	inline []markdownInline
}

type markdownTableAlign uint8

const (
	markdownAlignDefault markdownTableAlign = iota
	markdownAlignLeft
	markdownAlignCenter
	markdownAlignRight
)

type markdownInlineKind uint8

const (
	markdownInlineText markdownInlineKind = iota + 1
	markdownInlineCode
	markdownInlineBreak
)

type markdownInline struct {
	kind   markdownInlineKind
	text   string
	url    string
	bold   bool
	italic bool
	strike bool
	image  bool
}

type markdownInlineStyle struct {
	bold   bool
	italic bool
	strike bool
	url    string
}

var markdownParser = parser.New(parser.WithExtensions(extension.GFMParser))

func parseMarkdownDocument(source string) markdownDocument {
	if source == "" {
		return markdownDocument{}
	}
	sourceBytes := []byte(source)
	root := markdownParser.Parse(sourceBytes)
	return markdownDocument{blocks: convertMarkdownBlocks(root, sourceBytes)}
}

func convertMarkdownBlocks(parent goldast.Node, source []byte) []markdownBlock {
	blocks := make([]markdownBlock, 0, parent.ChildCount())
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		block, ok := convertMarkdownBlock(node, source)
		if ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func convertMarkdownBlock(node goldast.Node, source []byte) (markdownBlock, bool) {
	switch n := node.(type) {
	case *goldast.Paragraph:
		return markdownBlock{kind: markdownParagraph, inline: convertMarkdownInlines(n, source, markdownInlineStyle{})}, true
	case *goldast.Heading:
		return markdownBlock{kind: markdownHeading, level: n.Level, inline: convertMarkdownInlines(n, source, markdownInlineStyle{})}, true
	case *goldast.Blockquote:
		return markdownBlock{kind: markdownBlockquote, children: convertMarkdownBlocks(n, source)}, true
	case *goldast.List:
		list := &markdownListModel{ordered: n.IsOrdered(), start: n.Start}
		for itemNode := n.FirstChild(); itemNode != nil; itemNode = itemNode.NextSibling() {
			item, ok := itemNode.(*goldast.ListItem)
			if !ok {
				continue
			}
			task := ""
			if status, ok := extension.TaskStatusOf(item); ok {
				if status == extension.TaskStatusCompleted {
					task = "[x]"
				} else {
					task = "[ ]"
				}
			}
			list.items = append(list.items, markdownListItem{task: task, blocks: convertMarkdownBlocks(item, source)})
		}
		return markdownBlock{kind: markdownBlockList, list: list}, true
	case *goldast.CodeBlock:
		language, _ := n.Language(source)
		return markdownBlock{
			kind:     markdownCodeBlock,
			code:     strings.TrimSuffix(n.Value.Str(source), "\n"),
			language: language,
		}, true
	case *goldast.ThematicBreak:
		return markdownBlock{kind: markdownThematicBreak}, true
	case *goldextast.Table:
		return convertMarkdownTable(n, source), true
	case *goldast.HTMLBlock:
		// Raw HTML is intentionally rendered as literal text. The native
		// renderer never evaluates or injects HTML/WebView content.
		return markdownBlock{kind: markdownRawBlock, code: n.Value.Str(source)}, true
	default:
		return markdownBlock{}, false
	}
}

func convertMarkdownTable(table *goldextast.Table, source []byte) markdownBlock {
	model := &markdownTableModel{}
	for section := table.FirstChild(); section != nil; section = section.NextSibling() {
		isHeader := section.Kind() == goldextast.KindTableHeader
		if isHeader {
			for cellNode := section.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
				cell, ok := cellNode.(*goldextast.TableCell)
				if !ok {
					continue
				}
				align := convertTableAlign(cell.Alignment)
				inlines := convertMarkdownInlines(cell, source, markdownInlineStyle{})
				model.columns = append(model.columns, markdownTableColumn{title: plainMarkdownText(inlines), align: align})
			}
			continue
		}
		for rowNode := section.FirstChild(); rowNode != nil; rowNode = rowNode.NextSibling() {
			row, ok := rowNode.(*goldextast.TableRow)
			if !ok {
				continue
			}
			cells := make([]markdownTableCell, 0, row.ChildCount())
			for cellNode := row.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
				cell, ok := cellNode.(*goldextast.TableCell)
				if !ok {
					continue
				}
				align := convertTableAlign(cell.Alignment)
				inlines := convertMarkdownInlines(cell, source, markdownInlineStyle{})
				cells = append(cells, markdownTableCell{align: align, inline: inlines})
			}
			if !isHeader {
				model.rows = append(model.rows, markdownTableRow{cells: cells})
			}
		}
	}
	return markdownBlock{kind: markdownBlockTable, table: model}
}

func convertTableAlign(align goldextast.Alignment) markdownTableAlign {
	switch align {
	case goldextast.AlignLeft:
		return markdownAlignLeft
	case goldextast.AlignCenter:
		return markdownAlignCenter
	case goldextast.AlignRight:
		return markdownAlignRight
	default:
		return markdownAlignDefault
	}
}

func convertMarkdownInlines(parent goldast.Node, source []byte, style markdownInlineStyle) []markdownInline {
	inlines := make([]markdownInline, 0, parent.ChildCount())
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		appendMarkdownInline(&inlines, node, source, style)
	}
	return inlines
}

func appendMarkdownInline(out *[]markdownInline, node goldast.Node, source []byte, style markdownInlineStyle) {
	appendInline := func(inline markdownInline) {
		if inline.text == "" && inline.kind != markdownInlineBreak {
			return
		}
		if inline.kind == markdownInlineText && !inline.image && len(*out) > 0 {
			last := &(*out)[len(*out)-1]
			if last.kind == markdownInlineText && !last.image && last.url == inline.url &&
				last.bold == inline.bold && last.italic == inline.italic && last.strike == inline.strike {
				last.text += inline.text
				return
			}
		}
		*out = append(*out, inline)
	}

	switch n := node.(type) {
	case *goldast.Text:
		appendInline(markdownInline{kind: markdownInlineText, text: n.Value.Value(source), url: style.url, bold: style.bold, italic: style.italic, strike: style.strike})
		if n.SoftLineBreak() || n.HardLineBreak() {
			appendInline(markdownInline{kind: markdownInlineBreak})
		}
	case *goldast.CodeSpan:
		appendInline(markdownInline{kind: markdownInlineCode, text: n.Value.Value(source), url: style.url, bold: style.bold, italic: style.italic, strike: style.strike})
	case *goldast.Emphasis:
		childStyle := style
		childStyle.italic = true
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			appendMarkdownInline(out, child, source, childStyle)
		}
	case *goldast.Strong:
		childStyle := style
		childStyle.bold = true
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			appendMarkdownInline(out, child, source, childStyle)
		}
	case *goldextast.Strikethrough:
		childStyle := style
		childStyle.strike = true
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			appendMarkdownInline(out, child, source, childStyle)
		}
	case *goldast.Link:
		childStyle := style
		childStyle.url = safeMarkdownURL(n.Destination.Value(source))
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			appendMarkdownInline(out, child, source, childStyle)
		}
	case *goldast.AutoLink:
		appendInline(markdownInline{
			kind:   markdownInlineText,
			text:   n.Label.Value(source),
			url:    safeMarkdownURL(n.Destination.Value(source)),
			bold:   style.bold,
			italic: style.italic,
			strike: style.strike,
		})
	case *goldast.Image:
		alt := plainMarkdownText(convertMarkdownInlines(n, source, markdownInlineStyle{}))
		if alt == "" {
			alt = "图片"
		}
		appendInline(markdownInline{kind: markdownInlineText, text: "[图片未加载：" + alt + "]", image: true, bold: style.bold, italic: style.italic, strike: style.strike})
	case *goldast.RawHTML:
		appendInline(markdownInline{kind: markdownInlineText, text: n.Value.Value(source), url: style.url, bold: style.bold, italic: style.italic, strike: style.strike})
	default:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			appendMarkdownInline(out, child, source, style)
		}
	}
}

func plainMarkdownText(inlines []markdownInline) string {
	var builder strings.Builder
	for _, inline := range inlines {
		if inline.kind == markdownInlineBreak {
			builder.WriteByte('\n')
			continue
		}
		builder.WriteString(inline.text)
	}
	return builder.String()
}

func safeMarkdownURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" {
		return raw
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto", "file":
		return raw
	default:
		return ""
	}
}

func (a *App) renderTextBlocks(c *ui.Context, p palette, cacheKey, source string) {
	document, ready := a.markdownDocumentForRender(cacheKey, source)
	if !ready && len(document.blocks) == 0 {
		// Each SSE update changes the cumulative source. A single RichText
		// invalidates every paragraph's native layout, including intrinsic
		// width probes. Keep completed lines as separate text values so the
		// platform layout cache can reuse them while Markdown parsing waits
		// for the UI queue. The container owns one selection across all
		// lines, including empty paragraphs, so copying preserves source.
		ui.Column(c).WidthPercent(100).MinWidth(0).Selectable().Children(func() {
			for _, line := range strings.Split(source, "\n") {
				ui.RichText(c, ui.Span{Text: line}).FontSize(15.5).LineHeight(1.55).TextColor(p.text).SelectionColor(p.selection)
			}
		})
		return
	}
	a.renderMarkdownBlocks(c, p, document.blocks)
}

// markdownDocumentForRender keeps parsing off the UI thread whenever the app
// has a dispatcher. A cache entry reserves the newest source before work
// starts, and completion compares that source again before publishing.
func (a *App) markdownDocumentForRender(cacheKey, source string) (markdownDocument, bool) {
	a.mu.RLock()
	cached, ok := a.markdownCache[cacheKey]
	dispatch := a.dispatch
	closed := a.closed
	a.mu.RUnlock()
	if ok && cached.text == source && cached.ready {
		return cached.document, true
	}
	if closed {
		return markdownDocument{}, false
	}
	if dispatch == nil {
		document := parseMarkdownDocument(source)
		a.mu.Lock()
		current, exists := a.markdownCache[cacheKey]
		if !a.closed && (!exists || current.text == source) {
			a.markdownCache[cacheKey] = cachedMessageRender{text: source, documentText: source, document: document, ready: true, generation: current.generation}
		}
		a.mu.Unlock()
		return document, true
	}

	a.mu.Lock()
	if current := a.markdownCache[cacheKey]; current.text != source {
		// Appending should not alternate the entire native tree between raw
		// text and Markdown while the next parse waits on the UI thread.
		// Keep the last parsed prefix; replacements discard it immediately.
		appendOnly := strings.HasPrefix(source, current.text)
		if !appendOnly {
			current.generation++
		}
		if current.documentText == "" || !appendOnly {
			current.document, current.documentText = markdownDocument{}, ""
		}
		current.text, current.ready = source, false
		a.markdownCache[cacheKey] = current
	}
	document := a.markdownCache[cacheKey].document
	generation := a.markdownCache[cacheKey].generation
	pending := a.markdownPending[cacheKey]
	if !pending && !a.closed {
		a.markdownPending[cacheKey] = true
	}
	a.mu.Unlock()
	if !pending {
		go a.parseMarkdownAsync(cacheKey, source, generation)
	}
	return document, false
}

func (a *App) parseMarkdownAsync(cacheKey, source string, generation uint64) {
	document := parseMarkdownDocument(source)
	a.post(func() {
		var nextSource string
		var nextGeneration uint64
		var parseNext bool
		a.mu.Lock()
		delete(a.markdownPending, cacheKey)
		if !a.closed {
			cached := a.markdownCache[cacheKey]
			if cached.generation == generation && cached.text == source {
				a.markdownCache[cacheKey] = cachedMessageRender{text: source, documentText: source, document: document, ready: true, generation: generation}
			} else {
				// A completed append is still a valid prefix of the newest
				// source. Publish it so continuous streaming can progress even
				// when every completion is a little behind the next update.
				if cached.generation == generation && strings.HasPrefix(cached.text, source) {
					cached.document, cached.documentText = document, source
					a.markdownCache[cacheKey] = cached
				}
				// A streamed message may have advanced while this parse was
				// running. Queue the newest source immediately after dropping
				// this stale result so it does not depend on another frame.
				nextSource = cached.text
				nextGeneration = cached.generation
				parseNext = true
				a.markdownPending[cacheKey] = true
			}
		}
		a.mu.Unlock()
		if parseNext {
			go a.parseMarkdownAsync(cacheKey, nextSource, nextGeneration)
		}
	})
}

func (a *App) renderMarkdownBlocks(c *ui.Context, p palette, blocks []markdownBlock) {
	for index, block := range blocks {
		if index > 0 {
			ui.Box(c).Height(7)
		}
		a.renderMarkdownBlock(c, p, block)
	}
}

func (a *App) renderMarkdownBlock(c *ui.Context, p palette, block markdownBlock) {
	switch block.kind {
	case markdownParagraph:
		renderMarkdownInlineParagraph(c, p, block.inline, 15.5, 400)
	case markdownHeading:
		renderMarkdownInlineParagraph(c, p, block.inline, markdownHeadingSize(block.level), 650)
	case markdownBlockquote:
		ui.Column(c).WidthPercent(100).Background(p.surfaceAlt).Border(1, p.border).Radius(6).Padding(8, 11).Children(func() {
			a.renderMarkdownBlocks(c, p, block.children)
		})
	case markdownBlockList:
		a.renderMarkdownList(c, p, block.list)
	case markdownCodeBlock:
		a.renderCodeBlock(c, p, block.language, block.code)
	case markdownBlockTable:
		a.renderMarkdownTable(c, p, block.table)
	case markdownThematicBreak:
		ui.Box(c).WidthPercent(100).Height(1).Background(p.border)
	case markdownRawBlock:
		ui.RichText(c, ui.Span{Text: block.code, Font: "SFMono-Regular, Menlo, Consolas, monospace", Size: 13, Color: p.muted}).LineHeight(1.45).SelectionColor(p.selection).Selectable()
	}
}

func markdownHeadingSize(level int) float32 {
	switch {
	case level <= 1:
		return 19
	case level == 2:
		return 17.5
	case level == 3:
		return 16.5
	default:
		return 15.5
	}
}

func (a *App) renderMarkdownList(c *ui.Context, p palette, list *markdownListModel) {
	if list == nil {
		return
	}
	ui.Column(c).Gap(4).WidthPercent(100).Children(func() {
		for index, item := range list.items {
			marker := "-"
			if list.ordered {
				marker = strconv.Itoa(list.start+index) + "."
			}
			if item.task != "" {
				marker += " " + item.task
			}
			ui.Row(c).WidthPercent(100).AlignItems(ui.Start).Gap(8).Children(func() {
				ui.Text(c, marker).Width(30).FontSize(15).TextColor(p.muted)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					a.renderMarkdownBlocks(c, p, item.blocks)
				})
			})
		}
	})
}

func renderMarkdownInlineParagraph(c *ui.Context, p palette, inlines []markdownInline, size float32, weight int) {
	paragraph := ui.RichText(c).FontSize(size).FontWeight(weight).LineHeight(1.55).TextColor(p.text).SelectionColor(p.selection).Selectable()
	paragraph.Children(func() {
		for _, inline := range inlines {
			if inline.kind == markdownInlineBreak {
				ui.Text(c, "\n")
				continue
			}
			span := markdownSpan(inline, p)
			if inline.url != "" {
				ui.Link(c, "", inline.url).TextColor(p.accent).Underline().Children(func() {
					ui.RichText(c, span)
				})
				continue
			}
			ui.RichText(c, span)
		}
	})
}

func markdownSpan(inline markdownInline, p palette) ui.Span {
	span := ui.Span{
		Text:          inline.text,
		Color:         p.text,
		Weight:        400,
		Underline:     inline.url != "",
		Strikethrough: inline.strike,
	}
	if inline.bold {
		span.Weight = 650
	}
	if inline.italic {
		span.Italic = true
	}
	if inline.kind == markdownInlineCode {
		span.Font = "SFMono-Regular, Menlo, Consolas, monospace"
		span.Size = 13
		span.Color = p.codeText
		span.Background = p.code
	}
	if inline.url != "" && inline.kind != markdownInlineCode {
		span.Color = p.accent
	}
	return span
}

func (a *App) renderMarkdownTable(c *ui.Context, p palette, table *markdownTableModel) {
	if table == nil || len(table.columns) == 0 {
		return
	}
	columns := make([]ui.TableColumn, len(table.columns))
	var totalWidth float32
	for index, column := range table.columns {
		width := markdownTableColumnWidth(table, index)
		columns[index] = ui.TableColumn{Title: column.title, Width: width, Fixed: true, Align: markdownTableUIAlign(column.align)}
		totalWidth += width
	}
	ui.ScrollHorizontal(c).WidthPercent(100).MinWidth(0).Children(func() {
		ui.Table(c, nil, columns, len(table.rows), func(row, column int) {
			if row >= len(table.rows) || column >= len(table.rows[row].cells) {
				return
			}
			renderMarkdownInlineParagraph(c, p, table.rows[row].cells[column].inline, 13.5, 400)
		}).MinWidth(totalWidth)
	})
}

func markdownTableUIAlign(align markdownTableAlign) ui.Align {
	switch align {
	case markdownAlignCenter:
		return ui.Center
	case markdownAlignRight:
		return ui.End
	default:
		return ui.Start
	}
}

func markdownTableColumnWidth(table *markdownTableModel, column int) float32 {
	maxRunes := utf8.RuneCountInString(table.columns[column].title)
	for _, row := range table.rows {
		if column < len(row.cells) {
			maxRunes = max(maxRunes, utf8.RuneCountInString(plainMarkdownText(row.cells[column].inline)))
		}
	}
	width := float32(maxRunes)*7.5 + 28
	return max(88, min(width, 360))
}

func (a *App) renderCodeBlock(c *ui.Context, p palette, language, code string) {
	language = normalizeCodeLanguage(language)
	key := codeHighlightKey(p.dark, language, code)
	spans, ready := a.codeSpans(key)
	if !ready {
		spans = []ui.Span{{Text: code, Color: p.codeText}}
		a.scheduleCodeHighlight(key, p, language, code)
	}
	width := codeDisplayWidth(code)
	ui.Column(c).WidthPercent(100).Background(p.code).Radius(7).Children(func() {
		ui.Row(c).Height(27).Padding(7, 10, 0, 10).AlignItems(ui.Center).Children(func() {
			label := language
			if label == "" {
				label = "code"
			}
			ui.Text(c, label).FontSize(10.5).TextColor(p.codeText).SingleLine()
			ui.Spacer(c)
			copyButton := iconButton(c, p, "copy", "复制代码").Size(24, 24).Radius(6).TextColor(p.codeText)
			if copyButton.Clicked() {
				c.WriteClipboard(code)
			}
		})
		ui.ScrollHorizontal(c).WidthPercent(100).MinWidth(0).Children(func() {
			ui.Row(c).MinWidth(width).Padding(9, 12).Children(func() {
				ui.RichText(c, spans...).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(13).LineHeight(1.4).TextColor(p.codeText).SelectionColor(p.selection).Selectable()
			})
		})
	})
}

func codeDisplayWidth(code string) float32 {
	maxRunes := 1
	for _, line := range strings.Split(code, "\n") {
		maxRunes = max(maxRunes, utf8.RuneCountInString(line))
	}
	return float32(maxRunes)*7.8 + 28
}

func (a *App) codeSpans(key string) ([]ui.Span, bool) {
	a.mu.RLock()
	spans, ok := a.codeHighlights[key]
	a.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return append([]ui.Span(nil), spans...), true
}

func (a *App) scheduleCodeHighlight(key string, p palette, language, code string) {
	a.mu.RLock()
	dispatch := a.dispatch
	pending := a.codeHighlightPending[key]
	closed := a.closed
	a.mu.RUnlock()
	if closed || pending {
		return
	}
	if dispatch == nil {
		spans, _ := highlightCodeSpans(p, language, code)
		a.mu.Lock()
		a.codeHighlights[key] = spans
		a.mu.Unlock()
		return
	}
	a.mu.Lock()
	if a.codeHighlightPending[key] || a.closed {
		a.mu.Unlock()
		return
	}
	a.codeHighlightPending[key] = true
	a.mu.Unlock()
	go func() {
		spans, _ := highlightCodeSpans(p, language, code)
		a.post(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			delete(a.codeHighlightPending, key)
			if !a.closed {
				a.codeHighlights[key] = spans
			}
		})
	}()
}

func codeHighlightKey(dark bool, language, code string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(code))
	return language + ":" + boolString(dark) + ":" + strconv.FormatUint(h.Sum64(), 16)
}

func boolString(value bool) string {
	if value {
		return "dark"
	}
	return "light"
}

func normalizeCodeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if index := strings.IndexAny(language, " \t"); index >= 0 {
		language = language[:index]
	}
	switch language {
	case "golang":
		return "go"
	case "js", "jsx":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "py":
		return "python"
	case "sh", "shell", "zsh":
		return "bash"
	case "yml":
		return "yaml"
	default:
		return language
	}
}

func highlightCodeSpans(p palette, language, code string) ([]ui.Span, bool) {
	if language == "" {
		language = inferCodeLanguage(code)
	}
	lexer := lexers.Get(language)
	if lexer == nil {
		return []ui.Span{{Text: code, Color: p.codeText}}, false
	}
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return []ui.Span{{Text: code, Color: p.codeText}}, false
	}
	spans := make([]ui.Span, 0, 16)
	for _, token := range iterator.Tokens() {
		if token.Value == "" {
			continue
		}
		spans = append(spans, ui.Span{Text: token.Value, Color: syntaxColor(p, token.Type)})
	}
	if len(spans) == 0 {
		spans = []ui.Span{{Text: code, Color: p.codeText}}
	}
	return spans, true
}

func inferCodeLanguage(code string) string {
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "package "), strings.HasPrefix(trimmed, "func "):
			return "go"
		case strings.HasPrefix(trimmed, "def "), strings.HasPrefix(trimmed, "import "), strings.HasPrefix(trimmed, "from "):
			return "python"
		case strings.HasPrefix(trimmed, "const "), strings.HasPrefix(trimmed, "let "), strings.HasPrefix(trimmed, "var "), strings.HasPrefix(trimmed, "function "):
			return "javascript"
		case strings.HasPrefix(trimmed, "<"):
			return "html"
		}
	}
	return "text"
}

func syntaxColor(p palette, token chroma.TokenType) ui.Color {
	switch {
	case token.InCategory(chroma.Comment):
		return p.muted
	case token.InCategory(chroma.Keyword), token == chroma.NameFunction, token == chroma.NameClass, token == chroma.NameBuiltin:
		return p.accent
	case token.InSubCategory(chroma.LiteralString):
		return p.success
	case token.InSubCategory(chroma.LiteralNumber):
		return p.warning
	case token.InCategory(chroma.Operator), token.InCategory(chroma.Punctuation):
		return p.codeText
	default:
		return p.codeText
	}
}
