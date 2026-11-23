package desktop

import (
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// 保存表单每份作用域最多看多少份定义，用来判断「同名」与「同脚本」。
// 这是定位已保存事实的推断上限，不是全盘扫描。
const workflowSavedScanLimit = 50

// workflowSaveState 是一次「保存」表单的草稿与状态。草稿在弹窗打开那一刻
// 冻住：背景事件更新不覆盖用户正在敲的输入。
type workflowSaveState struct {
	open        bool
	name        string
	scope       string
	description string
	errorText   string
	// taken 说明这个名字在目标作用域里已经有定义：提交因此变成一次覆盖。
	taken bool
}

// workflowSaveController 是完成卡与详情头共用的那一份「保存 / 再次运行」事实。
// 同一个 run 不能一边显示「保存」、另一边显示「再次运行」。
type workflowSaveController struct {
	// definition 是这次运行已经保存下来的定义；零值表示还没保存。
	definition contracts.WorkflowDefinition
	saved      bool
}

// openWorkflowSave 打开一次运行的保存表单。
func (a *App) openWorkflowSave(runID string) {
	run, ok := a.workflowRun(runID)
	if !ok {
		return
	}
	// 远端运行只能存进项目：家目录在宿主机器上，本机的中枢读不到它。
	scope := workflow.SavedScopeProject
	draft := workflow.InitialSaveDraft(run.Name, scope)
	if a.workflowUI.save == nil {
		a.workflowUI.save = map[string]*workflowSaveState{}
	}
	a.workflowUI.save[runID] = &workflowSaveState{
		open:        true,
		name:        draft.Name,
		scope:       draft.Scope,
		description: draft.Description,
	}
}

// workflowSaveOf 是这次运行的保存事实。已经有定义在先（run.DefinitionID 指着一份
// 现存定义）就是已保存；否则看脚本是否与某份定义逐字相同。
func (a *App) workflowSaveOf(run contracts.WorkflowRun) workflowSaveController {
	if run.DefinitionID != "" {
		for _, definition := range a.state.WorkflowDefinitions {
			if definition.ID == run.DefinitionID {
				return workflowSaveController{definition: definition, saved: true}
			}
		}
	}
	if definition, ok := workflow.SavedByScript(run, a.state.WorkflowDefinitions, workflowSavedScanLimit); ok {
		return workflowSaveController{definition: definition, saved: true}
	}
	return workflowSaveController{}
}

// workflowSaveDialog draws every open save form.
func (a *App) workflowSaveDialog(c *ui.Context) {
	for runID, state := range a.workflowUI.save {
		if !state.open {
			continue
		}
		run, ok := a.workflowRun(runID)
		if !ok {
			state.open = false
			continue
		}
		a.workflowSaveSheet(c, run, state)
	}
}

// workflowSaveSheet draws the modal form that saves a finished run as a
// reusable definition. Direct saving reads the script the run actually
// executed: it never goes through a model turn.
func (a *App) workflowSaveSheet(c *ui.Context, run contracts.WorkflowRun, state *workflowSaveState) {
	busy := a.workflowUI.busy[run.ID]
	name := strings.TrimSpace(state.name)
	nameInvalid := name != "" && !workflow.ValidSavedName(name)
	canSave := name != "" && !nameInvalid && !busy
	width, height := c.Size()
	ui.DialogBase(c.Key("workflow-save:"+run.ID), &state.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .45))
		panel.Width(min(480, width-40)).MaxHeight(height - 40).Padding(20).Gap(16).Radius(12).
			Label(tr(c, "Save workflow"))
		panel.Background(a.palette.Dialog).Border(1, a.palette.Border)
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, "Save workflow")).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600)
			iconButton(c.Key("workflow-save-close:"+run.ID), "close", tr(c, "Close dialog")).Disabled(busy).OnClick(func() { state.open = false })
		})
		ui.Scroll(c.Key("workflow-save-content:" + run.ID)).FillWidth().MaxHeight(360).Gap(12).Children(func() {
			ui.Text(c, tr(c, "Name")).FillWidth().FontWeight(500)
			ui.Text(c, tr(c, "Letters, digits and dashes, dots or underscores.")).FillWidth().
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Wrap()
			ui.TextInput(c.Key("workflow-save-name:"+run.ID), &state.name).
				FillWidth().Placeholder(tr(c, "Workflow name")).Label(tr(c, "Name"))
			if nameInvalid {
				ui.Text(c, tr(c, "This name cannot be a file name.")).FillWidth().Wrap().
					TextColor(a.palette.Readable(a.palette.Danger))
			}
			ui.Text(c, tr(c, "Scope")).FillWidth().FontWeight(500)
			a.workflowSaveScopeChoices(c, run, state)
			ui.Text(c, tr(c, "Description")).FillWidth().FontWeight(500)
			ui.TextInput(c.Key("workflow-save-description:"+run.ID), &state.description).
				FillWidth().Placeholder(tr(c, "What this workflow does")).Label(tr(c, "Description"))
			if name != "" {
				// 落点预览：用户要能一眼看出这份定义会落在哪儿。
				ui.Text(c, workflow.SavedPathPreview(state.scope, name)).FillWidth().Wrap().
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			}
			if state.taken {
				ui.Text(c, tr(c, "A workflow with this name already exists; saving replaces it.")).FillWidth().Wrap().
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Readable(a.palette.Warning))
			}
			if run.Script == "" {
				ui.Text(c, tr(c, "This run did not record a script, so there is nothing to save.")).FillWidth().Wrap().
					TextColor(a.palette.Readable(a.palette.Danger))
			}
			if state.errorText != "" {
				ui.Text(c, state.errorText).FillWidth().Wrap().TextColor(a.palette.Readable(a.palette.Danger))
			}
		})
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			textButton(c.Key("workflow-save-cancel:"+run.ID), tr(c, "Cancel")).Disabled(busy).OnClick(func() { state.open = false })
			primaryButton(c.Key("workflow-save-apply:"+run.ID), tr(c, "Save")).Disabled(!canSave || run.Script == "").OnClick(func() {
				a.saveWorkflowRun(run, state)
			})
		})
	})
}

