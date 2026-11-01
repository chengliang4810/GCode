package desktop

import (
	"crypto/rand"
	"fmt"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

var customSubagentTools = []string{"Read", "ListDirectory", "Glob", "Grep", "Bash", "Write", "Edit", "AskUserQuestion"}

func (a *App) customSubagentToolbar(c *ui.Context) string {
	state := &a.capabilityUI
	if state.queries == nil {
		state.queries = map[string]*string{}
	}
	query := state.queries["Subagents"]
	if query == nil {
		query = new(string)
		state.queries["Subagents"] = query
	}
	busy := a.runtimeUI.subagentPendingID != ""
	ui.Row(c.Key("custom-subagent-toolbar")).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
		count := 0
		for _, profile := range a.state.Settings.Subagents {
			if !contracts.IsBuiltinSubagent(profile.ID) {
				count++
			}
		}
		ui.Text(c, fmt.Sprintf("%s · %d", tr(c, "Custom subagents"), count)).Grow(1).FontWeight(500)
		controlTextInput(c.Key("resource-search-Subagents"), query).Width(240).MinWidth(120).
			Label(tr(c, "Search Subagents")).Placeholder(tr(c, "Search"))
		iconButton(c.Key("refresh-subagents"), "refresh", tr(c, "Refresh")).Disabled(busy).
			OnClick(func() { a.dispatch(contracts.Command{Kind: "resource.refresh"}) })
		buttonWithIcon(c.Key("add-subagent"), tr(c, "Add subagent"), "plus", buttonPrimary).
			Disabled(busy).OnClick(func() { a.editSubagent(nil) })
	})
	if a.runtimeUI.subagentError != "" {
		ui.Text(c, a.runtimeUI.subagentError).FillWidth().TextColor(a.palette.Danger)
	}
	return strings.ToLower(strings.TrimSpace(*query))
}

func (a *App) customSubagentModelLabel(profile contracts.SubagentProfile) string {
	if profile.Model == nil {
		return a.t("Default main model")
	}
	for _, candidate := range a.state.Models {
		if candidate.ID == profile.Model.ModelID {
			label := a.runtimeModelLabel(candidate)
			if profile.Model.Reasoning != "" {
				label += " · " + a.t(profile.Model.Reasoning)
			}
			return label
		}
	}
	return a.t("Unavailable") + " · " + profile.Model.ModelID
}

func (a *App) customSubagentList(c *ui.Context, query string) {
	var profiles []contracts.SubagentProfile
	for _, profile := range a.state.Settings.Subagents {
		if !contracts.IsBuiltinSubagent(profile.ID) && matchesResource(query, profile.Name, profile.Description,
			a.customSubagentModelLabel(profile), strings.Join(profile.Tools, " ")) {
			profiles = append(profiles, profile)
		}
	}
	a.settingsCard(c, func() {
		ui.Column(c.Key("custom-subagent-list")).FillWidth().Children(func() {
			if len(profiles) == 0 {
				label := "No custom subagents"
				if query != "" {
					label = "No matching subagents"
				}
				ui.Text(c, tr(c, label)).FillWidth().Padding(24, 16).TextColor(a.palette.Muted)
			}
			for index, profile := range profiles {
				if index > 0 {
					ui.Box(c).FillWidth().Height(1).Background(a.palette.Border)
				}
				a.customSubagentRow(c, profile)
			}
		})
	})
}

