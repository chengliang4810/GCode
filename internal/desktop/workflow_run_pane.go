package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowRunView draws the run detail tab: a vertical spine of the run's
// phases, the questions it escalated, the holes still waiting, and the
// artifacts it delivered.
func (a *App) workflowRunView(c *ui.Context, runID string) {
	run, ok := a.workflowRun(runID)
	if !ok {
		ui.Column(c.Key("workflow-run:" + runID)).Fill().Padding(16).Children(func() {
			ui.Text(c, tr(c, "This run is no longer available.")).TextColor(a.palette.Muted).Wrap()
		})
		return
	}
	view := workflow.ProjectRun(run)
	p := a.palette
	ui.Column(c.Key("workflow-run:" + runID)).Label(run.Name).Fill().Gap(12).Padding(16).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, run.Name).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextLarge)).FontWeight(600).SingleLine()
			// 保存/再次运行、恢复、配置、停止走与运行卡同一个控制器：
			// 同一份运行不会一个面上写「保存」、另一个面上写「再次运行」。
			a.workflowRunControls(c, view, "workflow-pane")
			iconButton(c.Key("workflow-run-open-session:"+runID), "chat-round-line", tr(c, "Open conversation")).Disabled(run.SessionID == "").
				OnClick(func() { a.selectSession(run.SessionID) })
			// 从这一次运行能看到同类：这条对话里所有的工作流运行。
			iconButton(c.Key("workflow-run-open-directory:"+runID), "checklist", tr(c, "All runs in this conversation")).Disabled(run.SessionID == "").
				OnClick(func() { a.openWorkflowRuns(run.SessionID) })
		})
		a.workflowRunStatusLine(c, view)
		ui.Scroll(c.Key("workflow-run-scroll:" + runID)).FillWidth().Grow(1).Gap(16).Children(func() {
			a.workflowRunPhases(c, view)
			a.workflowRunQuestions(c, view)
			a.workflowRunHoles(c, view)
			a.workflowStepsSection(c, runID)
			a.workflowRunArtifacts(c, view)
			if run.Error != "" {
				a.workflowRunError(c, run)
			}
		})
	})
	_ = p
}

// workflowRunStatusLine draws the state word, its reason, the counts, and
// how long the run has been going.
func (a *App) workflowRunStatusLine(c *ui.Context, view workflow.RunView) {
	parts := []string{a.workflowRunKindWord(c, view)}
	if reason := a.workflowRunStopReason(view); reason != "" {
		parts = append(parts, reason)
	}
	parts = append(parts, tr(c, "%d phases", len(view.Phases)))
	if view.Active() {
		parts = append(parts, tr(c, "%d agents working", view.AgentWorking()))
	} else {
		parts = append(parts, tr(c, "%d agents", view.AgentTotal()))
	}
	parts = append(parts, tr(c, "%d concurrent", view.Concurrency))
	if view.ModelID != "" {
		parts = append(parts, a.workflowModelLabel(view.ModelID))
	}
	ui.Row(c.Key("workflow-run-status:" + view.RunID)).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		for _, part := range parts {
			ui.Text(c, part).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
		}
		ui.Text(c, a.workflowElapsed(view)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Muted).FontFeatures("tnum")
	})
}

// workflowRunPhases draws the vertical spine of the run's phases.
func (a *App) workflowRunPhases(c *ui.Context, view workflow.RunView) {
	ui.Column(c.Key("workflow-run-phases:" + view.RunID)).FillWidth().Gap(8).Children(func() {
		for _, phase := range view.Phases {
			ui.Column(c.Key("workflow-run-phase:" + view.RunID + ":" + phase.ID)).FillWidth().Gap(6).Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					lamp := a.workflowPhaseLampColor(phase.Status)
					ui.Box(c).Size(8, 8).Shrink(0).Draw(func(painter *ui.Painter, bounds ui.Rect) {
						if phase.Status == contracts.WorkflowPhasePending {
							painter.StrokePath((&ui.Path{}).Circle(bounds.X+bounds.W/2, bounds.Y+bounds.H/2, 4), 1, a.palette.Border)
							return
						}
						painter.FillPath((&ui.Path{}).Circle(bounds.X+bounds.W/2, bounds.Y+bounds.H/2, 4), lamp)
					})
					ui.Text(c, phase.Name).Grow(1).MinWidth(0).FontWeight(500).SingleLine()
					ui.Text(c, a.workflowPhaseCountLabel(c, phase)).FontSize(design.TextSize(c, design.TextSmall)).
						TextColor(a.palette.Muted).FontFeatures("tnum").SingleLine()
				})
				if len(phase.Actors) > 0 {
					ui.Column(c).FillWidth().Padding(20, 0, 0, 0).Gap(6).Children(func() {
						for _, actor := range phase.Actors {
							a.workflowRunActorRow(c, view, actor)
						}
					})
				}
			})
		}
	})
}

// workflowPhaseCountLabel counts what the phase actually observed and how
// many of them settled. It is not the static definition total.
func (a *App) workflowPhaseCountLabel(c *ui.Context, phase workflow.RunPhaseView) string {
	if phase.Observed == 0 {
		return ""
	}
	return tr(c, "%d/%d settled", phase.Settled, phase.Observed)
}

// workflowRunActorRow draws one instance row of the detail spine.
func (a *App) workflowRunActorRow(c *ui.Context, view workflow.RunView, actor workflow.RunActorView) {
	label := actor.Name
	if label == "" {
		label = actor.ID
	}
	ui.Row(c.Key("workflow-run-actor:" + view.RunID + ":" + actor.ID)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
		ui.Text(c, tr(c, a.workflowActorStatusWord(actor.Status))).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.workflowActorDotColor(actor.Status)).SingleLine()
		if actor.SessionID != "" {
			iconButton(c.Key("workflow-run-actor-open:"+view.RunID+":"+actor.ID), "chat-round-line", tr(c, "Open agent conversation")).
				OnClick(func() { a.workspace.OpenSession(actor.SessionID) })
		}
	})
}