// workflowSaveScopeChoices draws the two places a definition can live.
func (a *App) workflowSaveScopeChoices(c *ui.Context, run contracts.WorkflowRun, state *workflowSaveState) {
	choices := []struct{ value, label string }{
		{value: workflow.SavedScopeProject, label: tr(c, "This project")},
		{value: workflow.SavedScopeGlobal, label: tr(c, "This machine")},
	}
	if run.HostID != "" {
		// 远端运行不给全局档：家目录在宿主机器上，本机的中枢读不到它。
		choices = choices[:1]
	}
	ui.Row(c.Key("workflow-save-scopes:" + run.ID)).FillWidth().Gap(8).Children(func() {
		for _, choice := range choices {
			option := controlButton(c, choice.label, buttonSecondary).Grow(1)
			if state.scope == choice.value {
				option.Background(a.palette.Active)
			}
			chosen := choice.value
			option.OnClick(func() {
				state.scope = chosen
				a.refreshWorkflowSaveTaken(run, state)
			})
		}
	})
}

// refreshWorkflowSaveTaken 更新「这个名字已经被占用」的事实，据此决定提交是
// 新建还是覆盖。它不假装保存失败：真正的裁决仍在核心。
func (a *App) refreshWorkflowSaveTaken(run contracts.WorkflowRun, state *workflowSaveState) {
	name := strings.TrimSpace(state.name)
	state.taken = false
	if name == "" {
		return
	}
	state.taken = slices.ContainsFunc(a.state.WorkflowDefinitions, func(definition contracts.WorkflowDefinition) bool {
		return definition.Scope == state.scope && strings.EqualFold(definition.Name, name) && definition.ID != run.DefinitionID
	})
}

