package desktop

import (
	"crypto/rand"
	"fmt"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowLaunchState is the draft of one launch from the hub. It holds what
// the user typed; what the launch sends is decided by the workflow rules.
type workflowLaunchState struct {
	definitionID string
	// projectID is the project a global definition runs on. A project
	// definition keeps its own and never shows a picker.
	projectID string
	// values are the fields themselves: each one is a stable pointer, so what
	// the user types survives the frame that drew it.
	values    map[string]*string
	issue     *workflow.LaunchIssue
	errorText string
	pending   bool
	// direct marks a launch that never asked anything:它不画表单。
	direct bool
	// reveal is the field the form scrolls to after a refusal: the first bad
	// one in declaration order.
	reveal bool
}

// workflowLaunchPending is the launch in flight. A launch needs a conversation
// to run in, and the core creates that conversation asynchronously, so the
// hub remembers what it asked for until the run exists.
type workflowLaunchPending struct {
	request   contracts.WorkflowRunRequest
	requestID string
	sessionID string
	// dispatched is set once the launch command left the interface thread.
	dispatched bool
	// dialog says whether this launch came from the form, so a refusal can
	// go back into it instead of a toast alone.
	dialog bool
}

// startWorkflowLaunch opens the launch form, or starts straight away when the
// definition asks for nothing and the project is already settled.
func (a *App) startWorkflowLaunch(definition contracts.WorkflowDefinition, run *contracts.WorkflowRun) {
	a.errorText = ""
	prefill := workflow.LaunchPrefill(definition, run)
	values := make(map[string]*string, len(prefill))
	for name, text := range prefill {
		text := text
		values[name] = &text
	}
	state := &workflowLaunchState{
		definitionID: definition.ID,
		projectID:    definition.ProjectID,
		values:       values,
	}
	if definition.Scope == workflow.SavedScopeGlobal || state.projectID == "" {
		// 全局定义不绑定项目：总是先选本机项目。
		state.projectID = ""
	}
	a.workflowUI.hub.launch = state
	if len(definition.Arguments) == 0 && state.projectID != "" && definition.Scope != workflow.SavedScopeGlobal {
		// 项目定义、没有参数：直接起，不弹一张空表单。
		state.direct = true
		a.launchWorkflow(state, definition)
	}
}

// workflowLaunchDialog draws the launch form. Nothing is drawn when there is
// nothing to ask.
func (a *App) workflowLaunchDialog(c *ui.Context) {
	state := a.workflowUI.hub.launch
	if state == nil || state.direct {
		return
	}
	definition, ok := a.workflowDefinition(state.definitionID)
	if !ok {
		a.workflowUI.hub.launch = nil
		return
	}
	open := true
	a.settingsDialogWithFooter(c, "workflow-launch-dialog", "Run workflow", &open, 480, func() {
		a.workflowLaunchFields(c, definition, state)
	}, func() {
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c.Key("workflow-launch-cancel"), tr(c, "Cancel")).Disabled(state.pending).OnClick(func() {
				a.workflowUI.hub.launch = nil
			})
			caption := tr(c, "Run workflow")
			if state.pending {
				caption = tr(c, "Starting…")
			}
			primaryButton(c.Key("workflow-launch-start"), caption).Disabled(state.pending).OnClick(func() {
				a.launchWorkflow(state, definition)
			})
		})
	}, state.pending)
	if !open && !state.pending {
		a.workflowUI.hub.launch = nil
	}
}

// workflowLaunchFields is the middle of the form: where it runs, then every
// declared argument in declaration order.
func (a *App) workflowLaunchFields(c *ui.Context, definition contracts.WorkflowDefinition, state *workflowLaunchState) {
	ui.Column(c.Key("workflow-launch-target")).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Project")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		if definition.Scope == workflow.SavedScopeGlobal {
			options := a.localProjectChoices()
			if len(options) == 0 {
				ui.Text(c, tr(c, "Add a project before running a global workflow to it.")).
					FillWidth().Wrap().TextColor(a.palette.Readable(a.palette.Danger))
				return
			}
			a.resourceSelect(c, "workflow-launch-project", "Project", &state.projectID, options)
			return
		}
		ui.Text(c, a.workflowScopeLabel(c, definition)).TextColor(a.palette.Muted).SingleLine()
	})
	for _, argument := range definition.Arguments {
		a.workflowLaunchField(c, definition, state, argument)
	}
	if issue := state.issue; issue != nil {
		ui.Text(c, a.workflowLaunchIssueText(c, issue)).FillWidth().Wrap().
			TextColor(a.palette.Readable(a.palette.Danger))
	}
	if state.errorText != "" {
		ui.Text(c, state.errorText).FillWidth().Wrap().TextColor(a.palette.Readable(a.palette.Danger))
	}
}

