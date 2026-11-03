package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

const questionAutoAdvance = 220 * time.Millisecond

// The panel borrows the composer's place, never its message draft. Each pane
// keeps its own answer edits across navigation until the core resolves them.
type questionFormState struct {
	RequestID    string
	Page         int
	Picked       [][]bool
	Typed        []string
	Input        ui.Handle
	Root         ui.Handle
	AdvanceAt    time.Time
	Pending      bool
	Failure      string
	FocusPending bool
}

func (a *App) pendingQuestionRequest(sessionID string) *contracts.Question {
	// A stopped or failed run cannot accept answers. Its history remains
	// visible as a passive record without trapping the message composer.
	awaiting := false
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			awaiting = session.Status == contracts.AwaitingInput
			break
		}
	}
	if !awaiting {
		return nil
	}
	messages := a.state.Messages[sessionID]
	for i := len(messages) - 1; i >= 0; i-- {
		for j := len(messages[i].Blocks) - 1; j >= 0; j-- {
			q := messages[i].Blocks[j].Question
			if q != nil && !q.Resolved && len(q.Pages) > 0 {
				return q
			}
		}
	}
	return nil
}

func (a *App) questionForm(scope string, q *contracts.Question) *questionFormState {
	if a.questionForms == nil {
		a.questionForms = map[string]*questionFormState{}
	}
	state := a.questionForms[scope]
	if state == nil || state.RequestID != q.ID {
		state = &questionFormState{RequestID: q.ID, Typed: make([]string, len(q.Pages)), Picked: make([][]bool, len(q.Pages)), FocusPending: true}
		for i, page := range q.Pages {
			state.Picked[i] = make([]bool, len(page.Options))
		}
		a.questionForms[scope] = state
		// Existing menus belong to the message composer being replaced.
		a.modelMenu, a.effortMenu = false, false
	}
	return state
}

func (s *questionFormState) answers(q *contracts.Question) []contracts.QuestionAnswer {
	answers := make([]contracts.QuestionAnswer, len(q.Pages))
	for i, page := range q.Pages {
		if text := strings.TrimSpace(s.Typed[i]); text != "" {
			answers[i].Text = text
			continue
		}
		for j, option := range page.Options {
			if s.Picked[i][j] {
				answers[i].Options = append(answers[i].Options, option.Label)
			}
		}
	}
	return answers
}

func (a *App) advanceQuestion(q *contracts.Question, state *questionFormState, sessionID string) {
	state.AdvanceAt = time.Time{}
	if state.Pending {
		return
	}
	answer := state.answers(q)
	if err := contracts.ValidateQuestionAnswers(q.Pages[state.Page:state.Page+1], answer[state.Page:state.Page+1]); err != nil {
		return
	}
	if state.Page+1 < len(q.Pages) {
		state.Page++
		state.Failure = ""
		return
	}
	if err := contracts.ValidateQuestionAnswers(q.Pages, answer); err != nil {
		state.Failure = err.Error()
		return
	}
	for _, form := range a.questionForms {
		if form.RequestID == q.ID {
			form.Pending, form.Failure = true, ""
			form.AdvanceAt = time.Time{}
		}
	}
	a.dispatch(contracts.Command{Kind: "request.answer", SessionID: sessionID, ID: q.ID, QuestionAnswers: answer})
}

func (a *App) questionCommandFinished(cmd contracts.Command, err error) bool {
	if cmd.Kind != "request.answer" || len(cmd.QuestionAnswers) == 0 {
		return false
	}
	for _, form := range a.questionForms {
		if form.RequestID == cmd.ID && err != nil {
			form.Pending, form.Failure = false, err.Error()
		}
	}
	return true
}

