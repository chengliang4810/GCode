package desktop

import (
	"maps"
	"math"
	"runtime"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The compact panel geometry follows pickers/compact.rs.
const compactModelWidth float32 = 256

func (a *composerController) compactPanelShortcuts(c *ui.Context) {
	for _, key := range []ui.Key{ui.KeyUp, ui.KeyDown, ui.KeyEnter, ui.KeySpace} {
		c.OnShortcut(0, key, func() {
			a.openModelList()
			if count := len(a.modelRows()); count > 0 && (key == ui.KeyUp || key == ui.KeyDown) {
				delta := 1
				if key == ui.KeyUp {
					delta = -1
				}
				a.modelCursor = (a.modelCursor + delta + count) % count
				a.modelList.ScrollIntoView(a.modelCursor)
			}
		})
	}
	for _, key := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyHome, ui.KeyEnd} {
		c.OnShortcut(0, key, func() {
			effort := a.compactEffort()
			if len(effort.levels) == 0 {
				return
			}
			next := effort.selected
			switch key {
			case ui.KeyLeft:
				next = max(0, next-1)
			case ui.KeyRight:
				next = min(len(effort.levels)-1, next+1)
			case ui.KeyHome:
				next = 0
			case ui.KeyEnd:
				next = len(effort.levels) - 1
			}
			a.pickCompactEffort(effort, next)
		})
	}
	c.OnShortcut(0, ui.KeyF, func() {
		model := a.selectedModel()
		option, on, off, fast := a.compactFastOption(model)
		if option.ID == "" {
			return
		}
		choice := on
		if fast {
			choice = off
		}
		a.pickModelOption(model, option, choice)
	})
	c.OnShortcut(0, ui.KeyTab, func() { a.cycleCompactProvider(1) })
	c.OnShortcut(ui.Shift, ui.KeyTab, func() { a.cycleCompactProvider(-1) })
}

func (a *composerController) cycleCompactProvider(delta int) {
	if a.modelProviderLocked() {
		return
	}
	var providers []string
	for _, provider := range a.state.Providers {
		if a.modelProviderOffered(provider) {
			providers = append(providers, provider.ID)
		}
	}
	if len(providers) == 0 {
		return
	}
	current := slices.Index(providers, a.selectedModel().ProviderID)
	next := 0
	if current >= 0 {
		next = (current + delta + len(providers)) % len(providers)
	} else if delta < 0 {
		next = len(providers) - 1
	}
	a.selectModelProvider(providers[next])
}

type compactEffortState struct {
	levels   []string
	selected int
	option   contracts.ModelOption
}

func (e compactEffortState) label() string {
	if e.option.ID != "" {
		return modelOptionChoiceLabel(e.option, e.levels[e.selected])
	}
	return e.levels[e.selected]
}

func (a *composerController) compactEffort() compactEffortState {
	if a.sessionID == "" && a.modelNoAgents() {
		return compactEffortState{}
	}
	model := a.selectedModel()
	effort := compactEffortState{levels: model.ReasoningLevels}
	value := a.reasoning
	if len(effort.levels) == 0 {
		for _, option := range model.Options {
			if (option.ID == "effort" || option.ID == "reasoning" || option.ID == "reasoning_effort") && len(option.Choices) > 1 {
				effort.levels, effort.option = option.Choices, option
				value = a.modelOptionValue(model, option)
				break
			}
		}
	}
	effort.selected = max(0, slices.Index(effort.levels, value))
	return effort
}

func (a *composerController) pickCompactEffort(effort compactEffortState, index int) {
	if index < 0 || index >= len(effort.levels) || index == effort.selected {
		return
	}
	if effort.option.ID != "" {
		a.pickModelOption(a.selectedModel(), effort.option, effort.levels[index])
		return
	}
	a.reasoning = effort.levels[index]
	a.effortReason = a.reasoning
	a.effortValue = float64(index)
	a.modelChanged()
}

