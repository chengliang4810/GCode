package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"strings"
)

func accessLabel(mode contracts.AccessMode) string {
	switch mode {
	case contracts.AccessSupervised:
		return "Supervised"
	case contracts.AccessAutoAcceptEdits:
		return "Auto-accept edits"
	default:
		return "Full access"
	}
}
func (a *composerController) runControls(c *ui.Context) {
	if a.sessionID == "" && a.modelNoAgents() {
		return
	}
	provider := a.selectedProvider()
	if len(provider.AccessModes) == 0 {
		return
	}
	button := ui.ButtonBase(c).Label(tr(c, a.controlLabel("Access mode"))).Height(32).Padding(0, 10).Radius(8).MinWidth(0).Shrink(1).Gap(6).AlignItems(ui.Center).Justify(ui.Start)
	if button.Hovered() || a.accessMenu {
		button.Background(a.palette.Hover)
	}
	button.Children(func() {
		ui.Text(c, tr(c, accessLabel(a.accessMode))).SingleLine().MinWidth(0).Shrink(1).FontSize(design.UISize(c, 12)).FontWeight(500).TextColor(a.palette.Muted)
		if a.plan {
			ui.Text(c, tr(c, "Plan")).FontSize(design.UISize(c, 12)).TextColor(a.palette.Accent)
		}
	})
	button.OnClick(func() {
		a.accessMenu = !a.accessMenu
		if a.accessMenu {
			a.modelMenu = false
			a.accessCursor = 0
		}
	})
	ui.PopoverBase(c, button, &a.accessMenu, func(panel ui.Element) {
		_, height := c.Size()
		panel.Label(tr(c, a.controlLabel("Access options"))).Width(288).MaxHeight(min(float32(640), height*.75)).Padding(4).Radius(12).Background(a.palette.Dialog).Border(1, a.palette.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, .18)).AttachTo(button, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 6, 0).Focusable().AutoFocus()
		count := len(provider.AccessModes)
		if provider.SupportsPlan {
			count++
		}
		move := func(delta int) { a.accessCursor = (a.accessCursor + delta + count) % count }
		choose := func() {
			if a.accessCursor < len(provider.AccessModes) {
				a.accessMode = provider.AccessModes[a.accessCursor]
				a.accessMenu = false
			} else {
				a.plan = !a.plan
			}
			a.saveRunConfig()
		}
		panel.OnShortcut(0, ui.KeyDown, func() { move(1) }).OnShortcut(0, ui.KeyUp, func() { move(-1) }).OnShortcut(0, ui.KeyEnter, choose).OnShortcut(0, ui.KeySpace, choose)
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for i, mode := range provider.AccessModes {
				row := ui.ButtonBase(c).FillWidth().Padding(6, 8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).Label(tr(c, accessLabel(mode)))
				if mode == a.accessMode || a.accessCursor == i || row.Hovered() {
					row.Background(modelSelectionColor(a.palette))
				}
				row.OnClick(func() { a.accessMode = mode; a.accessMenu = false; a.saveRunConfig() }).OnHover(func(on bool) {
					if on {
						a.accessCursor = i
					}
				})
				row.Children(func() {
					accessRowText(c, accessLabel(mode), accessDescription(mode), a.palette.Text, a.palette.Muted)
					if mode == a.accessMode {
						icon(c, "check", 14)
					}
				})
			}
			if len(provider.AccessModes) == 1 {
				ui.Text(c, provider.Name+tr(c, " can't ask before acting, so it always runs with ")+strings.ToLower(tr(c, accessLabel(provider.AccessModes[0])))+".").FontSize(design.UISize(c, 11)).TextColor(a.palette.Muted).Padding(4, 8)
			}
			if provider.SupportsPlan {
				ui.Box(c).FillWidth().Height(1).Background(a.palette.Border).Margin(4, -4)
				row := ui.ToggleBase(c, &a.plan).FillWidth().Padding(6, 8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).Label(tr(c, "Plan mode")).OnClick(a.saveRunConfig)
				if row.Hovered() || a.accessCursor == len(provider.AccessModes) {
					row.Background(modelSelectionColor(a.palette))
				}
				row.Children(func() {
					accessRowText(c, "Plan mode", "Explore and propose a plan before changing anything.", a.palette.Text, a.palette.Muted)
					a.composerPlanSwitch(c, a.plan)
				})
			}
		})
	})
}

