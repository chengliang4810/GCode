package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowHubState 是中枢自己的界面状态：搜索词、选中的定义、详情页的标签页
// 和待确认的删除。定义、运行与产物始终从核心读，界面不另存一份。
type workflowHubState struct {
	query      string
	selected   string
	tab        string
	removeOpen bool
	launch     *workflowLaunchState
}

// workflowProjectRefs 把项目快照折成中枢分组要用的三个事实：身份、名字、
// 是不是远端。远端身份来自设备，不看路径。
func (a *App) workflowProjectRefs() []workflow.ProjectRef {
	refs := make([]workflow.ProjectRef, 0, len(a.state.Projects))
	for _, project := range a.state.Projects {
		refs = append(refs, workflow.ProjectRef{ID: project.ID, Name: project.Name, Remote: footerCloudDevice(project.DeviceID)})
	}
	return refs
}

// workflowDefinition finds one saved definition in the snapshot.
func (a *App) workflowDefinition(id string) (contracts.WorkflowDefinition, bool) {
	for _, definition := range a.state.WorkflowDefinitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return contracts.WorkflowDefinition{}, false
}

// workflowHubView is the saved-workflow hub: the definitions grouped by
// project, and one definition's own page behind them. It lives in the
// automations page's workflow module, not in a settings section of its own.
func (a *App) workflowHubView(c *ui.Context) {
	s := &a.workflowUI.hub
	if s.tab == "" {
		s.tab = "definition"
	}
	if s.selected != "" {
		if definition, ok := a.workflowDefinition(s.selected); ok {
			a.workflowDefinitionDetail(c, definition)
			return
		}
		s.selected = ""
	}
	groups := workflow.GroupDefinitions(a.state.WorkflowDefinitions, a.workflowProjectRefs(), a.projectID)
	query := strings.ToLower(strings.TrimSpace(s.query))
	ui.Row(c.Key("workflow-hub-toolbar")).FillWidth().Gap(8).Children(func() {
		controlTextInput(c.Key("workflow-hub-search"), &s.query).Grow(1).Label(tr(c, "Search workflows")).
			Placeholder(tr(c, "Search"))
		primaryButton(c.Key("workflow-hub-new"), tr(c, "New workflow")).OnClick(func() { a.newWorkflowConversation() })
	})
	a.workflowModeLine(c)
	shown := 0
	for _, group := range groups.Projects {
		shown += a.workflowHubGroup(c, group.Name, workflowGroupNotes(c, group), group.Definitions, query)
	}
	if len(groups.Global) > 0 {
		shown += a.workflowHubGroup(c, tr(c, "This machine"), nil, groups.Global, query)
	}
	if groups.Count() == 0 {
		ui.Text(c, tr(c, "No saved workflows yet")).FillWidth().Wrap().TextColor(a.palette.Muted)
		return
	}
	if shown == 0 {
		ui.Text(c, tr(c, "No workflows match this search")).FillWidth().Wrap().TextColor(a.palette.Muted)
	}
}

// workflowGroupNotes is what a project group says about itself: whether it is
// the project the user is looking at, and whether it lives on another host.
func workflowGroupNotes(c *ui.Context, group workflow.DefinitionProjectGroup) []string {
	var notes []string
	if group.Current {
		notes = append(notes, tr(c, "Current project"))
	}
	if group.Remote {
		notes = append(notes, tr(c, "Remote host"))
	}
	if group.Gone {
		notes = append(notes, tr(c, "Project not found"))
	}
	return notes
}

