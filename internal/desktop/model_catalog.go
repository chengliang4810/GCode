package desktop

import (
	"math"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// modelCatalogStatus returns the catalog status this composer shows. The
// aggregate status may describe local agents while this composer targets
// another device, so the provider details own availability.
func (a *composerController) modelCatalogStatus() contracts.ModelCatalogStatus {
	status := a.state.ModelCatalog.State
	if status != contracts.CatalogLoading && status != contracts.CatalogError && a.modelNoAgents() {
		if a.sessionID != "" && a.selectedModel().ID != "" {
			return contracts.CatalogReady
		}
		return contracts.CatalogNoAgents
	}
	if status == contracts.CatalogNoAgents {
		// The catalog's aggregate status may describe local agents while this
		// composer targets another device. Its provider details own availability.
		return contracts.CatalogReady
	}
	return status
}

// modelCatalogSkeleton reports whether the picker waits for the remote
// catalog. The model list comes from the local provider configuration and never
// waits for the network, so a refresh only shows a skeleton while no model is
// configured at all.
func (a *composerController) modelCatalogSkeleton() bool {
	return a.modelCatalogStatus() == contracts.CatalogLoading && len(a.state.Models) == 0
}

// modelCatalogReplacesList reports whether the catalog panel stands in for the
// model list. A loading catalog keeps an existing list.
func (a *composerController) modelCatalogReplacesList() bool {
	switch a.state.ModelCatalog.State {
	case contracts.CatalogReady, "":
		return false
	case contracts.CatalogLoading:
		return a.modelCatalogSkeleton()
	}
	return true
}

func (a *composerController) modelCatalogState(c *ui.Context, budget float32) bool {
	switch a.modelCatalogStatus() {
	case contracts.CatalogLoading:
		if !a.modelCatalogSkeleton() {
			return false
		}
		ui.Column(c).FillWidth().Height(min(float32(144), budget)).Padding(8).Label(tr(c, "Loading models")).Children(func() { a.modelSkeleton(c) })
	case contracts.CatalogError:
		ui.Column(c).FillWidth().Padding(16).Gap(6).Children(func() {
			ui.Text(c, a.state.ModelCatalog.Error).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Readable(a.palette.Danger))
			secondaryButton(c, tr(c, "Retry")).OnClick(func() { a.dispatch(contracts.Command{Kind: "catalog.retry"}) })
		})
	case contracts.CatalogNoAgents:
		ui.Column(c).FillWidth().Padding(16).Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, "terminal", 20)
			if a.state.Runtime != nil {
				ui.Text(c, tr(c, "Add your provider to start a conversation.")).FontSize(design.TextSize(c, design.TextBase))
				textButton(c, tr(c, "Add provider")).OnClick(func() { a.navigate("settings/Providers") })
			} else {
				ui.Text(c, tr(c, "No models configured")).FontSize(design.TextSize(c, design.TextBase))
				ui.Text(c, tr(c, "Configure a provider and enable a model in Settings.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted).TextAlign(ui.Center)
			}
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
	}
	for _, provider := range a.state.Providers {
		if a.modelProviderAvailable(provider.ID) {
			return false
		}
	}
	return true
}

func (a *composerController) modelTarget() string {
	if a.side != nil {
		if target := a.currentSession().DeviceID; target != "" {
			return target
		}
	} else if a.deviceID != "" {
		return a.deviceID
	}
	for _, device := range a.state.Devices {
		if device.Current {
			return device.ID
		}
	}
	return "local"
}

func (a *composerController) modelProviderDetails(id string) contracts.ProviderDetails {
	target := a.modelTarget()
	for _, provider := range a.state.Integrations.Providers {
		if provider.TargetID == target && provider.ProviderID == id {
			return provider
		}
	}
	return contracts.ProviderDetails{}
}

func (a *composerController) modelProviderAvailable(id string) bool {
	provider := a.modelProviderDetails(id)
	return provider.Installed && provider.Enabled
}

func (a *composerController) modelProviderOffered(provider contracts.Provider) bool {
	if a.picker != nil {
		return true
	}
	details := a.modelProviderDetails(provider.ID)
	return details.Installed && (details.Enabled || a.sessionID != "" && provider.ID == a.selectedModel().ProviderID)
}

func (a *composerController) restoreModelSelection() {
	if a.sessionID != "" {
		return
	}
	if current := a.selectedModel(); current.ID != "" {
		if a.modelProviderAvailable(current.ProviderID) {
			return
		}
	}
	if len(a.state.Models) == 0 && (a.state.ModelCatalog.State == contracts.CatalogLoading || a.state.ModelCatalog.State == contracts.CatalogError) {
		return
	}
	a.modelID, a.reasoning = "", ""
	preferred := a.state.Settings.ModelPicker.ProviderID
	remembered := a.state.Settings.ModelPicker.Selections[preferred]
	for _, provider := range a.state.Providers {
		if !a.modelProviderAvailable(provider.ID) || provider.ID != preferred {
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
		if !a.modelProviderAvailable(provider.ID) {
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

// modelSkeletonPeriod is the shared breathing cycle of every ghost row.
const modelSkeletonPeriod = 2 * time.Second

// Ghost labels follow popover.rs::skeleton_menu_rows. Layout and timing use
// native boxes and the window clock; no task is started from rendering.
func (a *composerController) modelSkeleton(c *ui.Context) {
	root := ui.Column(c).FillWidth().Padding(6, 4).Gap(8)
	start := ui.Local(root, "pulse-start", func() time.Time { return c.Now() })
	// A dense list breathes in lockstep: every ghost shares one phase over a
	// two-second cycle, so the rows read as one placeholder instead of a wave.
	phase := float64(c.Now().Sub(*start)) / float64(modelSkeletonPeriod)
	opacity := float32(.55)
	if !a.reduceMotion(c) {
		opacity = .35 + .4*float32((1-math.Cos(2*math.Pi*phase))/2)
	}
	root.Children(func() {
		widths := [4]float32{42, 58, 48, 66}
		for i := 0; i < 5; i++ {
			ui.Box(c).Height(14).WidthPercent(widths[i%4]).Radius(7).Background(a.palette.Text.Alpha(.05)).Opacity(opacity)
		}
	})
	if !a.reduceMotion(c) {
		c.After(time.Second / 30)
	}
}