func (a *App) customSubagentRow(c *ui.Context, profile contracts.SubagentProfile) {
	busy := a.runtimeUI.subagentPendingID != ""
	ui.Row(c.Key("subagent-row-"+profile.ID)).FillWidth().Padding(12, 16).Gap(12).AlignItems(ui.Center).
		Label(tr(c, "Custom subagent") + " " + profile.ID).Disabled(busy).
		OnClick(func() { a.editSubagent(&profile) }).Children(func() {
		controlResourceAvatar(c.Key("avatar"), "bot")
		controlRowButton(c.Key("subagent-edit-" + profile.ID)).Grow(1).MinWidth(0).
			Label(tr(c, "Edit subagent") + " " + profile.Name).OnClick(func() { a.editSubagent(&profile) }).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, profile.Name).MinWidth(0).SingleLine().FontWeight(500)
					controlResourceBadge(c, a.customSubagentModelLabel(profile))
					tools := tr(c, "All default tools")
					if len(profile.Tools) > 0 {
						tools = fmt.Sprintf(tr(c, "%d tools"), len(profile.Tools))
					}
					controlResourceBadge(c, tools).Tooltip(strings.Join(profile.Tools, ", "))
				})
				ui.Text(c, profile.Description).FillWidth().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			})
		})
		enabled := !profile.Disabled
		a.controlSwitch(c.Key("subagent-enabled-"+profile.ID), &enabled, tr(c, "Enable")+" "+profile.Name, busy, func(value bool) {
			profile.Disabled = !value
			a.storeSubagentProfile(profile)
			a.startCustomSubagentSave(profile.ID, false)
		})
		iconButton(c.Key("subagent-delete-"+profile.ID), "trash-bin-minimalistic", tr(c, "Remove subagent")+" "+profile.Name).
			OnClick(func() { a.confirmSettingsRemoval("subagent", profile.ID, profile.Name) })
	})
}

func (a *App) customSubagentField(c *ui.Context, label, key string, draw func()) {
	ui.Column(c).FillWidth().MinWidth(0).Gap(6).Children(func() {
		ui.Text(c, tr(c, label)).FontWeight(500).FontSize(design.TextSize(c, design.TextBase))
		draw()
		message := a.runtimeUI.subagentFieldErrors[key]
		if key == "model" && !a.subagentSelectionValid() {
			message = "The configured model or reasoning level is unavailable."
		}
		if message != "" {
			ui.Text(c, tr(c, message)).FillWidth().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
		}
	})
}

