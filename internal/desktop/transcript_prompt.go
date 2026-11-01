package desktop

import (
	"strings"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/linkui"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (a *App) userMarkdownBlock(c *ui.Context, document *parser.Document, index int) bool {
	if a.transcriptTableBlock(c, document, index) {
		return true
	}
	if a.promptMarkdownBlock(c, document, index) {
		return true
	}
	if a.state.Settings.Appearance.RenderMath {
		if a.mathMarkdownBlock(c, document, index) {
			return true
		}
	}
	return a.linkMarkdownBlock(c, document, index, a.userMD.Theme)
}

// Only paragraphs with actual reference links use this renderer. Code blocks
// and ordinary Markdown retain their existing renderer and selection behavior.
func (a *App) promptMarkdownBlock(c *ui.Context, document *parser.Document, index int) bool {
	block := document.Block(index)
	if block.Kind != parser.Paragraph && block.Kind != parser.Heading {
		return false
	}
	inlines := document.Inlines(index)
	hasReference := false
	for _, in := range inlines {
		if in.Kind == parser.Math || in.Kind == parser.HTML || in.Kind == parser.Image || in.Kind == parser.FootnoteRef {
			return false
		}
		hasReference = hasReference || in.Kind == parser.Link && strings.HasPrefix(in.Dest, "gcode://")
	}
	if !hasReference || a.userMD.Theme == nil {
		return false
	}
	theme := a.userMD.Theme
	size := theme.FontSize
	if block.Kind == parser.Heading {
		size *= theme.Headings[block.Level-1].Size
	}
	runs := mathInlineRuns(inlines, ui.Span{Font: theme.Font, Size: size}, theme.CodeFont)
	var spans []ui.Span
	var links []string
	var glyphs []string
	for _, run := range runs {
		if strings.HasPrefix(run.link, "gcode://") {
			parts := contracts.ParsePrompt("[token](" + run.link + ")")
			if len(parts) == 1 && parts[0].Reference != nil {
				spans = append(spans, ui.Span{Text: "\ufffc", ObjectWidth: size + 3, ObjectHeight: size})
				links = append(links, "")
				glyphs = append(glyphs, promptGlyph(*parts[0].Reference))
				run.span.Color, run.span.Weight = a.palette.Accent, 600
				run.link = ""
			}
		} else if run.link != "" {
			run.span.Color = theme.Link
		}
		spans = append(spans, run.span)
		links = append(links, run.link)
	}
	element := ui.RichText(c).Font(theme.Font).FontSize(size).FixedLineHeight(theme.FixedLineHeight).Selectable().Label(promptDisplay(document.Lines(index)))
	element.Children(func() {
		config := a.linkConfig(a.linkSource, "", c.Services())
		for i, span := range spans {
			if links[i] == "" || strings.HasPrefix(links[i], "gcode://") {
				ui.RichText(c, span)
			} else {
				if destination, ok := config.Resolve(links[i]); ok {
					linkui.Link(c, []ui.Span{span}, destination, config)
				} else {
					ui.RichText(c, span)
				}
			}
		}
	})
	if block.Kind == parser.Heading {
		element.Role(ui.RoleHeading).Level(block.Level)
	}
	source, services := document.Lines(index), c.Services()
	element.Menu(func(menu *ui.Menu) {
		if menu.Item(tr(c, "Copy")).Chosen() {
			services.WriteClipboard(source)
		}
	})
	font := ui.Font{Family: theme.Font, Size: size}
	element.DrawOver(func(painter *ui.Painter, bounds ui.Rect) {
		layout := ui.ShapeRichText(spans, font, bounds.W)
		for i, object := range layout.Objects() {
			if i >= len(glyphs) {
				break
			}
			rect := object.Rect
			rect.X, rect.Y, rect.W = rect.X+bounds.X, rect.Y+bounds.Y, size
			if svg := assets.Icon(glyphs[i]); svg != nil {
				painter.Icon(svg, rect, a.palette.Accent)
			}
		}
	})
	return true
}
