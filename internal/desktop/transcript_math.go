package desktop

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"gcode/internal/design"
	"gcode/internal/linkui"
	"gcode/internal/mathrender"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type mathViewState struct{ cache *mathViewCache }
type mathViewKey struct{ source, family string }
type mathViewResult struct {
	svg           *ui.SVG
	width, height float32
	err           error
}
type mathViewEntry struct {
	result  mathViewResult
	ready   bool
	waiters []ui.Services
	access  uint64
	bytes   int
}
type mathViewCache struct {
	mu      sync.Mutex
	entries map[mathViewKey]*mathViewEntry
	queue   chan mathViewKey
	done    chan struct{}
	clock   uint64
	bytes   int
}

func newMathViewCache(ctx context.Context, asynchronous bool) *mathViewCache {
	cache := &mathViewCache{entries: make(map[mathViewKey]*mathViewEntry)}
	if asynchronous {
		cache.queue = make(chan mathViewKey, 32)
		cache.done = make(chan struct{})
		go func() {
			defer close(cache.done)
			for {
				select {
				case <-ctx.Done():
					return
				case key := <-cache.queue:
					if ctx.Err() != nil {
						return
					}
					result, size := buildMathView(key)
					cache.complete(key, result, size)
				}
			}
		}()
	}
	return cache
}

func buildMathView(key mathViewKey) (mathViewResult, int) {
	formula, err := mathrender.Render(key.source, key.family)
	if err != nil {
		return mathViewResult{err: err}, len(key.source)
	}
	svg, err := ui.ParseSVG(formula.SVG)
	return mathViewResult{svg: svg, width: formula.Width, height: formula.Height, err: err}, len(formula.SVG) + len(key.source)
}

func (cache *mathViewCache) evict() {
	for len(cache.entries) >= 256 || cache.bytes > 8<<20 {
		var oldest mathViewKey
		found := false
		var at uint64
		for key, candidate := range cache.entries {
			if candidate.ready && (!found || candidate.access < at) {
				oldest, at, found = key, candidate.access, true
			}
		}
		if !found {
			break
		}
		cache.bytes -= cache.entries[oldest].bytes
		delete(cache.entries, oldest)
	}
}

func (cache *mathViewCache) lookup(key mathViewKey, services ui.Services) mathViewResult {
	cache.mu.Lock()
	cache.clock++
	if entry := cache.entries[key]; entry != nil {
		entry.access = cache.clock
		if !entry.ready && !slices.Contains(entry.waiters, services) {
			entry.waiters = append(entry.waiters, services)
		}
		result := entry.result
		cache.mu.Unlock()
		return result
	}
	cache.evict()
	entry := &mathViewEntry{access: cache.clock, waiters: []ui.Services{services}}
	cache.entries[key] = entry
	if cache.queue != nil {
		select {
		case cache.queue <- key:
		default:
			delete(cache.entries, key)
		}
		cache.mu.Unlock()
		return mathViewResult{}
	}
	cache.mu.Unlock()
	result, size := buildMathView(key)
	cache.complete(key, result, size)
	return result
}

func (cache *mathViewCache) complete(key mathViewKey, result mathViewResult, size int) {
	cache.mu.Lock()
	entry := cache.entries[key]
	if entry == nil {
		cache.mu.Unlock()
		return
	}
	waiters := entry.waiters
	entry.result, entry.ready, entry.bytes, entry.waiters = result, true, size, nil
	cache.bytes += size
	cache.evict()
	cache.mu.Unlock()
	for _, waiter := range waiters {
		waiter.Invalidate()
	}
}

func (a *App) mathFormula(c *ui.Context, source string) mathViewResult {
	if a.math.cache == nil {
		a.math.cache = newMathViewCache(a.ctx, a.win != nil)
	}
	family := a.state.Settings.Appearance.MathFont
	if family == "" {
		family = "Fira Math"
	}
	result := a.math.cache.lookup(mathViewKey{source: source, family: family}, c.Services())
	// MyGo's SVG atlas declines images above 4096 device pixels. Preserve the
	// source for oversized expressions instead of reserving an invisible image.
	scale := a.mathScale() * max(1, c.DeviceScale())
	if result.svg != nil && (result.width*scale >= 4094 || result.height*scale >= 4094) {
		return mathViewResult{err: errors.New("formula exceeds native image size; showing TeX source")}
	}
	return result
}

func (a *App) mathScale() float32 {
	scale := float32(a.state.Settings.Appearance.MathScale) / 100
	if scale <= 0 {
		scale = 1
	}
	fontSize := float32(14)
	if a.md.Theme != nil && a.md.Theme.FontSize > 0 {
		fontSize = a.md.Theme.FontSize
	}
	return fontSize * scale / 20
}

func (a *App) mathView(c *ui.Context, source string) {
	result := a.mathFormula(c, source)
	if result.svg == nil {
		view := ui.Text(c, source).Font("Geist Mono").Selectable().Label(source)
		if result.err != nil {
			view.Tooltip(result.err.Error())
		}
		return
	}
	scale, services := a.mathScale(), c.Services()
	ui.ScrollHorizontal(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Padding(6, 0).Children(func() {
			ui.Image(c, result.svg).Size(result.width*scale, result.height*scale).Shrink(0).Label(source).Menu(func(menu *ui.Menu) {
				if menu.Item(tr(c, "Copy")).Chosen() {
					services.WriteClipboard(source)
				}
			})
		})
	})
}