func (a *App) customSubagentForm(c *ui.Context) {
	state := &a.runtimeUI
	busy := state.subagentPendingID != ""
	title := "Add subagent"
	if state.subagentExisting {
		title = "Edit subagent"
	}
	ui.Column(c).FillWidth().Disabled(busy).Children(func() {
		a.settingsFormHeader(c, "Subagents", title, func() { state.subagentOpen = false })
	})
	a.settingsCard(c, func() {
		ui.Column(c.Key("custom-subagent-form")).FillWidth().Padding(16).Gap(18).Children(func() {
			ui.Column(c).FillWidth().Gap(18).Disabled(busy).Children(func() {
				// The top row contains identity and execution settings. Flex wrapping
				// follows the available content width, including the settings sidebar.
				ui.Row(c.Key("subagent-identity-fields")).FillWidth().Wrap().Gap(16).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Basis(220).Grow(1).MinWidth(180).Children(func() {
						a.customSubagentField(c, "Name", "name", func() {
							controlTextInput(c.Key("subagent-name"), &state.subagent.Name).FillWidth().
								Label(tr(c, "Subagent name")).Placeholder(tr(c, "Name")).
								OnChange(func() { delete(state.subagentFieldErrors, "name") })
						})
					})
					ui.Column(c).Basis(320).Grow(2).MinWidth(180).Children(func() {
						a.customSubagentField(c, "Model", "model", func() {
							ui.Row(c.Key("subagent-execution-fields")).FillWidth().Wrap().Gap(8).Children(func() {
								ui.Column(c).Basis(200).Grow(1).MinWidth(124).Children(func() { a.subagentModelPicker(c) })
								a.customSubagentReasoning(c)
							})
						})
					})
				})
				a.customSubagentField(c, "Description", "description", func() {
					controlTextInput(c.Key("subagent-description"), &state.subagent.Description).FillWidth().
						Label(tr(c, "Subagent description")).Placeholder(tr(c, "Subagent description")).
						OnChange(func() { delete(state.subagentFieldErrors, "description") })
				})
				a.customSubagentField(c, "Available tools", "tools", func() { a.customSubagentToolPicker(c) })
				a.customSubagentField(c, "System prompt", "prompt", func() {
					controlTextArea(c.Key("subagent-prompt"), &state.subagent.Prompt).FillWidth().Height(140).
						Label(tr(c, "Subagent prompt")).Placeholder(tr(c, "Subagent prompt")).
						OnChange(func() { delete(state.subagentFieldErrors, "prompt") })
				})
				ui.Row(c.Key("subagent-project-instructions")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, tr(c, "Use project instructions")).Grow(1)
					useProject := !state.subagent.SkipProjectInstructions
					a.controlSwitch(c.Key("subagent-project-instructions-toggle"), &useProject, tr(c, "Use project instructions"), busy, func(value bool) {
						state.subagent.SkipProjectInstructions = !value
					})
				})
			})
			if state.subagentError != "" {
				ui.Text(c, state.subagentError).FillWidth().TextColor(a.palette.Danger)
			}
			ui.Row(c.Key("subagent-form-actions")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				if state.subagentExisting {
					buttonWithIcon(c.Key("delete-subagent"), tr(c, "Remove"), "trash-bin-minimalistic", buttonDanger).
						Label(tr(c, "Remove subagent") + " " + state.subagent.Name).Disabled(busy).
						OnClick(func() { a.confirmSettingsRemoval("subagent", state.subagent.ID, state.subagent.Name) })
				}
				ui.Spacer(c)
				caption := tr(c, "Save")
				if busy {
					caption = tr(c, "Saving…")
				}
				primaryButton(c.Key("save-subagent"), caption).Label(tr(c, "Save")).Disabled(busy).OnClick(a.saveCustomSubagent)
				textButton(c.Key("cancel-subagent"), tr(c, "Cancel")).Disabled(busy).OnClick(func() { state.subagentOpen = false })
			})
		})
	})
}

func (a *App) customSubagentReasoning(c *ui.Context) {
	state := &a.runtimeUI
	// Inherited selections carry the parent model and reasoning together.
	if state.subagentModel == "" {
		return
	}
	selected := a.subagentSelectedModel()
	if len(selected.ReasoningLevels) == 0 {
		return
	}
	sel := controlSelect(c.Key("subagent-reasoning"), &state.subagentReasoning)
	label := state.subagentReasoning
	if !slices.Contains(selected.ReasoningLevels, label) {
		label = tr(c, "Unavailable") + " · " + label
	}
	sel.Trigger.Label(tr(c, "Subagent reasoning level")).OnChange(func() { delete(state.subagentFieldErrors, "model") }).Children(func() {
		ui.Text(c, tr(c, label)).Grow(1)
		icon(c, "alt-arrow-down", 13)
	})
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).Children(func() {
			for _, level := range selected.ReasoningLevels {
				controlSelectItem(c, sel.Item(level)).Label(tr(c, "Subagent reasoning level") + " " + level).Children(func() {
					ui.Text(c, tr(c, level))
				})
			}
		})
	})
}

