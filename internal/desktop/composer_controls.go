package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// composerControls is UI/configuration state, shared by the central and side
// composers. It owns no service, goroutine, settings transaction or window.
type composerControls struct {
	prompt                                               promptEditor
	modelID, reasoning                                   string
	modelQuery, modelProvider, effortModel, effortReason string
	modelMenu, effortMenu, modelSearchOpen               bool
	modelList                                            ui.ListState
	modelCursor                                          int
	modelSearch                                          ui.Handle
	effortValue                                          float64
	effortMotion                                         effortMotion
}

// composerController binds reusable native controls to one captured session.
// The root App remains the only owner of backend dispatch and settings writes.
type composerController struct {
	*App
	*composerControls
	sessionID  string
	pendingNew bool
	side       *sideComposerState
}

func (a *App) centralControls() *composerController {
	return &composerController{App: a, composerControls: &a.composerControls, sessionID: a.sessionID, pendingNew: a.pendingNew}
}

func (a *App) sideControls(id string, state *sideComposerState) *composerController {
	return &composerController{App: a, composerControls: &state.composerControls, sessionID: id, side: state}
}

func (a *composerController) currentSession() contracts.Session {
	for _, session := range a.state.Sessions {
		if session.ID == a.sessionID {
			return session
		}
	}
	return contracts.Session{}
}

func (a *composerController) selectedModel() contracts.Model {
	for _, model := range a.state.Models {
		if model.ID == a.modelID {
			return model
		}
	}
	return contracts.Model{}
}

func (a *composerController) modelLabel() string {
	if a.sessionID == "" && a.modelNoAgents() {
		return a.t("Model management")
	}
	if model := a.selectedModel(); model.Label != "" {
		return model.Label
	}
	return a.t("Select model")
}

func (a *composerController) selectedProvider() contracts.Provider {
	for _, provider := range a.state.Providers {
		if provider.ID == a.selectedModel().ProviderID {
			return provider
		}
	}
	return contracts.Provider{}
}

func (a *App) runtimeModelLabel(model contracts.Model) string {
	for _, provider := range a.state.Providers {
		if provider.ID == model.ProviderID {
			return provider.Name + "/" + model.Label
		}
	}
	return model.Label
}

func (a *composerController) controlLabel(label string) string {
	if a.side != nil {
		return a.t(label) + a.t(" in ") + a.currentSession().Title
	}
	return a.t(label)
}

func (a *composerController) normalizeReasoning() {
	model := a.selectedModel()
	if a.state.Runtime != nil {
		if a.reasoning == "" && len(model.ReasoningLevels) > 0 {
			a.reasoning = model.ReasoningLevels[len(model.ReasoningLevels)-1]
		}
		return
	}
	if len(model.ReasoningLevels) == 0 {
		a.reasoning = ""
		return
	}
	for _, level := range model.ReasoningLevels {
		if level == a.reasoning {
			return
		}
	}
	a.reasoning = model.ReasoningLevels[0]
	for _, level := range model.ReasoningLevels {
		if level == "High" {
			a.reasoning = level
			break
		}
	}
}

func (a *composerController) modelChanged() {
	if a.side == nil {
		a.App.modelChanged()
		return
	}
	a.normalizeReasoning()
	if model := a.selectedModel(); model.ID != "" {
		if a.state.Settings.ModelPicker.Selections == nil {
			a.state.Settings.ModelPicker.Selections = make(map[string]contracts.ModelSelection)
		}
		a.state.Settings.ModelPicker.Selections[model.ProviderID] = contracts.ModelSelection{ModelID: model.ID, Reasoning: a.reasoning}
		a.state.Settings.ModelPicker.ProviderID = model.ProviderID
		a.saveSettings()
	}
	model, reasoning := a.modelID, a.reasoning
	a.saveComposerConfig(&contracts.SessionUpdate{ID: a.sessionID, ModelID: &model, ReasoningLevel: &reasoning})
}

func (a *composerController) saveComposerConfig(update *contracts.SessionUpdate) {
	command := contracts.Command{Kind: "session.update", Session: update}
	if a.side != nil {
		a.side.configSequence++
		command.Version = a.side.configSequence
		if a.side.configPending == nil {
			a.side.configPending = make(map[uint64]bool)
		}
		a.side.configPending[command.Version] = true
	}
	a.dispatch(command)
}

func (a *composerController) modelTrigger(c *ui.Context) (anchor, panel ui.Element) {
	label := "Select model"
	manageModels := a.sessionID == "" && a.modelNoAgents()
	if manageModels {
		label = "Model management"
	}
	button := ui.ButtonBase(c.Key("model-trigger")).Height(32).Padding(0, 6).Radius(8).MaxWidth(248).MinWidth(24).Shrink(1).Gap(6).AlignItems(ui.Center).FontWeight(500).Label(tr(c, a.controlLabel(label)))
	if a.modelMenu || button.Hovered() {
		button.Background(a.palette.Hover)
	}
	button.Children(func() {
		if !manageModels && a.selectedProvider().ID != "" {
			providerLogo(c, a.selectedProvider().Logo, 16)
		}
		ui.Text(c, a.modelLabel()).SingleLine().MinWidth(0).Shrink(1).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
	})
	button.OnClick(func() {
		if manageModels {
			a.openModelSettings()
			return
		}
		a.modelMenu = !a.modelMenu
		if a.modelMenu {
			a.effortMenu = false
			a.modelQuery = ""
			a.modelSearchOpen = false
			a.modelProvider = ""
			a.openModelList()
		}
	})
	return button, a.modelPopover(c, button)
}

// Root shell entry points delegate to the central composer.
func (a *App) modelRows() []modelRow     { return a.centralControls().modelRows() }
func (a *App) modelNoAgents() bool       { return a.centralControls().modelNoAgents() }
func (a *App) restoreModelSelection()    { a.centralControls().restoreModelSelection() }
func (a *App) modelProviderLocked() bool { return a.centralControls().modelProviderLocked() }