// workflowActorStatusWord names an instance's state.
func (a *App) workflowActorStatusWord(status string) string {
	switch status {
	case contracts.WorkflowActorRunning:
		return "Running"
	case contracts.WorkflowActorAsking:
		return "Waiting for an answer"
	case contracts.WorkflowActorDone:
		return "Completed"
	case contracts.WorkflowActorFailed:
		return "Failed"
	default:
		return "Pending"
	}
}

// workflowRunQuestions draws the questions an instance escalated to the main
// agent. The answer is never asked for here: the main agent answers them.
func (a *App) workflowRunQuestions(c *ui.Context, view workflow.RunView) {
	if len(view.Questions) == 0 {
		return
	}
	ui.Column(c.Key("workflow-run-questions:" + view.RunID)).FillWidth().Gap(8).Children(func() {
		ui.Text(c, tr(c, "Questions for the agent")).FontWeight(600)
		for _, question := range view.Questions {
			ui.Column(c.Key("workflow-run-question:" + view.RunID + ":" + question.ID)).FillWidth().Gap(4).
				Padding(10).Radius(8).Background(a.palette.Raised).Children(func() {
				ui.Text(c, question.Text).FillWidth().Wrap()
				if question.Answer != "" {
					ui.Text(c, question.Answer).FillWidth().Wrap().TextColor(a.palette.Muted)
				}
			})
		}
	})
}

// workflowRunHoles draws the holes of the run: the ones still waiting for
// their fill first, then the ones already filled. A filled hole keeps its
// title and its text: the fill extended this same run.
func (a *App) workflowRunHoles(c *ui.Context, view workflow.RunView) {
	var waiting, filled []contracts.WorkflowHole
	for _, hole := range view.Holes {
		switch hole.Status {
		case contracts.WorkflowHoleWaiting:
			waiting = append(waiting, hole)
		case contracts.WorkflowHoleFilled:
			filled = append(filled, hole)
		}
	}
	if len(waiting) > 0 {
		a.workflowRunHoleGroup(c, view, "workflow-run-holes:"+view.RunID, tr(c, "Holes waiting to be filled"), waiting, true)
	}
	if len(filled) > 0 {
		a.workflowRunHoleGroup(c, view, "workflow-run-filled-holes:"+view.RunID, tr(c, "Filled holes"), filled, false)
	}
}

// workflowRunHoleGroup draws one group of holes under its heading.
func (a *App) workflowRunHoleGroup(c *ui.Context, view workflow.RunView, key, heading string, holes []contracts.WorkflowHole, waiting bool) {
	ui.Column(c.Key(key)).FillWidth().Gap(8).Children(func() {
		ui.Text(c, heading).FontWeight(600)
		for _, hole := range holes {
			ui.Column(c.Key("workflow-run-hole:" + view.RunID + ":" + hole.ID)).FillWidth().Gap(4).
				Padding(10).Radius(8).Background(a.palette.Raised).Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					title := hole.Title
					if title == "" {
						title = hole.Text
					}
					ui.Text(c, title).Grow(1).MinWidth(0).Wrap()
					if waiting {
						ui.Text(c, tr(c, "Waiting for fill")).
							FontSize(design.TextSize(c, design.TextSmall)).
							TextColor(a.palette.Readable(a.palette.Warning)).SingleLine()
						return
					}
					icon(c, "pen", design.Controls().IconSize)
				})
				if !waiting && hole.Text != "" {
					ui.Text(c, hole.Text).FillWidth().Wrap().TextColor(a.palette.Muted)
				}
			})
		}
	})
}

// workflowRunArtifacts draws the run's delivered artifacts and their content.
func (a *App) workflowRunArtifacts(c *ui.Context, view workflow.RunView) {
	if len(view.Artifacts.Shown) == 0 {
		return
	}
	ui.Column(c.Key("workflow-run-artifacts:" + view.RunID)).FillWidth().Gap(8).Children(func() {
		ui.Text(c, tr(c, "Artifacts")).FontWeight(600)
		for _, artifact := range view.Artifacts.Shown {
			ui.Column(c.Key("workflow-run-artifact:" + view.RunID + ":" + artifact.ID)).FillWidth().Gap(6).Children(func() {
				ui.Text(c, artifact.Name).FontWeight(500).SingleLine()
				if artifact.Content != "" {
					ui.Text(c, artifact.Content).FillWidth().Selectable().
						Font(a.state.Settings.Appearance.CodeFont).
						FontSize(design.TextSize(c, design.TextSmall)).
						Padding(6, 8).Background(a.palette.CodeWash).Radius(4)
				}
			})
		}
		if view.Artifacts.Overflow > 0 {
			ui.Text(c, tr(c, "and %d more", view.Artifacts.Overflow)).TextColor(a.palette.Muted)
		}
	})
}

// workflowRunError draws the run's recorded error, never invented.
func (a *App) workflowRunError(c *ui.Context, run contracts.WorkflowRun) {
	ui.Column(c.Key("workflow-run-error:" + run.ID)).FillWidth().Gap(6).Children(func() {
		ui.Text(c, tr(c, "Error")).FontWeight(600)
		ui.Text(c, run.Error).FillWidth().Wrap().Selectable().TextColor(a.palette.Readable(a.palette.Danger))
	})
}