type mathInlineRun struct {
	span   ui.Span
	link   string
	source string
}

// mathInlineRuns preserves native text/link styles while reserving a shaped
// inline object for a formula. No Markdown source is rewritten or executed.
func mathInlineRuns(inlines []parser.Inline, base ui.Span, codeFont string) []mathInlineRun {
	var runs []mathInlineRun
	var walk func(int, int, ui.Span, string)
	walk = func(start, end int, style ui.Span, link string) {
		for i := start; i < end; {
			value := inlines[i]
			next := max(i+1, value.Next)
			switch value.Kind {
			case parser.Strong:
				strong := style
				strong.Weight = 600
				walk(i+1, next, strong, link)
			case parser.Emphasis:
				italic := style
				italic.Italic = true
				walk(i+1, next, italic, link)
			case parser.Strikethrough:
				strike := style
				strike.Strikethrough = true
				walk(i+1, next, strike, link)
			case parser.Link:
				walk(i+1, next, style, value.Dest)
			case parser.Image:
				walk(i+1, next, style, link)
			default:
				run := mathInlineRun{span: style, link: link}
				run.span.Text = value.Text
				switch value.Kind {
				case parser.Math:
					run.source = value.Text
				case parser.Code:
					run.span.Font = codeFont
				case parser.SoftBreak:
					run.span.Text = " "
				case parser.LineBreak:
					run.span.Text = "\n"
				case parser.HTML, parser.FootnoteRef:
					// Leave these to the native Markdown renderer: hiding an
					// HTML marker or breaking a footnote would lose content.
					return
				}
				runs = append(runs, run)
			}
			i = next
		}
	}
	walk(0, len(inlines), base, "")
	return runs
}

func (a *App) mathMarkdownBlock(c *ui.Context, document *parser.Document, index int) bool {
	block := document.Block(index)
	if block.Kind != parser.Paragraph && block.Kind != parser.Heading {
		return false
	}
	inlines := document.Inlines(index)
	hasMath := false
	for _, in := range inlines {
		if in.Kind == parser.HTML || in.Kind == parser.FootnoteRef || in.Kind == parser.Image {
			return false
		}
		hasMath = hasMath || in.Kind == parser.Math
	}
	if !hasMath {
		return false
	}
	theme := a.md.Theme
	if theme == nil {
		return false
	}
	fontSize, weight := theme.FontSize, 0
	if block.Kind == parser.Heading {
		fontSize *= theme.Headings[block.Level-1].Size
		weight = theme.Headings[block.Level-1].Weight
	}
	runs := mathInlineRuns(inlines, ui.Span{Font: theme.Font, Size: fontSize, Weight: weight}, theme.CodeFont)
	formulaAt := make(map[int]*ui.SVG)
	copyObjects := make(map[int]string)
	var display strings.Builder
	runeOffset := 0
	for i := range runs {
		run := &runs[i]
		if run.source != "" {
			result := a.mathFormula(c, run.source)
			if result.svg != nil {
				run.span.Text = "\ufffc"
				run.span.ObjectWidth, run.span.ObjectHeight = result.width*a.mathScale(), result.height*a.mathScale()
				formulaAt[runeOffset] = result.svg
				copyObjects[runeOffset] = `\(` + run.source + `\)`
			} else {
				run.span.Font, run.span.Text = theme.CodeFont, "$"+run.source+"$"
			}
		}
		if run.link != "" {
			run.span.Color = theme.Link
		}
		display.WriteString(run.span.Text)
		runeOffset += utf8.RuneCountInString(run.span.Text)
	}
	source, services := strings.TrimRight(document.Lines(index), "\n"), c.Services()
	element := ui.RichText(c).Font(theme.Font).FontSize(fontSize).Label(source)
	if block.Kind == parser.Heading {
		element.Role(ui.RoleHeading).Level(block.Level)
	}
	element.Children(func() {
		config := a.linkConfig(a.linkSource, "", c.Services())
		for _, run := range runs {
			if run.link == "" {
				ui.RichText(c, run.span)
			} else {
				if destination, ok := config.Resolve(run.link); ok {
					linkui.Link(c, []ui.Span{run.span}, destination, config)
				} else {
					ui.RichText(c, run.span)
				}
			}
		}
	})
	a.linkSelection.RegisterObjects(element.ID(), display.String(), source, copyObjects)
	element.ContextMenu(func(menu *ui.Menu) {
		if menu.Item(tr(c, "Copy")).Chosen() {
			services.WriteClipboard(source)
		}
	})
	element.DrawObjects(func(painter *ui.Painter, at int, rect ui.Rect) {
		if formula := formulaAt[at]; formula != nil {
			painter.Image(formula, rect, ui.Contain)
		}
	})
	return true
}

func (a *App) configureMathMarkdown() {
	for _, options := range []*markdown.Options{&a.md, &a.userMD} {
		options.Extensions = parser.Default | parser.ExtDollarMath
		options.Math, options.Block, options.Code = nil, nil, nil
		options.CodeBlock = design.MarkdownCodeBlock
		options.Block = a.transcriptMarkdownBlock
		if !a.state.Settings.Appearance.RenderMath {
			continue
		}
		options.Math = a.mathView
		code := func(c *ui.Context, block *markdown.CodeBlockView) {
			if block.Open {
				design.MarkdownCodeBlock(c, block)
				return
			}
			a.mathView(c, strings.TrimRight(block.Code(), "\n"))
		}
		options.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
			"math": code, "latex": code,
		}
	}
}