func (a *composerController) modelOptionValue(model contracts.Model, option contracts.ModelOption) string {
	if value := a.state.Settings.ModelPicker.Options[model.ID+"/"+option.ID]; value != "" {
		return value
	}
	return option.DefaultChoice
}

func modelOptionChoiceLabel(option contracts.ModelOption, id string) string {
	if label := option.ChoiceLabels[id]; label != "" {
		return label
	}
	return id
}

func (a *composerController) pickModelOption(model contracts.Model, option contracts.ModelOption, choice string) {
	key := model.ID + "/" + option.ID
	values := maps.Clone(a.state.Settings.ModelPicker.Options)
	if choice == option.DefaultChoice {
		delete(values, key)
	} else {
		if values == nil {
			values = make(map[string]string)
		}
		values[key] = choice
	}
	a.state.Settings.ModelPicker.Options = values
	a.saveSettings()
}

// Fast mode follows pickers.rs::fast_mode_values, including Cursor's
// default-on boolean option and tier/speed options that offer "fast".
func fastModeValues(option contracts.ModelOption) (on, off string) {
	switch {
	case (option.ID == "fastMode" || option.ID == "fast_mode") && slices.Contains(option.Choices, "on"):
		on = "on"
	case option.ID == "fast" && slices.Contains(option.Choices, "true"):
		on = "true"
	case slices.Contains(option.Choices, "fast"):
		on = "fast"
	default:
		return "", ""
	}
	if option.DefaultChoice != on && slices.Contains(option.Choices, option.DefaultChoice) {
		return on, option.DefaultChoice
	}
	for _, choice := range option.Choices {
		if choice != on {
			return on, choice
		}
	}
	return "", ""
}

func (a *composerController) compactFastOption(model contracts.Model) (option contracts.ModelOption, on, off string, fast bool) {
	for _, option := range model.Options {
		on, off := fastModeValues(option)
		if on != "" {
			return option, on, off, a.modelOptionValue(model, option) == on
		}
	}
	return contracts.ModelOption{}, "", "", false
}

func (a *composerController) modelPopupPalette() design.Tokens {
	palette := a.palette
	// popover.rs::surface_bg uses Theme::input_glass_bg. Native opaque
	// surfaces flatten the input tint over the page, not the dialog plane.
	palette.Dialog = palette.Input.Over(palette.Background)
	return palette
}

func (a *composerController) modelPopover(c *ui.Context, anchor ui.Element) ui.Element {
	palette := a.modelPopupPalette()
	return ui.PopoverBase(c, anchor, &a.modelMenu, func(panel ui.Element) {
		if a.state.Settings.General.CompactModelPicker && a.modelListPage && panel.Shortcut(0, ui.KeyEscape) {
			a.modelListPage = false
		}
		width := compactModelWidth
		if !a.state.Settings.General.CompactModelPicker {
			width = 304
		}
		budget, openBelow := composerMenuPlacement(c, panel, anchor, a.sessionID == "", a.state.Settings.General.CompactModelPicker)
		if openBelow {
			panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0)
		} else {
			panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 6, 0)
		}
		panel.SnapToWindow(8)
		// GPUI attaches this overlay inside the medium-weight trigger chip.
		// MyGo overlays have a separate parent, so retain that inherited weight.
		panel.Label(tr(c, a.controlLabel("Model picker"))).Width(width).MaxHeight(min(float32(640), budget)).Padding(0).Radius(12).FontWeight(500).Background(palette.Dialog).Border(1, palette.Border).Shadow(0, 10, 2*15, -3, ui.RGBA(0, 0, 0, .1)).Shadow(0, 4, 2*6, -4, ui.RGBA(0, 0, 0, .1)).Clip()
		if a.state.Settings.General.CompactModelPicker && !a.modelListPage {
			ui.Column(c).Key("compact-model-panel").FillWidth().Padding(4).Focusable().KeepFocus().AutoFocus().Children(func() { a.compactModelPanel(c) })
		} else {
			a.modelListPanel(c, min(float32(640), budget))
		}
	})
}

