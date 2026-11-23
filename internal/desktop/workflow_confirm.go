package desktop

// 运行确认弹窗。脚本编译好之后不直接开跑：用户先看到规模、设置和脚本，
// 再决定这一次允不允许跑。拒绝时反馈交回主 Agent，运行不会假装开始过。

import (
	"strconv"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowConfirmState 是一次运行确认在界面上的草稿：用户可以在批准前
// 改模型与并发，也可以写一句拒绝理由。
type workflowConfirmState struct {
	open            bool
	modelID         string
	hasModel        bool
	concurrency     int
	concurrencyText string
	feedback        string
	showScript      bool
	busy            bool
	errorText       string
}

// workflowConfirmationDialog 画出所有还在等决定的运行确认。
func (a *App) workflowConfirmationDialog(c *ui.Context) {
	pending := map[string]bool{}
	for _, confirmation := range a.state.WorkflowConfirmations {
		if confirmation.Status != contracts.WorkflowConfirmationPending {
			continue
		}
		pending[confirmation.ID] = true
		a.workflowConfirmationSheet(c, confirmation)
	}
	for id := range a.workflowUI.confirm {
		if !pending[id] {
			delete(a.workflowUI.confirm, id)
		}
	}
}

// workflowConfirmForm 取出一次确认的界面草稿。
func (a *App) workflowConfirmForm(confirmation contracts.WorkflowConfirmation) *workflowConfirmState {
	if a.workflowUI.confirm == nil {
		a.workflowUI.confirm = map[string]*workflowConfirmState{}
	}
	state := a.workflowUI.confirm[confirmation.ID]
	if state == nil {
		state = &workflowConfirmState{
			open:        true,
			modelID:     confirmation.ModelID,
			concurrency: workflow.NormalizeConcurrency(confirmation.Concurrency),
		}
		a.workflowUI.confirm[confirmation.ID] = state
	}
	return state
}

// workflowConfirmationSheet 画一次运行确认。内容顺序固定：问句与名称、
// 修订关系、两项设置、参数、阶段线、脚本。
func (a *App) workflowConfirmationSheet(c *ui.Context, confirmation contracts.WorkflowConfirmation) {
	state := a.workflowConfirmForm(confirmation)
	if !state.open {
		return
	}
	width, height := c.Size()
	open := state.open
	ui.DialogBase(c.Key("workflow-confirm:"+confirmation.ID), &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .45))
		panel.Width(min(520, width-40)).MaxHeight(height - 40).Padding(20).Gap(14).Radius(12).
			Label(tr(c, "Run workflow"))
		panel.Background(design.FloatingSurface(a.palette, "opaque", false)).Border(1, a.palette.Border)
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			ui.Text(c, tr(c, "Run workflow")).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600).SingleLine()
			ui.Text(c, confirmation.Name).FillWidth().MinWidth(0).FontWeight(500).Wrap()
		})
		if confirmation.Question != "" {
			ui.Text(c, confirmation.Question).FillWidth().Wrap()
		}
		if confirmation.Revision == "amend" && confirmation.PreviousRun != "" {
			ui.Text(c, tr(c, "This revision replaces run %s.", confirmation.PreviousRun)).
				FillWidth().Wrap().FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		}
		summary := workflow.SummarizeProgram(confirmation.Program)
		ui.Text(c, tr(c, "%d phases, %d instances", summary.Phases, summary.Actors)).
			FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		ui.Scroll(c.Key("workflow-confirm-content:" + confirmation.ID)).FillWidth().MaxHeight(max(float32(140), height-320)).Gap(12).Children(func() {
			a.workflowConfirmSettings(c, confirmation, state)
			a.workflowConfirmArguments(c, confirmation)
			a.workflowConfirmDiagnostics(c, confirmation)
			a.workflowConfirmScript(c, confirmation, state)
		})
		if state.errorText != "" {
			ui.Text(c, state.errorText).FillWidth().Wrap().TextColor(a.palette.Readable(a.palette.Danger))
		}
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			controlTextArea(c.Key("workflow-confirm-feedback:"+confirmation.ID), &state.feedback).
				FillWidth().Height(48).Label(tr(c, "Feedback")).Placeholder(tr(c, "Tell the agent what to change…")).
				Disabled(state.busy)
			secondaryButton(c.Key("workflow-confirm-reject:"+confirmation.ID), tr(c, "Reject")).Disabled(state.busy).
				OnClick(func() { a.decideWorkflowConfirmation(confirmation, state, contracts.WorkflowDecisionReject) })
			secondaryButton(c.Key("workflow-confirm-once:"+confirmation.ID), tr(c, "Allow once")).Disabled(state.busy).
				OnClick(func() { a.decideWorkflowConfirmation(confirmation, state, contracts.WorkflowDecisionOnce) })
			primaryButton(c.Key("workflow-confirm-session:"+confirmation.ID), tr(c, "Allow for this session")).Disabled(state.busy).
				OnClick(func() { a.decideWorkflowConfirmation(confirmation, state, contracts.WorkflowDecisionSession) })
		})
	})
	state.open = open
}

