package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowUIState holds the per-run interface state: whether the card is
// expanded, which run's settings sheet is open, and which requests are in
// flight. The run facts themselves always come from the core.
type workflowUIState struct {
	foldedPhases map[[2]string]bool
	expanded     map[string]bool
	settings     map[string]*workflowSettingsState
	busy         map[string]bool
	// scroll remembers how far each run card's stage line was scrolled and
	// what the shared scrollbar reports back.
	scroll   map[string]*ui.ScrollState
	position map[string]*float64
	// save holds the draft of every run whose save form is open.
	save map[string]*workflowSaveState
	// hub is the saved-workflow hub's own state, and hubLaunch is the launch
	// it is carrying through the core.
	hub              workflowHubState
	hubLaunch        *workflowLaunchPending
	artifactFolds    map[string]bool
	artifactVersions map[string]int
}

type workflowSettingsState struct {
	open            bool
	modelID         string
	hasModel        bool
	concurrency     int
	concurrencyText string
	hasConc         bool
	errorText       string
}

func workflowLaunchBlock(blocks []contracts.Block) string {
	for _, block := range blocks {
		if block.Tool == nil || block.Tool.IsError {
			continue
		}
		switch block.Tool.Name {
		case "CreateWorkflow", "AmendWorkflow":
			return block.ID
		}
	}
	return ""
}

func (a *App) workflowActivityCards(c *ui.Context, sessionID string, blocks []contracts.Block, width float32) {
	if len(a.state.WorkflowRuns) == 0 || workflowLaunchBlock(blocks) == "" {
		return
	}
	// The transcript merges assistant continuations, but a run points to the
	// original message. Its tool block keeps its identity in the merged activity.
	for _, message := range a.state.Messages[sessionID] {
		launch := workflowLaunchBlock(message.Blocks)
		if launch == "" || !slices.ContainsFunc(blocks, func(block contracts.Block) bool { return block.ID == launch }) {
			continue
		}
		a.workflowCompletionCards(c, message.ID, width)
		a.workflowRunCards(c, message.ID, width)
	}
}

// workflowRunCards renders every run this turn launched. The same run is
// never drawn twice.
func (a *App) workflowRunCards(c *ui.Context, messageID string, width float32) {
	runs := a.workflowRunsForMessage(messageID)
	if len(runs) == 0 {
		return
	}
	if width <= 0 {
		width = a.chatWidth - 48
	}
	ui.Column(c.Key("workflow-cards:"+messageID)).FillWidth().Gap(8).Margin(8, 0, 0, 0).Children(func() {
		for _, run := range runs {
			a.workflowRunCard(c, run, width)
		}
	})
}

// workflowCompletionCards draws the completion card of every run this turn
// launched that has already delivered something. It runs before the run cards,
// so the turn reads: body, what was delivered, then the run itself.
func (a *App) workflowCompletionCards(c *ui.Context, messageID string, width float32) {
	runs := a.workflowRunsForMessage(messageID)
	if len(runs) == 0 {
		return
	}
	if width <= 0 {
		width = a.chatWidth - 48
	}
	ui.Column(c.Key("workflow-completions:"+messageID)).FillWidth().Gap(8).Margin(8, 0, 0, 0).Children(func() {
		for _, run := range runs {
			a.workflowCompletionCard(c, run, width)
		}
	})
}

// workflowRunCard renders the run card of one run: the kind word, the name,
// the summary line, the stage line, and the artifact strip.
func (a *App) workflowRunCard(c *ui.Context, run contracts.WorkflowRun, width float32) {
	view := workflow.ProjectRun(run)
	timeline := workflow.BuildTimeline(run)
	expanded := a.workflowCardExpanded(run.ID)
	p := a.palette
	ui.Column(c.Key("workflow-card:"+run.ID)).FillWidth().MaxWidth(width).Gap(8).
		Padding(12).Radius(12).Border(1, p.Border).Background(p.Card).Children(func() {
		a.workflowRunHeader(c, view)
		a.workflowRunSummary(c, view, func() { a.toggleWorkflowCard(run.ID) })
		if len(timeline.Stations) > 0 {
			a.workflowRunTimeline(c, view, timeline, expanded)
		}
		if view.Truncated {
			ui.Text(c.Key("workflow-card-truncated:"+run.ID), tr(c, "Some instance details are not shown here.")).
				FillWidth().FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Muted).Wrap()
		}
		if len(view.Artifacts.Shown) > 0 {
			a.workflowArtifactStrip(c, view)
		}
	})
}

