package desktop

import (
	"fmt"
	"strings"

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
	ui.Column(c.Key("workflow-run:" + runID)).Label(run.Name).Fill().MinWidth(0).Gap(0).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(12, 16).Gap(4).Children(func() {
			a.workflowRunPaneHeader(c, run, view)
			a.workflowRunStatusLine(c, run, view)
			if run.PreviousRunID != "" {
				textButton(c.Key("workflow-previous:"+run.ID), tr(c, "Previous run")).
					OnClick(func() { a.openWorkflowRun(run.PreviousRunID) })
			}
			for _, successor := range a.state.WorkflowRuns {
				if successor.PreviousRunID == run.ID {
					textButton(c.Key("workflow-successor:"+run.ID), tr(c, "Open restarted run")).
						OnClick(func() { a.openWorkflowRun(successor.ID) })
					break
				}
			}
		})
		ui.Divider(c)
		ui.Scroll(c.Key("workflow-run-scroll:" + runID)).FillWidth().MinWidth(0).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(12, 16).Gap(12).Children(func() {
				if run.Error != "" {
					a.workflowRunError(c, run)
				} else if strings.TrimSpace(run.Result) != "" {
					ui.Column(c.Key("workflow-run-result:" + runID)).FillWidth().Gap(4).Children(func() {
						ui.Text(c, tr(c, "Result")).FontWeight(600)
						ui.Text(c, run.Result).FillWidth().Wrap().Selectable()
					})
				}
				a.workflowRunPhases(c, view)
				a.workflowRunQuestions(c, view)
				a.workflowRunHoles(c, view)
			})
		})
		a.workflowRunArtifacts(c, run)
	})
}

func (a *App) workflowRunPaneHeader(c *ui.Context, run contracts.WorkflowRun, view workflow.RunView) {
	runID := run.ID
	ui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Text(c, run.Name).Grow(1).MinWidth(112).FontSize(design.TextSize(c, design.TextLarge)).
			FontWeight(600).SingleLine().Ellipsis("…").Tooltip(run.Name)
		// 保存/再次运行、恢复、配置、停止走与运行卡同一个控制器：
		// 同一份运行不会一个面上写「保存」、另一个面上写「再次运行」。
		a.workflowRunControls(c, view, "workflow-pane")
		iconButton(c.Key("workflow-run-open-session:"+runID), "chat-round-line", tr(c, "Open conversation")).Disabled(run.SessionID == "").
			OnClick(func() { a.selectSession(run.SessionID) })
		// 从这一次运行能看到同类：这条对话里所有的工作流运行。
		iconButton(c.Key("workflow-run-open-directory:"+runID), "checklist", tr(c, "All runs in this conversation")).Disabled(run.SessionID == "").
			OnClick(func() { a.openWorkflowRuns(run.SessionID) })
	})
}

// workflowRunStatusLine draws the state word, its reason, the counts, and
// how long the run has been going.
func (a *App) workflowRunStatusLine(c *ui.Context, run contracts.WorkflowRun, view workflow.RunView) {
	var parts []string
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
	if tokens := a.workflowRunTokens(run); tokens.Known {
		parts = append(parts, fmt.Sprintf(tr(c, "%d tokens"), tokens.Total()))
	}
	if count := len(workflow.RunArtifacts(run.Artifacts)); count > 0 {
		parts = append(parts, fmt.Sprintf(tr(c, "%d artifacts"), count))
	}
	ui.Row(c.Key("workflow-run-status:" + view.RunID)).FillWidth().Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.workflowRunKindWord(c, view)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Readable(a.workflowRunStatusColor(view))).SingleLine()
		for _, part := range parts {
			ui.Text(c, part).MaxWidthPercent(100).FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.palette.Muted).SingleLine().Ellipsis("…").Tooltip(part)
		}
		ui.Text(c, a.workflowElapsed(view)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Muted).FontFeatures("tnum")
	})
}

