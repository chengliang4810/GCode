package desktop

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"github.com/egoist/mygo/ui"
)

type fetchedModelRow struct {
	ID                  string
	Configured, Checked bool
	Draft               runtimeModelDraft
}

// Open before dispatch so network latency and failures are visible in the dialog.
func (a *App) fetchProviderModels(providerID string) {
	if a.modelSettingsBusy() {
		return
	}
	a.dismissModelFetch()
	state := &a.modelsUI
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID == providerID && config.ModelFetch != nil {
			state.fetchSeen = config.ModelFetch.ProviderID + config.ModelFetch.StartedAt.String()
		}
	}
	state.fetchDialogOpen, state.fetchLoading = true, true
	a.dispatch(contracts.Command{Kind: "provider.models.fetch", ID: providerID})
}

// Only consume a new terminal result for an explicitly opened dialog.
func (a *App) syncModelFetchDialog() {
	state := &a.modelsUI
	if !state.fetchDialogOpen || a.section != "Model settings" {
		return
	}
	for _, config := range a.state.CustomProviders {
		fetch := config.ModelFetch
		if config.Provider.ID != state.inlineID || fetch == nil || fetch.State == "fetching" {
			continue
		}
		signature := fetch.ProviderID + fetch.StartedAt.String()
		if state.fetchSeen == signature {
			return
		}
		state.fetchSeen = signature
		state.fetchLoading = false
		state.fetchError = ""
		if fetch.State != "ready" {
			state.fetchError = fetch.Error
			return
		}
		state.fetchRows = make([]fetchedModelRow, 0, len(fetch.Models))
		for _, id := range fetch.Models {
			index := slices.IndexFunc(config.Provider.Models, func(m model.Model) bool { return m.ID == id })
			m := model.Model{ID: id, ContextWindow: 200000, MaxOutput: 12800, InputTypes: []string{"text"}}
			if index >= 0 {
				m = config.Provider.Models[index]
			}
			state.fetchRows = append(state.fetchRows, fetchedModelRow{
				ID: id, Configured: index >= 0, Checked: true, Draft: draftRuntimeModel(m),
			})
		}
		a.lookupFetchedModelMetadata(config.Provider)
		return
	}
}

func (a *App) dismissModelFetch() {
	state := &a.modelsUI
	if state.fetchCancel != nil {
		state.fetchCancel()
		state.fetchCancel = nil
	}
	state.fetchRequest++
	state.fetchDialogOpen, state.fetchLoading, state.fetchMetadataLoading = false, false, false
	state.fetchRows, state.fetchError, state.fetchValidation = nil, "", ""
}

// Reuse the host's catalog service. One cancellable worker keeps large lists
// bounded; all draft writes are published back on the window thread.
func (a *App) lookupFetchedModelMetadata(provider model.Provider) {
	state := &a.modelsUI
	service, ok := a.service.(contracts.ModelMetadataService)
	if !ok {
		return
	}
	ids := make([]string, len(state.fetchRows))
	for i, row := range state.fetchRows {
		if !row.Configured {
			ids[i] = row.ID
		}
	}
	if !slices.ContainsFunc(ids, func(id string) bool { return id != "" }) {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	state.fetchCancel, state.fetchMetadataLoading = cancel, true
	request := state.fetchRequest
	baseURL, protocol := provider.BaseURL, provider.Protocol
	a.backgroundTask(func() func() {
		defer cancel()
		results := make([]modelcatalog.Metadata, len(ids))
		errors := make([]error, len(ids))
		for i, id := range ids {
			if id != "" {
				results[i], errors[i] = service.LookupModelMetadata(ctx, modelcatalog.Query{ID: id, BaseURL: baseURL})
			}
		}
		return func() {
			current := &a.modelsUI
			if !current.fetchDialogOpen || current.fetchRequest != request {
				return
			}
			current.fetchMetadataLoading, current.fetchCancel = false, nil
			for i, result := range results {
				row := &current.fetchRows[i]
				if row.Configured {
					continue
				}
				if errors[i] != nil || !result.Found {
					continue
				}
				if result.ContextWindow > 0 {
					row.Draft.context = strconv.Itoa(result.ContextWindow)
				}
				if result.MaxOutput > 0 {
					row.Draft.output = strconv.Itoa(result.MaxOutput)
				}
				row.Draft.model.InputTypes = slices.Clone(result.InputTypes)
				row.Draft.model.ReasoningLevels = slices.Clone(result.ReasoningLevels)
				if protocol == model.Messages {
					row.Draft.model.ReasoningLevels = slices.DeleteFunc(row.Draft.model.ReasoningLevels, func(s string) bool { return s == "minimal" })
				}
			}
		}
	})
}

func (a *App) modelFetchDialog(c *ui.Context) {
	state := &a.modelsUI
	if !state.fetchDialogOpen {
		if state.fetchCancel != nil || state.fetchLoading || state.fetchRows != nil {
			a.dismissModelFetch()
		}
		return
	}
	a.settingsDialogWithFooter(c, "model-fetch-dialog", "Fetched models", &state.fetchDialogOpen, 1080, func() {
		switch {
		case state.fetchLoading:
			ui.Text(c.Key("fetch-models-loading"), tr(c, "Fetching models…")).FillWidth().TextColor(a.palette.Muted)
		case state.fetchError != "":
			ui.Text(c.Key("fetch-models-error"), tr(c, state.fetchError)).FillWidth().TextColor(a.palette.Danger)
		case len(state.fetchRows) == 0:
			ui.Text(c, tr(c, "The provider returned no models.")).FillWidth().TextColor(a.palette.Muted)
		default:
			a.modelFetchSelection(c)
			if state.fetchMetadataLoading {
				ui.Text(c.Key("fetch-metadata-loading"), tr(c, "Loading model details…")).FillWidth().TextColor(a.palette.Muted)
			}
			for i := range state.fetchRows {
				a.modelFetchRow(c.Key("fetched-row-"+state.fetchRows[i].ID), &state.fetchRows[i])
			}
		}
		if state.fetchValidation != "" {
			ui.Text(c, state.fetchValidation).FillWidth().TextColor(a.palette.Danger)
		}
	}, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			selected := a.selectedFetchedModelCount()
			ui.Text(c, fmt.Sprintf(tr(c, "%d selected"), selected)).Grow(1).TextColor(a.palette.Muted)
			if state.fetchError != "" {
				secondaryButton(c.Key("retry-fetch"), tr(c, "Retry")).OnClick(func() { a.fetchProviderModels(state.inlineID) })
			}
			textButton(c.Key("cancel-fetch"), tr(c, "Cancel")).OnClick(a.dismissModelFetch)
			primaryButton(c.Key("confirm-fetch"), tr(c, "Save")).Label(tr(c, "Save models")).
				Disabled(selected == 0 || state.fetchLoading || state.fetchMetadataLoading || state.fetchError != "" || a.modelSettingsBusy()).
				OnClick(a.confirmFetchModels)
		})
	})
	// Escape, backdrop and the shared close button also cancel pending metadata.
	if !state.fetchDialogOpen {
		a.dismissModelFetch()
	}
}