func (a *composerController) compactModelPanel(c *ui.Context) {
	a.compactPanelShortcuts(c)
	model := a.selectedModel()
	effort := a.compactEffort()
	levels := effort.levels
	height := float32(36)
	if len(levels) > 0 {
		height = 48
	}
	fastOption, fastOn, fastOff, fast := a.compactFastOption(model)
	hoverInk := ui.Hex("#ffffff").Alpha(.05)
	if !a.palette.Dark {
		hoverInk = ui.Hex("#000000").Alpha(.05)
	}
	ui.Row(c).Height(height).FillWidth().Gap(4).Children(func() {
		button := ui.ButtonBase(c).Height(height).Grow(1).MinWidth(0).Padding(8).Radius(7).Label(tr(c, a.controlLabel("Change model"))).AlignItems(ui.Center).Justify(ui.Start).OnClick(a.openModelList)
		if button.Hovered() {
			button.Background(hoverInk)
		}
		button.Children(func() {
			ui.Column(c).Grow(1).AlignItems(ui.Start).Children(func() {
				if len(levels) > 0 {
					ui.Text(c, tr(c, effort.label())).FontSize(design.UISize(c, 14)).FontWeight(600).FixedLineHeight(17)
				}
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					size := float32(12)
					if len(levels) == 0 {
						size = 14
					}
					color, weight, lineHeight := a.palette.Muted, 500, float32(15)
					if len(levels) == 0 {
						color, weight, lineHeight = a.palette.Text, 600, 17
					}
					ui.Text(c, a.modelLabel()).FontSize(design.UISize(c, size)).FontWeight(weight).TextColor(color).FixedLineHeight(lineHeight).SingleLine()
					ui.Box(c).Size(10, 10).TextColor(a.palette.Muted.Alpha(.55)).Children(func() { icon(c, "alt-arrow-right", 10) })
				})
			})
		})
		if fastOption.ID != "" {
			glyph, hint := "fast-tier", "Fast mode off · Turn on"
			color := a.palette.Muted
			if fast {
				glyph, hint, color = "fast-tier-bold", "Fast mode on · Turn off", a.palette.Accent
			}
			fastButton := ui.ToggleBase(c, &fast).Width(32).Height(height).Radius(7).Role(ui.RoleButton).Label(tr(c, a.controlLabel("Fast mode"))).Tooltip(tr(c, hint)).AlignItems(ui.Center).Justify(ui.Center).TextColor(color)
			if fastButton.Hovered() {
				fastButton.Background(hoverInk)
			}
			fastButton.Children(func() {
				icon(c, glyph, 15)
			}).OnClick(func() {
				choice := fastOff
				if fast {
					choice = fastOn
				}
				a.pickModelOption(model, fastOption, choice)
			})
		}
	})
	if len(levels) > 0 {
		value := levels[effort.selected]
		if a.effortModel != model.ID || a.effortReason != value {
			a.effortValue = float64(effort.selected)
			a.effortModel, a.effortReason = model.ID, value
		}
		ui.Column(c).FillWidth().Padding(2, 8, 4, 8).Children(func() {
			c.Key("compact-effort-slider")
			slider := ui.SliderBase(c, &a.effortValue, 0, float64(len(levels)-1)).Step(1).Label(tr(c, a.controlLabel("Reasoning effort"))).Height(36).FillWidth().Padding(0, 22)
			fraction := float32(a.effortValue / float64(max(1, len(levels)-1)))
			pressed := slider.Pressed()
			slider.Children(func() { a.compactEffortMaterial(c, fraction, len(levels), fast, pressed) }).OnChange(func() {
				index := max(0, min(len(levels)-1, int(math.Round(a.effortValue))))
				a.pickCompactEffort(effort, index)
			})
		})
	}
	a.modelOptionRows(c, model, fastOption.ID, effort.option.ID)
}

func modelVerticalGradient(top, bottom ui.Color) ui.LinearGradient {
	return ui.LinearGradient{Angle: 180, From: top, To: bottom}
}

