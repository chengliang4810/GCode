package desktop

import (
	"strconv"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// openWorkflowSettings opens the settings sheet of one run. The sheet starts
// from the run's current settings and only sends the fields that changed.
func (a *App) openWorkflowSettings(runID string) {
	run, ok := a.workflowRun(runID)
	if !ok {
		return
	}
	if a.workflowUI.settings == nil {
		a.workflowUI.settings = map[string]*workflowSettingsState{}
	}
	state := &workflowSettingsState{
		modelID:     run.ModelID,
		hasModel:    true,
		concurrency: workflow.NormalizeConcurrency(run.Concurrency),
		hasConc:     true,
	}
	a.workflowUI.settings[runID] = state
	state.open = true
}

// workflowSettingsDialog draws the settings sheet of one run.
func (a *App) workflowSettingsDialog(c *ui.Context) {
	for runID, state := range a.workflowUI.settings {
		if !state.open {
			continue
		}
		run, ok := a.workflowRun(runID)
		if !ok {
			state.open = false
			continue
		}
		a.workflowSettingsSheet(c, run, state)
	}
}

// workflowSettingsSheet draws the modal form of one run's settings.
func (a *App) workflowSettingsSheet(c *ui.Context, run contracts.WorkflowRun, state *workflowSettingsState) {
	busy := a.workflowUI.busy[run.ID]
	changed := a.workflowSettingsChanged(run, state)
	width, height := c.Size()
	ui.DialogBase(c.Key("workflow-settings:"+run.ID), &state.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .45))
		panel.Width(min(460, width-40)).MaxHeight(height - 40).Padding(20).Gap(16).Radius(12).
			Label(tr(c, "Configure run"))
		panel.Background(a.palette.Dialog).Border(1, a.palette.Border)
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, "Configure run")).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600)
			iconButton(c.Key("workflow-settings-close:"+run.ID), "close", tr(c, "Close dialog")).Disabled(busy).OnClick(func() { state.open = false })
		})
		ui.Scroll(c.Key("workflow-settings-content:" + run.ID)).FillWidth().MaxHeight(320).Gap(12).Children(func() {
			ui.Text(c, tr(c, "Instance model")).FillWidth().FontWeight(500)
			ui.Text(c, tr(c, "Leave the session model to inherit it.")).FillWidth().
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Wrap()
			a.workflowModelOptions(c, run, state)
			ui.Text(c, tr(c, "Max concurrency")).FillWidth().FontWeight(500)
			ui.Text(c, tr(c, "The limit only has a lower bound of one.")).FillWidth().
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Wrap()
			ui.TextInput(c.Key("workflow-settings-concurrency:"+run.ID), &state.concurrencyText).
				FillWidth().Placeholder(strconv.Itoa(state.concurrency)).
				Label(tr(c, "Max concurrency"))
			if consequence := a.workflowSettingsConsequence(run, state); consequence != "" {
				ui.Text(c, consequence).FillWidth().Wrap().
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			}
			if state.errorText != "" {
				ui.Text(c, state.errorText).FillWidth().Wrap().TextColor(a.palette.Readable(a.palette.Danger))
			}
		})
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			textButton(c.Key("workflow-settings-cancel:"+run.ID), tr(c, "Cancel")).Disabled(busy).OnClick(func() { state.open = false })
			primaryButton(c.Key("workflow-settings-apply:"+run.ID), tr(c, "Apply")).Disabled(busy || !changed).OnClick(func() {
				a.applyWorkflowSettings(run, state)
			})
		})
	})
}

// workflowModelOptions draws the instance model choices of one run.
func (a *App) workflowModelOptions(c *ui.Context, run contracts.WorkflowRun, state *workflowSettingsState) {
	a.workflowModelChoices(c, "workflow-settings-models:"+run.ID, a.workflowSessionModel(run), state.modelID, func(modelID string) {
		state.modelID = choiceModelID(modelID)
		state.hasModel = true
	})
}