// workflowRunHeader draws the card head: the kind word, the name, the pending
// question and hole chips, and the controls.
func (a *App) workflowRunHeader(c *ui.Context, view workflow.RunView) {
	p := a.palette
	ui.Row(c.Key("workflow-card-header:" + view.RunID)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.workflowRunKindWord(c, view)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.workflowRunStatusColor(view)).SingleLine()
		ui.Text(c, view.Name).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase)).SingleLine()
		if pending := view.PendingQuestions(); pending > 0 {
			workflowStatusChip(c, "workflow-card-questions:"+view.RunID, tr(c, "%d questions", pending), p.Readable(p.Warning), "", func() {
				a.openWorkflowRun(view.RunID)
			})
		}
		if waiting := view.WaitingHoles(); waiting > 0 {
			// 留白芯片带笔：说的是主代理还没写这段代码。点它开的是这次运行，
			// 界面不提供写代码的输入框。
			workflowStatusChip(c, "workflow-card-holes:"+view.RunID, tr(c, "%d holes", waiting), p.Readable(p.Warning), "pen", func() {
				a.openWorkflowRun(view.RunID)
			})
		}
		a.workflowRunControls(c, view, "workflow-card")
		iconButton(c.Key("workflow-card-open:"+view.RunID), "alt-arrow-right", tr(c, "Open run details")).OnClick(func() {
			a.openWorkflowRun(view.RunID)
		})
	})
}

// workflowRunControls draws the run's own actions: save or run again, resume,
// configure, and stop. The card and the detail head share this one controller,
// so a run never says "save" on one face and "run again" on the other.
func (a *App) workflowRunControls(c *ui.Context, view workflow.RunView, scope string) {
	run, ok := a.workflowRun(view.RunID)
	if !ok {
		return
	}
	// 两个面同时在场时 key 不能撞车：卡片与详情各带自己的前缀。
	key := ":" + scope + ":" + view.RunID
	if workflow.CanResume(run) {
		textButton(c.Key("workflow-resume"+key), tr(c, "Resume run")).OnClick(func() {
			a.resumeWorkflowRun(run)
		})
	}
	// 保存与再次运行只在跑完的运行上出现：跑到一半的脚本还是一次性的。
	if view.Status == contracts.WorkflowStatusCompleted {
		if saved := a.workflowSaveOf(run); saved.saved {
			a.workflowSavedChip(c, "workflow-saved"+key, saved.definition.Name)
			textButton(c.Key("workflow-again"+key), tr(c, "Run again")).OnClick(func() {
				a.runWorkflowAgain(run)
			})
		} else {
			textButton(c.Key("workflow-save"+key), tr(c, "Save workflow")).OnClick(func() {
				a.openWorkflowSave(run.ID)
			})
		}
	}
	if workflow.CanConfigure(run) {
		trigger := iconButton(c.Key("workflow-settings"+key), "settings-minimalistic", tr(c, "Configure run")).OnClick(func() {
			a.openWorkflowSettings(key, run.ID)
		})
		if state := a.workflowUI.settings[key]; state != nil {
			a.workflowSettingsPopover(c.Key("workflow-settings-panel"+key), trigger, run, state)
		}
	}
	if view.Active() {
		textButton(c.Key("workflow-stop"+key), tr(c, "Stop run")).OnClick(func() {
			a.dispatch(contracts.Command{Kind: "workflow.stop", Workflow: &contracts.WorkflowCommand{RunID: run.ID}})
		})
	}
}

// markWorkflowBusy remembers that one run has a request in flight.
func (a *App) markWorkflowBusy(runID string) {
	if a.workflowUI.busy == nil {
		a.workflowUI.busy = map[string]bool{}
	}
	a.workflowUI.busy[runID] = true
}

