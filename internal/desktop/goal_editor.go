package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type goalEditorState struct {
	input                            ui.Handle
	focusPending                     bool
	goalID, original, draft, failure string
	revision                         uint64
	busy                             bool
}

func (a *App) sessionGoal(sessionID string) *contracts.SessionGoal {
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			return session.Goal
		}
	}
	return nil
}

func (a *App) goalEditor(sessionID string) *goalEditorState {
	if a.goalEditors == nil {
		a.goalEditors = make(map[string]*goalEditorState)
	}
	state := a.goalEditors[sessionID]
	if state == nil {
		state = &goalEditorState{}
		a.goalEditors[sessionID] = state
	}
	if goal := a.sessionGoal(sessionID); goal != nil && !state.busy && state.draft == state.original {
		state.goalID, state.original, state.draft = goal.ID, goal.Objective, goal.Objective
		state.revision = goal.Revision
	}
	return state
}

func (a *App) openGoalEditor(sessionID string) {
	if a.sessionGoal(sessionID) == nil {
		return
	}
	a.goalEditor(sessionID).focusPending = true
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			a.workspace.OpenGoal(sessionID, session.Title)
			return
		}
	}
}

func (a *App) saveGoalEditor(sessionID string, state *goalEditorState) {
	if state.busy || state.draft == state.original {
		return
	}
	edit := contracts.GoalEditInput{Objective: strings.TrimSpace(state.draft), ExpectedRevision: state.revision}
	if err := edit.Validate(); err != nil {
		state.failure = a.t("The goal must contain 1 to 4000 characters.")
		return
	}
	command := contracts.Command{Kind: "goal.edit", SessionID: sessionID, ID: state.goalID, GoalEdit: &edit}
	submitted := state.draft
	state.busy, state.failure = true, ""
	a.backgroundTask(func() func() {
		err := a.service.Execute(a.ctx, command)
		snapshot, snapshotErr := a.service.Snapshot(a.ctx)
		return func() {
			state.busy = false
			if err != nil {
				state.failure = err.Error()
				return
			}
			if snapshotErr != nil {
				state.failure = snapshotErr.Error()
				return
			}
			state.original = edit.Objective
			state.revision = edit.ExpectedRevision + 1
			if state.draft == submitted {
				state.draft = edit.Objective
			}
			a.apply(snapshot)
		}
	})
}

func (a *App) goalEditorView(c *ui.Context, sessionID string) {
	state := a.goalEditor(sessionID)
	goal := a.sessionGoal(sessionID)
	c.OnShortcut(ui.Cmd, ui.KeyS, func() {
		if goal != nil {
			a.saveGoalEditor(sessionID, state)
		}
	})
	ui.Column(c.Key("goal-editor:" + sessionID)).Fill().Children(func() {
		ui.Row(c).FillWidth().Padding(8, 12).Gap(8).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			textButton(c.Key("goal-editor-reload:"+sessionID), tr(c, "Reload goal")).Disabled(goal == nil || state.busy).OnClick(func() {
				if current := a.sessionGoal(sessionID); current != nil {
					state.goalID, state.original, state.draft, state.failure = current.ID, current.Objective, current.Objective, ""
					state.revision = current.Revision
				}
			})
			saveLabel := "Save goal"
			if state.busy {
				saveLabel = "Saving…"
			}
			primaryButton(c.Key("goal-editor-save:"+sessionID), tr(c, saveLabel)).Label(tr(c, "Save goal")).
				Disabled(goal == nil || state.busy || state.draft == state.original).
				OnClick(func() { a.saveGoalEditor(sessionID, state) })
		})
		ui.Scroll(c).Grow(1).FillWidth().Padding(16).Children(func() {
			controlTextArea(c.Key("goal-editor-input:"+sessionID), &state.draft).Bind(&state.input).FillWidth().Height(280).Label(tr(c, "Goal objective"))
			if state.focusPending {
				state.input.Focus(c)
				state.focusPending = false
			}
			if state.failure != "" {
				ui.Text(c, state.failure).FillWidth().Margin(10, 0).TextColor(a.palette.Danger)
			}
			if goal == nil {
				ui.Text(c, tr(c, "This conversation no longer has a goal.")).Margin(10, 0)
				return
			}
			// Keep the existing usage and verification information available
			// without adding it to the reference's compact attached header.
			ui.Column(c).FillWidth().Margin(16, 0).Gap(8).Children(func() {
				ui.Text(c, tr(c, goalStatusLabel(goal.Status))+" · "+fmt.Sprintf(tr(c, "Round %d"), goal.Iteration)).
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				ui.Text(c, a.goalUsageText(goal)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				if goal.Reason != "" {
					ui.Text(c, goal.Reason).FillWidth()
				}
				if goal.NextAction != "" {
					ui.Text(c, a.t("Next: ")+goal.NextAction).FillWidth()
				}
				if len(goal.Verifications) > 0 {
					last := goal.Verifications[len(goal.Verifications)-1]
					result := "Needs more work"
					if last.Passed {
						result = "Passed"
					}
					ui.Text(c, fmt.Sprintf(tr(c, "Verification · Round %d · %s"), last.Iteration, tr(c, result))).TextColor(a.palette.Muted)
					if last.Reason != "" && last.Reason != goal.Reason {
						ui.Text(c, last.Reason).FillWidth()
					}
					if last.NextAction != "" && last.NextAction != goal.NextAction {
						ui.Text(c, a.t("Next: ")+last.NextAction).FillWidth()
					}
				}
			})
		})
	})
}