func (a *App) selectedFetchedModelCount() int {
	count := 0
	for _, row := range a.modelsUI.fetchRows {
		if row.Checked {
			count++
		}
	}
	return count
}

func (a *App) modelFetchSelection(c *ui.Context) {
	state := &a.modelsUI
	all := len(state.fetchRows) > 0
	for _, row := range state.fetchRows {
		all = all && row.Checked
	}
	controlCheckbox(c.Key("fetched-select-all"), &all, tr(c, "Select all models")).OnClick(func() {
		for i := range state.fetchRows {
			state.fetchRows[i].Checked = all
		}
	})
}

func (a *App) modelFetchRow(c *ui.Context, row *fetchedModelRow) {
	disabled := a.modelsUI.fetchMetadataLoading || a.modelSettingsBusy()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 0).
		BorderWidth(1, 0, 0, 0).BorderColor(a.palette.Border).Children(func() {
		controlCheckbox(c.Key("fetched-model-"+row.ID), &row.Checked, "").Label(row.ID).Shrink(0)
		ui.Text(c, row.ID).Grow(1).MinWidth(0).SingleLine()
		ui.Text(c, tr(c, "Context")).Shrink(0).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
		controlTextInput(c.Key("fetched-context-"+row.ID), &row.Draft.context).Width(136).Shrink(0).
			Label(fmt.Sprintf(tr(c, "Context window for %s"), row.ID)).Disabled(disabled)
		image := slices.Contains(row.Draft.model.InputTypes, "image")
		controlCheckboxOption(c.Key("fetched-input-"+row.ID+":image"), &image, tr(c, "Image")).Shrink(0).
			Label(fmt.Sprintf(tr(c, "%s input for %s"), tr(c, "Image"), row.ID)).Disabled(disabled).OnClick(func() {
			row.Draft.model.InputTypes = modelMetadataSelection(row.Draft.model.InputTypes, "image", image, []string{"text", "image", "audio", "video", "pdf"})
		})
	})
}

func (a *App) confirmFetchModels() {
	state := &a.modelsUI
	if state.fetchLoading || state.fetchMetadataLoading || state.fetchError != "" || a.modelSettingsBusy() {
		return
	}
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID != state.inlineID {
			continue
		}
		provider := model.CloneProvider(config.Provider)
		for _, row := range state.fetchRows {
			if !row.Checked {
				continue
			}
			contextWindow, contextErr := parseTokenLimit(row.Draft.context)
			output, outputErr := parseTokenLimit(row.Draft.output)
			if contextErr != "" || outputErr != "" {
				state.fetchValidation = row.ID + ": " + a.t("Token limits must be nonnegative integers")
				return
			}
			m := row.Draft.model
			m.ContextWindow, m.MaxOutput = contextWindow, output
			index := slices.IndexFunc(provider.Models, func(saved model.Model) bool { return saved.ID == row.ID })
			if index >= 0 {
				// Only the two exposed fields change on an existing model.
				provider.Models[index].ContextWindow = contextWindow
				inputs := append([]string{"text"}, provider.Models[index].InputTypes...)
				provider.Models[index].InputTypes = modelMetadataSelection(inputs, "image", slices.Contains(m.InputTypes, "image"), []string{"text", "image", "audio", "video", "pdf"})
			} else {
				provider.Models = append(provider.Models, m)
			}
		}
		// Keep the draft open on synchronous validation/queue failure.
		if a.submitProviderSettings(provider) {
			a.dismissModelFetch()
		} else {
			state.fetchValidation = state.inlineError
		}
		return
	}
	state.fetchError = a.t("The provider no longer exists.")
}