// These three plates follow glass.rs; layout remains owned by SliderBase.
// GPUI's blur_radius is sigma; MyGo's CSS box-shadow blur is 2*sigma.
func modelRailPlate(c *ui.Context, dark bool) ui.Element {
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	plate := ui.Box(c).Radius(14)
	if dark {
		return plate.LinearGradient(modelVerticalGradient(white.Alpha(.08), white.Alpha(.05))).Border(1, white.Alpha(.09)).InsetShadow(0, 1, 0, 0, white.Alpha(.07))
	}
	return plate.LinearGradient(modelVerticalGradient(black.Alpha(.075), black.Alpha(.04))).Border(1, black.Alpha(.08)).InsetShadow(0, 1, 2*2, 0, black.Alpha(.08)).InsetShadow(0, -1, 0, 0, white.Alpha(.55))
}

func modelAccentPlate(c *ui.Context, palette design.Tokens, fast bool) ui.Element {
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	lift, highlight, ring, halo := float32(.22), float32(.32), float32(.22), float32(.14)
	rim := palette.AccentStrong.Mix(black, .14)
	if palette.Dark {
		lift, highlight, ring, halo = .06, .12, 0, 0
		rim = palette.AccentStrong.Mix(white, .35).Alpha(.35)
	}
	glow := float32(0)
	if fast {
		// Reduced-motion source state: intensity=1, breath=.5.
		glow = .325
	}
	plate := ui.Box(c).Radius(14).LinearGradient(modelVerticalGradient(palette.AccentStrong.Mix(white, lift), palette.AccentStrong)).Border(1, rim).
		Shadow(0, 2+2*glow, 2*(6+14*glow), 0, palette.AccentStrong.Alpha(halo+.3*glow))
	plate.Children(func() {
		modelAccentInsetOverlay(c).InsetShadow(0, 1, 0, 0, white.Alpha(highlight)).InsetShadow(0, 0, 0, 1, white.Alpha(ring))
	})
	return plate
}

func modelAccentInsetOverlay(c *ui.Context) ui.Element {
	// GPUI clips inset shadows to the plate's outer bounds, including its rim.
	// MyGo's CSS inset excludes borders. Absolute children use the padding box,
	// so extending past this plate's 1 DIP border restores the source bounds.
	return ui.Box(c).Absolute().Left(-1).Right(-1).Top(-1).Bottom(-1).Radius(14)
}

func modelThumbPlate(c *ui.Context, palette design.Tokens, fast, pressed bool) ui.Element {
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	rim, drop := float32(.11), float32(.14)
	if palette.Dark {
		rim, drop = .12, .22
	}
	plate := ui.Box(c).Radius(16).LinearGradient(modelVerticalGradient(white, ui.RGB(246, 246, 246))).Border(1, black.Alpha(rim)).InsetShadow(0, 1, 0, 0, white.Alpha(.8))
	if fast {
		plate.Shadow(0, 1, 2*10, 0, palette.Accent.Alpha(.35*.775))
	}
	if pressed {
		plate.Shadow(0, 2, 2*8, 0, black.Alpha(.12))
	}
	plate.Shadow(0, 1, 2*2, 0, black.Alpha(drop))
	if !palette.Dark {
		plate.Shadow(0, 2, 2*6, 0, black.Alpha(.06))
	}
	return plate
}

