package desktop

import (
	"crypto/rand"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) editSubagent(profile *contracts.SubagentProfile) {
	a.errorText = ""
	state := &a.runtimeUI
	if state.subagentPendingID != "" {
		return
	}
	state.subagentOpen, state.subagentExisting = true, profile != nil
	state.subagent = contracts.SubagentProfile{ID: "agent-" + rand.Text()}
	state.subagentModel, state.subagentReasoning = "", ""
	state.subagentError, state.subagentFieldErrors = "", nil
	state.subagentToolsCustom = false
	if profile != nil {
		state.subagent = contracts.CloneSubagentProfiles([]contracts.SubagentProfile{*profile})[0]
		state.subagentToolsCustom = len(profile.Tools) > 0
		if profile.Model != nil {
			state.subagentModel, state.subagentReasoning = profile.Model.ModelID, profile.Model.Reasoning
		}
	}
}

func (a *App) subagentSelectedModel() contracts.Model {
	return a.subagentModelByID(a.runtimeUI.subagentModel)
}

func (a *App) subagentModelByID(id string) contracts.Model {
	for _, candidate := range a.state.Models {
		if candidate.ID == id {
			return candidate
		}
	}
	return contracts.Model{}
}

func (a *App) subagentModelChanged() {
	state := &a.runtimeUI
	delete(state.subagentFieldErrors, "model")
	state.subagentReasoning = defaultSubagentReasoning(a.subagentSelectedModel())
}

func defaultSubagentReasoning(model contracts.Model) string {
	if levels := model.ReasoningLevels; len(levels) > 0 {
		return levels[len(levels)-1]
	}
	return ""
}

func (a *App) subagentSelectionValid() bool {
	state := &a.runtimeUI
	if state.subagentModel == "" {
		return true
	}
	selected := a.subagentSelectedModel()
	if selected.ID == "" {
		return false
	}
	if len(selected.ReasoningLevels) == 0 {
		return state.subagentReasoning == ""
	}
	return slices.Contains(selected.ReasoningLevels, state.subagentReasoning)
}

func (a *App) subagentSettings(c *ui.Context) {

	query := a.customSubagentToolbar(c)
	a.customSubagentList(c, query)
	for _, profile := range contracts.EffectiveSubagentProfiles(a.state.Settings.Subagents) {
		if !contracts.IsBuiltinSubagent(profile.ID) {
			continue
		}
		name, description := a.subagentDisplayText(profile)
		if !matchesResource(query, profile.ID, profile.Name, profile.Description, name, description) {
			continue
		}
		ui.Row(c.Key("subagent-row-"+profile.ID)).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Padding(10, 4).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
			controlRowButton(c).Grow(1).Width(180).MinWidth(0).Label(tr(c, "Edit subagent") + " " + name).OnClick(func() { a.editSubagent(&profile) }).Children(func() {
				ui.Column(c).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, name).FontWeight(500)
						if contracts.IsBuiltinSubagent(profile.ID) {
							ui.Text(c, tr(c, "Built-in")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
						}
					})
					ui.Text(c, description).MinWidth(0).Wrap().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				})
			})
			modelID, reasoning := "", ""
			if profile.Model != nil {
				modelID, reasoning = profile.Model.ModelID, profile.Model.Reasoning
			}
			saveSelection := func() {
				profile.Model = nil
				if modelID != "" {
					profile.Model = &contracts.ModelSelection{ModelID: modelID, Reasoning: reasoning}
				}
				a.storeSubagentProfile(profile)
				a.saveSettings()
			}
			a.renderSubagentModelPicker(c, &modelID, name, 180, func() {
				reasoning = defaultSubagentReasoning(a.subagentModelByID(modelID))
				saveSelection()
			})
			if modelID != "" {
				a.renderSubagentReasoning(c, a.subagentModelByID(modelID).ReasoningLevels, &reasoning, name, 112, saveSelection)
			}
			enabled := !profile.Disabled
			a.controlSwitch(c.Key("enabled"), &enabled, tr(c, "Enable")+" "+name, false, func(enabled bool) {
				profile.Disabled = !enabled
				a.storeSubagentProfile(profile)
				a.saveSettings()
			})
		})
	}
	if a.runtimeUI.subagentOpen {
		if contracts.IsBuiltinSubagent(a.runtimeUI.subagent.ID) {
			a.subagentSettingsForm(c)
		} else {
			a.customSubagentForm(c)
		}
	}
}

func (a *App) subagentDisplayText(profile contracts.SubagentProfile) (string, string) {
	for _, builtin := range contracts.DefaultSubagentProfiles() {
		if profile.ID == builtin.ID {
			name, description := profile.Name, profile.Description
			if name == builtin.Name {
				name = a.t(name)
			}
			if description == builtin.Description {
				description = a.t(description)
			}
			return name, description
		}
	}
	return profile.Name, profile.Description
}

func (a *App) storeSubagentProfile(profile contracts.SubagentProfile) {
	profiles := contracts.CloneSubagentProfiles(a.state.Settings.Subagents)
	index := slices.IndexFunc(profiles, func(candidate contracts.SubagentProfile) bool { return candidate.ID == profile.ID })
	if index < 0 {
		profiles = append(profiles, profile)
	} else {
		profiles[index] = profile
	}
	a.state.Settings.Subagents = profiles
}

