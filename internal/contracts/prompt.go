package contracts

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

// PromptReference is an explicit user-selected capability, not an instruction
// to install, enable, authenticate or invoke an external service.
type PromptReference struct {
	Kind, ID, Label string
}

type PromptPart struct {
	Text       string
	Start, End int // byte offsets in the canonical prompt
	Reference  *PromptReference
}

var promptReferencePattern = regexp.MustCompile(`\[((?:\\.|[^\]\\\r\n])*)\]\(gcode://(command|skill|mcp)/([A-Za-z0-9._%~-]+)\)`)

func (r PromptReference) Markdown() string {
	label := strings.NewReplacer(
		"\\", "&#92;", "[", "&#91;", "]", "&#93;", "*", "&#42;", "_", "&#95;", "~", "&#126;", "`", "&#96;",
		"&", "&amp;", "<", "&lt;", ">", "&gt;", "\n", "&#10;", "\r", "&#13;",
	).Replace(r.Label)
	return "[" + label + "](gcode://" + r.Kind + "/" + url.PathEscape(r.ID) + ")"
}

func (r PromptReference) Value() string {
	if r.Kind == "command" {
		return "/" + r.ID
	}
	return "@" + r.Kind + ":" + r.ID
}

func ParsePrompt(text string) []PromptPart {
	var parts []PromptPart
	offset := 0
	for _, match := range promptReferencePattern.FindAllStringSubmatchIndex(text, -1) {
		if match[0] > offset {
			parts = append(parts, PromptPart{Text: text[offset:match[0]], Start: offset, End: match[0]})
		}
		id, err := url.PathUnescape(text[match[6]:match[7]])
		if err != nil || id == "" || strings.ContainsAny(id, "/\\ \t\r\n") {
			parts = append(parts, PromptPart{Text: text[match[0]:match[1]], Start: match[0], End: match[1]})
		} else {
			label := text[match[2]:match[3]]
			var decoded strings.Builder
			for i := 0; i < len(label); i++ {
				if label[i] == '\\' && i+1 < len(label) {
					i++
				}
				decoded.WriteByte(label[i])
			}
			ref := &PromptReference{Label: html.UnescapeString(decoded.String()), Kind: text[match[4]:match[5]], ID: id}
			parts = append(parts, PromptPart{Text: ref.Label, Start: match[0], End: match[1], Reference: ref})
		}
		offset = match[1]
	}
	if offset < len(text) {
		parts = append(parts, PromptPart{Text: text[offset:], Start: offset, End: len(text)})
	}
	return parts
}

func PlainPrompt(text string) string {
	var out strings.Builder
	for _, part := range ParsePrompt(text) {
		if part.Reference != nil {
			out.WriteString(part.Reference.Value())
		} else {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

func DisplayPrompt(text string) string {
	var out strings.Builder
	for _, part := range ParsePrompt(text) {
		out.WriteString(part.Text)
	}
	return out.String()
}