// workflowHubGroup draws one heading and its definitions. It reports how many
// rows the search kept.
func (a *App) workflowHubGroup(c *ui.Context, title string, notes []string, definitions []contracts.WorkflowDefinition, query string) int {
	kept := make([]contracts.WorkflowDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if !matchesResource(query, definition.Name, definition.Description) {
			continue
		}
		kept = append(kept, definition)
	}
	if len(kept) == 0 {
		if len(definitions) == 0 {
			return 0
		}
		// A group the search emptied still names itself: the list did not
		// lose the project, the search hid it.
		return 0
	}
	ui.Column(c.Key("workflow-hub-group:" + strings.ToLower(title))).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontWeight(600).SingleLine()
			for _, note := range notes {
				ui.Text(c, note).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
			}
			ui.Text(c, fmt.Sprintf("%d", len(kept))).FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.palette.Muted).FontFeatures("tnum")
		})
		for _, definition := range kept {
			a.workflowHubRow(c, definition)
		}
	})
	return len(kept)
}

// workflowHubRow is one saved definition: the whole row opens its page, the
// Run button next to it does not. Buttons never bubble into the row.
func (a *App) workflowHubRow(c *ui.Context, definition contracts.WorkflowDefinition) {
	ui.Row(c.Key("workflow-hub-row:"+definition.ID)).FillWidth().AlignItems(ui.Center).Gap(8).
		Padding(8, 4).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
		controlRowButton(c.Key("workflow-hub-open:" + definition.ID)).Grow(1).MinWidth(0).
			Label(tr(c, "Open workflow") + " " + definition.Name).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, definition.Name).FontWeight(500).SingleLine()
				if definition.Description != "" {
					ui.Text(c, definition.Description).FontSize(design.TextSize(c, design.TextBase)).
						TextColor(a.palette.Muted).MaxLines(2)
				}
				a.workflowHubRowFacts(c, definition)
			})
		}).OnClick(func() {
			a.workflowUI.hub.selected = definition.ID
			a.workflowUI.hub.removeOpen = false
		})
		textButton(c.Key("workflow-hub-run:"+definition.ID), tr(c, "Run workflow")).OnClick(func() {
			a.startWorkflowLaunch(definition, nil)
		})
	})
}

// workflowHubRowFacts is the third line of a row: the declared arguments and
// the newest run's state.
func (a *App) workflowHubRowFacts(c *ui.Context, definition contracts.WorkflowDefinition) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if names, overflow := workflow.ArgumentPreview(definition.Arguments, 3); len(names) > 0 {
			line := strings.Join(names, ", ")
			if overflow > 0 {
				line += fmt.Sprintf(" +%d", overflow)
			}
			ui.Text(c, line).FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.palette.Muted).SingleLine()
		}
		if run, ok := workflow.DefinitionLastRun(definition.ID, a.state.WorkflowRuns); ok {
			view := workflow.ProjectRun(run)
			ui.Text(c, a.workflowRunKindWord(c, view)).FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.workflowRunStatusColor(view)).SingleLine()
		} else {
			ui.Text(c, tr(c, "Never run")).FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.palette.Muted).SingleLine()
		}
	})
}

// workflowDefinitionDetail is one definition's own page: what it is, what it
// declares, the script it runs, and the runs it produced.
func (a *App) workflowDefinitionDetail(c *ui.Context, definition contracts.WorkflowDefinition) {
	s := &a.workflowUI.hub
	ui.Row(c.Key("workflow-detail-head:" + definition.ID)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		textButton(c.Key("workflow-detail-back"), tr(c, "Back to workflows")).OnClick(func() {
			s.selected, s.removeOpen = "", false
		})
	})
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, definition.Name).Grow(1).MinWidth(0).
			FontSize(design.TextSize(c, design.TextXL)).FontWeight(600).SingleLine()
		ui.Text(c, a.workflowScopeLabel(c, definition)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Muted).SingleLine()
		primaryButton(c.Key("workflow-detail-run"), tr(c, "Run workflow")).OnClick(func() {
			a.startWorkflowLaunch(definition, nil)
		})
		textButton(c.Key("workflow-detail-copy"), tr(c, "Copy path")).OnClick(func() {
			c.WriteClipboard(workflow.SavedPathPreview(definition.Scope, definition.Name))
			a.notification(toastInfo, a.t("Path copied"))
		})
		if definition.Scope == workflow.SavedScopeProject {
			textButton(c.Key("workflow-detail-promote"), tr(c, "Promote to this machine")).OnClick(func() {
				a.promoteWorkflowDefinition(definition)
			})
		}
		dangerButton(c.Key("workflow-detail-remove"), tr(c, "Delete workflow")).OnClick(func() {
			s.removeOpen = true
		})
	})
	if s.removeOpen {
		a.workflowDefinitionRemoveConfirm(c, definition)
	}
	a.workflowDetailTabs(c, definition)
	if s.tab == "history" {
		a.workflowDefinitionHistory(c, definition)
		return
	}
	a.workflowDefinitionFacts(c, definition)
	a.workflowDefinitionArguments(c, definition)
	a.workflowDefinitionScript(c, definition)
	a.workflowDefinitionArtifacts(c, definition)
}