func (a *App) questionComposer(c *ui.Context, sessionID, scope string) bool {
	q := a.pendingQuestionRequest(sessionID)
	if q == nil {
		if state := a.questionForms[scope]; state != nil {
			delete(a.questionForms, scope)
			if strings.HasPrefix(scope, "main:") {
				a.composer.Focus(c)
			} else if composer := a.sideComposers[sessionID]; composer != nil {
				composer.Input.Focus(c)
			}
		}
		return false
	}
	state := a.questionForm(scope, q)
	if !state.AdvanceAt.IsZero() {
		if c.Now().Before(state.AdvanceAt) {
			c.After(state.AdvanceAt.Sub(c.Now()))
		} else {
			a.advanceQuestion(q, state, sessionID)
		}
	}
	page := q.Pages[state.Page]
	selectOption := func(index int) {
		if state.Pending {
			return
		}
		state.Failure = ""
		if page.MultiSelect {
			state.Picked[state.Page][index] = !state.Picked[state.Page][index]
		} else {
			clear(state.Picked[state.Page])
			state.Picked[state.Page][index] = true
			state.AdvanceAt = c.Now().Add(questionAutoAdvance)
		}
	}
	panel := ui.Column(c.Key("question-panel")).Bind(&state.Root).Label(tr(c, "Question panel")).Role(ui.RoleGroup).
		Focusable().FocusRing(false).FillWidth().Shrink(0)
	if state.FocusPending {
		state.Root.Focus(c)
		state.FocusPending = false
	}
	panel = a.frostedComposer(c, panel, 22)
	// Keep the footer reachable when descriptions or previews are long.
	_, viewportHeight := c.Size()
	maxBody := max(float32(100), min(float32(420), viewportHeight*.6-70))
	panel.Padding(16).Gap(12).Children(func() {
		ui.Scroll(c.Key("question-body")).FillWidth().MaxHeight(maxBody).Children(func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, page.Header).Grow(1).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
					if len(q.Pages) > 1 {
						ui.Text(c, fmt.Sprintf("%d/%d", state.Page+1, len(q.Pages))).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
					}
				})
				ui.Text(c, page.Question).FontWeight(600).FontSize(design.TextSize(c, design.TextBase))
				for i, option := range page.Options {
					selected := state.Picked[state.Page][i] && strings.TrimSpace(state.Typed[state.Page]) == ""
					controlQuestionOption(c.Key(fmt.Sprintf("question-option-%s-%d-%d", q.ID, state.Page, i)), option, i+1, selected, page.MultiSelect).
						Disabled(state.Pending).OnClick(func() { selectOption(i) })
				}
				input := controlTextArea(c.Key(fmt.Sprintf("question-answer-%s-%d", q.ID, state.Page)), &state.Typed[state.Page]).
					Bind(&state.Input).FillWidth().Height(64).Label(tr(c, "Your answer")).
					Placeholder(tr(c, "Type your own answer…")).Disabled(state.Pending).
					OnChange(func() { state.AdvanceAt = time.Time{} })
				composing := input.Composing()
				input.OnTextKey(func(key ui.TextKey) bool {
					if key.Key != ui.KeyEnter || key.Mods != 0 && key.Mods != ui.Ctrl && key.Mods != ui.Cmd {
						return false
					}
					if !composing {
						state.Typed[state.Page] = key.Text()
						a.advanceQuestion(q, state, sessionID)
					}
					return true
				})
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if state.Page > 0 {
				textButton(c.Key("question-back-"+q.ID), tr(c, "Back")).Label(tr(c, "Previous question")).Disabled(state.Pending).OnClick(func() {
					state.AdvanceAt = time.Time{}
					state.Page--
					state.Failure = ""
				})
			}
			ui.Spacer(c).Grow(1)
			answer := state.answers(q)[state.Page]
			canAdvance := answer.Text != "" || len(answer.Options) > 0
			label := "Submit"
			if state.Page+1 < len(q.Pages) {
				label = "Next"
			}
			primaryButton(c.Key("question-submit-"+q.ID), tr(c, label)).Disabled(!canAdvance || state.Pending).
				OnClick(func() { a.advanceQuestion(q, state, sessionID) })
		})
		if state.Failure != "" {
			ui.Text(c, state.Failure).TextColor(a.palette.Danger).FontSize(design.TextSize(c, design.TextSmall))
		}
	})
	// Number keys choose only outside the text area; typing a number must
	// remain possible in a custom answer.
	if !state.Input.Focused(c) && !state.Pending {
		for i := range page.Options {
			state.Root.OnShortcut(c, 0, ui.Key1+ui.Key(i), func() { selectOption(i) })
		}
		for _, mods := range []ui.Modifiers{0, ui.Cmd} {
			state.Root.OnShortcut(c, mods, ui.KeyEnter, func() { a.advanceQuestion(q, state, sessionID) })
		}
	}
	return true
}
