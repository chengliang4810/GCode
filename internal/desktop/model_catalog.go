package desktop

import (
	"math"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *composerController) modelCatalogState(c *ui.Context, budget float32) bool {
	status := a.state.ModelCatalog.State
	if status != contracts.CatalogLoading && status != contracts.CatalogError && a.modelNoAgents() {
		if a.sessionID != "" && a.selectedModel().ID != "" {
			return false
		}
		status = contracts.CatalogNoAgents
	}
	switch status {
	case contracts.CatalogLoading:
		ui.Column(c).FillWidth().Height(min(float32(144), budget)).Padding(8).Label("Loading models").Children(func() { a.modelSkeleton(c) })
	case contracts.CatalogError:
		ui.Column(c).FillWidth().Padding(16).Gap(6).Children(func() {
			ui.Text(c, a.state.ModelCatalog.Error).FontSize(design.UISize(c, 12)).TextColor(a.palette.Danger)
			textButton(c, "Retry").Border(1, a.palette.Border).OnClick(func() { a.dispatch(contracts.Command{Kind: "catalog.retry"}) })
		})
	case contracts.CatalogNoAgents:
		ui.Column(c).FillWidth().Padding(16).Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, "terminal", 20)
			ui.Text(c, "No agents available").FontSize(design.UISize(c, 13))
			ui.Text(c, "Enable an installed agent in Settings → Providers, or install an agent CLI.").FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted).TextAlign(ui.Center)
		})
	default:
		return false
	}
	return true
}

func (a *composerController) modelNoAgents() bool {
	switch a.state.ModelCatalog.State {
	case contracts.CatalogLoading, contracts.CatalogError:
		return false
	case contracts.CatalogNoAgents:
		return true
	}
	for _, provider := range a.state.Providers {
		if provider.Enabled {
			return false
		}
	}
	return true
}

func (a *composerController) modelProviderOffered(provider contracts.Provider) bool {
	return provider.Enabled || a.sessionID != "" && provider.ID == a.selectedModel().ProviderID
}

func (a *composerController) restoreModelSelection() {
	if a.sessionID != "" {
		return
	}
	if current := a.selectedModel(); current.ID != "" {
		for _, provider := range a.state.Providers {
			if provider.ID == current.ProviderID && provider.Enabled {
				return
			}
		}
	}
	if len(a.state.Models) == 0 && (a.state.ModelCatalog.State == contracts.CatalogLoading || a.state.ModelCatalog.State == contracts.CatalogError) {
		return
	}
	a.modelID, a.reasoning = "", ""
	preferred := a.state.Settings.ModelPicker.ProviderID
	remembered := a.state.Settings.ModelPicker.Selections[preferred]
	for _, provider := range a.state.Providers {
		if !provider.Enabled || provider.ID != preferred {
			continue
		}
		for _, model := range a.state.Models {
			if model.ProviderID == preferred && model.ID == remembered.ModelID {
				a.modelID, a.reasoning = model.ID, remembered.Reasoning
				a.normalizeReasoning()
				return
			}
		}
		for _, model := range a.state.Models {
			if model.ProviderID == preferred {
				a.modelID = model.ID
				a.normalizeReasoning()
				return
			}
		}
	}
	for _, provider := range a.state.Providers {
		if !provider.Enabled {
			continue
		}
		for _, model := range a.state.Models {
			if model.ProviderID == provider.ID {
				a.modelID = model.ID
				a.normalizeReasoning()
				return
			}
		}
	}
}

// Ghost labels follow popover.rs::skeleton_menu_rows. Layout and timing use
// native boxes and the window clock; no task is started from rendering.
func (a *composerController) modelSkeleton(c *ui.Context) {
	root := ui.Column(c).FillWidth().Padding(6, 4).Gap(8)
	start := ui.Local(root, "pulse-start", func() time.Time { return c.Now() })
	phase := float64(c.Now().Sub(*start)) / float64(2400*time.Millisecond)
	root.Children(func() {
		widths := [4]float32{42, 58, 48, 66}
		for i := 0; i < 5; i++ {
			opacity := float32(.55)
			if !a.reduceMotion(c) {
				opacity = .35 + .4*float32((1-math.Cos(2*math.Pi*(phase-float64(i)*.08)))/2)
			}
			ui.Box(c).Height(14).WidthPercent(widths[i%4]).Radius(7).Background(a.palette.Text.Alpha(.05)).Opacity(opacity)
		}
	})
	if !a.reduceMotion(c) {
		c.After(time.Second / 60)
	}
}