// promoteWorkflowDefinition copies a project definition into this machine's
// group. Promotion is its own action, not a scope field flipped in place: the
// project keeps the definition it owns, and the copy starts without one.
func (a *App) promoteWorkflowDefinition(definition contracts.WorkflowDefinition) {
	save := contracts.WorkflowDefinitionSave{
		Name:        definition.Name,
		Description: definition.Description,
		Scope:       workflow.SavedScopeGlobal,
		Script:      definition.Script,
		Program:     definition.Program,
		Arguments:   definition.Arguments,
	}
	a.dispatch(contracts.Command{Kind: "workflow.save", Workflow: &contracts.WorkflowCommand{Definition: &save}})
	a.notification(toastSuccess, a.t("Workflow promoted to this machine"))
}

// workflowScopeLabel says where a definition lives: its project, or this
// machine.
func (a *App) workflowScopeLabel(c *ui.Context, definition contracts.WorkflowDefinition) string {
	if definition.Scope == workflow.SavedScopeGlobal {
		return tr(c, "This machine")
	}
	for _, project := range a.state.Projects {
		if project.ID == definition.ProjectID {
			return project.Name
		}
	}
	return definition.ProjectID
}

// workflowDefinitionRemoveConfirm asks before a definition is deleted.
func (a *App) workflowDefinitionRemoveConfirm(c *ui.Context, definition contracts.WorkflowDefinition) {
	ui.Column(c.Key("workflow-detail-remove-confirm:" + definition.ID)).FillWidth().Gap(8).
		Padding(10).Radius(8).Background(a.palette.Raised).Children(func() {
		ui.Text(c, fmt.Sprintf(tr(c, "Remove “%s”?"), definition.Name)).FillWidth().Wrap()
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			textButton(c.Key("workflow-detail-remove-cancel"), tr(c, "Cancel")).OnClick(func() {
				a.workflowUI.hub.removeOpen = false
			})
			dangerButton(c.Key("workflow-detail-remove-yes"), tr(c, "Delete workflow")).OnClick(func() {
				a.workflowUI.hub.removeOpen = false
				a.workflowUI.hub.selected = ""
				a.dispatch(contracts.Command{Kind: "workflow.delete", Workflow: &contracts.WorkflowCommand{ID: definition.ID}})
			})
		})
	})
}

// workflowDetailTabs switches between the definition and its runs.
func (a *App) workflowDetailTabs(c *ui.Context, definition contracts.WorkflowDefinition) {
	s := &a.workflowUI.hub
	ui.Row(c.Key("workflow-detail-tabs:" + definition.ID)).FillWidth().Gap(6).Children(func() {
		for _, tab := range []struct{ key, label string }{{"definition", "Definition"}, {"history", "History"}} {
			tab := tab
			controlNavButton(c.Key("workflow-detail-tab-"+tab.key), tr(c, tab.label), s.tab == tab.key).
				Children(func() { ui.Text(c, tr(c, tab.label)) }).
				OnClick(func() { s.tab = tab.key })
		}
	})
}