// saveWorkflowRun sends the definition of one finished run. It reads the
// script the run actually executed, so the saved definition is the one that
// ran, not a re-derivation.
func (a *App) saveWorkflowRun(run contracts.WorkflowRun, state *workflowSaveState) {
	definition := workflow.SavedDefinition(run, workflow.SaveDraft{
		Name:        state.name,
		Scope:       state.scope,
		Description: state.description,
	})
	if !workflow.ValidSavedName(definition.Name) {
		state.errorText = a.t("This name cannot be a file name.")
		return
	}
	if definition.Scope == workflow.SavedScopeProject && definition.ProjectID == "" {
		state.errorText = a.t("A project workflow needs a project.")
		return
	}
	if definition.Scope == workflow.SavedScopeGlobal {
		// 全局定义不属于任何项目：项目字段留着会把一份全局定义钉在某个项目上。
		definition.ProjectID = ""
	}
	if state.taken {
		// 覆盖写的是同一份定义：身份沿用，名字与内容更新。
		for _, existing := range a.state.WorkflowDefinitions {
			if existing.Scope == definition.Scope && strings.EqualFold(existing.Name, definition.Name) {
				definition.ID = existing.ID
				break
			}
		}
	}
	a.markWorkflowBusy(run.ID)
	state.open = false
	a.dispatch(contracts.Command{Kind: "workflow.save", Workflow: &contracts.WorkflowCommand{
		Definition: &contracts.WorkflowDefinitionSave{
			ID:          definition.ID,
			Name:        definition.Name,
			Description: definition.Description,
			Scope:       definition.Scope,
			ProjectID:   definition.ProjectID,
			HostID:      definition.HostID,
			Script:      definition.Script,
			Program:     definition.Program,
			Arguments:   definition.Arguments,
		},
	}})
}

// workflowCommandFinished 收拾工作流命令的界面状态：清掉在途标记，把失败
// 放回原表单，并在保存成功后播一次无障碍通知。
func (a *App) workflowCommandFinished(cmd contracts.Command, err error) {
	if cmd.Workflow == nil {
		return
	}
	switch cmd.Kind {
	case "workflow.save":
		a.clearWorkflowBusy()
		if err != nil {
			a.notification(toastError, err.Error())
			return
		}
		a.notification(toastSuccess, a.t("Workflow saved"))
	case "workflow.launch", "workflow.resume":
		a.clearWorkflowBusy()
		if err != nil {
			if a.workflowUI.hubLaunch != nil || cmd.Workflow != nil && cmd.Workflow.Run != nil && cmd.Workflow.Run.Origin == contracts.WorkflowOriginHub {
				a.workflowHubLaunchFailed(err)
				return
			}
			a.notification(toastError, err.Error())
		}
	}
}

// runWorkflowAgain starts a new run of the saved definition. It is not
// Resume: a new session and a new run, with the definition's identity and the
// run's own args carried over.
func (a *App) runWorkflowAgain(run contracts.WorkflowRun) {
	controller := a.workflowSaveOf(run)
	if !controller.saved {
		return
	}
	request := workflow.LaunchRequest(run, controller.definition, "")
	a.markWorkflowBusy(run.ID)
	a.dispatch(contracts.Command{Kind: "workflow.launch", Workflow: &contracts.WorkflowCommand{Run: &request}})
}

// resumeWorkflowRun resumes a stopped run that the core says can continue.
// Resume keeps the same run: settled phases stay, the interrupted one runs
// again.
func (a *App) resumeWorkflowRun(run contracts.WorkflowRun) {
	a.markWorkflowBusy(run.ID)
	a.dispatch(contracts.Command{Kind: "workflow.resume", Workflow: &contracts.WorkflowCommand{RunID: run.ID}})
}
