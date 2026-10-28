package desktop

import (
	"gcode/internal/design"
	"maps"
	"math"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// The compact panel geometry follows pickers/compact.rs.
const compactModelWidth float32 = 256

func (a *composerController) modelPopover(c *ui.Context, anchor ui.Element) {
	ui.PopoverBase(c, anchor, &a.modelMenu, func(panel ui.Element) {
		if a.state.Settings.General.CompactModelPicker && a.modelListPage && panel.Shortcut(0, ui.KeyEscape) {
			a.modelListPage = false
		}
		_, height := c.Size()
		bounds := anchor.Bounds()
		above, below := max(float32(0), bounds.Y-14), max(float32(0), height-bounds.Y-bounds.H-14)
		openBelow := a.sessionID == "" && below >= 180 || above < 180 && below > above
		budget := above
		if openBelow {
			panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0)
			budget = below
		} else {
			panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 6, 0)
		}
		panel.Label(a.controlLabel("Model picker")).Width(compactModelWidth).MaxHeight(min(float32(640), budget)).Padding(0).Radius(12).Background(a.palette.Dialog).Border(1, a.palette.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, .18))
		if a.state.Settings.General.CompactModelPicker && !a.modelListPage {
			ui.Column(c).FillWidth().Padding(4).Children(func() { a.compactModelPanel(c) })
		} else {
			if !a.state.Settings.General.CompactModelPicker {
				panel.Width(304)
			}
			a.modelListPanel(c, min(float32(640), budget))
		}
	})
}

func (a *composerController) compactModelPanel(c *ui.Context) {
	model := a.selectedModel()
	levels := model.ReasoningLevels
	if a.sessionID == "" && a.modelNoAgents() {
		levels = nil
	}
	height := float32(36)
	if len(levels) > 0 {
		height = 48
	}
	button := ui.ButtonBase(c).Height(height).FillWidth().Padding(8).Radius(7).Label(a.controlLabel("Change model")).AlignItems(ui.Center).Justify(ui.Start).OnClick(a.openModelList)
	if button.Hovered() {
		button.Background(a.palette.Hover)
	}
	button.Children(func() {
		ui.Column(c).Grow(1).AlignItems(ui.Start).Children(func() {
			if len(levels) > 0 {
				ui.Text(c, a.reasoning).FontSize(design.UISize(c, 14)).FontWeight(600).FixedLineHeight(17)
			}
			ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
				size := float32(12)
				if len(levels) == 0 {
					size = 14
				}
				color, weight, lineHeight := a.palette.Muted, 400, float32(15)
				if len(levels) == 0 {
					color, weight, lineHeight = a.palette.Text, 600, 17
				}
				ui.Text(c, a.modelLabel()).FontSize(design.UISize(c, size)).FontWeight(weight).TextColor(color).FixedLineHeight(lineHeight).SingleLine()
				icon(c, "alt-arrow-right", 10)
			})
		})
	})
	if len(levels) > 1 {
		if a.effortModel != model.ID || a.effortReason != a.reasoning {
			a.effortValue = 0
			for i, level := range levels {
				if level == a.reasoning {
					a.effortValue = float64(i)
				}
			}
			a.effortModel, a.effortReason = model.ID, a.reasoning
		}
		ui.Column(c).FillWidth().Padding(2, 8, 4, 8).Children(func() {
			ui.SliderBase(c, &a.effortValue, 0, float64(len(levels)-1)).Step(1).Label(a.controlLabel("Reasoning effort")).Height(36).FillWidth().Padding(0, 22).Draw(func(p *ui.Painter, r ui.Rect) {
				fraction := float32(a.effortValue / float64(len(levels)-1))
				x := r.X + 22 + (r.W-44)*fraction
				p.Fill(ui.Rect{X: r.X, Y: r.Y + 4, W: r.W, H: 28}, a.palette.Input, 14)
				p.Fill(ui.Rect{X: r.X, Y: r.Y + 4, W: min(r.W, x-r.X+14), H: 28}, a.palette.Accent.Alpha(.3).Over(a.palette.Input), 14)
				for i := range levels {
					dot := r.X + 22 + (r.W-44)*float32(i)/float32(len(levels)-1)
					p.Fill(ui.Rect{X: dot - 1.5, Y: r.Y + 16.5, W: 3, H: 3}, a.palette.Muted.Alpha(.45), 1.5)
				}
				thumb := ui.Rect{X: x - 22, Y: r.Y + 2, W: 44, H: 32}
				shadowAlpha := float32(.14)
				if a.palette.Dark {
					shadowAlpha = .22
				}
				p.Shadow(thumb, 16, 0, 1, 2, 0, ui.Hex("#000000").Alpha(shadowAlpha))
				if !a.palette.Dark {
					p.Shadow(thumb, 16, 0, 2, 6, 0, ui.Hex("#000000").Alpha(.06))
				}
				p.FillGradient(thumb, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{{At: 0, Color: ui.Hex("#ffffff")}, {At: 1, Color: ui.Hex("#f6f6f6")}}}, 16)
				p.Stroke(ui.Rect{X: thumb.X + .5, Y: thumb.Y + .5, W: 43, H: 31}, ui.Hex("#000000").Alpha(.11), 15.5, 1)
			}).OnChange(func() {
				index := max(0, min(len(levels)-1, int(math.Round(a.effortValue))))
				a.reasoning = levels[index]
				a.effortReason = a.reasoning
				a.modelChanged()
			})
		})
	}
	a.modelOptionRows(c, model)
}