func (a *App) customSubagentToolPicker(c *ui.Context) {
	state := &a.runtimeUI
	mode := "all"
	if state.subagentToolsCustom {
		mode = "custom"
	}
	sel := controlSelect(c.Key("subagent-tools-mode"), &mode)
	label := "All default tools"
	if state.subagentToolsCustom {
		label = "Custom tools"
	}
	sel.Trigger.Label(tr(c, "Tool access")).OnChange(func() {
		delete(state.subagentFieldErrors, "tools")
		state.subagentToolsCustom = mode == "custom"
		if state.subagentToolsCustom && len(state.subagent.Tools) == 0 {
			state.subagent.Tools = slices.Clone(customSubagentTools)
		}
	}).Children(func() {
		ui.Text(c, tr(c, label)).Grow(1)
		icon(c, "alt-arrow-down", 13)
	})
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).Children(func() {
			for _, option := range []struct{ value, label string }{{"all", "All default tools"}, {"custom", "Custom tools"}} {
				controlSelectItem(c, sel.Item(option.value)).Label(tr(c, option.label)).Children(func() { ui.Text(c, tr(c, option.label)) })
			}
		})
	})
	if state.subagentToolsCustom {
		ui.Row(c.Key("subagent-custom-tools")).FillWidth().Wrap().Gap(10).Children(func() {
			for _, name := range customSubagentTools {
				enabled := slices.Contains(state.subagent.Tools, name)
				controlCheckbox(c.Key("subagent-tool-"+name), &enabled, name).Width(148).
					Label(tr(c, "Available tools") + " " + name).OnClick(func() {
					delete(state.subagentFieldErrors, "tools")
					if enabled {
						if !slices.Contains(state.subagent.Tools, name) {
							state.subagent.Tools = append(state.subagent.Tools, name)
						}
					} else {
						state.subagent.Tools = slices.DeleteFunc(state.subagent.Tools, func(value string) bool { return value == name })
					}
				})
			}
		})
	}
}

func (a *App) saveCustomSubagent() {
	state := &a.runtimeUI
	if state.subagentPendingID != "" {
		return
	}
	state.subagentFieldErrors = map[string]string{}
	profile := contracts.CloneSubagentProfiles([]contracts.SubagentProfile{state.subagent})[0]
	profile.Name, profile.Description, profile.Prompt = strings.TrimSpace(profile.Name), strings.TrimSpace(profile.Description), strings.TrimSpace(profile.Prompt)
	for _, field := range []struct{ key, value, message string }{
		{"name", profile.Name, "Enter a subagent name."},
		{"description", profile.Description, "Enter a subagent description."},
		{"prompt", profile.Prompt, "Enter a system prompt."},
	} {
		if field.value == "" {
			state.subagentFieldErrors[field.key] = field.message
		}
	}
	if !a.subagentSelectionValid() {
		state.subagentFieldErrors["model"] = "The configured model or reasoning level is unavailable."
	}
	profile.Model = nil
	if state.subagentModel != "" {
		profile.Model = &contracts.ModelSelection{ModelID: state.subagentModel, Reasoning: state.subagentReasoning}
	}
	if !state.subagentToolsCustom {
		profile.Tools = nil
	} else if len(profile.Tools) == 0 {
		state.subagentFieldErrors["tools"] = "Select at least one tool."
	}
	if len(state.subagentFieldErrors) > 0 {
		return
	}
	if err := contracts.ValidateSubagentProfiles([]contracts.SubagentProfile{profile}); err != nil {
		state.subagentError = err.Error()
		return
	}
	a.storeSubagentProfile(profile)
	a.startCustomSubagentSave(profile.ID, true)
}

func (a *App) startCustomSubagentSave(profileID string, closeAfter bool) {
	state := &a.runtimeUI
	state.subagentError = ""
	state.subagentPendingID, state.subagentPendingProfile = "subagent-save-"+rand.Text(), profileID
	state.subagentCloseAfterSave = closeAfter
	a.saveSettingsWithID(state.subagentPendingID)
}

func (a *App) customSubagentCommandFinished(command contracts.Command, err error) {
	state := &a.runtimeUI
	if command.Kind != "settings.update" || command.ID == "" || command.ID != state.subagentPendingID {
		return
	}
	profileID, closeAfter := state.subagentPendingProfile, state.subagentCloseAfterSave
	state.subagentPendingID, state.subagentPendingProfile, state.subagentCloseAfterSave = "", "", false
	if err != nil {
		state.subagentError = err.Error()
		return
	}
	state.subagentError = ""
	if closeAfter && state.subagent.ID == profileID {
		state.subagentOpen = false
	}
}