func accessRowText(c *ui.Context, label, description string, text, muted ui.Color) {
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		ui.Text(c, tr(c, label)).SingleLine().FontSize(design.UISize(c, 13)).FixedLineHeight(design.UISize(c, 19.5)).TextColor(text)
		ui.Text(c, tr(c, description)).FontSize(design.UISize(c, 11)).FixedLineHeight(design.UISize(c, 16.5)).TextColor(muted)
	})
}

func accessDescription(mode contracts.AccessMode) string {
	switch mode {
	case contracts.AccessSupervised:
		return "Ask before commands and file changes."
	case contracts.AccessAutoAcceptEdits:
		return "Apply edits, ask before other actions."
	default:
		return "Run commands and edits without asking."
	}
}

func resolveComposerAccess(wanted contracts.AccessMode, offered []contracts.AccessMode) contracts.AccessMode {
	if len(offered) == 0 {
		return contracts.AccessFullAccess
	}
	rank := func(mode contracts.AccessMode) int {
		switch mode {
		case contracts.AccessSupervised:
			return 0
		case contracts.AccessAutoAcceptEdits:
			return 1
		default:
			return 2
		}
	}
	resolved := offered[0]
	found := false
	for _, mode := range offered {
		if mode == wanted {
			return wanted
		}
		if rank(mode) <= rank(wanted) && (!found || rank(mode) > rank(resolved)) {
			resolved = mode
			found = true
		}
	}
	return resolved
}

func (a *composerController) selectedProvider() contracts.Provider {
	for _, provider := range a.state.Providers {
		if provider.ID == a.selectedModel().ProviderID {
			return provider
		}
	}
	return contracts.Provider{AccessModes: []contracts.AccessMode{contracts.AccessFullAccess}}
}

func (a *composerController) normalizeRunConfig() {
	provider := a.selectedProvider()
	a.accessMode = resolveComposerAccess(a.accessMode, provider.AccessModes)
	if !provider.SupportsPlan {
		a.plan = false
	}
}
func (a *composerController) saveRunConfig() {
	if a.sessionID != "" {
		mode, plan := a.accessMode, a.plan
		a.saveComposerConfig(&contracts.SessionUpdate{ID: a.sessionID, AccessMode: &mode, Plan: &plan})
	}
}
func (a *App) attachAnnotation(annotation contracts.Annotation) {
	annotation = annotation.Bounded()
	a.annotations = append(a.annotations, annotation)
	a.route = "chat"
	a.focusComposer = true
}
func (a *App) annotationChips(c *ui.Context) {
	for i, annotation := range a.annotations {
		index, item := i, annotation
		ui.Row(c).Padding(4, 7).Gap(5).Radius(7).Border(1, a.palette.Border).Tooltip(item.Title + " · " + item.Element).OnHover(func(hovered bool) {
			if hovered {
				a.workspace.FocusAnnotation(item.Index)
			} else {
				a.workspace.FocusAnnotation(0)
			}
		}).Children(func() {
			icon(c, "cursor-square", 13)
			ui.Text(c, a.annotationLabel(item.Index)).FontSize(design.UISize(c, 11))
			iconButton(c, "close", tr(c, "Remove ")+a.annotationLabel(item.Index)).Size(16, 16).OnClick(func() { a.removeAnnotation(index) })
		})
	}
}

func (a *App) removeAnnotation(index int) {
	if index < 0 || index >= len(a.annotations) {
		return
	}
	a.annotations = append(a.annotations[:index], a.annotations[index+1:]...)
	indices := make([]uint32, 0, len(a.annotations))
	for _, annotation := range a.annotations {
		indices = append(indices, annotation.Index)
	}
	a.workspace.KeepAnnotations(indices)
}
