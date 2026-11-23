package desktop

import (
	"fmt"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowRunDirectoryView draws one conversation's workflow run directory:
// the runs still moving, then the ones that ended, each opening its own detail.
// It is a list, not a second stage line, so it says nothing about what happened
// inside a run.
func (a *App) workflowRunDirectoryView(c *ui.Context, sessionID string) {
	directory := workflow.BuildRunDirectory(a.state.WorkflowRuns, sessionID, workflow.RunDirectoryLimit)
	title := a.workflowDirectoryTitle(sessionID)
	ui.Column(c.Key("workflow-runs:" + sessionID)).Label(title).Fill().Gap(10).Padding(16).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, "checklist", design.Controls().IconSize)
			ui.Text(c, title).Grow(1).MinWidth(0).
				FontSize(design.TextSize(c, design.TextLarge)).FontWeight(600).SingleLine()
			ui.Text(c, fmt.Sprintf(tr(c, "%d runs"), directory.Count())).
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).FontFeatures("tnum")
		})
		if directory.Empty() {
			ui.Text(c, tr(c, "This conversation has not run a workflow")).FillWidth().Wrap().
				TextColor(a.palette.Muted)
			return
		}
		ui.Scroll(c.Key("workflow-runs-scroll:" + sessionID)).FillWidth().Grow(1).Gap(8).Children(func() {
			a.workflowDirectoryGroup(c, sessionID, tr(c, "Running"), directory.Running)
			a.workflowDirectoryGroup(c, sessionID, tr(c, "Ended"), directory.Ended)
			if directory.Truncated {
				// 取满一页就说后面可能还有，不拿过滤后的行数假装「就这些」。
				ui.Text(c, tr(c, "Older runs are not listed here.")).FillWidth().Wrap().
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			}
		})
	})
}

// workflowDirectoryGroup draws one group of rows under its heading.
func (a *App) workflowDirectoryGroup(c *ui.Context, sessionID, heading string, rows []workflow.RunDirectoryRow) {
	if len(rows) == 0 {
		return
	}
	ui.Column(c.Key("workflow-runs-group:" + sessionID + ":" + heading)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, heading).FontWeight(600)
		for _, row := range rows {
			row := row
			controlRowButton(c.Key("workflow-runs-row:" + row.RunID)).FillWidth().
				Label(tr(c, "Open run details")).Children(func() {
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					name := row.Name
					if name == "" {
						name = row.RunID
					}
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, name).FillWidth().SingleLine().Ellipsis("…").Tooltip(name)
						ui.Text(c, a.workflowDirectoryStatusWord(c, row)).FillWidth().Wrap().
							FontSize(design.TextSize(c, design.TextSmall)).
							TextColor(a.palette.Readable(a.workflowRunStatusColor(workflow.RunView{Status: row.Status})))
					})
					if !row.UpdatedAt.IsZero() {
						ui.Text(c, relativeAge(row.UpdatedAt, time.Now(), a.state.Settings.General.Language)).
							FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).FontFeatures("tnum").SingleLine()
					}
					icon(c, "alt-arrow-right", design.Controls().IconSize)
				})
			}).OnClick(func() { a.openWorkflowRun(row.RunID) })
		}
	})
}

// workflowDirectoryStatusWord names one row's state. A stopped run says why
// instead of hiding it behind the word stopped.
func (a *App) workflowDirectoryStatusWord(c *ui.Context, row workflow.RunDirectoryRow) string {
	if row.StopReason == contracts.WorkflowStopSuperseded {
		return tr(c, "Workflow superseded")
	}
	switch row.Status {
	case contracts.WorkflowStatusPending:
		return tr(c, "Pending")
	case contracts.WorkflowStatusRunning:
		return tr(c, "Running")
	case contracts.WorkflowStatusCompleted:
		return tr(c, "Completed")
	case contracts.WorkflowStatusErrored:
		return tr(c, "Failed")
	case contracts.WorkflowStatusStopped:
		return tr(c, "Stopped")
	}
	return tr(c, "Workflow ended")
}

// workflowDirectoryTitle names the directory after the conversation it lists.
// It is also used from a click handler, so it takes no build context.
func (a *App) workflowDirectoryTitle(sessionID string) string {
	for _, session := range a.state.Sessions {
		if session.ID != sessionID {
			continue
		}
		if session.Title != "" {
			return fmt.Sprintf(a.t("Runs of %s"), session.Title)
		}
		break
	}
	return a.t("Workflow runs")
}

// openWorkflowRuns opens one conversation's run directory beside it.
func (a *App) openWorkflowRuns(sessionID string) {
	if sessionID == "" {
		return
	}
	a.workspace.OpenWorkflowRuns(sessionID, a.workflowDirectoryTitle(sessionID))
}
