package desktop

import (
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

// Keep a stable list while browsing: a running conversation can append messages
// between key presses. A document edit starts a fresh browse from that draft.
type promptHistory struct {
	entries []string // newest first
	next    int
	version uint64
	draft   richtext.Text
	caret   int
}

func userPromptHistory(messages []contracts.Message) []string {
	var entries []string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		var parts []string
		for _, block := range messages[i].Blocks {
			if block.Kind == "text" {
				parts = append(parts, block.Text)
			}
		}
		if text := strings.Join(parts, "\n"); strings.TrimSpace(text) != "" {
			entries = append(entries, text)
		}
	}
	return entries
}

func (a *App) browsePromptHistory(s *promptEditor, draft *string, key ui.Key, font ui.Font, textWidth float32) bool {
	if s.scope == "" {
		return false
	}
	h := &s.history
	if h.version != s.doc.Version() {
		*h = promptHistory{}
	}
	anchor, caret := s.doc.Selection()
	if anchor != caret {
		return false
	}
	current := s.doc.Text()
	if h.next == 0 {
		if key != ui.KeyUp {
			return false
		}
		// A single visible line (or the start of a multiline draft) has no
		// line above it. Measure the current text: the last paint may still
		// describe a different draft when keys arrive before a redraw.
		if caret != 0 {
			text := current.String()
			if composerHasNewline(text) || (textWidth > 0 && len(ui.ShapeRichText(promptMeasureSpans(s, text), font, textWidth).Lines()) > 1) {
				return false
			}
		}
		h.entries = userPromptHistory(a.state.Messages[s.scope])
		if len(h.entries) == 0 {
			return false
		}
		h.draft, h.caret = current, caret
	} else if caret != current.Len() && (key != ui.KeyUp || caret != 0) {
		return false
	}

	var text richtext.Text
	if key == ui.KeyUp {
		if h.next == len(h.entries) {
			return true
		}
		h.next++
	} else {
		h.next--
	}
	if h.next == 0 {
		text, caret = h.draft, h.caret
		*h = promptHistory{}
	} else {
		text = a.promptText(h.entries[h.next-1])
		caret = text.Len()
	}
	s.doc.Edit(func(edit *richtext.Edit) {
		edit.Replace(0, current.Len(), text)
		edit.Select(caret, caret)
	})
	h.version = s.doc.Version()
	s.raw = promptCanonical(s.doc.Text())
	*draft = s.raw
	// Recalled literal triggers should not take the next arrow as a picker key.
	s.start, _, _, _ = promptQuery(s.doc.Text().String(), caret)
	s.end = caret
	s.dismissed, s.open = s.signature(), false
	return true
}
