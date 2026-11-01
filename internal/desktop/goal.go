package desktop

import (
	"fmt"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

type goalViewState struct {
	Expanded bool
}

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

func (a *App) goalBanner(c *ui.Context, session contracts.Session) {
	goal := session.Goal
	if goal == nil || session.SubagentType != "" {
		return
	}
	c.Key("goal:" + session.ID + ":" + goal.ID)
	panel := ui.Column(c).FillWidth().Shrink(0).Padding(8, 16).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border)
	view := ui.Local(panel, "details", func() goalViewState { return goalViewState{} })
	panel.Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			label, glyph := "Show goal details", "alt-arrow-down"
			if view.Expanded {
				label, glyph = "Hide goal details", "alt-arrow-up"
			}
			ui.ButtonBase(c.Key("goal-details:"+session.ID)).Label(tr(c, label)).Grow(1).MinWidth(0).Padding(4, 0).Gap(8).AlignItems(ui.Center).Children(func() {
				icon(c, glyph, 12)
				ui.Text(c, goal.Objective).Grow(1).MinWidth(0).SingleLine().FontSize(design.TextSize(c, design.TextBase))
			}).OnClick(func() { view.Expanded = !view.Expanded })
			ui.Text(c, tr(c, goalStatusLabel(goal.Status))).Shrink(0).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			ui.Text(c, fmt.Sprintf(tr(c, "Round %d"), goal.Iteration)).Shrink(0).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			switch goal.Status {
			case contracts.GoalActive, contracts.GoalVerifying:
				ui.ButtonBase(c).Label(tr(c, "Pause goal")).Tooltip(tr(c, "Pause goal")).Size(24, 24).Center().Children(func() {
					ui.Row(c).Gap(3).Children(func() {
						ui.Box(c).Size(3, 12).Background(a.palette.Text)
						ui.Box(c).Size(3, 12).Background(a.palette.Text)
					})
				}).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "goal.pause", SessionID: session.ID, ID: goal.ID})
				})
			case contracts.GoalPaused:
				iconButton(c, "action-play", tr(c, "Resume goal")).Size(24, 24).Disabled(session.Status == contracts.Working || session.Status == contracts.AwaitingInput).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "goal.resume", SessionID: session.ID, ID: goal.ID})
				})
			}
		})
		if !view.Expanded {
			return
		}
		ui.Scroll(c).FillWidth().MaxHeight(240).Gap(8).Children(func() {
			ui.Text(c, goal.Objective).FillWidth().FontSize(design.TextSize(c, design.TextBase))
			ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
				ui.Text(c, a.goalUsageText(goal)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				elapsed := goalElapsed(goal, a.now())
				ui.Text(c, a.t("Worked for ")+i18n.Duration(a.state.Settings.General.Language, elapsed)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			})
			if goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying {
				c.After(time.Second)
			}
			if goal.Reason != "" {
				ui.Text(c, goal.Reason).FillWidth().FontSize(design.TextSize(c, design.TextBase))
			}
			if goal.NextAction != "" {
				ui.Text(c, a.t("Next: ")+goal.NextAction).FillWidth().FontSize(design.TextSize(c, design.TextBase))
			}
			if len(goal.Verifications) > 0 {
				last := goal.Verifications[len(goal.Verifications)-1]
				result := "Needs more work"
				if last.Passed {
					result = "Passed"
				}
				ui.Text(c, fmt.Sprintf(tr(c, "Verification · Round %d · %s"), last.Iteration, tr(c, result))).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				if last.Reason != "" && last.Reason != goal.Reason {
					ui.Text(c, last.Reason).FillWidth().FontSize(design.TextSize(c, design.TextBase))
				}
				if last.NextAction != "" && last.NextAction != goal.NextAction {
					ui.Text(c, a.t("Next: ")+last.NextAction).FillWidth().FontSize(design.TextSize(c, design.TextBase))
				}
			}
			if goal.Status != contracts.GoalActive && goal.Status != contracts.GoalVerifying {
				textButton(c, tr(c, "Clear goal")).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "goal.clear", SessionID: session.ID, ID: goal.ID})
				})
			}
		})
	})
}
