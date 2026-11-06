package desktop

import (
	"fmt"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

func goalStatusLabel(status contracts.GoalStatus) string {
	switch status {
	case contracts.GoalActive:
		return "In progress"
	case contracts.GoalVerifying:
		return "Verifying"
	case contracts.GoalPaused:
		return "Paused"
	case contracts.GoalBudgetLimited:
		return "Budget reached"
	case contracts.GoalComplete:
		return "Complete"
	default:
		return string(status)
	}
}

func goalHeaderLabel(status contracts.GoalStatus) string {
	switch status {
	case contracts.GoalActive:
		return "Active goal"
	case contracts.GoalVerifying:
		return "Verifying goal"
	case contracts.GoalPaused:
		return "Paused goal"
	case contracts.GoalBudgetLimited:
		return "Goal budget reached"
	case contracts.GoalComplete:
		return "Completed goal"
	default:
		return string(status)
	}
}

func goalDuration(language string, milliseconds int64) string {
	seconds := max(milliseconds/1000, 0)
	switch {
	case seconds < 60:
		return i18n.In(language, "%ds", seconds)
	case seconds < 3600:
		return i18n.In(language, "%dm", seconds/60)
	case seconds < 86400:
		return i18n.In(language, "%dh", seconds/3600)
	default:
		return i18n.In(language, "%dd", seconds/86400)
	}
}

func goalElapsed(goal *contracts.SessionGoal, now time.Time) int64 {
	elapsed := max(int64(0), goal.TimeUsedMS)
	if (goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying) && goal.ActiveSince != nil {
		elapsed += max(int64(0), now.Sub(*goal.ActiveSince).Milliseconds())
	}
	return elapsed
}

func (a *App) goalUsageText(goal *contracts.SessionGoal) string {
	used := footerCount(uint64(max(int64(0), goal.TokensUsed)))
	if goal.UnknownUsageCount > 0 {
		if goal.TokenBudget != nil {
			return i18n.In(a.state.Settings.General.Language, "At least %s / %s tokens · Usage incomplete", used, footerCount(uint64(max(int64(0), *goal.TokenBudget))))
		}
		return i18n.In(a.state.Settings.General.Language, "At least %s tokens · Usage incomplete", used)
	}
	if goal.TokenBudget != nil {
		return i18n.In(a.state.Settings.General.Language, "%s / %s tokens", used, footerCount(uint64(max(int64(0), *goal.TokenBudget))))
	}
	return i18n.In(a.state.Settings.General.Language, "%s tokens used", used)
}

func (a *App) updateGoal(command contracts.Command) {
	if a.closed || a.goalActions[command.SessionID] {
		return
	}
	if a.goalActions == nil {
		a.goalActions = make(map[string]bool)
	}
	a.goalActions[command.SessionID] = true
	scope, generation := a.captureSessionScope()
	a.backgroundTask(func() func() {
		err := a.service.Execute(a.ctx, command)
		snapshot, snapshotErr := a.service.Snapshot(a.ctx, scope...)
		return func() {
			delete(a.goalActions, command.SessionID)
			if snapshotErr == nil && a.sessionGenerationCurrent(generation, scope) {
				a.apply(snapshot)
			} else if snapshotErr != nil && err == nil {
				err = snapshotErr
			}
			if err != nil {
				if a.sessionID == command.SessionID {
					a.errorText = err.Error()
				}
				if side := a.sideComposers[command.SessionID]; side != nil {
					side.Failure = err.Error()
				}
			}
		}
	})
}

func (a *App) goalBanner(c *ui.Context, session contracts.Session) {
	goal := session.Goal
	if goal == nil || session.SubagentType != "" {
		return
	}
	s := design.AttachedPanels()
	a.attachedComposerPanel(c, "composer-goal:"+session.ID, tr(c, "Goal"), s.Inset, false, func(c *ui.Context) {
		row := ui.Row(c).FillWidth().MinHeight(s.Height-s.Overlap-2).Padding(0, s.HeaderPadding).Gap(s.Gap).AlignItems(ui.Center)
		if goal.Status == contracts.GoalPaused && goal.Reason != "" {
			row.Tooltip(goal.Reason)
		}
		row.Children(func() {
			icon(c, "goal", 13)
			ui.Text(c, tr(c, goalHeaderLabel(goal.Status))).Shrink(0).SingleLine().FontSize(design.TextSize(c, design.TextSmall)).FontWeight(500)
			ui.Text(c.Key("goal-objective:"+session.ID), goal.Objective).Grow(1).MinWidth(0).SingleLine().FontSize(design.TextSize(c, design.TextSmall)).Tooltip(goal.Objective + " · " + fmt.Sprintf(tr(c, "Round %d"), goal.Iteration))
			ui.Text(c.Key("goal-duration:"+session.ID), "· "+goalDuration(a.state.Settings.General.Language, goalElapsed(goal, a.now()))).Shrink(0).SingleLine().FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			pending := a.goalActions[session.ID]
			inlineIconButton(c.Key("goal-clear:"+session.ID), "trash-bin-minimalistic", tr(c, "Clear goal")).Disabled(pending).OnClick(func() { a.updateGoal(contracts.Command{Kind: "goal.clear", SessionID: session.ID, ID: goal.ID}) })
			switch goal.Status {
			case contracts.GoalActive, contracts.GoalVerifying:
				inlineIconButton(c.Key("goal-toggle:"+session.ID), "stop", tr(c, "Pause goal")).Disabled(pending).OnClick(func() { a.updateGoal(contracts.Command{Kind: "goal.pause", SessionID: session.ID, ID: goal.ID}) })
			case contracts.GoalPaused:
				inlineIconButton(c.Key("goal-toggle:"+session.ID), "action-play", tr(c, "Resume goal")).Disabled(pending || session.Status == contracts.Working || session.Status == contracts.AwaitingInput).OnClick(func() { a.updateGoal(contracts.Command{Kind: "goal.resume", SessionID: session.ID, ID: goal.ID}) })
			}
			inlineIconButton(c.Key("goal-edit:"+session.ID), "expand-arrows", tr(c, "Edit goal")).OnClick(func() { a.openGoalEditor(session.ID) })
		})
		if goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying {
			c.After(time.Second)
		}
	})
}
