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
	for _, candidate := range a.state.Models {
		if candidate.ID == a.runtimeUI.subagentModel {
			return candidate
		}
	}
	return contracts.Model{}
}

func (a *App) subagentModelChanged() {
	state := &a.runtimeUI
	delete(state.subagentFieldErrors, "model")
	state.subagentReasoning = ""
	if levels := a.subagentSelectedModel().ReasoningLevels; len(levels) > 0 {
		state.subagentReasoning = levels[len(levels)-1]
	}
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
	if a.runtimeUI.subagentOpen {
		if contracts.IsBuiltinSubagent(a.runtimeUI.subagent.ID) {
			a.subagentSettingsForm(c)
		} else {
			a.customSubagentForm(c)
		}
		return
	}
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
		ui.Row(c.Key("subagent-row-"+profile.ID)).FillWidth().AlignItems(ui.Center).Gap(8).Padding(10, 4).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
			controlRowButton(c).Grow(1).MinWidth(0).Label(tr(c, "Edit subagent") + " " + name).OnClick(func() { a.editSubagent(&profile) }).Children(func() {
				ui.Column(c).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, name).FontWeight(500)
						if contracts.IsBuiltinSubagent(profile.ID) {
							ui.Text(c, tr(c, "Built-in")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
						}
					})
					ui.Text(c, description).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				})
			})
			enabled := !profile.Disabled
			a.controlSwitch(c.Key("enabled"), &enabled, tr(c, "Enable")+" "+name, false, func(enabled bool) {
				profile.Disabled = !enabled
				a.storeSubagentProfile(profile)
				a.saveSettings()
			})
			if !contracts.IsBuiltinSubagent(profile.ID) {
				textButton(c, tr(c, "Remove")).Label(tr(c, "Remove subagent") + " " + name).OnClick(func() {
					a.confirmSettingsRemoval("subagent", profile.ID, profile.Name)
				})
			}
		})
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
	a.settingsFormHeader(c, "Subagents", title, func() { state.subagentOpen = false })
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
			selected := a.subagentSelectedModel()
			if len(selected.ReasoningLevels) > 0 {
				sel := controlSelect(c.Key("subagent-reasoning"), &state.subagentReasoning)
				label := state.subagentReasoning
				if !slices.Contains(selected.ReasoningLevels, label) {
					label = tr(c, "Unavailable") + " · " + label
				}
				sel.Trigger.Label(tr(c, "Subagent reasoning level")).FillWidth().Children(func() {
					ui.Text(c, tr(c, label)).Grow(1)
					icon(c, "alt-arrow-down", 13)
				})
				sel.Popup(func(panel ui.Element) {
					controlSelectPopup(c, panel).Children(func() {
						for _, level := range selected.ReasoningLevels {
							controlSelectItem(c, sel.Item(level)).Label(tr(c, "Subagent reasoning level") + " " + level).Children(func() { ui.Text(c, tr(c, level)) })
						}
					})
				})
			}
			if !a.subagentSelectionValid() {
				ui.Text(c, tr(c, "The configured model or reasoning level is unavailable.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
			}
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
					state.subagentOpen = false
					a.saveSettings()
				})
			})
		})
	})
}

func (a *App) subagentModelPicker(c *ui.Context) {
	state := &a.runtimeUI
	selected := a.subagentSelectedModel()
	label := tr(c, "Default main model")
	if state.subagentModel != "" {
		label = tr(c, "Unavailable") + " · " + state.subagentModel
		if selected.ID != "" {
			label = a.runtimeModelLabel(selected)
		}
	}
	sel := controlSelect(c.Key("subagent-model"), &state.subagentModel)
	sel.Trigger.Label(tr(c, "Subagent model")).FillWidth().OnChange(a.subagentModelChanged).Children(func() {
		ui.Text(c, label).Grow(1).SingleLine()
		icon(c, "alt-arrow-down", 13)
	})
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).Children(func() {
			controlSelectItem(c, sel.Item("")).Label(tr(c, "Default main model")).Children(func() { ui.Text(c, tr(c, "Default main model")) })
			ui.Scroll(c.Key("subagent-model-options")).MaxHeight(280).Children(func() {
				for _, provider := range a.state.Providers {
					ui.Column(c.Key(provider.ID)).FillWidth().Children(func() {
						ui.Text(c, provider.Name).Padding(8, 10, 4, 10).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
						for _, candidate := range a.state.Models {
							if candidate.ProviderID == provider.ID {
								controlSelectItem(c, sel.Item(candidate.ID)).Label(tr(c, "Subagent model") + " " + candidate.ID).Children(func() { ui.Text(c, candidate.Label) })
							}
						}
					})
				}
			})
		})
	})
}