func (a *composerController) modelOptionRows(c *ui.Context, model contracts.Model) {
	if len(model.Options) == 0 {
		return
	}
	ui.Scroll(c).FillWidth().MaxHeight(170).Gap(2).Padding(4, 4, 0, 4).Children(func() {
		for _, option := range model.Options {
			option := option
			key := model.ID + "/" + option.ID
			value := a.state.Settings.ModelPicker.Options[key]
			if value == "" {
				value = option.DefaultChoice
			}
			ui.Row(c).Height(26).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, option.Label).FontSize(design.UISize(c, 12)).Grow(1)
				ui.MenuButton(c, value, func(menu *ui.Menu) {
					for _, choice := range option.Choices {
						if menu.Item(choice).Checked(value == choice).Chosen() {
							values := maps.Clone(a.state.Settings.ModelPicker.Options)
							if values == nil {
								values = make(map[string]string)
							}
							values[key] = choice
							a.state.Settings.ModelPicker.Options = values
							a.saveSettings()
						}
					}
				}).FontSize(design.UISize(c, 12)).Background(ui.Transparent)
			})
		}
	})
}

func (a *composerController) openModelList() {
	a.modelListPage = true
	a.modelCursor = 0
	for i, row := range a.modelRows() {
		if row.model.ID == a.modelID {
			a.modelCursor = i
			break
		}
	}
	a.modelList.ScrollIntoView(a.modelCursor)
}

func (a *composerController) activateModelRow(rows []modelRow, index int) {
	if index < 0 || index >= len(rows) {
		return
	}
	if a.modelProviderLocked() && rows[index].model.ProviderID != a.selectedModel().ProviderID {
		return
	}
	a.modelID = rows[index].model.ID
	a.modelChanged()
	a.modelListPage = false
}

