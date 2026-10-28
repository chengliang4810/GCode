package desktop

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui/markdown/parser"
)

type thoughtStyle struct{ bold, italic, code, link, strike bool }
type thoughtRun struct {
	text  string
	style thoughtStyle
}
type thoughtLine []thoughtRun
type thoughtCache struct {
	text      string
	live      bool
	lines     []thoughtLine
	truncated int
}

// thoughtLines ports thought_block_lines/wrap_styled_runs from transcript.rs.
// The official Markdown parser supplies blocks and inline styles; the native
// RichText widget paints fixed 18px lines, without interactive links or cards.
func thoughtLines(text string, live bool) ([]thoughtLine, int) {
	doc := parser.Parse(text)
	doc.SetStreaming(live)
	var lines []thoughtLine
	for i := range doc.Children(0) {
		if len(lines) > 0 {
			lines = append(lines, nil)
		}
		thoughtBlock(doc, i, 0, &lines)
	}
	for len(lines) > 0 && strings.TrimSpace(thoughtPlain(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	truncated := max(len(lines)-activityDetailLines, 0)
	if truncated > 0 {
		if live {
			lines = lines[truncated:]
			for len(lines) > 0 && strings.TrimSpace(thoughtPlain(lines[0])) == "" {
				lines = lines[1:]
			}
		} else {
			lines = lines[:activityDetailLines]
		}
	}
	return lines, truncated
}

func thoughtPlain(line thoughtLine) string {
	var text strings.Builder
	for _, run := range line {
		text.WriteString(run.text)
	}
	return text.String()
}

func thoughtBlock(doc *parser.Document, index, indent int, out *[]thoughtLine) {
	b := doc.Block(index)
	switch b.Kind {
	case parser.Paragraph, parser.Heading:
		runs := thoughtInlines(doc.Inlines(index), thoughtStyle{bold: b.Kind == parser.Heading})
		wrapThought(runs, indent, out)
	case parser.CodeBlock, parser.MathBlock, parser.HTMLBlock:
		for _, raw := range strings.Split(strings.TrimRight(doc.Lines(index), "\n"), "\n") {
			runes := []rune(raw)
			budget := max(96-indent, 16)
			for len(runes) > budget {
				*out = append(*out, thoughtLine{{strings.Repeat(" ", indent), thoughtStyle{}}, {string(runes[:budget]), thoughtStyle{code: true}}})
				runes = runes[budget:]
			}
			*out = append(*out, thoughtLine{{strings.Repeat(" ", indent), thoughtStyle{}}, {string(runes), thoughtStyle{code: true}}})
		}
	case parser.List:
		itemIndex := 0
		for item := range doc.Children(index) {
			marker := "• "
			if b.Ordered {
				marker = fmt.Sprintf("%d. ", b.Number+itemIndex)
			}
			mark := len(*out)
			for child := range doc.Children(item) {
				thoughtBlock(doc, child, indent+len([]rune(marker)), out)
			}
			if len(*out) == mark {
				*out = append(*out, thoughtLine{{"", thoughtStyle{}}})
			}
			(*out)[mark][0].text = strings.Repeat(" ", indent) + marker
			itemIndex++
		}
	case parser.Quote:
		mark := len(*out)
		for child := range doc.Children(index) {
			if len(*out) > mark {
				*out = append(*out, nil)
			}
			thoughtBlock(doc, child, indent+2, out)
		}
		for i := mark; i < len(*out); i++ {
			if len((*out)[i]) > 0 && len((*out)[i][0].text) >= indent+2 {
				text := (*out)[i][0].text
				(*out)[i][0].text = text[:indent] + "│ " + text[indent+2:]
			}
		}
	case parser.Table:
		for row := range doc.Children(index) {
			var runs thoughtLine
			for cell := range doc.Children(row) {
				if len(runs) > 0 {
					runs = append(runs, thoughtRun{text: " · "})
				}
				runs = append(runs, thoughtInlines(doc.Inlines(cell), thoughtStyle{bold: doc.Block(row).Header})...)
			}
			wrapThought(runs, indent, out)
		}
	case parser.ThematicBreak:
		*out = append(*out, thoughtLine{{strings.Repeat(" ", indent) + "———", thoughtStyle{}}})
	default:
		for child := range doc.Children(index) {
			thoughtBlock(doc, child, indent, out)
		}
	}
}

func thoughtInlines(inlines []parser.Inline, base thoughtStyle) thoughtLine {
	var out thoughtLine
	var walk func(int, int, thoughtStyle)
	walk = func(start, end int, style thoughtStyle) {
		for i := start; i < end; {
			in := inlines[i]
			next := max(i+1, in.Next)
			switch in.Kind {
			case parser.Strong:
				s := style
				s.bold = true
				walk(i+1, next, s)
			case parser.Emphasis:
				s := style
				s.italic = true
				walk(i+1, next, s)
			case parser.Strikethrough:
				s := style
				s.strike = true
				walk(i+1, next, s)
			case parser.Link:
				s := style
				s.link = true
				walk(i+1, next, s)
			case parser.Image:
				walk(i+1, next, style)
			case parser.SoftBreak:
				pushThought(&out, " ", style)
			case parser.LineBreak:
				pushThought(&out, "\n", style)
			case parser.Code, parser.Math:
				s := style
				s.code = true
				text := in.Text
				if in.Kind == parser.Math {
					text = "$" + text + "$"
				}
				pushThought(&out, text, s)
			default:
				pushThought(&out, in.Text, style)
			}
			i = next
		}
	}
	walk(0, len(inlines), base)
	return out
}

func pushThought(line *thoughtLine, text string, style thoughtStyle) {
	if text == "" {
		return
	}
	if len(*line) > 0 && (*line)[len(*line)-1].style == style {
		(*line)[len(*line)-1].text += text
		return
	}
	*line = append(*line, thoughtRun{text, style})
}

func wrapThought(runs thoughtLine, indent int, out *[]thoughtLine) {
	budget := max(96-indent, 16)
	segments := []thoughtLine{nil}
	for _, run := range runs {
		for i, piece := range strings.Split(run.text, "\n") {
			if i > 0 {
				segments = append(segments, nil)
			}
			pushThought(&segments[len(segments)-1], piece, run.style)
		}
	}
	for _, segment := range segments {
		var tokens []thoughtLine
		inToken := false
		for _, run := range segment {
			for _, r := range run.text {
				if unicode.IsSpace(r) {
					inToken = false
					continue
				}
				if !inToken {
					tokens = append(tokens, nil)
					inToken = true
				}
				pushThought(&tokens[len(tokens)-1], string(r), run.style)
			}
		}
		var line thoughtLine
		length := 0
		flush := func() {
			*out = append(*out, append(thoughtLine{{strings.Repeat(" ", indent), thoughtStyle{}}}, line...))
			line = nil
			length = 0
		}
		for _, token := range tokens {
			tokenLength := len([]rune(thoughtPlain(token)))
			if tokenLength > budget {
				if length > 0 {
					flush()
				}
				for _, piece := range token {
					runes := []rune(piece.text)
					for len(runes) > 0 {
						n := min(len(runes), budget-length)
						pushThought(&line, string(runes[:n]), piece.style)
						runes = runes[n:]
						length += n
						if length == budget {
							flush()
						}
					}
				}
				continue
			}
			if length > 0 && length+1+tokenLength > budget {
				flush()
			}
			if length > 0 {
				pushThought(&line, " ", line[len(line)-1].style)
				length++
			}
			for _, piece := range token {
				pushThought(&line, piece.text, piece.style)
			}
			length += tokenLength
		}
		if length > 0 {
			flush()
		}
	}
}