func (a *App) subagentSettingsForm(c *ui.Context) {
	state := &a.runtimeUI
	title := "Add subagent"
	if state.subagentExisting {
		title = "Edit subagent"
	}
	a.settingsEditorDialog(c, "subagent-editor-dialog", title, &state.subagentOpen, a.capabilityUI.pendingSubagentSave != nil, 800, a.errorText, func() {
		a.settingsCard(c, func() {
			ui.Column(c).FillWidth().Padding(16).Gap(10).Children(func() {
				controlTextInput(c.Key("subagent-name"), &state.subagent.Name).Label(tr(c, "Subagent name")).Placeholder(tr(c, "Name"))
				controlTextInput(c.Key("subagent-description"), &state.subagent.Description).Label(tr(c, "Subagent description")).Placeholder(tr(c, "Description"))
				controlTextArea(c.Key("subagent-prompt"), &state.subagent.Prompt).Label(tr(c, "Subagent prompt")).Placeholder(tr(c, "Prompt")).Height(140)
				useProject := !state.subagent.SkipProjectInstructions
				controlCheckbox(c.Key("subagent-project-instructions"), &useProject, tr(c, "Use project instructions")).OnClick(func() { state.subagent.SkipProjectInstructions = !useProject })
				ui.Text(c, tr(c, "Available tools")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				all := len(state.subagent.Tools) == 0
				controlCheckbox(c.Key("subagent-all-tools"), &all, tr(c, "All default tools")).OnClick(func() {
					if all {
						state.subagent.Tools = nil
					} else {
						state.subagent.Tools = []string{"Read", "ListDirectory", "Glob", "Grep"}
					}
				})
				tools := []string{"Read", "ListDirectory", "Glob", "Grep", "Bash", "Write", "Edit", "AskUserQuestion"}
				if contracts.IsReadOnlySubagent(state.subagent.ID) {
					tools = tools[:4]
					ui.Text(c, tr(c, "This built-in agent can only inspect files.")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				}
				for _, group := range [][]string{tools[:4], tools[4:]} {
					ui.Row(c).Gap(12).Children(func() {
						for _, name := range group {
							enabled := len(state.subagent.Tools) == 0 || slices.Contains(state.subagent.Tools, name)
							controlCheckbox(c.Key("subagent-tool-"+name), &enabled, name).Label(tr(c, "Available tools") + " " + name).Disabled(all).OnClick(func() {
								if enabled {
									if !slices.Contains(state.subagent.Tools, name) {
										state.subagent.Tools = append(state.subagent.Tools, name)
									}
								} else if len(state.subagent.Tools) > 1 {
									state.subagent.Tools = slices.DeleteFunc(state.subagent.Tools, func(candidate string) bool { return candidate == name })
								}
							})
						}
					})
				}
				a.subagentModelPicker(c)
				a.renderSubagentReasoning(c, a.subagentSelectedModel().ReasoningLevels, &state.subagentReasoning, "", 0, func() {})
				if !a.subagentSelectionValid() {
					ui.Text(c, tr(c, "The configured model or reasoning level is unavailable.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Readable(a.palette.Danger))
				}

			})
		})
	}, func() { a.subagentFormActions(c) })
}

func (a *App) subagentModelPicker(c *ui.Context) {
	a.renderSubagentModelPicker(c, &a.runtimeUI.subagentModel, "", 0, a.subagentModelChanged)
}

func (a *App) renderSubagentModelPicker(c *ui.Context, modelID *string, name string, width float32, changed func()) {
	a.renderSettingsModelPicker(c, *modelID, width, settingsModelPicker{
		label: strings.TrimSpace(tr(c, "Subagent model") + " " + name), optionLabel: tr(c, "Subagent model"),
		providers: a.state.Providers, models: a.state.Models,
		choices:  []settingOption{{"", "Inherit conversation model"}},
		selectID: func(id string) { *modelID = id; changed() },
	})
}

func (a *App) subagentFormActions(c *ui.Context) {
	state := &a.runtimeUI
	ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
		textButton(c, tr(c, "Cancel")).OnClick(func() { state.subagentOpen = false })
		primaryButton(c, tr(c, "Save")).Disabled(strings.TrimSpace(state.subagent.Name) == "" || strings.TrimSpace(state.subagent.Description) == "" || !a.subagentSelectionValid()).OnClick(func() {
			profile := state.subagent
			profile.Name, profile.Description, profile.Prompt = strings.TrimSpace(profile.Name), strings.TrimSpace(profile.Description), strings.TrimSpace(profile.Prompt)
			profile.Model = nil
			if state.subagentModel != "" {
				profile.Model = &contracts.ModelSelection{ModelID: state.subagentModel, Reasoning: state.subagentReasoning}
			}
			profiles := contracts.CloneSubagentProfiles(a.state.Settings.Subagents)
			index := slices.IndexFunc(profiles, func(candidate contracts.SubagentProfile) bool { return candidate.ID == profile.ID })
			if index >= 0 {
				profiles[index] = profile
			} else {
				profiles = append(profiles, profile)
			}
			if err := contracts.ValidateSubagentProfiles(profiles); err != nil {
				a.errorText = err.Error()
				return
			}
			a.state.Settings.Subagents = profiles
			a.capabilityUI.pendingSubagentSave = &contracts.CloneSubagentProfiles([]contracts.SubagentProfile{profile})[0]
			a.saveSettings()
		})
	})
}