// clearWorkflowBusy drops every in-flight mark. Workflow commands settle one at
// a time, so the whole set can go at once.
func (a *App) clearWorkflowBusy() {
	for runID := range a.workflowUI.busy {
		delete(a.workflowUI.busy, runID)
	}
}

// workflowSavedChip is the neutral "saved" badge. It reports a file fact,
// not a run outcome, and it stays even when the run can no longer be written.
func (a *App) workflowSavedChip(c *ui.Context, key, name string) {
	p := a.palette
	ui.Row(c.Key(key)).Padding(2, 8).Radius(999).Background(p.Hover).Gap(4).AlignItems(ui.Center).Children(func() {
		icon(c, "pen-new-square", design.Controls().IconSize)
		ui.Text(c, name).FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Muted).SingleLine()
	})
}

// workflowCardExpanded reports whether the run card shows its instance
// pills. The card starts expanded; hiding the pills never hides the stage
// line or the artifacts.
func (a *App) workflowCardExpanded(runID string) bool {
	if a.workflowUI.expanded == nil {
		return true
	}
	expanded, chosen := a.workflowUI.expanded[runID]
	if !chosen {
		return true
	}
	return expanded
}

// toggleWorkflowCard flips one card's instance pills.
func (a *App) toggleWorkflowCard(runID string) {
	if a.workflowUI.expanded == nil {
		a.workflowUI.expanded = make(map[string]bool)
	}
	a.workflowUI.expanded[runID] = !a.workflowCardExpanded(runID)
}

// workflowRunSummary draws the summary line: the phase count, the instance
// count, and the model the instances use.
func (a *App) workflowRunSummary(c *ui.Context, view workflow.RunView, toggle func()) {
	parts := []string{tr(c, "%d phases", len(view.Phases))}
	if view.Active() {
		parts = append(parts, tr(c, "%d agents working", view.AgentWorking()))
	} else {
		parts = append(parts, tr(c, "%d agents", view.AgentTotal()))
	}
	if view.ModelID != "" {
		parts = append(parts, a.workflowModelLabel(view.ModelID))
	}
	ui.Row(c.Key("workflow-card-summary:" + view.RunID)).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		for _, part := range parts {
			ui.Text(c, part).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
		}
		if view.AgentTotal() > 0 {
			glyph, label := "alt-arrow-right", "Show agents"
			if a.workflowCardExpanded(view.RunID) {
				glyph, label = "alt-arrow-down", "Hide agents"
			}
			iconButton(c.Key("workflow-card-agent-toggle:"+view.RunID), glyph, tr(c, label)).OnClick(toggle)
		}
	})
}

// workflowArtifactStrip draws the delivered artifacts: at most three links
// and a static count for the rest.
func (a *App) workflowArtifactStrip(c *ui.Context, view workflow.RunView) {
	ui.Row(c.Key("workflow-card-artifacts:" + view.RunID)).FillWidth().Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
		for _, artifact := range view.Artifacts.Shown {
			textButton(c.Key("workflow-artifact:"+view.RunID+":"+artifact.ID), artifact.Name).OnClick(func() {
				a.openWorkflowArtifact(view.RunID, artifact)
			})
		}
		if view.Artifacts.Overflow > 0 {
			textButton(c.Key("workflow-card-more-artifacts:"+view.RunID), fmt.Sprintf(tr(c, "+%d"), view.Artifacts.Overflow)).
				OnClick(func() {
					delete(a.workflowUI.artifactFolds, view.RunID)
					a.openWorkflowRun(view.RunID)
				})
		}
	})
}

// workflowModelLabel names a run's instance model the way the composer names
// providers, never by a bare id when the catalog knows the pair.
func (a *App) workflowModelLabel(modelID string) string {
	for _, model := range a.state.Models {
		if model.ID == modelID {
			return a.runtimeModelLabel(model)
		}
	}
	return modelID
}