// workflowConfirmSettings 画两项设置：实例模型与最大并发。
func (a *App) workflowConfirmSettings(c *ui.Context, confirmation contracts.WorkflowConfirmation, state *workflowConfirmState) {
	ui.Column(c.Key("workflow-confirm-settings:" + confirmation.ID)).FillWidth().Gap(6).Children(func() {
		ui.Text(c, tr(c, "Instance model")).FillWidth().FontWeight(500)
		a.workflowModelChoices(c, "workflow-confirm-models:"+confirmation.ID,
			a.workflowSessionModelByID(confirmation.SessionID), confirmation.ModelID, func(modelID string) {
				state.modelID, state.hasModel = modelID, true
			})
		ui.Text(c, tr(c, "Max concurrency")).FillWidth().FontWeight(500)
		ui.TextInput(c.Key("workflow-confirm-concurrency:"+confirmation.ID), &state.concurrencyText).
			FillWidth().Placeholder(strconv.Itoa(state.concurrency)).Label(tr(c, "Max concurrency"))
	})
}

// workflowConfirmArguments 画这次运行实际收到的参数。
func (a *App) workflowConfirmArguments(c *ui.Context, confirmation contracts.WorkflowConfirmation) {
	if len(confirmation.Args) == 0 {
		return
	}
	ui.Column(c.Key("workflow-confirm-arguments:" + confirmation.ID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Arguments")).FillWidth().FontWeight(500)
		for _, argument := range confirmation.Args {
			ui.Text(c, argument.Name+" ("+argument.Kind+"): "+argument.Text).
				FillWidth().Wrap().FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		}
	})
}

// workflowConfirmDiagnostics 画编译诊断。错误先说，警告后说。
func (a *App) workflowConfirmDiagnostics(c *ui.Context, confirmation contracts.WorkflowConfirmation) {
	if len(confirmation.Diagnostics) == 0 {
		return
	}
	ui.Column(c.Key("workflow-confirm-diagnostics:" + confirmation.ID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Diagnostics")).FillWidth().FontWeight(500)
		for _, diagnostic := range confirmation.Diagnostics {
			color := a.palette.Muted
			if diagnostic.Severity == contracts.WorkflowDiagnosticError {
				color = a.palette.Readable(a.palette.Danger)
			}
			ui.Text(c, diagnostic.Message).FillWidth().Wrap().
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(color)
		}
	})
}

// workflowConfirmScript 画脚本。有阶段线时脚本默认收起。
func (a *App) workflowConfirmScript(c *ui.Context, confirmation contracts.WorkflowConfirmation, state *workflowConfirmState) {
	if confirmation.Script == "" {
		return
	}
	ui.Column(c.Key("workflow-confirm-script:" + confirmation.ID)).FillWidth().Gap(6).Children(func() {
		glyph, label := "alt-arrow-down", tr(c, "Show script")
		if state.showScript {
			glyph, label = "alt-arrow-up", tr(c, "Hide script")
		}
		disclosureIconButton(c.Key("workflow-confirm-script-toggle:"+confirmation.ID), glyph, label).
			OnClick(func() { state.showScript = !state.showScript })
		if state.showScript {
			ui.Text(c, confirmation.Script).FillWidth().Wrap().Selectable().
				Font(a.state.Settings.Appearance.CodeFont).
				FontSize(design.TextSize(c, design.TextSmall)).
				Padding(8, 10).Background(a.palette.CodeWash).Radius(6)
		}
	})
}

// decideWorkflowConfirmation 把用户的决定交给核心。
func (a *App) decideWorkflowConfirmation(confirmation contracts.WorkflowConfirmation, state *workflowConfirmState, decision string) {
	payload := &contracts.WorkflowConfirmationDecision{
		ID:       confirmation.ID,
		Decision: decision,
		Approve:  decision != contracts.WorkflowDecisionReject,
		Feedback: strings.TrimSpace(state.feedback),
	}
	if state.hasModel {
		payload.ModelID, payload.HasModel = state.modelID, true
	}
	if state.concurrencyText != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(state.concurrencyText))
		if err != nil || parsed < 1 {
			state.errorText = a.t("The concurrency must be a whole number of at least one.")
			return
		}
		payload.Concurrency, payload.HasConc = parsed, true
	}
	state.busy = true
	state.open = false
	a.dispatch(contracts.Command{Kind: "workflow.confirm", Workflow: &contracts.WorkflowCommand{Confirmation: payload}})
}
