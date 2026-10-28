package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// composerControls is UI/configuration state, shared by the central and side
// composers. It owns no service, goroutine, settings transaction or window.
type composerControls struct {
	modelID, reasoning                                   string
	accessMode                                           contracts.AccessMode
	plan                                                 bool
	modelQuery, modelProvider, effortModel, effortReason string
	modelMenu, modelListPage                             bool
	accessMenu                                           bool
	accessCursor                                         int
	modelList                                            ui.ListState
	modelCursor                                          int
	modelSearch                                          ui.Handle
	effortValue                                          float64
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
		return "No agents"
	}
	if model := a.selectedModel(); model.Label != "" {
		return model.Label
	}
	return "Select model"
}

func (a *composerController) controlLabel(label string) string {
	if a.side != nil {
		return label + " in " + a.currentSession().Title
	}
	return label
}

func (a *composerController) normalizeReasoning() {
	model := a.selectedModel()
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
	a.normalizeRunConfig()
	if model := a.selectedModel(); model.ID != "" {
		if a.state.Settings.ModelPicker.Selections == nil {
			a.state.Settings.ModelPicker.Selections = make(map[string]contracts.ModelSelection)
		}
		a.state.Settings.ModelPicker.Selections[model.ProviderID] = contracts.ModelSelection{ModelID: model.ID, Reasoning: a.reasoning}
		a.state.Settings.ModelPicker.ProviderID = model.ProviderID
		a.saveSettings()
	}
	model, reasoning, mode, plan := a.modelID, a.reasoning, a.accessMode, a.plan
	a.saveComposerConfig(&contracts.SessionUpdate{ID: a.sessionID, ModelID: &model, ReasoningLevel: &reasoning, AccessMode: &mode, Plan: &plan})
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

func (a *composerController) modelTrigger(c *ui.Context) {
	button := ui.ButtonBase(c).Height(32).Padding(0, 6).Radius(8).MaxWidth(248).MinWidth(24).Shrink(1).Gap(6).AlignItems(ui.Center).Label(a.controlLabel("Select model"))
	button.Children(func() {
		glyph := "terminal"
		for _, provider := range a.state.Providers {
			if a.sessionID == "" && a.modelNoAgents() {
				break
			}
			if provider.ID == a.selectedModel().ProviderID && provider.Icon != "" {
				glyph = provider.Icon
			}
		}
		icon(c, glyph, 16)
		ui.Text(c, a.modelLabel()).SingleLine().MinWidth(0).Shrink(1).FontSize(design.UISize(c, 12)).FontWeight(500)
		if len(a.selectedModel().ReasoningLevels) > 0 && (a.sessionID != "" || !a.modelNoAgents()) {
			ui.Text(c, a.reasoning).SingleLine().MinWidth(0).Shrink(1).FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted.Alpha(.7))
		}
	})
	button.OnClick(func() {
		a.modelMenu = !a.modelMenu
		if a.modelMenu {
			a.accessMenu = false
		}
		if a.modelMenu {
			a.modelListPage = false
			a.modelQuery = ""
			a.modelProvider = ""
			if !a.state.Settings.General.CompactModelPicker {
				a.openModelList()
			}
		}
	})
	a.modelPopover(c, button)
}

// Compatibility wrappers preserve the root shell's central-only entry points.
func (a *App) normalizeRunConfig()           { a.centralControls().normalizeRunConfig() }
func (a *App) modelRows() []modelRow         { return a.centralControls().modelRows() }
func (a *App) modelNoAgents() bool           { return a.centralControls().modelNoAgents() }
func (a *App) restoreModelSelection()        { a.centralControls().restoreModelSelection() }
func (a *App) modelProviderLocked() bool     { return a.centralControls().modelProviderLocked() }
func (a *App) selectModelProvider(id string) { a.centralControls().selectModelProvider(id) }
func (a *App) isModelFavorite(model contracts.Model) bool {
	return a.centralControls().isModelFavorite(model)
}
func (a *App) toggleModelFavorite(model contracts.Model) {
	a.centralControls().toggleModelFavorite(model)
}