func (a *composerController) compactEffortMaterial(c *ui.Context, fraction float32, count int, fast, pressed bool) {
	palette := a.modelPopupPalette()
	if palette.Dark {
		// GPUI paints the whole drop beneath this translucent rail. MyGo's
		// Shadow deliberately cuts out its casting box, so retain the full
		// source shadow as a blurred underlay rather than changing its tint.
		ui.Box(c).Absolute().Left(0).Right(0).Top(5).Height(28).Radius(14).Background(ui.Hex("#000000").Alpha(.16)).Blur(2)
	}
	modelRailPlate(c, palette.Dark).Absolute().Left(0).Right(0).Top(4).Height(28)
	ui.Box(c).Absolute().Left(22).Right(22).Top(0).Bottom(0).Children(func() {
		modelAccentPlate(c, palette, fast).Absolute().Left(-22).RightPercent(100*(1-fraction)).Margin(0, -14, 0, 0).Top(4).Height(28)
	})
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Draw(func(p *ui.Painter, r ui.Rect) {
		for i := 0; i < count; i++ {
			stop := float32(i) / float32(max(1, count-1))
			x := r.X + 22 + (r.W-44)*stop
			color := palette.Muted.Alpha(.28)
			if stop <= fraction {
				color = ui.Hex("#ffffff").Alpha(.4)
			}
			p.Fill(ui.Rect{X: x - 2, Y: r.Y + 16, W: 4, H: 4}, color, 2)
		}
	})
	ui.Box(c).Absolute().Left(22).Right(22).Top(2).Height(32).Children(func() {
		modelThumbPlate(c, palette, fast, pressed).Absolute().LeftPercent(fraction*100).Margin(0, 0, 0, -22).Width(44).Height(32)
	})
}

// modelOptionPixelSnap limits the measured Windows typography adaptation to
// 1x displays. Unattached test views use the Tester's 1x layout coordinates.
func (a *composerController) modelOptionPixelSnap() bool {
	return runtime.GOOS == "windows" && (a.win == nil || mygo.Screen.DisplayMatching(a.win.Bounds()).ScaleFactor == 1)
}

func modelOptionText(c *ui.Context, text string, fontSize, rowHeight float32, pixelSnap bool) ui.Element {
	element := ui.Text(c, tr(c, text)).FontSize(fontSize).SingleLine()
	if pixelSnap {
		// GPUI's default phi line height is pixel-snapped before flex layout.
		lineHeight := float32(math.Ceil(float64(fontSize*1.618034) - .5))
		// At 1x, with integer row origins and nonnegative centered text,
		// rounding the absolute boundary half toward zero equals this floor.
		// Fractional origins/transforms and other DPI scales are not covered.
		top := float32(math.Floor(float64((rowHeight - lineHeight) / 2)))
		element.Height(lineHeight).FixedLineHeight(lineHeight).AlignSelf(ui.Start).Top(top)
	}
	return element
}

func (a *composerController) modelOptionRows(c *ui.Context, model contracts.Model, hidden ...string) {
	options := slices.DeleteFunc(slices.Clone(model.Options), func(option contracts.ModelOption) bool { return slices.Contains(hidden, option.ID) })
	if len(options) == 0 {
		return
	}
	compact := a.state.Settings.General.CompactModelPicker
	height, gap, bottom := float32(30), float32(0), float32(4)
	if compact {
		height, gap, bottom = 26, 4, 0
	}
	pixelSnap := a.modelOptionPixelSnap()
	fontSize := design.UISize(c, 13)
	ui.Scroll(c).FillWidth().MaxHeight(170).Margin(gap, 0, 0, 0).Gap(2).Padding(4, 0, bottom, 0).Children(func() {
		for _, option := range options {
			option := option
			value := a.modelOptionValue(model, option)
			c.Key("model-option:" + model.ID + "/" + option.ID)
			row := ui.ButtonBase(c).Height(height).FillWidth().Padding(0, 8).Radius(7).AlignItems(ui.Center).Justify(ui.Start).Gap(10).Label(a.controlLabel(option.Label))
			if row.Hovered() {
				row.Background(modelSelectionColor(a.modelPopupPalette()))
			}
			row.Menu(func(menu *ui.Menu) {
				for _, choice := range option.Choices {
					if menu.Item(tr(c, modelOptionChoiceLabel(option, choice))).Checked(value == choice).Chosen() {
						a.pickModelOption(model, option, choice)
					}
				}
			}).Children(func() {
				modelOptionText(c, option.Label, fontSize, height, pixelSnap).TextColor(a.palette.Text.Alpha(.9)).Grow(1).MinWidth(0)
				label := tr(c, modelOptionChoiceLabel(option, value))
				valueText := modelOptionText(c, label, fontSize, height, pixelSnap).TextColor(a.palette.Muted).MaxWidth(100).MinWidth(0)
				if pixelSnap {
					// The source text's intrinsic width is ceil(advance); the label
					// then grows into the remaining space before the value/chevron.
					advance, _ := c.MeasureText(0, ui.Span{Text: label, Font: c.Theme().Font, Size: fontSize, Weight: 500, Features: "kern=0"})
					valueText.Width(min(float32(100), float32(math.Ceil(float64(advance)))))
				}
				ui.Box(c).Size(12, 12).TextColor(a.palette.Muted).Children(func() { icon(c, "alt-arrow-right", 12) })
			})
		}
	})
}

