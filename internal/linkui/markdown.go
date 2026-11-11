package linkui

import (
	"strings"
	"unicode/utf8"

	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type piece struct {
	spans       []ui.Span
	destination *Destination
	copySource  string
}

// Block renders paragraphs and headings containing resolved links. Unsupported
// inline extensions remain with the caller's existing Markdown renderer.
func Block(c *ui.Context, document *parser.Document, index int, cfg Config, theme *markdown.Theme) bool {
	b := document.Block(index)
	if b.Kind != parser.Paragraph && b.Kind != parser.Heading || cfg.Resolve == nil {
		return false
	}
	inlines := document.Inlines(index)
	for _, in := range inlines {
		switch in.Kind {
		case parser.HTML, parser.Math, parser.FootnoteRef, parser.Image:
			return false
		case parser.Link:
			// In particular, unresolved document anchors must retain the
			// Markdown renderer's scroll behavior in mixed paragraphs.
			if !in.Pending {
				if _, ok := cfg.Resolve(in.Dest); !ok {
					return false
				}
			}
		}
	}
	th := paragraphTheme(c, theme)
	base := ui.Span{Font: th.Font, Size: th.FontSize, Color: th.Text}
	lineHeight := th.FixedLineHeight
	if lineHeight <= 0 {
		lineHeight = th.FontSize * th.LineHeight
	}
	if b.Kind == parser.Heading {
		h := th.Headings[b.Level-1]
		base.Size, base.Weight, base.Color = th.FontSize*h.Size, h.Weight, h.Color
		if base.Color == (ui.Color{}) {
			base.Color = th.Text
		}
		if h.FixedLineHeight > 0 {
			lineHeight = h.FixedLineHeight
		}
	}
	pieces, linked := inlinePieces(inlines, cfg, &th, base)
	if !linked {
		return false
	}
	element := ui.RichText(c).Font(th.Font).FontSize(base.Size).FixedLineHeight(lineHeight)
	if b.Kind == parser.Heading {
		element.Role(ui.RoleHeading).Level(b.Level)
		h := th.Headings[b.Level-1]
		if h.Rule {
			element.BorderWidth(0, 0, 1, 0).BorderColor(th.Rule).Padding(0, 0, th.FontSize*.3, 0)
		}
	}
	var spans []ui.Span
	type decoration struct{ start, end int }
	var decorations []decoration
	offset := 0
	runeOffset := 0
	var display strings.Builder
	var replacements []selectionReplacement
	element.Children(func() {
		for _, pc := range pieces {
			start := offset
			runeStart := runeOffset
			for _, span := range pc.spans {
				offset += utf16Length(span.Text)
				runeOffset += utf8.RuneCountInString(span.Text)
				display.WriteString(span.Text)
			}
			if pc.copySource != "" {
				replacements = append(replacements, selectionReplacement{runeStart, runeOffset, pc.copySource})
			}
			if pc.destination == nil {
				ui.RichText(c, pc.spans...)
				spans = append(spans, pc.spans...)
				continue
			}
			linkElement := link(c, pc.spans, *pc.destination, cfg, false)
			for _, span := range pc.spans {
				span.Color, span.Weight = c.Theme().Accent, max(500, span.Weight)
				span.Background, span.Radius = ui.Color{}, 0
				span.Underline, span.WavyUnderline = false, false
				spans = append(spans, span)
			}
			if linkElement.Hovered() {
				decorations = append(decorations, decoration{start, offset})
			}
		}
	})
	cfg.Selection.register(element.ID(), display.String(), document.Lines(index), replacements)
	if len(decorations) > 0 {
		color := c.Theme().Accent
		font := ui.Font{Family: th.Font, Size: base.Size}
		element.DrawOver(func(p *ui.Painter, bounds ui.Rect) {
			for _, decoration := range decorations {
				drawUnderline(p, bounds, spans, font, lineHeight, decoration.start, decoration.end, color)
			}
		})
	}
	return true
}

func inlinePieces(inlines []parser.Inline, cfg Config, th *markdown.Theme, base ui.Span) ([]piece, bool) {
	var result []piece
	hasLinks := false
	var walk func(int, int, ui.Span, *Destination)
	add := func(span ui.Span, d *Destination) {
		if span.Text == "" {
			return
		}
		if len(result) == 0 || result[len(result)-1].destination != d {
			result = append(result, piece{destination: d})
		}
		result[len(result)-1].spans = append(result[len(result)-1].spans, span)
	}
	walk = func(start, end int, style ui.Span, destination *Destination) {
		for i := start; i < end; {
			in := inlines[i]
			next := max(i+1, min(in.Next, end))
			switch in.Kind {
			case parser.Strong:
				s := style
				s.Weight, s.Color = max(600, th.StrongWeight), th.Strong
				walk(i+1, next, s, destination)
			case parser.Emphasis:
				s := style
				s.Italic = true
				walk(i+1, next, s, destination)
			case parser.Strikethrough:
				s := style
				s.Strikethrough = true
				walk(i+1, next, s, destination)
			case parser.Link:
				d := destination
				if !in.Pending {
					if resolved, ok := cfg.Resolve(in.Dest); ok {
						if resolved.Raw == "" {
							resolved.Raw = in.Dest
						}
						d, hasLinks = &resolved, true
					}
				}
				walk(i+1, next, style, d)
			case parser.Code:
				s := style
				s.Text = in.Text
				if destination == nil {
					target, err := linktarget.Parse(in.Text)
					if resolved, ok := cfg.Resolve(in.Text); ok && resolved.Kind == linktarget.File &&
						(resolved.KnownFile || err == nil && target.IsAbsoluteFile()) {
						if resolved.Raw == "" {
							resolved.Raw = in.Text
						}
						if resolved.Label != "" {
							s.Text = resolved.Label
						}
						add(s, &resolved)
						result[len(result)-1].copySource = in.Text
						hasLinks = true
						i = next
						continue
					}
				}
				s.Font, s.Size = th.CodeFont, style.Size*th.CodeSize
				s.Background, s.Color, s.Radius = th.CodeBackground, th.Code, 3
				add(s, destination)
			case parser.SoftBreak, parser.LineBreak:
				s := style
				s.Text = " "
				if in.Kind == parser.LineBreak {
					s.Text = "\n"
				}
				add(s, destination)
			default:
				s := style
				s.Text = in.Text
				add(s, destination)
			}
			i = next
		}
	}
	walk(0, len(inlines), base, nil)
	return result, hasLinks
}

// Resolve the fields used by this renderer against the pinned Markdown defaults.
func paragraphTheme(c *ui.Context, override *markdown.Theme) markdown.Theme {
	th := *markdown.DefaultTheme(c.Theme())
	if override == nil {
		return th
	}
	if override.FontSize != 0 {
		th.FontSize = override.FontSize
	}
	if override.Font != "" {
		th.Font = override.Font
	}
	if override.CodeFont != "" {
		th.CodeFont = override.CodeFont
	}
	if override.LineHeight != 0 {
		th.LineHeight = override.LineHeight
	}
	th.FixedLineHeight = override.FixedLineHeight
	for to, from := range map[*ui.Color]ui.Color{
		&th.Text: override.Text, &th.Strong: override.Strong, &th.Code: override.Code,
		&th.CodeBackground: override.CodeBackground, &th.Rule: override.Rule,
	} {
		if from != (ui.Color{}) {
			*to = from
		}
	}
	if override.CodeSize != 0 {
		th.CodeSize = override.CodeSize
	}
	if override.StrongWeight != 0 {
		th.StrongWeight = override.StrongWeight
	}
	for i, h := range override.Headings {
		if h.Size != 0 {
			th.Headings[i].Size = h.Size
		}
		if h.Weight != 0 {
			th.Headings[i].Weight = h.Weight
		}
		if h.Color != (ui.Color{}) {
			th.Headings[i].Color = h.Color
		}
		th.Headings[i].FixedLineHeight, th.Headings[i].Rule = h.FixedLineHeight, h.Rule
	}
	return th
}
