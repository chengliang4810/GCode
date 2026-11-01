package linkui

import (
	"html"
	"strings"
	"unicode/utf8"

	i18nview "gcode/internal/i18n/view"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

type selectionKey struct{}

// Selection records source strings and rune ranges for the current build of
// one selectable scope. It never retains Elements or Contexts.
type Selection struct {
	paragraphs map[uint64]selectionParagraph
}

type selectionParagraph struct {
	display, source string
	replacements    []selectionReplacement
}

type selectionReplacement struct {
	start, end int
	source     string
}

// BeginSelection attaches copying to an existing shared selectable scope. Pass
// its result in Config.Selection for every Block rendered in that scope. Calling
// it each build discards removed paragraphs while keeping cross-paragraph native
// selection and focus in the original scope.
func BeginSelection(scope ui.Element) *Selection {
	s := ui.Local(scope, selectionKey{}, func() Selection { return Selection{} })
	// A list builds only the rows near its view, so a selection that reaches
	// across more keeps the sources of the paragraphs it holds whose rows the
	// list no longer builds: copying it still yields their Markdown.
	previous := s.paragraphs
	s.paragraphs = make(map[uint64]selectionParagraph, len(previous))
	for _, part := range scope.SelectedTexts() {
		if p, ok := previous[part.ID]; ok {
			s.paragraphs[part.ID] = p
		}
	}
	scope.CopyData(s.CopyData)
	return s
}

// Menu adds the source-aware Markdown copy command after the scope's children
// have registered their paragraphs. Plain Copy continues using CopyData.
func (s *Selection) Menu(c *ui.Context, scope ui.Element) {
	services := c.Services()
	label := i18nview.T(c, "Copy as Markdown")
	copyMarkdown := func() {
		data := s.CopyData(scope.SelectedTexts())
		if source, err := data.Read(transfer.Markdown); err == nil {
			services.WriteClipboardData(transfer.New(transfer.NewItem(
				transfer.Bytes(transfer.Text, source), transfer.Bytes(transfer.Markdown, source))))
		}
	}
	scope.ContextMenu(func(menu *ui.Menu) {
		menu.EditItems()
		if menu.Item(label).Chosen() {
			copyMarkdown()
		}
	})
	scope.OnShortcut(ui.Cmd|ui.Shift, ui.KeyC, copyMarkdown)
}

func (s *Selection) register(id uint64, display, source string, replacements []selectionReplacement) {
	if s == nil {
		return
	}
	s.paragraphs[id] = selectionParagraph{
		display: display, source: source,
		replacements: append([]selectionReplacement(nil), replacements...),
	}
}

// CopyData preserves visible Markdown labels. A fully selected automatic file
// label copies its original path; selecting only part of that label copies that
// part. Whole custom paragraphs retain their Markdown source. Partial selections
// use plain text as Markdown because no exact parser offset map is available.
func (s *Selection) CopyData(parts []ui.SelectedText) transfer.Data {
	var text, markdown []string
	for _, part := range parts {
		selected := runeSlice(part.Text, part.Start, part.End)
		source := selected
		if p, ok := s.paragraphs[part.ID]; ok && part.Text == p.display {
			selected = p.copyText(part.Start, part.End)
			source = selected
			if part.Start == 0 && part.End >= utf8.RuneCountInString(p.display) {
				source = strings.TrimRight(p.source, "\r\n")
			}
		}
		text = append(text, selected)
		markdown = append(markdown, source)
	}
	plain, md := strings.Join(text, "\n"), strings.Join(markdown, "\n\n")
	return transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte(plain)),
		transfer.Bytes(transfer.Markdown, []byte(md)),
		transfer.Bytes(transfer.HTML, []byte("<p>"+strings.ReplaceAll(html.EscapeString(plain), "\n", "<br>")+"</p>")),
	))
}

func (p selectionParagraph) copyText(start, end int) string {
	var result strings.Builder
	at := start
	for _, replacement := range p.replacements {
		if replacement.start < start || replacement.end > end {
			continue
		}
		result.WriteString(runeSlice(p.display, at, replacement.start))
		result.WriteString(replacement.source)
		at = replacement.end
	}
	result.WriteString(runeSlice(p.display, at, end))
	return result.String()
}

func runeSlice(text string, start, end int) string {
	runes := []rune(text)
	start = min(max(0, start), len(runes))
	end = min(max(start, end), len(runes))
	return string(runes[start:end])
}
