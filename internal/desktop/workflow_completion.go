package desktop

import (
	"fmt"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowCompletionCard draws the completion card of a finished workflow run:
// the primary delivery first, the rest as an index, then what the run actually
// cost. It only appears for a run that succeeded and really delivered
// something; a failed or stopped run has no card, because it delivered
// nothing.
func (a *App) workflowCompletionCard(c *ui.Context, run contracts.WorkflowRun, width float32) {
	view, ok := workflow.Completion(run, a.workflowRunTokens(run))
	if !ok {
		return
	}
	p := a.palette
	ui.Column(c.Key("workflow-completion:"+run.ID)).FillWidth().MaxWidth(width).Gap(8).
		Padding(12).Radius(12).Border(1, p.Border).Background(p.Card).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, "Workflow completed")).FontWeight(600).SingleLine()
			ui.Text(c, view.Name).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase)).
				TextColor(p.Muted).SingleLine()
			ui.Text(c, fmt.Sprintf(tr(c, "%d delivered"), view.Delivered)).
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Muted).SingleLine()
		})
		if view.HasPrimary {
			a.workflowCompletionPreview(c, view)
		}
		a.workflowCompletionIndex(c, run, view)
		a.workflowCompletionStats(c, view)
	})
}

// workflowCompletionPreview shows the primary delivery: its name and its body.
// A delivery without a primary has no preview, and the card says so by
// showing only the index.
func (a *App) workflowCompletionPreview(c *ui.Context, view workflow.CompletionView) {
	ui.Column(c.Key("workflow-completion-preview:" + view.RunID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, view.Primary.Name).FontWeight(500).SingleLine()
		if view.Primary.Content != "" {
			ui.Text(c, view.Primary.Content).FillWidth().Selectable().MaxLines(8).
				Font(a.state.Settings.Appearance.CodeFont).
				FontSize(design.TextSize(c, design.TextSmall)).
				Padding(8).Background(a.palette.CodeWash).Radius(8)
		}
	})
}

// workflowCompletionIndex lists the deliveries that are not the preview.
func (a *App) workflowCompletionIndex(c *ui.Context, run contracts.WorkflowRun, view workflow.CompletionView) {
	if len(view.Index.Shown) == 0 && view.Index.Overflow == 0 {
		return
	}
	ui.Column(c.Key("workflow-completion-index:" + view.RunID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Deliveries")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			for _, artifact := range view.Index.Shown {
				artifact := artifact
				textButton(c.Key("workflow-completion-artifact:"+view.RunID+":"+artifact.ID), artifact.Name).OnClick(func() {
					a.openWorkflowArtifact(run.ID, artifact)
				})
			}
			if view.Index.Overflow > 0 {
				ui.Text(c, fmt.Sprintf(tr(c, "+%d"), view.Index.Overflow)).
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).FontFeatures("tnum")
			}
		})
	})
}

// workflowCompletionStats is the four facts the card ends with: how long it
// took, how many tokens it used, how many instances ran, and how many phases
// it entered. An unknown fact shows —, never a zero.
func (a *App) workflowCompletionStats(c *ui.Context, view workflow.CompletionView) {
	unknown := "—"
	duration := unknown
	if view.Duration > 0 {
		duration = i18n.Duration(a.state.Settings.General.Language, view.Duration.Milliseconds())
	}
	tokens := unknown
	if view.Tokens.Known {
		tokens = fmt.Sprintf("%d", view.Tokens.Total())
	}
	agents := unknown
	if view.Agents > 0 {
		agents = fmt.Sprintf("%d", view.Agents)
	}
	phases := unknown
	if view.Phases > 0 {
		phases = fmt.Sprintf("%d", view.Phases)
	}
	ui.Row(c.Key("workflow-completion-stats:" + view.RunID)).FillWidth().Gap(14).Wrap().Children(func() {
		for _, stat := range []struct{ label, value string }{
			{tr(c, "Duration"), duration},
			{tr(c, "Tokens"), tokens},
			{tr(c, "Instances"), agents},
			{tr(c, "Phases entered"), phases},
		} {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, stat.label).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
				ui.Text(c, stat.value).FontSize(design.TextSize(c, design.TextSmall)).
					TextColor(a.palette.Text).FontFeatures("tnum").SingleLine()
			})
		}
	})
}

// workflowRunTokens totals the tokens the run's own instances used. The run
// itself never asks a model, so the count comes from the sessions the run
// created. Without a single reported counter the total stays unknown.
func (a *App) workflowRunTokens(run contracts.WorkflowRun) workflow.TokenTotals {
	totals := workflow.TokenTotals{}
	sessions := map[string]bool{}
	for _, session := range a.state.Sessions {
		if session.ParentRunID == run.ID {
			sessions[session.ID] = true
		}
	}
	if len(sessions) == 0 {
		return totals
	}
	for _, request := range a.state.ModelRequests {
		if !sessions[request.SessionID] || !request.Usage.Known {
			continue
		}
		totals.Input += int64(request.Usage.Input)
		totals.Output += int64(request.Usage.Output)
		totals.Known = true
	}
	return totals
}