// workflowDefinitionFacts is the metadata line: what it declares, when it
// changed, how often it ran.
func (a *App) workflowDefinitionFacts(c *ui.Context, definition contracts.WorkflowDefinition) {
	runs := workflow.DefinitionRuns(definition.ID, a.state.WorkflowRuns, 0)
	parts := []string{tr(c, "%d phases", len(definition.Program.Phases)), tr(c, "%d runs", len(runs))}
	if !definition.UpdatedAt.IsZero() {
		parts = append(parts, tr(c, "Updated")+" "+i18n.Date(a.state.Settings.General.Language, definition.UpdatedAt, "Jan 2, 3:04 PM"))
	}
	ui.Column(c.Key("workflow-detail-facts:" + definition.ID)).FillWidth().Gap(4).Children(func() {
		if definition.Description != "" {
			ui.Text(c, definition.Description).FillWidth().Wrap()
		}
		ui.Row(c).FillWidth().Gap(10).Wrap().Children(func() {
			for _, part := range parts {
				ui.Text(c, part).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
			}
		})
	})
}

// workflowDefinitionArguments lists what a launch asks for, in declaration
// order. The declaration is metadata, so it stays apart from the script.
func (a *App) workflowDefinitionArguments(c *ui.Context, definition contracts.WorkflowDefinition) {
	if len(definition.Arguments) == 0 {
		return
	}
	ui.Column(c.Key("workflow-detail-arguments:" + definition.ID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Arguments")).FontWeight(600)
		for _, argument := range definition.Arguments {
			ui.Row(c.Key("workflow-detail-argument:" + definition.ID + ":" + argument.Name)).FillWidth().
				Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, argument.Name).FontWeight(500).SingleLine()
				ui.Text(c, argument.Type).FontSize(design.TextSize(c, design.TextSmall)).
					TextColor(a.palette.Muted).SingleLine()
				if argument.Required {
					ui.Text(c, tr(c, "Required")).FontSize(design.TextSize(c, design.TextSmall)).
						TextColor(a.palette.Muted).SingleLine()
				}
				if argument.HasDefault {
					ui.Text(c, tr(c, "Default")+": "+argument.Default).
						FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
				}
				if argument.Description != "" {
					ui.Text(c, argument.Description).Grow(1).MinWidth(0).
						FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
				}
			})
		}
	})
}

// workflowDefinitionScript shows the script read only: the hub is not an
// editor, and a definition is revised by talking to the agent.
func (a *App) workflowDefinitionScript(c *ui.Context, definition contracts.WorkflowDefinition) {
	if strings.TrimSpace(definition.Script) == "" {
		return
	}
	ui.Column(c.Key("workflow-detail-script:" + definition.ID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Script")).FontWeight(600)
		ui.Text(c, definition.Script).FillWidth().Selectable().Wrap().
			Font(a.state.Settings.Appearance.CodeFont).
			FontSize(design.TextSize(c, design.TextSmall)).
			Padding(10).Background(a.palette.CodeWash).Radius(8)
	})
}

// workflowDefinitionArtifacts shows what the most recent successful run
// delivered. A newer failure does not erase it.
func (a *App) workflowDefinitionArtifacts(c *ui.Context, definition contracts.WorkflowDefinition) {
	slice := workflow.DefinitionArtifacts(definition.ID, a.state.WorkflowRuns)
	if len(slice.Shown) == 0 {
		return
	}
	ui.Column(c.Key("workflow-detail-artifacts:" + definition.ID)).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Recent artifacts")).FontWeight(600)
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			for _, artifact := range slice.Shown {
				artifact := artifact
				textButton(c.Key("workflow-detail-artifact:"+definition.ID+":"+artifact.ID), artifact.Name).OnClick(func() {
					// 打开交付它的那一次运行，不是最近的那一次。
					if runID, ok := a.workflowArtifactRun(definition.ID, artifact.ID); ok {
						a.openWorkflowArtifact(runID, artifact)
					}
				})
			}
			if slice.Overflow > 0 {
				ui.Text(c, fmt.Sprintf(tr(c, "+%d"), slice.Overflow)).
					FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).FontFeatures("tnum")
			}
		})
	})
}