// workflowLaunchField draws one declared argument. The type decides the
// control: a string or number is one line, a boolean is a switch, and JSON
// gets room to breathe.
func (a *App) workflowLaunchField(c *ui.Context, definition contracts.WorkflowDefinition, state *workflowLaunchState, argument contracts.WorkflowArgument) {
	key := "workflow-launch-arg:" + definition.ID + ":" + argument.Name
	label := argument.Name
	if argument.Required {
		label += " *"
	}
	value := state.values[argument.Name]
	if value == nil {
		text := argument.Default
		value = &text
		state.values[argument.Name] = value
	}
	field := ui.Column(c.Key(key)).FillWidth().Gap(4)
	if state.reveal && state.issue != nil && state.issue.Name == argument.Name {
		field.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter})
	}
	field.Children(func() {
		ui.Text(c, label).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		switch argument.Type {
		case "boolean":
			checked := *value == "true"
			a.controlSwitch(c.Key(key+":switch"), &checked, label, state.pending, func(next bool) {
				*value = fmt.Sprintf("%t", next)
			})
		case "json":
			controlTextArea(c.Key(key+":json"), value).FillWidth().Height(96).Label(label).
				Placeholder(tr(c, "JSON"))
		default:
			input := controlTextInput(c.Key(key+":input"), value).FillWidth().Label(label)
			if argument.Required && state.reveal && state.issue != nil && state.issue.Name == argument.Name {
				input.Focus()
			}
		}
		if argument.Description != "" {
			ui.Text(c, argument.Description).FillWidth().Wrap().
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		}
	})
}

// workflowLaunchIssueText turns a refusal into a sentence next to its field.
func (a *App) workflowLaunchIssueText(c *ui.Context, issue *workflow.LaunchIssue) string {
	switch issue.Kind {
	case workflow.LaunchIssueRequired:
		return fmt.Sprintf(tr(c, "%s is required."), issue.Name)
	case workflow.LaunchIssueNumber:
		return fmt.Sprintf(tr(c, "%s must be a finite number."), issue.Name)
	case workflow.LaunchIssueJSON:
		return fmt.Sprintf(tr(c, "%s must be valid JSON."), issue.Name)
	default:
		return fmt.Sprintf(tr(c, "%s is not a declared argument."), issue.Name)
	}
}

// launchWorkflow validates the form and starts the run in a fresh
// conversation. A refusal stays in the form with the first bad field in view.
func (a *App) launchWorkflow(state *workflowLaunchState, definition contracts.WorkflowDefinition) {
	if state.pending {
		return
	}
	values := make(map[string]string, len(state.values))
	for name, text := range state.values {
		if text != nil {
			values[name] = *text
		}
	}
	args, issue := workflow.LaunchArgs(definition, values)
	if issue != nil {
		state.issue, state.reveal = issue, true
		return
	}
	projectID := definition.ProjectID
	if definition.Scope == workflow.SavedScopeGlobal {
		projectID = state.projectID
		if projectID == "" {
			state.errorText = a.t("Choose a project to run this workflow in.")
			return
		}
	}
	state.issue, state.reveal, state.errorText = nil, false, ""
	request := contracts.WorkflowRunRequest{
		DefinitionID: definition.ID,
		Name:         definition.Name,
		ProjectID:    projectID,
		HostID:       definition.HostID,
		Args:         args,
		Origin:       contracts.WorkflowOriginHub,
	}
	a.workflowUI.hubLaunch = &workflowLaunchPending{
		request:   request,
		requestID: "workflow-launch-" + rand.Text(),
		dialog:    len(definition.Arguments) > 0 || definition.Scope == workflow.SavedScopeGlobal,
	}
	state.pending = true
	a.dispatch(contracts.Command{Kind: "session.create", ProjectID: projectID, RequestID: a.workflowUI.hubLaunch.requestID})
}

// syncWorkflowLaunch carries a hub launch through its two steps: the
// conversation the core created, then the run itself. It runs on every
// snapshot, so a coalesced event cannot lose the launch.
func (a *App) syncWorkflowLaunch() {
	pending := a.workflowUI.hubLaunch
	if pending == nil {
		return
	}
	if pending.sessionID == "" {
		for _, session := range a.state.Sessions {
			if session.CreationRequestID != "" && session.CreationRequestID == pending.requestID {
				pending.sessionID = session.ID
				break
			}
		}
		if pending.sessionID == "" {
			return
		}
	}
	if !pending.dispatched {
		pending.dispatched = true
		request := pending.request
		request.SessionID = pending.sessionID
		a.dispatch(contracts.Command{Kind: "workflow.launch", Workflow: &contracts.WorkflowCommand{Run: &request}})
		return
	}
	// 运行出现：把它摆到眼前，然后结束这次待办。
	for _, run := range a.state.WorkflowRuns {
		if run.SessionID != pending.sessionID || run.DefinitionID != pending.request.DefinitionID {
			continue
		}
		a.workflowUI.hubLaunch = nil
		a.workflowUI.hub.launch = nil
		a.selectSession(pending.sessionID)
		a.workspace.OpenWorkflow(run.ID, run.Name)
		return
	}
}

// workflowHubLaunchFailed finishes a refused launch: the form keeps the
// reason, and the empty conversation it created is reclaimed. Reclaiming is
// best effort by nature, so a refusal there changes nothing the user sees.
func (a *App) workflowHubLaunchFailed(err error) {
	pending := a.workflowUI.hubLaunch
	if pending == nil {
		return
	}
	a.workflowUI.hubLaunch = nil
	if state := a.workflowUI.hub.launch; state != nil {
		state.pending = false
		state.errorText = err.Error()
		if !pending.dialog {
			a.notification(toastError, err.Error())
			a.workflowUI.hub.launch = nil
		}
	} else {
		a.notification(toastError, err.Error())
	}
	if pending.sessionID != "" {
		a.dispatch(contracts.Command{Kind: "session.delete", ID: pending.sessionID})
	}
}