func (a *composerController) openModelList() {
	if a.state.Settings.General.CompactModelPicker {
		a.modelQuery, a.modelProvider = "", ""
	}
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
	if !a.modelProviderOffered(rows[index].provider) {
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
	palette := a.modelPopupPalette()
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
			a.compactModelListIconButton(c, "alt-arrow-left", "Back to model options", palette.Muted).OnClick(func() { a.modelListPage = false })
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
	hairline := float32(.08)
	if !palette.Dark {
		hairline *= 1.35
	}
	ui.Row(c).Height(40).FillWidth().Padding(0, 10).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(modelMenuInk(palette, hairline)).Children(func() {
		ui.Box(c).Size(14, 14).TextColor(palette.Muted).Children(func() { icon(c, "magnifer", 14) })
		// ComposerInput overrides its parent's 13px style with these metrics.
		// Keep the native editor; its empty placeholder uses the source faint role.
		// Its custom layout uses px(configured_line_height), not the rem style.
		ui.TextInputBase(c, &a.modelQuery).Bind(&a.modelSearch).Label(tr(c, a.controlLabel("Search models"))).Placeholder(tr(c, "Search models…")).PlaceholderColor(palette.Faint).CaretWidth(2).Grow(1).FontSize(design.UISize(c, 14)).FixedLineHeight(22.75).AutoFocus().OnChange(func() { a.modelCursor = 0; a.modelList.ScrollIntoView(0) }).OnTextKey(func(k ui.TextKey) bool {
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
	// The source's transparent rows sit over the scrolling band's 2% ink.
	listPalette := palette
	listPalette.Dialog = modelMenuInk(palette, .02).Over(palette.Dialog)
	listTheme.Accent = modelSelectionColor(listPalette)
	listTheme.AccentText = a.palette.Text
	listTheme.SurfaceHover = modelMenuInk(palette, .05).Over(listPalette.Dialog)
	listTheme.Radius = 7
	if len(rows) == 0 {
		ui.Column(c).FillWidth().Height(height).Padding(8).Background(listPalette.Dialog).Children(func() {
			if strings.TrimSpace(a.modelQuery) != "" || a.modelProvider == "favorites" {
				note := "No models found"
				if a.modelQuery == "" && a.modelProvider == "favorites" {
					note = "No starred models yet — hit a row's star"
				}
				ui.Column(c).Fill().Center().Children(func() { ui.Text(c, tr(c, note)).FontSize(design.UISize(c, 12)).TextColor(a.palette.Muted) })
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
			button := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 4, 0, 8).Radius(7).Gap(8).Justify(ui.Start).AlignItems(ui.Center).TextColor(a.palette.Text).Label(m.Label).Tooltip(m.Label + " · " + row.provider.Name + tr(c, "\n") + m.Description)
			if selected {
				button.Background(modelSelectionColor(listPalette)).DrawOver(func(p *ui.Painter, r ui.Rect) {
					ring := ui.Hex("#000000").Alpha(.07)
					if a.palette.Dark {
						ring = ui.Hex("#ffffff").Alpha(.09)
					}
					p.Stroke(r, ring, 7, 1)
				})
			} else if button.Hovered() || a.modelCursor == i {
				button.Background(listTheme.SurfaceHover)
			}
			button.OnClick(func() { a.activateModelRow(rows, i) }).OnHover(func(hovered bool) {
				if hovered {
					a.modelCursor = i
				}
			})
			button.Children(func() {
				ui.Box(c).Size(14, 14).TextColor(palette.Muted).Children(func() { icon(c, row.provider.Icon, 14) })
				ui.Text(c, m.Label).Grow(1).SingleLine().FontSize(design.UISize(c, 12)).FontWeight(500)
				favorite := a.isModelFavorite(m)
				compact := a.state.Settings.General.CompactModelPicker
				if compact || favorite || button.Hovered() {
					glyph, label := "star", tr(c, "Add ")+m.Label+tr(c, " to favorites")
					if favorite {
						glyph, label = "star-bold", tr(c, "Remove ")+m.Label+tr(c, " from favorites")
					}
					if compact {
						// The source's `hovered` is the active row, including the
						// keyboard cursor. Hidden stars still reserve their slot.
						star := ui.ButtonBase(c).Size(24, 24).Radius(6).Label(tr(c, label)).Tooltip(tr(c, label)).AlignItems(ui.Center).Justify(ui.Center).TextColor(a.palette.Muted)
						if !favorite && a.modelCursor != i {
							star.Opacity(0)
						}
						if favorite {
							star.TextColor(a.palette.Accent)
						}
						if star.Hovered() {
							ink := ui.Hex("#000000")
							if a.palette.Dark {
								ink = ui.Hex("#ffffff")
							}
							star.Background(ink.Alpha(.08))
						}
						star.Children(func() { icon(c, glyph, 13) }).OnClick(func() { a.toggleModelFavorite(m) })
					} else {
						iconButton(c, glyph, tr(c, label)).Size(24, 24).OnClick(func() { a.toggleModelFavorite(m) })
					}
				}
			})
		}).Height(height).Gap(2).Padding(4).Background(listPalette.Dialog)
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
					ui.Text(c, tr(c, "Reasoning")).FontSize(design.UISize(c, 12)).Grow(1)
					ui.MenuButton(c, a.reasoning, func(menu *ui.Menu) {
						for _, level := range levels {
							if menu.Item(tr(c, level)).Checked(a.reasoning == level).Chosen() {
								a.reasoning = level
								a.modelChanged()
							}
						}
					}).FontSize(design.UISize(c, 12)).BorderWidth(0).Background(ui.Transparent).Label(tr(c, "Reasoning level"))
				})
			}
			a.modelOptionRows(c, a.selectedModel())
		})
	}
}

