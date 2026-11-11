package desktop

import (
	"strings"
	"unicode/utf8"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/linkui"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type promptInlineRun struct {
	span  ui.Span
	link  string
	glyph string
}

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
	promptRuns := make([]promptInlineRun, 0, len(runs)+1)
	for _, run := range runs {
		if strings.HasPrefix(run.link, "gcode://") {
			parts := contracts.ParsePrompt("[token](" + run.link + ")")
			if len(parts) == 1 && parts[0].Reference != nil {
				promptRuns = append(promptRuns, promptInlineRun{
					// Match MyGo's inline-icon baseline: keep part of the
					// square below it, beside both Latin and CJK labels.
					span: ui.Span{Text: "\ufffc", Size: size, ObjectWidth: size + 3,
						ObjectHeight: size * .86, ObjectDescent: size * .14},
					glyph: promptGlyph(*parts[0].Reference),
				})
				// Keep the icon with the reference's first word at a wrap.
				run.span.Text = "\u2060" + run.span.Text
				run.span.Color, run.span.Weight = a.palette.Accent, 600
				run.link = ""
			}
		} else if run.link != "" {
			run.span.Color = theme.Link
		}
		promptRuns = append(promptRuns, promptInlineRun{span: run.span, link: run.link})
	}
	glyphAt := make(map[int]string)
	copyObjects := make(map[int]string)
	runeOffset := 0
	var display strings.Builder
	for _, run := range promptRuns {
		if run.glyph != "" {
			glyphAt[runeOffset] = run.glyph
			copyObjects[runeOffset], copyObjects[runeOffset+1] = "", ""
		}
		display.WriteString(run.span.Text)
		runeOffset += utf8.RuneCountInString(run.span.Text)
	}
	element := ui.RichText(c).Font(theme.Font).FontSize(size).FixedLineHeight(theme.FixedLineHeight).Label(promptDisplay(document.Lines(index)))
	element.Children(func() {
		config := a.linkConfig(a.linkSource, "", c.Services())
		for _, run := range promptRuns {
			if run.link == "" || strings.HasPrefix(run.link, "gcode://") {
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
	if block.Kind == parser.Heading {
		element.Role(ui.RoleHeading).Level(block.Level)
	}
	source, services := document.Lines(index), c.Services()
	a.linkSelection.RegisterObjects(element.ID(), display.String(), source, copyObjects)
	element.ContextMenu(func(menu *ui.Menu) {
		if menu.Item(tr(c, "Copy")).Chosen() {
			services.WriteClipboard(source)
		}
	})
	element.DrawObjects(func(painter *ui.Painter, at int, rect ui.Rect) {
		if glyph := glyphAt[at]; glyph != "" {
			if svg := assets.Icon(glyph); svg != nil {
				rect.W = size
				painter.Icon(svg, rect, a.palette.Accent)
			}
		}
	})
	return true
}