// workflowModelChoices 画实例模型的选项。空值表示沿用会话模型。
func (a *App) workflowModelChoices(c *ui.Context, key, sessionModel, current string, pick func(string)) {
	choices := []struct{ value, label string }{
		{value: "", label: tr(c, "Session model")},
	}
	for _, model := range a.state.Models {
		choices = append(choices, struct{ value, label string }{value: model.ID, label: a.runtimeModelLabel(model)})
	}
	ui.Column(c.Key(key)).FillWidth().Gap(6).Children(func() {
		for _, choice := range choices {
			selected := current == choice.value
			label := choice.label
			if choice.value == "" && sessionModel != "" {
				label = tr(c, "Session model") + " (" + sessionModel + ")"
			}
			ui.Row(c).FillWidth().Children(func() {
				option := controlButton(c, label, buttonSecondary).FillWidth().Justify(ui.Start)
				if selected {
					option.Background(a.palette.Active)
				}
				option.OnClick(func() { pick(choice.value) })
			})
		}
	})
}

// choiceModelID 保留选项里的模型标识，空值就是空值。
func choiceModelID(modelID string) string { return modelID }

// workflowSettingsConsequence says what this change does to the run. The
// wording follows the change itself: an unchanged sheet says nothing, and a
// live concurrency change never claims it will stop the run.
func (a *App) workflowSettingsConsequence(run contracts.WorkflowRun, state *workflowSettingsState) string {
	if !a.workflowSettingsChanged(run, state) {
		return ""
	}
	modelChanged := state.hasModel
	target := state.modelID
	if target == "" {
		target = a.workflowSessionModel(run)
	}
	if !modelChanged || target == run.ModelID {
		if run.Status == contracts.WorkflowStatusRunning {
			return a.t("This applies to the running run right away; it does not start a new run.")
		}
		return a.t("This starts another run with the new settings; the finished steps are kept.")
	}
	switch run.Status {
	case contracts.WorkflowStatusPending:
		return a.t("This starts another run with the new settings; the current run has not started any step yet.")
	case contracts.WorkflowStatusStopped:
		return a.t("This starts another run with the new settings and keeps the finished steps.")
	case contracts.WorkflowStatusErrored:
		return a.t("This starts another run with the new settings and retries; the finished steps are kept.")
	default:
		return a.t("This stops the current run and starts another one with the new settings; the finished steps are kept.")
	}
}

// workflowSessionModel is the model the run's session uses.
func (a *App) workflowSessionModel(run contracts.WorkflowRun) string {
	return a.workflowSessionModelByID(run.SessionID)
}

// workflowSessionModelByID is the model a session uses.
func (a *App) workflowSessionModelByID(sessionID string) string {
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			return session.ModelID
		}
	}
	return ""
}

// workflowSettingsChanged reports whether the sheet differs from the run's
// current settings. An unchanged sheet cannot be applied.
func (a *App) workflowSettingsChanged(run contracts.WorkflowRun, state *workflowSettingsState) bool {
	if state.hasModel {
		target := state.modelID
		if target == "" {
			target = a.workflowSessionModel(run)
		}
		if target != run.ModelID {
			return true
		}
	}
	if state.hasConc {
		value := state.concurrency
		if state.concurrencyText != "" {
			parsed, err := strconv.Atoi(state.concurrencyText)
			if err != nil {
				return false
			}
			value = parsed
		}
		if workflow.NormalizeConcurrency(value) != workflow.NormalizeConcurrency(run.Concurrency) {
			return true
		}
	}
	return false
}

// applyWorkflowSettings sends the changed fields of one run's settings.
func (a *App) applyWorkflowSettings(run contracts.WorkflowRun, state *workflowSettingsState) {
	change := contracts.WorkflowSettingsChange{RunID: run.ID}
	if state.hasModel {
		target := state.modelID
		if target == "" {
			target = a.workflowSessionModel(run)
		}
		if target != run.ModelID {
			change.ModelID, change.HasModel = target, true
		}
	}
	if state.hasConc {
		value := state.concurrency
		if state.concurrencyText != "" {
			parsed, err := strconv.Atoi(state.concurrencyText)
			if err != nil {
				state.errorText = a.t("The concurrency must be a whole number.")
				return
			}
			value = parsed
		}
		if workflow.NormalizeConcurrency(value) != workflow.NormalizeConcurrency(run.Concurrency) {
			change.Concurrency, change.HasConc = value, true
		}
	}
	if !change.HasModel && !change.HasConc {
		state.errorText = a.t("This is the run's current settings.")
		return
	}
	a.markWorkflowBusy(run.ID)
	state.open = false
	a.dispatch(contracts.Command{Kind: "workflow.retune", Workflow: &contracts.WorkflowCommand{Settings: &change}})
}
