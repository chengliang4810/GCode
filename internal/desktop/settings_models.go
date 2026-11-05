package desktop

import (
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

type modelSettingsState struct {
	selected                              string
	modelOpen, modelNew                   bool
	providerID, originalID                string
	draft                                 runtimeModelDraft
	errorText, pendingKey, pendingCommand string
	inlineID                              string
	inlineSource                          model.Provider
	inlineDirty, inlineSaving, showKey    bool
	revealedKey                           string
	keyLoading                            bool
	keyRequest                            uint64
	inlineError, inlinePendingKey         string
	inlinePending                         *contracts.ProviderInput
	inlineRevision, pendingRevision       uint64
	inlineSaveRequested                   bool
	inlineEditedInput                     int
	nextSelection                         string
	nextSection                           string
	nextRoute                             string
	addAfterSave                          bool
	reorderPendingKey, reorderError       string
	fetchDialogOpen                       bool
	fetchSeen                             string
	fetchRows                             []fetchedModelRow
	fetchLoading, fetchMetadataLoading    bool
	fetchError, fetchValidation           string
	fetchRequest                          uint64
	fetchCancel                           context.CancelFunc
	inputs                                [3]ui.Handle
	focused                               [3]bool
	logoOpen, logoLoading                 bool
	logoProvider, logoError               string
	logoRequest                           uint64
	logoCancel                            context.CancelFunc
}

func (a *App) customProviderSettings(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
		ui.Text(c, tr(c, "Manage custom model providers for use in chat.")).Grow(1).MinWidth(0).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
		iconButton(c.Key("refresh-providers"), "refresh", tr(c, "Refresh providers")).Shrink(0).
			OnClick(func() { a.dispatch(contracts.Command{Kind: "providers.refresh"}) })
		buttonWithIcon(c.Key("add-provider"), tr(c, "Add provider"), "plus", buttonPrimary).Shrink(0).
			Disabled(a.modelSettingsBusy()).OnClick(a.addCustomProvider)
	})
	if a.modelsUI.reorderError != "" {
		ui.Text(c, a.modelsUI.reorderError).FillWidth().TextColor(a.palette.Danger)
	}
	pendingSelection := (a.modelsUI.inlineSaving || a.modelsUI.inlineDirty) && a.modelsUI.inlineID == a.modelsUI.selected
	if !pendingSelection && !slices.ContainsFunc(a.state.CustomProviders, func(p contracts.ProviderConfiguration) bool { return p.Provider.ID == a.modelsUI.selected }) {
		if len(a.state.CustomProviders) > 0 {
			a.modelsUI.selected = a.state.CustomProviders[0].Provider.ID
		} else {
			a.modelsUI.selected = ""
		}
	}
	_, height := c.Size()
	ui.Row(c.Key("model-settings-columns")).FillWidth().AlignItems(ui.Stretch).MinHeight(max(420, height-280)).Radius(12).
		Background(a.palette.Card).Border(1, a.palette.Border).Margin(4, 0, 0, 0).Children(func() {
		ui.Column(c.Key("model-provider-list")).BasisPercent(27).MinWidth(184).Shrink(0).Padding(12, 8).Gap(4).Justify(ui.Start).Children(func() {
			ui.Text(c, tr(c, "Custom providers")).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase)).Margin(2, 8, 8, 8)
			for _, config := range a.state.CustomProviders {
				provider := config.Provider
				row := ui.Row(c.Key(provider.ID)).FillWidth().Gap(2).AlignItems(ui.Center).
					Label(tr(c, "Provider row") + " " + provider.ID).Disabled(a.modelsUI.reorderPendingKey != "")
				row.Children(func() {
					button := controlNavButton(c.Key("select"), tr(c, "Select provider")+" "+provider.Name, a.modelsUI.selected == provider.ID).Grow(1).MinWidth(0)
					button.Children(func() {
						providerLogo(c, provider.Logo, 16)
						ui.Text(c, provider.Name).Grow(1).MinWidth(0).SingleLine().FontWeight(500)
						color, status := a.palette.Success, "Configured"
						if provider.Disabled {
							color, status = a.palette.Muted, "Disabled"
						} else if !provider.Ready() {
							color, status = a.palette.Warning, "Needs configuration"
						}
						ui.Box(c).Size(7, 7).Shrink(0).Radius(4).Background(color).Tooltip(tr(c, status))
					}).OnClick(func() {
						if a.modelsUI.inlineSaving {
							a.modelsUI.nextSelection = provider.ID
							return
						}
						if a.modelsUI.inlineDirty {
							a.modelsUI.nextSelection = provider.ID
							if !a.saveInlineProvider() {
								a.modelsUI.nextSelection = ""
							}
							return
						}
						a.modelsUI.selected = provider.ID
					})
				})
				a.providerReorderTarget(c, row, "", provider.ID)
			}
		})
		ui.Column(c.Key("model-provider-detail")).Grow(1).MinWidth(0).Padding(24).Gap(16).Justify(ui.Start).
			BorderWidth(0, 0, 0, 1).BorderColor(a.palette.Border).Children(func() {
			for _, config := range a.state.CustomProviders {
				if config.Provider.ID == a.modelsUI.selected {
					a.syncInlineProvider(config)
					a.customProviderDetail(c, config)
					return
				}
			}
			if pendingSelection {
				if a.modelsUI.inlineSaving {
					ui.Text(c, tr(c, "Saving…")).TextColor(a.palette.Muted)
				} else {
					a.customProviderDetail(c, contracts.ProviderConfiguration{Provider: a.runtimeUI.provider})
				}
				return
			}
			ui.Text(c, tr(c, "Add your provider to start a conversation.")).TextColor(a.palette.Muted)
		})
	})
	a.modelMetadataDialog(c)
	a.modelFetchDialog(c)
	a.providerLogoDialog(c)
}