// workflowStatusChip draws the warning chip of a pending question or hole.
// Both chips open the run they belong to: the answer is written by the main
// agent, so the chip is a way there, not a place to type.
func workflowStatusChip(c *ui.Context, key, label string, color ui.Color, glyph string, action func()) ui.Element {
	chip := ui.ButtonBase(c.Key(key)).Padding(2, 8).Radius(999).Background(color.Alpha(.12)).Gap(4).AlignItems(ui.Center)
	chip.Children(func() {
		if glyph != "" {
			icon(c, glyph, design.Controls().IconSize)
		}
		ui.Text(c, label).FontSize(design.TextSize(c, design.TextSmall)).TextColor(color).SingleLine()
	})
	if action != nil {
		chip.OnClick(action)
	}
	return chip
}

// workflowRunKindWord is the one word that names the run's state. A run that
// was superseded says so instead of saying stopped.
func (a *App) workflowRunKindWord(c *ui.Context, view workflow.RunView) string {
	if view.StopReason == contracts.WorkflowStopSuperseded {
		return tr(c, "Workflow superseded")
	}
	switch view.Status {
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
	default:
		return tr(c, "Workflow ended")
	}
}

// workflowRunStatusColor is the color of the state lamp.
func (a *App) workflowRunStatusColor(view workflow.RunView) ui.Color {
	switch view.Status {
	case contracts.WorkflowStatusPending:
		return a.palette.Muted
	case contracts.WorkflowStatusRunning:
		return a.palette.Readable(a.palette.Warning)
	case contracts.WorkflowStatusCompleted:
		return a.palette.Readable(a.palette.Success)
	case contracts.WorkflowStatusErrored:
		return a.palette.Readable(a.palette.Danger)
	default:
		return a.palette.Muted
	}
}

// workflowRunStopReason is the reason word that follows the state word.
func (a *App) workflowRunStopReason(view workflow.RunView) string {
	switch view.StopReason {
	case contracts.WorkflowStopUser:
		return a.t("Stopped by you")
	case contracts.WorkflowStopModel:
		return a.t("Stopped by the agent")
	case contracts.WorkflowStopProvider:
		return a.t("Provider error")
	case contracts.WorkflowStopInterrupted:
		return a.t("Process exited")
	case contracts.WorkflowStopSuperseded:
		return a.t("Superseded")
	default:
		return ""
	}
}

// workflowRunsForMessage returns the runs a message launched. The same run
// never appears twice.
func (a *App) workflowRunsForMessage(messageID string) []contracts.WorkflowRun {
	if messageID == "" {
		return nil
	}
	var runs []contracts.WorkflowRun
	seen := map[string]bool{}
	for _, run := range a.state.WorkflowRuns {
		if run.MessageID != messageID || seen[run.ID] {
			continue
		}
		seen[run.ID] = true
		runs = append(runs, run)
	}
	slices.SortFunc(runs, func(left, right contracts.WorkflowRun) int {
		return strings.Compare(left.StartedAt.Format("2006-01-02T15:04:05.000000000"), right.StartedAt.Format("2006-01-02T15:04:05.000000000"))
	})
	return runs
}

// workflowRun finds one run in the snapshot.
func (a *App) workflowRun(runID string) (contracts.WorkflowRun, bool) {
	for _, run := range a.state.WorkflowRuns {
		if run.ID == runID {
			return run, true
		}
	}
	return contracts.WorkflowRun{}, false
}

// openWorkflowRun opens the run detail tab beside the conversation.
func (a *App) openWorkflowRun(runID string) {
	run, ok := a.workflowRun(runID)
	if !ok {
		return
	}
	a.workspace.OpenWorkflow(run.ID, run.Name)
}

// openWorkflowArtifact opens the artifact in the run detail tab.
func (a *App) openWorkflowArtifact(runID string, artifact contracts.WorkflowArtifact) {
	a.workspace.OpenWorkflowArtifact(runID+"/"+artifact.Name, artifact.Name)
}

// workflowElapsed is the run's own duration, rounded to whole seconds.
func (a *App) workflowElapsed(view workflow.RunView) string {
	if view.StartedAt.IsZero() {
		return "—"
	}
	end := view.UpdatedAt
	if view.FinishedAt != nil {
		end = *view.FinishedAt
	}
	if end.Before(view.StartedAt) {
		return "—"
	}
	return i18n.Duration(a.state.Settings.General.Language, end.Sub(view.StartedAt).Milliseconds())
}