// workflowArtifactRun finds the run that published one artifact.
func (a *App) workflowArtifactRun(definitionID, artifactID string) (string, bool) {
	for _, run := range workflow.DefinitionRuns(definitionID, a.state.WorkflowRuns, 0) {
		for _, artifact := range run.Artifacts {
			if artifact.ID == artifactID {
				return run.ID, true
			}
		}
	}
	return "", false
}

// workflowDefinitionHistory lists the runs this definition produced, newest
// first. Choosing one opens its run detail.
func (a *App) workflowDefinitionHistory(c *ui.Context, definition contracts.WorkflowDefinition) {
	runs := workflow.DefinitionRuns(definition.ID, a.state.WorkflowRuns, 50)
	if len(runs) == 0 {
		ui.Text(c, tr(c, "This workflow has not run yet")).FillWidth().Wrap().TextColor(a.palette.Muted)
		return
	}
	ui.Column(c.Key("workflow-detail-history:" + definition.ID)).FillWidth().Gap(2).Children(func() {
		for _, run := range runs {
			run := run
			view := workflow.ProjectRun(run)
			controlRowButton(c.Key("workflow-detail-history-row:" + run.ID)).FillWidth().
				Label(tr(c, "Open run details")).Children(func() {
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.workflowRunKindWord(c, view)).FontSize(design.TextSize(c, design.TextSmall)).
						TextColor(a.workflowRunStatusColor(view)).SingleLine()
					ui.Text(c, a.workflowRunStartedLabel(run)).FontSize(design.TextSize(c, design.TextSmall)).
						TextColor(a.palette.Muted).SingleLine()
					ui.Spacer(c).Grow(1)
					ui.Text(c, a.workflowElapsed(view)).FontSize(design.TextSize(c, design.TextSmall)).
						TextColor(a.palette.Muted).FontFeatures("tnum").SingleLine()
				})
			}).OnClick(func() { a.openWorkflowRun(run.ID) })
		}
	})
}

// workflowRunStartedLabel is when a run started, in the interface language.
func (a *App) workflowRunStartedLabel(run contracts.WorkflowRun) string {
	if run.StartedAt.IsZero() {
		return "—"
	}
	return i18n.Date(a.state.Settings.General.Language, run.StartedAt, "Jan 2, 3:04 PM")
}

// workflowModeLine says which mode is in force right now. It is the effective
// mode, not the value a file happens to hold.
func (a *App) workflowModeLine(c *ui.Context) {
	mode := a.state.Settings.WorkflowMode
	word := "On demand"
	switch mode {
	case contracts.WorkflowModeDisabled:
		word = "Off"
	case contracts.WorkflowModeAlwaysOn:
		word = "Always on"
	}
	ui.Row(c.Key("workflow-hub-mode")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tr(c, "Mode")+": "+tr(c, word)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Muted).SingleLine()
		textButton(c.Key("workflow-hub-mode-settings"), tr(c, "Open settings")).OnClick(func() {
			a.selectSettingsSection("General")
		})
	})
}

// newWorkflowConversation starts an empty conversation with the workflow
// command already typed. A definition is written by talking to the agent, so
// this is the hub's one creation entry.
func (a *App) newWorkflowConversation() {
	a.newConversation()
	a.draft = "/workflow "
	a.drafts[""] = a.draft
}

// localProjectChoices lists the projects a global definition may run on. A
// global definition belongs to this machine, so a remote project is not a
// candidate: its path is not a path here.
func (a *App) localProjectChoices() []settingOption {
	options := make([]settingOption, 0, len(a.state.Projects))
	for _, project := range a.state.Projects {
		if footerCloudDevice(project.DeviceID) {
			continue
		}
		options = append(options, settingOption{value: project.ID, label: project.Name})
	}
	return options
}