func (a *App) syncInlineProvider(config contracts.ProviderConfiguration) {
	state := &a.modelsUI
	if state.inlineID == config.Provider.ID && (state.inlineDirty || state.inlineSaving || reflect.DeepEqual(state.inlineSource, config.Provider)) {
		return
	}
	if state.inlineID != config.Provider.ID {
		state.inlineID, state.inlineError, state.showKey = config.Provider.ID, "", false
		state.inlineDirty, state.inlineSaving = false, false
		state.inlineSaveRequested = false
		state.inlineEditedInput = -1
		state.revealedKey, state.keyLoading = "", false
		state.keyRequest++
		a.dismissModelFetch()
		a.dismissProviderLogo()
		state.fetchSeen = ""
		state.focused = [3]bool{}
		a.runtimeUI.key = ""
	}
	a.runtimeUI.provider = model.CloneProvider(config.Provider)
	state.inlineSource = model.CloneProvider(config.Provider)
}

func (a *App) markInlineProviderDirty() {
	a.modelsUI.inlineDirty = true
	a.modelsUI.inlineRevision++
	a.modelsUI.inlineError = ""
}

func (a *App) inlineProviderInputChanged(index int) {
	a.markInlineProviderDirty()
	a.modelsUI.inlineEditedInput = index
}

func (a *App) toggleProviderKey() {
	state := &a.modelsUI
	if state.showKey {
		state.showKey, state.revealedKey = false, ""
		return
	}
	if a.runtimeUI.key != "" {
		state.showKey = true
		return
	}
	service, ok := a.service.(contracts.ProviderKeyService)
	if !ok {
		state.inlineError = a.t("Provider API key unavailable")
		return
	}
	id, revision := state.inlineID, state.inlineRevision
	state.keyRequest++
	request := state.keyRequest
	state.keyLoading, state.inlineError = true, ""
	a.backgroundTask(func() func() {
		key, err := service.ReadProviderKey(a.ctx, id)
		return func() {
			if state.keyRequest != request || state.inlineID != id || state.selected != id {
				return
			}
			state.keyLoading = false
			if state.inlineRevision != revision {
				return
			}
			if err != nil {
				state.inlineError = err.Error()
				return
			}
			state.revealedKey, state.showKey = key, true
		}
	})
}

