package desktop

import (
	"fmt"
	"strings"
	"unicode"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

type promptChoice struct {
	ref         contracts.PromptReference
	description string
}

// Document owns text, atomic references, selection and attributed undo.
// No UI Element or Context survives a frame.
type promptEditor struct {
	scope, raw     string
	doc            *richtext.Document
	state          richtext.EditorState
	start, end     int
	query, trigger string
	dismissed      string
	open           bool
	selected       int
	choices        []promptChoice
	list           ui.ListState
	panel          ui.Handle
}

func promptDisplay(raw string) string {
	return contracts.DisplayPrompt(raw)
}

func promptCanonical(text richtext.Text) string {
	var out strings.Builder
	for _, run := range text.Runs(0, text.Len()) {
		if parts := contracts.ParsePrompt(run.Attributes.Tag); len(parts) == 1 && parts[0].Reference != nil && run.Attributes.Attachment != nil {
			for _, char := range run.Text {
				if char == richtext.ObjectReplacement {
					out.WriteString(run.Attributes.Tag)
				} else {
					out.WriteRune(char)
				}
			}
		} else {
			out.WriteString(run.Text)
		}
	}
	return out.String()
}

func (a *App) promptAttachment(ref contracts.PromptReference) *richtext.Attachment {
	attachment := &richtext.Attachment{Name: ref.Label, URL: ref.Markdown(), Data: ref}
	a.stylePromptAttachment(attachment, ref)
	return attachment
}

func (a *App) stylePromptAttachment(attachment *richtext.Attachment, ref contracts.PromptReference) {
	size := float32(a.state.Settings.Appearance.FontSize)
	if size <= 0 {
		size = design.UIFontSize
	}
	font := ui.Font{Family: design.Font(a.state.Settings.Appearance.Font), Size: size, Weight: 500}
	layout := ui.ShapeText(ref.Label, font, 0)
	width, height := layout.Size()
	attachment.Width, attachment.Height = width+size+5, max(height, size)
	attachment.Draw = func(painter *ui.Painter, bounds ui.Rect) {
		if glyph := assets.Icon(promptGlyph(ref)); glyph != nil {
			painter.Icon(glyph, ui.Rect{X: bounds.X, Y: bounds.Y + (bounds.H-size)/2, W: size, H: size}, a.palette.Muted)
		}
		painter.TextLayout(layout, bounds.X+size+5, bounds.Y+(bounds.H-height)/2, a.palette.Muted)
	}
}

func (a *App) promptText(raw string) richtext.Text {
	text := richtext.NewText("")
	for _, part := range contracts.ParsePrompt(raw) {
		if part.Reference == nil {
			text = text.ReplaceString(text.Len(), text.Len(), part.Text, richtext.Attributes{})
		} else {
			text = text.ReplaceString(text.Len(), text.Len(), string(richtext.ObjectReplacement), richtext.Attributes{
				Tag: part.Reference.Markdown(), Attachment: a.promptAttachment(*part.Reference),
			})
		}
	}
	return text
}

func (a *App) syncPrompt(s *promptEditor, raw string) {
	text := a.promptText(raw)
	if s.doc == nil {
		s.doc = richtext.NewDocument(text)
	} else {
		s.doc.SetText(text)
	}
	s.raw = raw
	s.state = richtext.EditorState{}
}

func promptMeasureSpans(s *promptEditor, display string) []ui.Span {
	var objects []*richtext.Attachment
	for _, run := range s.doc.Text().Runs(0, s.doc.Text().Len()) {
		if run.Attributes.Attachment != nil {
			objects = append(objects, run.Attributes.Attachment)
		}
	}
	var spans []ui.Span
	var text strings.Builder
	index := 0
	flush := func() {
		if text.Len() > 0 {
			spans = append(spans, ui.Span{Text: text.String()})
			text.Reset()
		}
	}
	for _, char := range display {
		if char == richtext.ObjectReplacement && index < len(objects) {
			flush()
			attachment := objects[index]
			spans = append(spans, ui.Span{Text: string(char), ObjectWidth: attachment.Width, ObjectHeight: attachment.Height})
			index++
		} else {
			text.WriteRune(char)
		}
	}
	flush()
	return spans
}

func promptQuery(text string, caret int) (start int, trigger, query string, ok bool) {
	runes := []rune(text)
	if caret < 0 || caret > len(runes) {
		return
	}
	start = caret
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	if start == caret || runes[start] != '/' && runes[start] != '@' && runes[start] != '$' {
		return
	}
	trigger, query, ok = string(runes[start]), string(runes[start+1:caret]), true
	return
}

func (a *App) promptChoices(trigger, query string) []promptChoice {
	if trigger != "/" && trigger != "@" && trigger != "$" {
		return nil
	}
	var all []promptChoice
	add := func(kind, id, label, description string) {
		all = append(all, promptChoice{contracts.PromptReference{Kind: kind, ID: id, Label: label}, description})
	}
	if trigger == "/" {
		add("command", "init", "Init", a.t("Create workspace instructions"))
		add("command", "goal", "Goal", a.t("Start a goal"))
		add("command", "compact", "Compact", a.t("Compact conversation context"))
		for _, entry := range a.state.UserResources.Commands {
			if entry.Enabled && entry.Name != "init" && entry.Name != "goal" && entry.Name != "compact" {
				add("command", entry.Name, entry.Name, entry.Description)
			}
		}
	}
	for _, entry := range a.state.UserResources.Skills {
		if entry.Enabled {
			add("skill", entry.ID, entry.Name, entry.Description)
		}
	}
	if trigger == "@" {
		for _, entry := range a.state.UserResources.MCPServers {
			if entry.Enabled {
				add("mcp", entry.ID, entry.Name, "MCP")
			}
		}
	}
	query = strings.ToLower(query)
	var result []promptChoice
	for _, choice := range all {
		if strings.Contains(strings.ToLower(choice.ref.Label+" "+choice.ref.ID+" "+choice.description), query) {
			result = append(result, choice)
		}
	}
	return result
}

func (s *promptEditor) signature() string {
	return fmt.Sprintf("%s:%d:%d", s.raw, s.start, s.end)
}

func (a *App) completePrompt(s *promptEditor, choice promptChoice, draft *string) {
	end := s.end
	text := []rune(s.doc.Text().String())
	// Consume only the matching remainder of a partially typed candidate.
	// A bare trigger before existing prose must never delete that prose.
	if s.query != "" {
		query := strings.ToLower(s.query)
		for _, candidate := range []string{choice.ref.ID, choice.ref.Label} {
			name := []rune(candidate)
			if !strings.HasPrefix(strings.ToLower(candidate), query) {
				continue
			}
			tail := name[len([]rune(s.query)):]
			if len(tail) > 0 && end+len(tail) <= len(text) && strings.EqualFold(string(text[end:end+len(tail)]), string(tail)) {
				end += len(tail)
				break
			}
		}
	}
	s.doc.Edit(func(edit *richtext.Edit) {
		edit.ReplaceString(s.start, end, string(richtext.ObjectReplacement), richtext.Attributes{Tag: choice.ref.Markdown(), Attachment: a.promptAttachment(choice.ref)})
		edit.ReplaceString(s.start+1, s.start+1, " ", richtext.Attributes{})
		edit.Select(s.start+2, s.start+2)
	})
	s.raw = promptCanonical(s.doc.Text())
	*draft = s.raw
	s.open = false
	s.dismissed = s.signature()
}

func (a *App) preparePromptEditor(p composerSurfaceProps) *promptEditor {
	s := &p.controls.prompt
	scope := p.controls.sessionID
	if s.scope != scope {
		*s = promptEditor{scope: scope}
	}
	if s.raw != *p.draft || s.doc == nil {
		a.syncPrompt(s, *p.draft)
	}
	return s
}

func (a *App) promptInput(c *ui.Context, input ui.Element, p composerSurfaceProps, s *promptEditor) {
	s.raw = promptCanonical(s.doc.Text())
	*p.draft = s.raw
	for _, run := range s.doc.Text().Runs(0, s.doc.Text().Len()) {
		parts := contracts.ParsePrompt(run.Attributes.Tag)
		if len(parts) == 1 && parts[0].Reference != nil && run.Attributes.Attachment != nil {
			a.stylePromptAttachment(run.Attributes.Attachment, *parts[0].Reference)
		}
	}
	start, end := s.doc.Selection()
	oldStart, oldEnd, oldQuery := s.start, s.end, s.query
	var ok bool
	s.start, s.trigger, s.query, ok = promptQuery(s.doc.Text().String(), end)
	s.end = end
	s.choices = a.promptChoices(s.trigger, s.query)
	s.open = ok && !s.state.Composing && start == end && (p.input.FocusWithin(c) || s.panel.FocusWithin(c)) && s.dismissed != s.signature()
	if oldStart != s.start || oldEnd != s.end || oldQuery != s.query {
		s.selected = 0
	}
	if len(s.choices) > 0 {
		s.selected = min(s.selected, len(s.choices)-1)
	}
}

func (a *App) promptKey(p composerSurfaceProps, s *promptEditor, key ui.InputEvent, composing bool) bool {
	if composing {
		// IME confirmation belongs to the editor and must never send a draft.
		return key.Key == ui.KeyEnter
	}
	if s.open && key.Mods == 0 {
		switch key.Key {
		case ui.KeyUp, ui.KeyDown:
			if count := len(s.choices); count > 0 {
				delta := 1
				if key.Key == ui.KeyUp {
					delta = -1
				}
				s.selected = (s.selected + delta + count) % count
				s.list.ScrollIntoView(s.selected)
			}
			return true
		case ui.KeyEnter, ui.KeyTab:
			if len(s.choices) > 0 {
				a.completePrompt(s, s.choices[s.selected], p.draft)
			}
			return true
		case ui.KeyEscape:
			s.dismissed, s.open = s.signature(), false
			return true
		}
	}
	if key.Key == ui.KeyEscape && key.Mods == 0 && p.stopWithEscape && p.onStop != nil {
		p.onStop()
		return true
	}
	behavior := a.state.Settings.General.SendBehavior
	shift := behavior == "shiftEnter" || behavior == "Shift+Enter"
	mod := behavior == "modEnter" || behavior == "mod-enter"
	if key.Key == ui.KeyEnter && ((!shift && !mod && key.Mods == 0) || (shift && key.Mods == ui.Shift) || (mod && key.Mods == ui.Cmd)) {
		s.raw = promptCanonical(s.doc.Text())
		*p.draft = s.raw
		if p.onSend(s.raw) {
			a.syncPrompt(s, *p.draft)
		}
		return true
	}
	return false
}

func (a *App) promptPanel(c *ui.Context, anchor ui.Element, p composerSurfaceProps, s *promptEditor) {
	if !s.open {
		return
	}
	ui.PopoverBase(c, anchor, &s.open, func(panel ui.Element) {
		panel.Bind(&s.panel)
		budget := promptMenuHeight(c, panel, anchor)
		controlSelectPopup(c, panel).Key("prompt-picker").Width(min(float32(420), max(float32(160), anchor.Bounds().W))).MaxHeight(budget).
			AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).SnapToWindow(8)
		panel.Children(func() {
			if len(s.choices) == 0 {
				ui.Text(c, tr(c, "No matching resources")).Padding(8).TextColor(a.palette.Muted)
				return
			}
			s.list.Key = func(i int) any { return s.choices[i].ref.Kind + ":" + s.choices[i].ref.ID }
			ui.List(c, &s.list, len(s.choices), func(i int) {
				choice := s.choices[i]
				key := "prompt-choice:" + choice.ref.Kind + ":" + choice.ref.ID
				button := controlPromptChoice(c.Key(key), i == s.selected).FillWidth().Label(choice.ref.Label)
				button.Children(func() {
					icon(c, promptGlyph(choice.ref), 15)
					ui.Text(c, choice.ref.Label).Grow(1).MinWidth(0).SingleLine()
					if choice.description != "" {
						ui.Text(c, choice.description).MinWidth(0).Shrink(1).SingleLine().TextColor(a.palette.Muted)
					}
				})
				button.OnClick(func() {
					a.completePrompt(s, choice, p.draft)
					p.input.Focus(c)
				})
			}).MaxHeight(max(0, min(290, budget-10))).FillWidth()
		})
		if panel.Dismissed() {
			s.dismissed = s.signature()
		}
	})
}

func promptGlyph(ref contracts.PromptReference) string {
	if ref.Kind == "skill" {
		return "magic-stick-3"
	}
	if ref.Kind == "mcp" {
		return "remote-server"
	}
	if ref.ID == "goal" {
		return "checklist"
	}
	return "terminal"
}