func (a *composerController) modelListPanel(c *ui.Context, budget float32) {
	if a.modelCatalogState(c, budget) {
		return
	}
	rows := a.modelRows()
	for i := 0; i < min(9, len(rows)); i++ {
		index := i
		c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { a.activateModelRow(rows, index) })
	}
	ui.Row(c).Height(40).FillWidth().Padding(4).AlignItems(ui.Center).Gap(2).Children(func() {
		if a.state.Settings.General.CompactModelPicker {
			iconButton(c, "alt-arrow-left", "Back to model options").Size(32, 32).OnClick(func() { a.modelListPage = false })
		}
		ui.ScrollHorizontal(c).Grow(1).Children(func() {
			ui.Row(c).Gap(2).Children(func() {
				if !a.state.Settings.General.CompactModelPicker || len(a.state.Settings.ModelPicker.Favorites) > 0 {
					a.modelGroupButton(c, "favorites", "star", "Favorites", rows)
				}
				for _, provider := range a.state.Providers {
					if a.modelProviderOffered(provider) && (!a.modelProviderLocked() || provider.ID == a.selectedModel().ProviderID) {
						a.modelGroupButton(c, provider.ID, provider.Icon, provider.Name, rows)
					}
				}
			})
		})
	})
	ui.Row(c).Height(40).FillWidth().Padding(0, 10).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border.Alpha(.6)).Children(func() {
		icon(c, "magnifer", 14)
		ui.TextInputBase(c, &a.modelQuery).Bind(&a.modelSearch).Label(a.controlLabel("Search models")).Placeholder("Search models…").Grow(1).FontSize(design.UISize(c, 13)).AutoFocus().OnChange(func() { a.modelCursor = 0; a.modelList.ScrollIntoView(0) }).OnTextKey(func(k ui.TextKey) bool {
			// The native input can deliver text and Enter before the next view.
			// Filter against that text rather than a previous frame's rows.
			a.modelQuery = k.Text()
			currentRows := a.modelRows()
			if len(currentRows) > 0 && (a.modelCursor < 0 || a.modelCursor >= len(currentRows)) {
				a.modelCursor = 0
			}
			switch k.Key {
			case ui.KeyDown, ui.KeyUp:
				if len(currentRows) > 0 {
					delta := 1
					if k.Key == ui.KeyUp {
						delta = -1
					}
					a.modelCursor = (a.modelCursor + delta + len(currentRows)) % len(currentRows)
					a.modelList.ScrollIntoView(a.modelCursor)
				}
				return true
			case ui.KeyEnter:
				a.activateModelRow(currentRows, a.modelCursor)
				return true
			}
			return false
		})
	})
	trayHeight := float32(0)
	if !a.state.Settings.General.CompactModelPicker && (len(a.selectedModel().ReasoningLevels) > 0 || len(a.selectedModel().Options) > 0) {
		trayHeight = 34 + 28*float32(len(a.selectedModel().Options))
	}
	rowCount := min(7, len(rows))
	if rowCount == 0 {
		rowCount = 4
	}
	height := min(float32(rowCount)*34+8, max(float32(40), budget-82-trayHeight))
	if len(rows) == 0 {
		a.modelCursor = -1
	} else if a.modelCursor >= len(rows) || a.modelCursor < 0 {
		a.modelCursor = 0
	}
	a.modelList.Key = func(i int) any { return rows[i].group + ":" + rows[i].model.ID }
	a.modelList.Label = func(i int) string { return rows[i].model.Label }
	a.modelList.Selected = &a.modelCursor
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyF, func() {
		if a.modelCursor >= 0 && a.modelCursor < len(rows) {
			a.toggleModelFavorite(rows[a.modelCursor].model)
		}
	})
	previousTheme := c.Theme()
	listTheme := *previousTheme
	listTheme.Accent = modelSelectionColor(a.palette)
	listTheme.AccentText = a.palette.Text
	listTheme.SurfaceHover = a.palette.Hover.Over(a.palette.Dialog)
	if len(rows) == 0 {
		ui.Column(c).FillWidth().Height(height).Padding(8).Children(func() {
			if strings.TrimSpace(a.modelQuery) != "" || a.modelProvider == "favorites" {
				note := "No models found"
				if a.modelQuery == "" && a.modelProvider == "favorites" {
					note = "No starred models yet — hit a row's star"
				}
				ui.Column(c).Fill().Center().Children(func() { ui.Text(c, note).FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted) })
			} else {
				a.modelSkeleton(c)
			}
		})
	} else {
		c.SetTheme(&listTheme)
		list := ui.List(c, &a.modelList, len(rows), func(i int) {
			row := rows[i]
			m := row.model
			selected := a.modelID == m.ID
			button := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 4, 0, 8).Radius(7).Gap(8).Justify(ui.Start).AlignItems(ui.Center).TextColor(a.palette.Text).Background(a.palette.Dialog).Label(m.Label).Tooltip(m.Label + " · " + row.provider.Name + "\n" + m.Description)
			if selected {
				button.Background(modelSelectionColor(a.palette)).DrawOver(func(p *ui.Painter, r ui.Rect) {
					ring := ui.Hex("#000000").Alpha(.07)
					if a.palette.Dark {
						ring = ui.Hex("#ffffff").Alpha(.09)
					}
					p.Stroke(ui.Rect{X: r.X + .5, Y: r.Y + .5, W: r.W - 1, H: r.H - 1}, ring, 6.5, 1)
				})
			} else if button.Hovered() || a.modelCursor == i {
				button.Background(a.palette.Hover.Over(a.palette.Dialog))
			}
			button.OnClick(func() { a.activateModelRow(rows, i) }).OnHover(func(hovered bool) {
				if hovered {
					a.modelCursor = i
				}
			})
			button.Children(func() {
				icon(c, row.provider.Icon, 14)
				ui.Text(c, m.Label).Grow(1).SingleLine().FontSize(design.UISize(c, 12)).FontWeight(500)
				favorite := a.isModelFavorite(m)
				if favorite || button.Hovered() {
					glyph, label := "star", "Add "+m.Label+" to favorites"
					if favorite {
						glyph, label = "star-bold", "Remove "+m.Label+" from favorites"
					}
					iconButton(c, glyph, label).Size(24, 24).OnClick(func() { a.toggleModelFavorite(m) })
				}
			})
		}).Height(height).Gap(2).Padding(4)
		c.SetTheme(previousTheme)
		// SetTheme also writes the root's fill; preserve the shell's native glass.
		c.Root().Background(design.Glass(a.palette, a.state.Settings.Appearance.Surface, c.Vibrancy()))
		list.OnSubmit(func() { a.activateModelRow(rows, a.modelCursor) })
	}
	if trayHeight > 0 {
		ui.Column(c).FillWidth().Padding(4).BorderWidth(1, 0, 0, 0).BorderColor(a.palette.Border.Alpha(.6)).Children(func() {
			levels := a.selectedModel().ReasoningLevels
			if len(levels) > 0 {
				ui.Row(c).FillWidth().Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Reasoning").FontSize(design.UISize(c, 12)).Grow(1)
					ui.MenuButton(c, a.reasoning, func(menu *ui.Menu) {
						for _, level := range levels {
							if menu.Item(level).Checked(a.reasoning == level).Chosen() {
								a.reasoning = level
								a.modelChanged()
							}
						}
					}).FontSize(design.UISize(c, 12)).BorderWidth(0).Background(ui.Transparent).Label("Reasoning level")
				})
			}
			a.modelOptionRows(c, a.selectedModel())
		})
	}
}