func (a *App) customProviderDetail(c *ui.Context, config contracts.ProviderConfiguration) {
	provider := config.Provider
	state := &a.modelsUI
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		controlProviderLogoButton(c.Key("provider-logo"), tr(c, "Choose provider logo")).
			Disabled(a.modelSettingsBusy()).Children(func() {
			providerLogo(c, a.runtimeUI.provider.Logo, 19)
		}).OnClick(a.openProviderLogo)
		controlTextInput(c.Key("custom-provider-name"), &a.runtimeUI.provider.Name).Grow(1).MinWidth(0).
			Label(tr(c, "Provider name")).Bind(&state.inputs[0]).OnChange(func() { a.inlineProviderInputChanged(0) })
		enabled := !provider.Disabled
		a.controlSwitch(c.Key("provider-enabled"), &enabled, tr(c, "Enable provider")+" "+provider.Name, a.modelSettingsBusy(), func(value bool) {
			provider.Disabled = !value
			a.saveProviderSettings(provider)
		})
		iconButton(c.Key("provider-actions"), "more-horizontal", tr(c, "Provider actions")).
			Disabled(a.modelSettingsBusy()).Menu(func(menu *ui.Menu) {
			if menu.Item(tr(c, "Remove provider")).Chosen() {
				a.confirmSettingsRemoval("provider", provider.ID, provider.Name)
			}
		})
	})
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Text(c, tr(c, "Base URL")).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
		controlTextInput(c.Key("custom-provider-url"), &a.runtimeUI.provider.BaseURL).FillWidth().MinWidth(0).Label(tr(c, "Provider Base URL")).Placeholder("https://api.example.com").
			Bind(&state.inputs[1]).OnChange(func() { a.inlineProviderInputChanged(1) })
	})
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Text(c, tr(c, "API format")).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
		controlMenuButton(c.Key("custom-provider-protocol"), protocolEndpointLabel(a.runtimeUI.provider.Protocol), func(menu *ui.Menu) {
			for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
				if menu.Item(protocolEndpointLabel(protocol)).Checked(a.runtimeUI.provider.Protocol == protocol).Chosen() {
					a.runtimeUI.provider.Protocol = protocol
					a.markInlineProviderDirty()
					a.saveInlineProvider()
				}
			}
		}).FillWidth().Label(tr(c, "Provider API format")).Disabled(a.modelSettingsBusy())
	})
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Text(c, tr(c, "API Key")).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
		placeholder := tr(c, "Enter API Key")
		if config.HasAPIKey {
			placeholder = "••••••••••••••••"
		}
		key := &a.runtimeUI.key
		if state.showKey && *key == "" {
			key = &state.revealedKey
		}
		controlSecretInput(c.Key("custom-provider-key"), key, state.showKey,
			!state.keyLoading && (config.HasAPIKey || a.runtimeUI.key != "" || state.showKey), func(input ui.Element) {
				input.Label(tr(c, "API key")).Placeholder(placeholder).Bind(&state.inputs[2]).
					OnChange(func() {
						a.runtimeUI.key = *key
						state.revealedKey = ""
						a.inlineProviderInputChanged(2)
					})
			}, a.toggleProviderKey)
	})
	if state.inlineError != "" {
		ui.Text(c, state.inlineError).FillWidth().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, tr(c, "Model list")).Grow(1).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
			fetching := config.ModelFetch != nil && config.ModelFetch.State == "fetching"
			buttonWithIcon(c.Key("fetch-models"), tr(c, "Fetch models"), "list", buttonSecondary).
				Disabled(a.modelSettingsBusy() || fetching).
				OnClick(func() { a.fetchProviderModels(provider.ID) })
			buttonWithIcon(c.Key("add-model"), tr(c, "Add model"), "plus", buttonPrimary).
				Disabled(a.modelSettingsBusy()).OnClick(func() { a.openSavedModel(provider.ID, "") })
		})
		if len(provider.Models) == 0 {
			ui.Row(c.Key("model-empty-state")).FillWidth().MinHeight(48).Padding(12, 16).Gap(9).AlignItems(ui.Center).
				Radius(8).Border(1, a.palette.Border).BorderStyle(ui.BorderDashed).TextColor(a.palette.Muted).Children(func() {
				icon(c, "info-circle", 15)
				ui.Text(c, tr(c, "No models configured. Add a model to use this provider in chat.")).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase))
			})
		}
		for i, candidate := range provider.Models {
			a.configuredModelRow(c, config, i, candidate)
		}
	})
	// Remember focus loss while a previous save is still in flight.
	lostFocus := false
	for i := range state.inputs {
		focused := state.inputs[i].FocusWithin(c)
		lostFocus = lostFocus || state.focused[i] && !focused
		state.focused[i] = focused
	}
	editedBlurred := state.inlineEditedInput >= 0 && !state.inputs[state.inlineEditedInput].FocusWithin(c)
	if (lostFocus || editedBlurred) && state.inlineDirty {
		state.inlineSaveRequested = true
	}
	if state.inlineSaveRequested && state.inlineDirty && state.inlineError == "" && !a.modelSettingsBusy() {
		a.saveInlineProvider()
	}
}

