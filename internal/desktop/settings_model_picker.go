package desktop

import (
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type settingsModelPicker struct {
	label       string
	optionLabel string
	choices     []settingOption
	providers   []contracts.Provider
	models      []contracts.Model
	selectID    func(string)
}

func (a *composerController) pickerProviders() []contracts.Provider {
	if a.picker != nil {
		return a.picker.providers
	}
	return a.state.Providers
}

func (a *composerController) pickerModels() []contracts.Model {
	if a.picker != nil {
		return a.picker.models
	}
	return a.state.Models
}

func (a *composerController) chooseSettingsModel(id string) {
	a.modelID = id
	a.modelMenu = false
	a.picker.selectID(id)
}

func (a *App) renderSettingsModelPicker(c *ui.Context, id string, width float32, picker settingsModelPicker) {
	trigger := controlSelectButton(c.Key("settings-model-trigger")).Label(picker.label)
	if width > 0 {
		trigger.Width(width)
	} else {
		trigger.FillWidth()
	}
	state := ui.Local(trigger, "picker", func() composerControls { return composerControls{} })
	state.modelID = id
	controller := &composerController{App: a, composerControls: state, picker: &picker}
	label := id
	found := false
	for _, choice := range picker.choices {
		if choice.value == id {
			label = tr(c, choice.label)
			found = true
		}
	}
	if selected := controller.selectedModel(); selected.Label != "" {
		label = selected.Label
		found = true
	}
	trigger.Children(func() {
		if provider := controller.selectedProvider(); provider.ID != "" {
			providerLogo(c, provider.Logo, 16)
		}
		if !found && id != "" {
			ui.Text(c, tr(c, "Unavailable")).Shrink(0)
		}
		ui.Text(c, label).Grow(1).MinWidth(0).SingleLine()
		icon(c, "alt-arrow-down", 13)
	}).OnClick(func() {
		state.modelMenu = !state.modelMenu
		if state.modelMenu {
			state.modelQuery, state.modelProvider = "", ""
			state.modelSearchOpen = false
			controller.openModelList()
		}
	})
	controller.modelPopover(c, trigger)
}