func modelSelectionColor(palette design.Tokens) ui.Color {
	wash := ui.Hex("#1a1a1a").Alpha(.06)
	if palette.Dark {
		wash = ui.Hex("#ebebeb").Alpha(.11)
	}
	return wash.Over(palette.Dialog)
}

type modelRow struct {
	model    contracts.Model
	provider contracts.Provider
	group    string
}

func (a *composerController) modelRows() []modelRow {
	var rows []modelRow
	query := strings.ToLower(strings.TrimSpace(a.modelQuery))
	providerID := a.modelProvider
	if providerID == "" {
		providerID = a.selectedProvider().ID
	}
	appendGroup := func(favorites bool) {
		for _, provider := range a.state.Providers {
			if !a.modelProviderOffered(provider) || a.modelProviderLocked() && provider.ID != a.selectedModel().ProviderID {
				continue
			}
			for _, model := range a.state.Models {
				if model.ProviderID != provider.ID || favorites && !a.isModelFavorite(model) {
					continue
				}
				if !a.state.Settings.General.CompactModelPicker && (providerID == "favorites" && !a.isModelFavorite(model) || providerID != "favorites" && provider.ID != providerID) {
					continue
				}
				matches := true
				for _, word := range strings.Fields(query) {
					if !strings.Contains(strings.ToLower(model.Label+" "+model.Description+" "+provider.Name), word) {
						matches = false
						break
					}
				}
				if !matches {
					continue
				}
				group := provider.ID
				if favorites {
					group = "favorites"
				}
				rows = append(rows, modelRow{model, provider, group})
			}
		}
	}
	if query == "" {
		if a.state.Settings.General.CompactModelPicker {
			appendGroup(true)
		}
	}
	appendGroup(false)
	return rows
}

func (a *composerController) modelGroupButton(c *ui.Context, id, glyph, label string, rows []modelRow) {
	b := iconButton(c, glyph, label).Size(32, 32)
	current := a.modelProvider
	if current == "" {
		current = a.selectedProvider().ID
	}
	if current == id {
		if a.state.Settings.General.CompactModelPicker {
			b.Background(a.palette.Text.Alpha(.08).Over(a.palette.Dialog))
		} else {
			b.Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(ui.Rect{X: r.X + 6, Y: r.Y + r.H + 2, W: r.W - 12, H: 2}, a.palette.Accent, 1)
			})
		}
	}
	b.OnClick(func() {
		a.modelProvider = id
		if !a.state.Settings.General.CompactModelPicker {
			if id != "favorites" {
				a.selectModelProvider(id)
			}
			a.openModelList()
			return
		}
		for i, row := range rows {
			if row.group == id {
				a.modelList.ScrollIntoView(i)
				break
			}
		}
	})
}

func (a *composerController) selectModelProvider(providerID string) {
	if a.modelProviderLocked() && providerID != a.selectedModel().ProviderID {
		return
	}
	remembered := a.state.Settings.ModelPicker.Selections[providerID]
	for _, model := range a.state.Models {
		if model.ProviderID == providerID && model.ID == remembered.ModelID {
			a.modelID, a.reasoning = model.ID, remembered.Reasoning
			a.modelChanged()
			return
		}
	}
	for _, model := range a.state.Models {
		if model.ProviderID == providerID {
			a.modelID, a.reasoning = model.ID, ""
			a.modelChanged()
			return
		}
	}
}

func (a *composerController) modelProviderLocked() bool {
	status := a.currentSession().Status
	return a.sessionID != "" && (a.pendingNew || status == contracts.Working || status == contracts.AwaitingInput)
}

func (a *composerController) isModelFavorite(model contracts.Model) bool {
	for _, favorite := range a.state.Settings.ModelPicker.Favorites {
		if favorite.ProviderID == model.ProviderID && favorite.ModelID == model.ID {
			return true
		}
	}
	return false
}

func (a *composerController) toggleModelFavorite(model contracts.Model) {
	items := append([]contracts.ModelRef(nil), a.state.Settings.ModelPicker.Favorites...)
	found := false
	for i, item := range items {
		if item.ProviderID == model.ProviderID && item.ModelID == model.ID {
			items = append(items[:i], items[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		items = append(items, contracts.ModelRef{ProviderID: model.ProviderID, ModelID: model.ID})
	}
	a.state.Settings.ModelPicker.Favorites = items
	a.saveSettings()
}