func (a *App) configuredModelRow(c *ui.Context, config contracts.ProviderConfiguration, index int, candidate model.Model) {
	provider := config.Provider
	row := ui.Column(c.Key("configured-model-"+candidate.ID)).FillWidth().Padding(10, 0).Gap(5).
		Label(tr(c, "Model row")+" "+candidate.ID).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border)
	row.Children(func() {
		ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, candidate.ID).Grow(1).MinWidth(0).SingleLine().FontWeight(500)
			iconButton(c.Key("test-model-"+provider.ID+"/"+candidate.ID), "action-test", tr(c, "Test model")+" "+candidate.ID).
				Disabled(a.modelSettingsBusy() || !provider.Ready() || candidate.Disabled || modelCheckRunning(config, provider.ID+"/"+candidate.ID)).
				OnClick(func() { a.dispatch(contracts.Command{Kind: "provider.test", ID: provider.ID + "/" + candidate.ID}) })
			iconButton(c.Key("edit-model-"+provider.ID+"/"+candidate.ID), "pen", tr(c, "Edit model")+" "+candidate.ID).
				Disabled(a.modelSettingsBusy()).OnClick(func() { a.openSavedModel(provider.ID, candidate.ID) })
			iconButton(c.Key("delete-model-"+provider.ID+"/"+candidate.ID), "trash-bin-minimalistic", tr(c, "Remove model")+" "+candidate.ID).
				Disabled(a.modelSettingsBusy()).OnClick(func() {
				updated := model.CloneProvider(provider)
				updated.Models = slices.Delete(updated.Models, index, index+1)
				a.saveProviderSettings(updated)
			})
		})
		for _, check := range config.ModelChecks {
			if check.ModelID != provider.ID+"/"+candidate.ID {
				continue
			}
			text, color := tr(c, "Testing model…"), a.palette.Muted
			if check.State == "ready" {
				text = tr(c, "Connection successful")
			} else if check.State == "error" {
				text, color = check.Error, a.palette.Danger
			}
			ui.Text(c, text).FillWidth().FontSize(design.TextSize(c, design.TextBase)).TextColor(color)
		}
	})
	a.providerReorderTarget(c, row, provider.ID, candidate.ID)
}

type settingsReorderDrag struct{ ProviderID, SourceID string }

func (a *App) modelSettingsBusy() bool {
	return a.modelsUI.inlineSaving || a.modelsUI.reorderPendingKey != ""
}

// providerReorderTarget makes the whole row draggable and reports a typed
// drop, the same drag the sidebar gives its conversation rows.
func (a *App) providerReorderTarget(c *ui.Context, row ui.Element, providerID, targetID string) {
	row = controlReorderRow(c, row, settingsReorderDrag{ProviderID: providerID, SourceID: targetID}, a.modelSettingsBusy())
	dragged, after, dropped := controlReorderTarget[settingsReorderDrag](c, row, func(value settingsReorderDrag) bool {
		return !a.modelSettingsBusy() && value.ProviderID == providerID && value.SourceID != targetID
	})
	if !dropped {
		return
	}
	kind := "provider.reorder"
	if providerID != "" {
		kind = "provider.models.reorder"
	}
	a.modelsUI.reorderPendingKey, a.modelsUI.reorderError = "reorder-"+rand.Text(), ""
	a.dispatch(contracts.Command{Kind: kind, ID: a.modelsUI.reorderPendingKey, Reorder: &contracts.ReorderInput{
		ProviderID: providerID, SourceID: dragged.SourceID, TargetID: targetID, After: after,
	}})
}

func modelCheckRunning(provider contracts.ProviderConfiguration, id string) bool {
	for _, check := range provider.ModelChecks {
		if check.ModelID == id {
			return check.State == "testing"
		}
	}
	return false
}

func (a *App) addCustomProvider() {
	if a.modelsUI.inlineSaving {
		a.modelsUI.addAfterSave = true
		return
	}
	if a.modelsUI.inlineDirty {
		a.modelsUI.addAfterSave = true
		if !a.saveInlineProvider() {
			a.modelsUI.addAfterSave = false
		}
		return
	}
	name := a.t("New provider")
	for n := 2; slices.ContainsFunc(a.state.CustomProviders, func(p contracts.ProviderConfiguration) bool { return p.Provider.Name == name }); n++ {
		name = fmt.Sprintf(a.t("New provider %d"), n)
	}
	provider := model.Provider{ID: "provider-" + rand.Text(), Name: name, Protocol: model.Chat}
	a.modelsUI.selected = provider.ID
	a.modelsUI.inlineID = provider.ID
	a.modelsUI.showKey, a.modelsUI.revealedKey, a.modelsUI.keyLoading = false, "", false
	a.modelsUI.keyRequest++
	a.runtimeUI.provider, a.runtimeUI.key = provider, ""
	a.markInlineProviderDirty()
	a.saveInlineProvider()
}