// workflowRunPhases draws the vertical spine of the run's phases.
func (a *App) workflowRunPhases(c *ui.Context, view workflow.RunView) {
	controls := design.Controls()
	ui.Column(c.Key("workflow-run-phases:" + view.RunID)).FillWidth().Gap(0).Children(func() {
		for index, phase := range view.Phases {
			key := [2]string{view.RunID, phase.ID}
			folded := a.workflowUI.foldedPhases[key]
			hasActors := len(phase.Actors) > 0
			ui.Column(c.Key("workflow-run-phase:"+view.RunID+":"+phase.ID)).FillWidth().
				Padding(0, 0, 8, 20).Gap(4).Draw(func(p *ui.Painter, r ui.Rect) {
				x, y := r.X+6, r.Y+controls.ButtonHeight/2
				if index > 0 {
					p.Line(x, r.Y, x, y-6, 1, a.palette.Border)
				}
				if index+1 < len(view.Phases) {
					p.Line(x, y+6, x, r.Y+r.H, 1, a.palette.Border)
				}
				dot := (&ui.Path{}).Circle(x, y, 5)
				if phase.Status == contracts.WorkflowPhasePending {
					p.StrokePath(dot, 1, a.palette.Muted)
				} else {
					p.FillPath(dot, a.workflowPhaseLampColor(phase.Status))
				}
			}).Children(func() {
				headerContext := c.Key("workflow-run-phase-toggle:" + view.RunID + ":" + phase.ID)
				var header ui.Element
				if hasActors {
					header = controlRowButton(headerContext).Label(phase.Name).OnClick(func() {
						if a.workflowUI.foldedPhases == nil {
							a.workflowUI.foldedPhases = make(map[[2]string]bool)
						}
						a.workflowUI.foldedPhases[key] = !folded
					})
				} else {
					header = ui.Row(headerContext).MinHeight(controls.ButtonHeight).Gap(8).AlignItems(ui.Center)
				}
				header.FillWidth().Children(func() {
					ui.Text(c, phase.Name).Grow(1).MinWidth(0).FontWeight(600).SingleLine().Ellipsis("…").Tooltip(phase.Name)
					ui.Text(c, a.workflowPhaseCountLabel(c, phase)).FontSize(design.TextSize(c, design.TextSmall)).
						TextColor(a.palette.Muted).FontFeatures("tnum").SingleLine()
					if hasActors {
						glyph := "alt-arrow-down"
						if folded {
							glyph = "alt-arrow-right"
						}
						icon(c, glyph, controls.IconSize)
					}
				})
				if hasActors && !folded {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						for _, actor := range phase.Actors {
							a.workflowRunActorRow(c, view, actor)
							for _, question := range view.Questions {
								if question.ActorID == actor.ID {
									a.workflowRunQuestion(c, view.RunID, question)
								}
							}
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
	c = c.Key("workflow-run-actor:" + view.RunID + ":" + actor.ID)
	var row ui.Element
	if actor.SessionID != "" {
		row = controlRowButton(c).Label(label).Tooltip(tr(c, "Open agent conversation")).
			OnClick(func() { a.workspace.OpenSession(actor.SessionID) })
	} else {
		row = ui.Row(c).MinHeight(design.Controls().ButtonHeight).Gap(8).AlignItems(ui.Center)
	}
	row.FillWidth().Children(func() {
		controlResourceAvatar(c.Key("avatar"), "bot")
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextSmall)).Wrap()
		ui.Text(c, tr(c, a.workflowActorStatusWord(actor.Status))).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.workflowActorDotColor(actor.Status)).SingleLine()
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
	var unassigned []contracts.WorkflowQuestion
	for _, question := range view.Questions {
		assigned := false
		for _, phase := range view.Phases {
			for _, actor := range phase.Actors {
				assigned = assigned || question.ActorID == actor.ID
			}
		}
		if !assigned {
			unassigned = append(unassigned, question)
		}
	}
	if len(unassigned) == 0 {
		return
	}
	ui.Column(c.Key("workflow-run-questions:" + view.RunID)).FillWidth().Gap(8).Children(func() {
		ui.Text(c, tr(c, "Questions for the agent")).FontWeight(600)
		for _, question := range unassigned {
			a.workflowRunQuestion(c, view.RunID, question)
		}
	})
}

func (a *App) workflowRunQuestion(c *ui.Context, runID string, question contracts.WorkflowQuestion) {
	ui.Column(c.Key("workflow-run-question:" + runID + ":" + question.ID)).FillWidth().Gap(4).
		Padding(10).Radius(8).Background(a.palette.Raised).Children(func() {
		ui.Text(c, question.Text).FillWidth().Wrap()
		if question.Answer != "" {
			ui.Text(c, question.Answer).FillWidth().Wrap().TextColor(a.palette.Muted)
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

func (a *App) workflowRunArtifacts(c *ui.Context, run contracts.WorkflowRun) {
	artifacts := workflow.RunArtifacts(run.Artifacts)
	if len(artifacts) == 0 {
		return
	}
	folded := a.workflowUI.artifactFolds[run.ID]
	primary, hasPrimary := workflow.PrimaryArtifact(artifacts)
	ui.Divider(c)
	ui.Column(c.Key("workflow-run-artifacts:"+run.ID)).FillWidth().Shrink(0).Padding(4, 12).Children(func() {
		controlRowButton(c.Key("workflow-artifacts-toggle:" + run.ID)).FillWidth().
			Label(tr(c, "Artifacts")).OnClick(func() {
			if a.workflowUI.artifactFolds == nil {
				a.workflowUI.artifactFolds = make(map[string]bool)
			}
			a.workflowUI.artifactFolds[run.ID] = !folded
		}).Children(func() {
			ui.Text(c, tr(c, "Artifacts")).FontWeight(600)
			ui.Text(c, fmt.Sprint(len(artifacts))).TextColor(a.palette.Muted).FontFeatures("tnum")
			ui.Spacer(c).Grow(1)
			glyph := "alt-arrow-down"
			if folded {
				glyph = "alt-arrow-right"
			}
			icon(c, glyph, design.Controls().IconSize)
		})
		if folded {
			return
		}
		ui.Scroll(c.Key("workflow-artifact-list:" + run.ID)).FillWidth().MaxHeight(220).Gap(4).Children(func() {
			if hasPrimary {
				a.workflowRunArtifactEntry(c, run.ID, primary, true)
				if len(artifacts) > 1 {
					ui.Divider(c)
				}
			}
			for _, artifact := range artifacts {
				if !hasPrimary || artifact.Name != primary.Name {
					a.workflowRunArtifactEntry(c, run.ID, artifact, false)
				}
			}
		})
	})
}

func (a *App) workflowRunArtifactEntry(c *ui.Context, runID string, artifact contracts.WorkflowArtifact, primary bool) {
	key := runID + ":" + artifact.ID
	controlRowButton(c.Key("workflow-run-artifact:" + key)).FillWidth().Label(artifact.Name).OnClick(func() {
		a.openWorkflowArtifact(runID, artifact)
	}).Children(func() {
		if primary && strings.TrimSpace(artifact.Content) != "" {
			ui.Column(c).Width(112).MinWidth(0).Padding(8).Gap(4).Radius(6).
				Background(a.palette.Card).Border(1, a.palette.Border).Children(func() {
				ui.Text(c, artifact.Content).FillWidth().MaxLines(3).Wrap().
					FontSize(design.TextSize(c, design.TextSmall))
			})
		} else {
			icon(c, "document", design.Controls().IconSize)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, artifact.Name).FillWidth().SingleLine().Ellipsis("…")
			if artifact.MIME != "" {
				ui.Text(c, artifact.MIME).FillWidth().SingleLine().Ellipsis("…").
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			}
		})
	})
}

// workflowRunError draws the run's recorded error, never invented.
func (a *App) workflowRunError(c *ui.Context, run contracts.WorkflowRun) {
	ui.Column(c.Key("workflow-run-error:" + run.ID)).FillWidth().Gap(6).Children(func() {
		ui.Text(c, tr(c, "Error")).FontWeight(600)
		ui.Text(c, run.Error).FillWidth().Wrap().Selectable().TextColor(a.palette.Readable(a.palette.Danger))
	})
}