func modelMenuInk(palette design.Tokens, alpha float32) ui.Color {
	ink := ui.Hex("#000000")
	if palette.Dark {
		ink = ui.Hex("#ffffff")
	}
	return ink.Alpha(alpha)
}

func (a *composerController) compactModelListIconButton(c *ui.Context, glyph, label string, color ui.Color) ui.Element {
	button := ui.ButtonBase(c).Size(32, 32).Radius(7).Label(tr(c, label)).Tooltip(tr(c, label)).AlignItems(ui.Center).Justify(ui.Center).TextColor(color)
	if button.Hovered() {
		button.Background(modelMenuInk(a.palette, .05))
	}
	button.Children(func() { icon(c, glyph, 14) })
	return button
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
	current := a.modelProvider
	if current == "" {
		current = a.selectedProvider().ID
	}
	var b ui.Element
	if a.state.Settings.General.CompactModelPicker {
		color := a.palette.Muted
		if current == id {
			color = a.palette.Text
		}
		b = a.compactModelListIconButton(c, glyph, label, color)
		if current != id && !b.Hovered() {
			b.Opacity(.7)
		}
	} else {
		b = iconButton(c, glyph, tr(c, label)).Size(32, 32)
	}
	if current == id {
		if a.state.Settings.General.CompactModelPicker {
			b.Background(modelMenuInk(a.palette, .08))
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
	offered := false
	for _, provider := range a.state.Providers {
		if provider.ID == providerID {
			offered = a.modelProviderOffered(provider)
			break
		}
	}
	if !offered {
		return
	}
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