func (a *App) saveInlineProvider() bool {
	if a.modelsUI.inlineSaving {
		return false
	}
	if !a.modelsUI.inlineDirty {
		return true
	}
	a.modelsUI.inlineSaveRequested = false
	a.modelsUI.inlineEditedInput = -1
	provider := model.CloneProvider(a.runtimeUI.provider)
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID == provider.ID {
			provider.Models, provider.Disabled = model.CloneProvider(config.Provider).Models, config.Provider.Disabled
			break
		}
	}
	return a.submitProviderSettings(provider)
}

func (a *App) saveProviderSettings(provider model.Provider) {
	if a.modelsUI.inlineSaving {
		return
	}
	a.submitProviderSettings(provider)
}

func (a *App) submitProviderSettings(provider model.Provider) bool {
	if a.modelsUI.reorderPendingKey != "" {
		return false
	}
	state := &a.modelsUI
	if state.inlineID == provider.ID && state.inlineDirty {
		provider.Name, provider.BaseURL, provider.Protocol = a.runtimeUI.provider.Name, a.runtimeUI.provider.BaseURL, a.runtimeUI.provider.Protocol
		provider.Logo = a.runtimeUI.provider.Logo
	}
	provider.Name, provider.BaseURL = strings.TrimSpace(provider.Name), strings.TrimSpace(provider.BaseURL)
	provider.APIKey = ""
	if state.inlineID == provider.ID {
		provider.APIKey = a.runtimeUI.key
	}
	if err := provider.Validate(); err != nil {
		state.inlineError = err.Error()
		return false
	}
	input := &contracts.ProviderInput{Provider: provider, PreserveKey: provider.APIKey == ""}
	state.inlinePending, state.inlinePendingKey = input, "provider-save-"+rand.Text()
	state.inlineSaving, state.pendingRevision = true, state.inlineRevision
	if state.pendingKey != "" && state.pendingKey == state.draft.key && state.providerID == provider.ID {
		state.pendingCommand = state.inlinePendingKey
	}
	a.dispatch(contracts.Command{Kind: "provider.save", ID: state.inlinePendingKey, Provider: input})
	return true
}

func (a *App) modelSettingsCommandFinished(command contracts.Command, err error) {
	if command.Kind == "provider.reorder" || command.Kind == "provider.models.reorder" {
		if command.ID != "" && command.ID == a.modelsUI.reorderPendingKey {
			a.modelsUI.reorderPendingKey = ""
			if err != nil {
				a.modelsUI.reorderError = err.Error()
			}
		}
		return
	}
	if command.Kind == "provider.models.fetch" {
		if err != nil {
			a.modelsUI.fetchLoading = false
			a.modelsUI.fetchError = err.Error()
		}
		return
	}
	if command.Kind != "provider.save" || command.Provider == nil {
		return
	}
	state := &a.modelsUI
	if state.pendingCommand == command.ID && state.pendingKey != "" && state.pendingKey == state.draft.key && state.providerID == command.Provider.Provider.ID {
		state.pendingKey, state.pendingCommand = "", ""
		if err != nil {
			state.modelOpen, state.errorText = true, err.Error()
		}
	}
	if command.ID == "" || command.ID != state.inlinePendingKey {
		return
	}
	state.inlineSaving, state.inlinePendingKey, state.inlinePending = false, "", nil
	if state.inlineID != command.Provider.Provider.ID {
		return
	}
	if err != nil {
		state.inlineDirty, state.inlineError = true, err.Error()
		state.nextSelection = ""
		state.nextSection = ""
		state.nextRoute = ""
		state.addAfterSave = false
		return
	}
	if state.inlineRevision == state.pendingRevision {
		state.inlineDirty, state.inlineError = false, ""
		a.runtimeUI.key = ""
		state.showKey, state.revealedKey, state.keyLoading = false, "", false
		state.keyRequest++
		if state.nextSelection != "" {
			state.selected, state.nextSelection = state.nextSelection, ""
		}
		if state.nextSection != "" {
			section := state.nextSection
			state.nextSection = ""
			a.selectSettingsSection(section)
		}
		if state.nextRoute != "" {
			route := state.nextRoute
			state.nextRoute = ""
			a.OpenRoute(route)
		}
		if state.addAfterSave {
			state.addAfterSave = false
			a.addCustomProvider()
		}
	}
}

