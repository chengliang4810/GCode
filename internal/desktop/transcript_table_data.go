package desktop

import (
	"html"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
)

type transcriptTableCell struct {
	markdown string
	text     string
}

type transcriptTableData struct {
	rows  [][]transcriptTableCell
	align []parser.Align
	live  bool
}

// Capture values, not parser blocks: streaming mutates its document in place.
func readTranscriptTable(document *parser.Document, index int) transcriptTableData {
	block := document.Block(index)
	table := transcriptTableData{align: append([]parser.Align(nil), block.Align...), live: document.Streaming() && block.Open}
	for row := range document.Children(index) {
		cells := make([]transcriptTableCell, 0, len(table.align))
		for cell := range document.Children(row) {
			inlines := document.Inlines(cell)
			md, plain := tableInlineText(inlines, 0, len(inlines))
			cells = append(cells, transcriptTableCell{markdown: md, text: strings.Join(strings.Fields(plain), " ")})
		}
		for len(cells) < len(table.align) {
			cells = append(cells, transcriptTableCell{})
		}
		table.rows = append(table.rows, cells[:min(len(cells), len(table.align))])
	}
	return table
}

// Normalize resolved inlines so reference links keep their destinations when a
// cell is rendered independently. Raw HTML remains inert MyGo Markdown text.
func tableInlineText(inlines []parser.Inline, start, end int) (string, string) {
	var md, plain strings.Builder
	for i := start; i < end; {
		in := inlines[i]
		next := min(end, max(i+1, in.Next))
		child, text := "", ""
		if next > i+1 {
			child, text = tableInlineText(inlines, i+1, next)
		}
		switch in.Kind {
		case parser.Text:
			md.WriteString(escapeTableInline(in.Text))
			plain.WriteString(in.Text)
		case parser.Code:
			fence := "`"
			for strings.Contains(in.Text, fence) {
				fence += "`"
			}
			text := in.Text
			if strings.TrimSpace(text) != "" {
				text = " " + text + " "
			}
			md.WriteString(fence + text + fence)
			plain.WriteString(in.Text)
		case parser.Strong, parser.Emphasis, parser.Strikethrough:
			mark := "**"
			if in.Kind == parser.Emphasis {
				mark = "*"
			} else if in.Kind == parser.Strikethrough {
				mark = "~~"
			}
			md.WriteString(mark + child + mark)
			plain.WriteString(text)
		case parser.Link, parser.Image:
			if in.Kind == parser.Image {
				md.WriteByte('!')
			}
			destination := strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D").Replace(in.Dest)
			md.WriteString("[" + child + "](<" + destination + ">)")
			plain.WriteString(text)
		case parser.Math:
			md.WriteString("$" + in.Text + "$")
			plain.WriteString(in.Text)
		case parser.SoftBreak, parser.LineBreak:
			md.WriteString("<br>")
			plain.WriteByte(' ')
		case parser.HTML:
			md.WriteString(in.Text)
			tag := strings.ToLower(strings.Trim(in.Text, "<>/ \t\n"))
			if at := strings.IndexAny(tag, " \t\n"); at >= 0 {
				tag = tag[:at]
			}
			if tag == "br" {
				plain.WriteByte(' ')
			} else if !strings.Contains("|b|strong|i|em|u|ins|s|del|strike|code|kbd|samp|mark|sub|sup|small|", "|"+tag+"|") {
				plain.WriteString(html.UnescapeString(in.Text))
			}
		case parser.FootnoteRef:
			value := "[" + in.Text + "]"
			md.WriteString(escapeTableInline(value))
			plain.WriteString(value)
		}
		i = next
	}
	return md.String(), plain.String()
}

func escapeTableInline(text string) string {
	var out strings.Builder
	for _, r := range text {
		if strings.ContainsRune("\\`*_{}[]()#+-.!|>~<$", r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}

func (table transcriptTableData) markdownText() string {
	if len(table.rows) == 0 {
		return ""
	}
	var out strings.Builder
	for i, row := range table.rows {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString("|")
		for _, cell := range row {
			out.WriteString(" " + strings.NewReplacer("\\", "\\\\", "|", "\\|", "\r\n", "<br>", "\n", "<br>").Replace(cell.text) + " |")
		}
		if i == 0 {
			out.WriteString("\n|")
			for range row {
				out.WriteString(" --- |")
			}
		}
	}
	return out.String()
}

func (table transcriptTableData) csvText() string {
	var out strings.Builder
	out.WriteString("\ufeff")
	for i, row := range table.rows {
		if i > 0 {
			out.WriteString("\r\n")
		}
		for j, cell := range row {
			if j > 0 {
				out.WriteByte(',')
			}
			text := cell.text
			// A model's cell must remain text when opened in spreadsheet software.
			if trimmed := strings.TrimSpace(text); trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) ||
				strings.HasPrefix(text, "\t") || strings.HasPrefix(text, "\r") || strings.HasPrefix(text, "\n") {
				text = "'" + text
			}
			if strings.ContainsAny(text, "\",\r\n") {
				text = "\"" + strings.ReplaceAll(text, "\"", "\"\"") + "\""
			}
			out.WriteString(text)
		}
	}
	return out.String()
}