func protocolEndpointLabel(protocol model.Protocol) string {
	switch protocol {
	case model.Responses:
		return "OpenAI Responses (/v1/responses)"
	case model.Messages:
		return "Anthropic Messages (/v1/messages)"
	default:
		return "OpenAI Chat Completions (/v1/chat/completions)"
	}
}

func (a *App) openSavedModel(providerID, modelID string) {
	state := &a.modelsUI
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID != providerID {
			continue
		}
		state.modelOpen, state.modelNew = true, modelID == ""
		state.providerID, state.originalID = providerID, modelID
		state.errorText, state.pendingKey, state.pendingCommand = "", "", ""
		state.draft = draftRuntimeModel(model.Model{ContextWindow: 200000, MaxOutput: 12800, InputTypes: []string{"text"}})
		for _, m := range config.Provider.Models {
			if m.ID == modelID {
				state.draft = draftRuntimeModel(m)
				state.draft.model.ReasoningLevels = slices.Clone(m.ReasoningLevels)
				state.draft.model.InputTypes = slices.Clone(m.InputTypes)
				break
			}
		}
		return
	}
}

func (a *App) modelMetadataDialog(c *ui.Context) {
	state := &a.modelsUI
	title := "Edit model"
	if state.modelNew {
		title = "Add model"
	}
	a.settingsDialogWithFooter(c, "model-metadata-dialog", title, &state.modelOpen, 680, func() {
		a.modelMetadataFields(c)
		if state.errorText != "" {
			ui.Text(c, state.errorText).FillWidth().TextColor(a.palette.Danger)
		}
	}, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			textButton(c.Key("reset-model"), tr(c, "Reset form")).OnClick(func() {
				state.draft = draftRuntimeModel(model.Model{ID: state.originalID, ContextWindow: 200000, MaxOutput: 12800, InputTypes: []string{"text"}})
				a.lookupModelMetadata()
			})
			ui.Spacer(c).Grow(1)
			textButton(c, tr(c, "Cancel")).OnClick(func() { state.modelOpen = false })
			primaryButton(c.Key("save-model"), tr(c, "Save")).Label(tr(c, "Save model")).Disabled(a.modelSettingsBusy() || state.draft.loading).OnClick(a.saveModelMetadata)
		})
	})
}

func (a *App) saveModelMetadata() {
	state := &a.modelsUI
	if state.inlineSaving {
		return
	}
	draft := state.draft
	context, contextErr := parseTokenLimit(draft.context)
	output, outputErr := parseTokenLimit(draft.output)
	if contextErr != "" || outputErr != "" {
		state.errorText = a.t("Token limits must be nonnegative integers")
		return
	}
	draft.model.ID = strings.TrimSpace(draft.model.ID)
	draft.model.ContextWindow, draft.model.MaxOutput = context, output
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID != state.providerID {
			continue
		}
		provider := model.CloneProvider(config.Provider)
		if state.modelNew {
			provider.Models = append(provider.Models, draft.model)
		} else {
			index := slices.IndexFunc(provider.Models, func(m model.Model) bool { return m.ID == state.originalID })
			if index < 0 {
				state.errorText = a.t("The model changed; reopen the editor.")
				return
			}
			provider.Models[index] = draft.model
		}
		state.pendingKey = state.draft.key
		state.modelOpen = false
		if !a.submitProviderSettings(provider) {
			state.modelOpen = true
			state.pendingKey = ""
			state.errorText = state.inlineError
		}
		return
	}
	state.errorText = a.t("The provider no longer exists.")
}

func protocolLabel(protocol model.Protocol) string {
	switch protocol {
	case model.Responses:
		return "OpenAI Responses"
	case model.Messages:
		return "Anthropic Messages"
	default:
		return "OpenAI Chat Completions"
	}
}
func parseTokenLimit(value string) (int, string) {
	if strings.TrimSpace(value) == "" {
		return 0, ""
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, "Token limits must be nonnegative integers"
	}
	return n, ""
}
