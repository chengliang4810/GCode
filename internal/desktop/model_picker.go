package desktop

import (
	"maps"
	"math"
	"runtime"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	modelPickerMaxWidth   float32 = 440
	modelPickerGap        float32 = 6
	providerPanelMinWidth float32 = 120
	providerPanelMaxWidth float32 = 240
	modelPanelMinWidth    float32 = 192
	providerRowChrome     float32 = 96
	managementRowChrome   float32 = 52
	modelRowChrome        float32 = 36
)

func (a *composerController) effortPanelShortcuts(c *ui.Context) {
	for _, key := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyHome, ui.KeyEnd} {
		c.OnShortcut(0, key, func() {
			effort := a.reasoningChoice()
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
			a.pickReasoningChoice(effort, next)
		})
	}
}

type reasoningChoiceState struct {
	levels      []string
	selected    int
	option      contracts.ModelOption
	unavailable string
}

func (e reasoningChoiceState) label() string {
	if e.unavailable != "" {
		return e.unavailable
	}
	if e.option.ID != "" {
		return modelOptionChoiceLabel(e.option, e.levels[e.selected])
	}
	return e.levels[e.selected]
}

func (a *composerController) reasoningChoice() reasoningChoiceState {
	if a.sessionID == "" && a.modelNoAgents() {
		return reasoningChoiceState{}
	}
	model := a.selectedModel()
	effort := reasoningChoiceState{levels: model.ReasoningLevels}
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
	if a.state.Runtime != nil && value != "" && !slices.Contains(effort.levels, value) {
		effort.unavailable = a.t("Unavailable") + " · " + value
	}
	return effort
}

func (a *composerController) pickReasoningChoice(effort reasoningChoiceState, index int) {
	if index < 0 || index >= len(effort.levels) || (index == effort.selected && effort.unavailable == "") {
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

func (a *composerController) fastModelOption(model contracts.Model) (option contracts.ModelOption, on, off string, fast bool) {
	for _, option := range model.Options {
		on, off := fastModeValues(option)
		if on != "" {
			return option, on, off, a.modelOptionValue(model, option) == on
		}
	}
	return contracts.ModelOption{}, "", "", false
}

// effortPalette is the device-local reasoning-slider preset resolved against
// the current theme and accent.
func (a *composerController) effortPalette() design.EffortPalette {
	return design.EffortPaletteFor(a.state.Settings.Appearance.EffortTheme, a.palette)
}

func (a *composerController) modelPopupPalette(c *ui.Context) design.Tokens {
	palette := a.palette
	// popover.rs::surface_bg: the frosted card's tint over the backdrop
	// blur; native opaque surfaces flatten the input tint over the page,
	// not the dialog plane.
	palette.Dialog = design.FloatingSurface(a.palette, a.state.Settings.Appearance.Surface, c.Vibrancy())
	return palette
}

// modelPickerColumns measures each card from its own content, as the
// reference picker does, so a fixed split cannot truncate long model names.
func (a *composerController) modelPickerColumns(c *ui.Context) (providerWidth, modelWidth float32) {
	fontSize := design.TextSize(c, design.TextBase)
	measure := func(text string, weight int) float32 {
		advance, _ := c.MeasureText(0, ui.Span{Text: text, Font: c.Theme().Font, Size: fontSize, Weight: weight, Features: "kern=0"})
		return advance
	}
	providerWidth = providerPanelMinWidth
	for _, provider := range a.state.Providers {
		if a.modelProviderOffered(provider) {
			providerWidth = max(providerWidth, measure(provider.Name, 400)+providerRowChrome)
		}
	}
	providerWidth = max(providerWidth, measure(tr(c, "Model management"), 400)+managementRowChrome)
	providerWidth = min(providerWidth, providerPanelMaxWidth)

	providerID := a.modelProvider
	if providerID == "" {
		providerID = a.selectedProvider().ID
	}
	modelWidth = modelPanelMinWidth
	measured := 0
	for _, option := range a.state.Models {
		if option.ProviderID != providerID {
			continue
		}
		modelWidth = max(modelWidth, measure(option.Label, 500)+modelRowChrome)
		measured++
		if measured == 40 {
			break
		}
	}
	modelWidth = max(modelWidth, measure(a.selectedModel().Label, 500)+modelRowChrome)
	return providerWidth, modelWidth
}

func (a *composerController) modelPopover(c *ui.Context, anchor ui.Element) ui.Element {
	return ui.PopoverBase(c, anchor, &a.modelMenu, func(panel ui.Element) {
		panel.OnShortcut(ui.Cmd, ui.KeyF, func() { a.modelSearchOpen = true; a.modelSearch.Focus(c) })
		a.modelOptionsShortcuts(c)
		budget := promptMenuHeight(c, panel, anchor)
		placeComposerSelector(panel, anchor, false)
		viewportWidth, _ := c.Size()
		providerWidth, modelWidth := a.modelPickerColumns(c)
		trigger := anchor.Bounds()
		providerWidth = max(providerWidth, trigger.W)
		width := min(providerWidth+modelPickerGap+modelWidth, min(modelPickerMaxWidth, max(float32(0), viewportWidth-16)))
		providerWidth = min(providerWidth, max(float32(0), width-modelPickerGap))
		modelWidth = max(float32(0), width-providerWidth-modelPickerGap)
		providerOffset := providerWidth / 2
		// Keep the provider card's centerline on the trigger. The model list
		// goes beside it, to the right unless the window lacks that room.
		modelsOnLeft := trigger.X+trigger.W/2+width-providerOffset > viewportWidth-8
		if modelsOnLeft {
			providerOffset = width - providerOffset
		}
		panel.Left(width - providerOffset - trigger.W/2)
		panel.Label(tr(c, a.controlLabel("Model picker"))).Width(width).MaxHeight(budget).Padding(0).FontWeight(500)
		a.modelListPanel(c, budget, providerWidth, modelWidth, modelsOnLeft)
	})
}

func placeComposerSelector(panel, anchor ui.Element, below bool) {
	if below {
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0)
	} else {
		panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 6, 0)
	}
	panel.SnapToWindow(8)
}

func (a *composerController) reasoningTrigger(c *ui.Context) (anchor, panel ui.Element) {
	effort := a.reasoningChoice()
	if len(effort.levels) == 0 {
		a.effortMenu = false
		return ui.Element{}, ui.Element{}
	}
	label := "Reasoning"
	if effort.unavailable == "" {
		label = effort.label()
	}
	button := ui.ButtonBase(c.Key("reasoning-trigger")).Height(32).Padding(0, 6).Radius(8).MinWidth(24).MaxWidth(100).Shrink(0).Gap(4).AlignItems(ui.Center).Label(tr(c, a.controlLabel("Select reasoning effort"))).Tooltip(tr(c, a.controlLabel("Reasoning effort")))
	if a.effortMenu || button.Hovered() {
		button.Background(a.palette.Hover)
	}
	button.Children(func() {
		icon(c, "brain", 14)
		ui.Text(c, tr(c, label)).SingleLine().MinWidth(0).Shrink(1).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
	})
	button.OnClick(func() {
		a.effortMenu = !a.effortMenu
		if a.effortMenu {
			a.modelMenu = false
		}
	})
	return button, a.reasoningPopover(c, button)
}

func (a *composerController) reasoningPopover(c *ui.Context, anchor ui.Element) ui.Element {
	return ui.PopoverBase(c, anchor, &a.effortMenu, func(panel ui.Element) {
		budget, below := composerMenuPlacement(c, panel, anchor, a.sessionID == "", false)
		placeComposerSelector(panel, anchor, below)
		panel.Label(tr(c, a.controlLabel("Reasoning picker"))).Width(256).MaxHeight(budget).Padding(4).Radius(12).FontWeight(500).Clip()
		if !a.frosted(c) {
			panel.Shadow(0, 8, 24, -3, ui.RGBA(0, 0, 0, .12))
		}
		a.floatingSurface(c, panel, 12)
		ui.Scroll(c).FillWidth().MaxHeight(max(float32(0), budget-10)).Focusable().KeepFocus().AutoFocus().Children(func() { a.effortPanel(c) })
	})
}

func (a *composerController) modelOptionsPanel(c *ui.Context) {
	model := a.selectedModel()
	effort := a.reasoningChoice()
	fastOption, fastOn, fastOff, fast := a.fastModelOption(model)
	if fastOption.ID != "" {
		ui.Row(c).Height(32).FillWidth().Padding(0, 8).Justify(ui.End).Children(func() {
			if fastOption.ID != "" {
				glyph, hint := "fast-tier", "Fast mode off · Turn on"
				color := a.palette.Muted
				if fast {
					glyph, hint, color = "fast-tier-bold", "Fast mode on · Turn off", a.palette.Accent
				}
				button := ui.ToggleBase(c, &fast).Size(28, 28).Radius(7).Role(ui.RoleButton).Label(tr(c, a.controlLabel("Fast mode"))).Tooltip(tr(c, hint)).Center().TextColor(color)
				if button.Hovered() {
					button.Background(a.palette.Hover)
				}
				button.Children(func() { icon(c, glyph, 15) }).OnClick(func() {
					choice := fastOff
					if fast {
						choice = fastOn
					}
					a.pickModelOption(model, fastOption, choice)
				})
			}
		})
	}

	a.modelOptionRows(c, model, fastOption.ID, effort.option.ID)
}

func (a *composerController) modelOptionsShortcuts(c *ui.Context) {
	model := a.selectedModel()
	fastOption, fastOn, fastOff, fast := a.fastModelOption(model)
	c.OnShortcut(0, ui.KeyF, func() {
		if fastOption.ID == "" {
			return
		}
		choice := fastOn
		if fast {
			choice = fastOff
		}
		a.pickModelOption(model, fastOption, choice)
	})
}

func (a *composerController) effortPanel(c *ui.Context) {
	a.effortPanelShortcuts(c)
	model := a.selectedModel()
	effort := a.reasoningChoice()
	levels := effort.levels
	if len(levels) > 0 {
		ui.Text(c, tr(c, effort.label())).FontSize(design.TextSize(c, design.TextBase)).Margin(4, 8)
		value := levels[effort.selected]
		if a.effortModel != model.ID || a.effortReason != value {
			a.effortValue = float64(effort.selected)
			a.effortModel, a.effortReason = model.ID, value
		}
		ui.Column(c).FillWidth().Padding(2, 8, 4, 8).Children(func() {
			slider := ui.SliderBase(c.Key("effort-slider"), &a.effortValue, 0, float64(len(levels)-1)).Step(1).Label(tr(c, a.controlLabel("Reasoning effort"))).Height(36).FillWidth().Padding(0, 22)
			pressed := slider.Pressed()
			still := a.reduceMotion(c) || c.Instant()
			paint := a.effortMotion.paint(c, effort.selected, len(levels), pressed, still, a.effortLive(c))
			if pressed {
				// A drag carries the fill under the pointer; letting go eases
				// it to the stop the pointer landed on.
				if fraction, ok := effortPointerFraction(slider); ok {
					paint.fraction = fraction
					a.effortMotion.front.jump(c.Now(), fraction)
				}
			}
			slider.Children(func() { a.reasoningSliderMaterial(c, paint) }).OnChange(func() {
				a.pickReasoningChoice(effort, max(0, min(len(levels)-1, int(math.Round(a.effortValue)))))
			})
		})
	}
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

func modelAccentPlate(c *ui.Context, palette design.Tokens, glow float32) ui.Element {
	return modelAccentPlateWith(c, palette, palette.AccentStrong, glow)
}

// modelAccentPlateWith keeps the source glass construction and takes the fill
// token from the caller, so the reasoning slider can paint its preset. glow
// is the source's intensity*(0.2+0.25*breath): the accent's coloured halo
// spreads and deepens as it breathes.
func modelAccentPlateWith(c *ui.Context, palette design.Tokens, accent ui.Color, glow float32) ui.Element {
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	lift, highlight, ring, halo := float32(.22), float32(.32), float32(.22), float32(.14)
	rim := accent.Mix(black, .14)
	if palette.Dark {
		lift, highlight, ring, halo = .06, .12, 0, 0
		rim = accent.Mix(white, .35).Alpha(.35)
	}
	plate := ui.Box(c).Radius(14).LinearGradient(modelVerticalGradient(accent.Mix(white, lift), accent)).Border(1, rim).
		Shadow(0, 2+2*glow, 2*(6+14*glow), 0, accent.Alpha(halo+.3*glow))
	plate.Children(func() {
		modelAccentInsetOverlay(c).InsetShadow(0, 1, 0, 0, white.Alpha(highlight)).InsetShadow(0, 0, 0, 1, white.Alpha(ring))
	})
	return plate
}

// modelEffortPlate paints the preset's three stops across the filled rail,
// inside the same rim and inset lighting as the accent plate. While the
// accent lives, the stops carry the reference's flowing light.
func modelEffortPlate(c *ui.Context, palette design.Tokens, effort design.EffortPalette, paint effortSliderPaint) ui.Element {
	plate := modelAccentPlateWith(c, palette, effort.Colors[0], effortGlow(paint.breath))
	plate.Draw(func(p *ui.Painter, r ui.Rect) {
		inner := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: max(0, r.W-2), H: max(0, r.H-2)}
		if paint.live && inner.W > 2 && inner.H > 2 {
			effortFlow(p, effort, paint, r)
			return
		}
		p.FillGradient(inner, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
			{Color: effort.Colors[0], At: 0},
			{Color: effort.Colors[1], At: .5},
			{Color: effort.Colors[2], At: 1},
		}}, inner.H/2)
	})
	return plate
}

func modelAccentInsetOverlay(c *ui.Context) ui.Element {
	// GPUI clips inset shadows to the plate's outer bounds, including its rim.
	// MyGo's CSS inset excludes borders. Absolute children use the padding box,
	// so extending past this plate's 1 DIP border restores the source bounds.
	return ui.Box(c).Absolute().Left(-1).Right(-1).Top(-1).Bottom(-1).Radius(14)
}

// modelThumbPlate draws the handle. halo is the source's live breathing light
// under it, and press how far its press shadow has eased in.
func modelThumbPlate(c *ui.Context, palette design.Tokens, effort design.EffortPalette, halo, press float32) ui.Element {
	white, black := ui.Hex("#ffffff"), ui.Hex("#000000")
	rim, drop := float32(.11), float32(.14)
	if palette.Dark {
		rim, drop = .12, .22
	}
	plate := ui.Box(c).Radius(16).LinearGradient(modelVerticalGradient(white, ui.RGB(246, 246, 246))).Border(1, black.Alpha(rim)).InsetShadow(0, 1, 0, 0, white.Alpha(.8))
	// The source lays the halo and the press under the handle, so they go on
	// first and the handle's own drop stays over them.
	if halo > .001 {
		glow := effort.Colors[2].Alpha(.35 * halo)
		if effort.Light {
			glow = black.Alpha(.12 * halo)
		}
		plate.Shadow(0, 1, 2*10, 0, glow)
	}
	if press > .001 {
		plate.Shadow(0, 2*press, 2*8, 0, black.Alpha(.12*press))
	}
	plate.Shadow(0, 1, 2*2, 0, black.Alpha(drop))
	if !palette.Dark {
		plate.Shadow(0, 2, 2*6, 0, black.Alpha(.06))
	}
	return plate
}

func (a *composerController) reasoningSliderMaterial(c *ui.Context, paint effortSliderPaint) {
	palette := a.modelPopupPalette(c)
	effort := a.effortPalette()
	if palette.Dark {
		// GPUI paints the whole drop beneath this translucent rail. MyGo's
		// Shadow deliberately cuts out its casting box, so retain the full
		// source shadow as a blurred underlay rather than changing its tint.
		ui.Box(c).Absolute().Left(0).Right(0).Top(5).Height(28).Radius(14).Background(ui.Hex("#000000").Alpha(.16)).Blur(2)
	}
	modelRailPlate(c, palette.Dark).Absolute().Left(0).Right(0).Top(4).Height(28)
	ui.Box(c).Absolute().Left(22).Right(22).Top(0).Bottom(0).Children(func() {
		modelEffortPlate(c, palette, effort, paint).Absolute().Left(-22).RightPercent(100*(1-paint.fraction)).Margin(0, -14, 0, 0).Top(4).Height(28)
	})
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Draw(func(p *ui.Painter, r ui.Rect) {
		filled := effort.Spark().Alpha(.55)
		if effort.Light {
			filled = effort.Spark().Alpha(.45)
		}
		// Filled stops follow the animated front, never jumping ahead of it.
		for i := 0; i < paint.count; i++ {
			stop := float32(i) / float32(max(1, paint.count-1))
			x := r.X + 22 + (r.W-44)*stop
			color := palette.Muted.Alpha(.28)
			if stop <= paint.fraction {
				color = filled
			}
			p.Fill(ui.Rect{X: x - 2, Y: r.Y + 16, W: 4, H: 4}, color, 2)
		}
	})
	ui.Box(c).Absolute().Left(22).Right(22).Top(2).Height(32).Children(func() {
		modelThumbPlate(c, palette, effort, effortHalo(paint.breath), paint.press).Absolute().LeftPercent(paint.fraction*100).Margin(0, 0, 0, -22).Width(44).Height(32)
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
	height := float32(30)

	pixelSnap := a.modelOptionPixelSnap()
	fontSize := design.TextSize(c, design.TextBase)
	ui.Scroll(c).FillWidth().MaxHeight(170).Gap(2).Padding(4, 0, 4, 0).Children(func() {
		for _, option := range options {
			option := option
			value := a.modelOptionValue(model, option)
			c.Key("model-option:" + model.ID + "/" + option.ID)
			row := ui.ButtonBase(c).Height(height).FillWidth().Padding(0, 8).Radius(7).AlignItems(ui.Center).Justify(ui.Start).Gap(10).Label(a.controlLabel(option.Label))
			if row.Hovered() {
				row.Background(modelSelectionColor(a.modelPopupPalette(c)))
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
	if a.modelProvider == "" {
		a.modelProvider = a.selectedProvider().ID
	}
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
	if a.state.Runtime != nil && a.modelID != rows[index].model.ID {
		a.reasoning = ""
	}
	a.modelID = rows[index].model.ID
	a.modelChanged()
	a.modelMenu = false
}

// The provider rail owns the overlay's height. The attached model card remains
// in the same overlay for focus/dismissal, but cannot move the provider rail.
func (a *composerController) modelListPanel(c *ui.Context, budget, providerWidth, modelWidth float32, modelsOnLeft bool) {
	palette := a.modelPopupPalette(c)
	rows := a.modelRows()
	providerCount := 0
	for _, provider := range a.state.Providers {
		if a.modelProviderOffered(provider) {
			providerCount++
		}
	}
	rowHeight := float32(32)
	supplierHeight := design.Controls().ButtonHeight
	leftHeight := min(float32(providerCount)*(supplierHeight+2)+supplierHeight+20, budget)
	searchHeight := float32(0)
	if a.modelSearchVisible() {
		searchHeight = 40
	}
	optionHeight := float32(0)
	if a.hasModelOptions() {
		optionHeight = 120
	}
	height := min(float32(max(1, min(7, len(rows))))*34+8+searchHeight+optionHeight, budget)
	if a.state.ModelCatalog.State != contracts.CatalogReady && a.state.ModelCatalog.State != "" {
		height = min(float32(180), budget)
	}
	card := func(element ui.Element) ui.Element {
		element = element.Padding(4).Radius(12).Clip()
		if !a.frosted(c) {
			element = element.Shadow(0, 8, 24, -3, ui.RGBA(0, 0, 0, .12))
		}
		return a.floatingSurface(c, element, 12)
	}
	providerID := a.modelProvider
	if providerID == "" {
		providerID = a.selectedProvider().ID
	}
	var providerCard, activeProvider ui.Element
	providers := func() {
		providerCard = card(ui.Column(c.Key("model-provider-menu"))).Width(providerWidth).Height(leftHeight).Shrink(0)
		providerCard.Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Gap(2).Children(func() {
				for _, provider := range a.state.Providers {
					if a.modelProviderOffered(provider) {
						row := a.modelGroupButton(c, provider.ID, provider.Name)
						if provider.ID == providerID {
							activeProvider = row
						}
					}
				}
			})
			ui.Box(c).FillWidth().Height(1).Margin(3, 0).Background(palette.Border)
			controlPromptChoice(c.Key("model-settings"), false).FillWidth().Label(tr(c, "Model management")).Children(func() {
				icon(c, "settings", 14)
				ui.Text(c, tr(c, "Model management")).SingleLine().MinWidth(0).Shrink(1)
			}).OnClick(a.openModelSettings)
		})
	}
	models := func() {
		modelCard := card(ui.Column(c.Key("model-list-menu"))).Width(modelWidth).Height(height)
		// Measure only scalar geometry across frames. The browsed row may move
		// inside its scroll rail; clamp the submenu above the trigger independently.
		providerBounds, rowBounds := providerCard.Bounds(), activeProvider.Bounds()
		measurement := ui.Local(modelCard, "provider-row-geometry", func() [2]ui.Rect { return [2]ui.Rect{} })
		next := [2]ui.Rect{providerBounds, rowBounds}
		if *measurement != next {
			*measurement = next
			c.After(time.Millisecond)
		}
		offset := min(max(float32(0), rowBounds.Y-providerBounds.Y-4), leftHeight-height)
		if modelsOnLeft {
			modelCard.AttachTo(providerCard, ui.AnchorTopStart, ui.AnchorTopEnd).Left(-modelPickerGap)
		} else {
			modelCard.AttachTo(providerCard, ui.AnchorTopEnd, ui.AnchorTopStart).Left(modelPickerGap)
		}
		modelCard.Top(offset).SnapToWindow(8).Children(func() {
			if a.modelCatalogState(c, max(float32(0), height-10)) {
				return
			}
			if a.hasModelOptions() {
				defer func() {
					ui.Scroll(c).Shrink(0).FillWidth().MaxHeight(min(float32(120), height*.4)).BorderWidth(1, 0, 0, 0).BorderColor(palette.Border).Children(func() { a.modelOptionsPanel(c) })
				}()
			}
			if a.modelSearchVisible() {
				a.modelSearchInput(c)
			}
			if len(rows) == 0 {
				a.modelCursor = -1
				ui.Column(c).FillWidth().Grow(1).Padding(8).Center().Children(func() {
					ui.Text(c, tr(c, "No models found")).FontSize(design.TextSize(c, design.TextBase)).TextColor(palette.Muted)
				})
				return
			}
			if a.modelCursor >= len(rows) || a.modelCursor < 0 {
				a.modelCursor = 0
			}
			a.modelList.Key = func(i int) any { return rows[i].model.ID }
			a.modelList.Label = func(i int) string { return rows[i].model.Label }
			// The keyboard cursor is navigation state, not a committed model.
			// Native List selection would paint it before the pointer arrives.
			a.modelList.Selected = nil
			list := ui.List(c, &a.modelList, len(rows), func(i int) {
				row := rows[i]
				m := row.model
				button := ui.ButtonBase(c.Key("model-choice:"+m.ID)).FillWidth().Height(rowHeight).Padding(0, 4, 0, 8).Radius(7).Gap(4).AlignItems(ui.Center).Justify(ui.SpaceBetween).Label(tr(c, a.controlLabel("Select model")) + " " + m.Label).Tooltip(m.Label).TextColor(a.palette.Text)
				if button.Hovered() {
					button.Background(a.palette.Hover)
				}
				if button.Hovered() {
					a.modelCursor = i
				}
				button.Children(func() {
					ui.Text(c, m.Label).MinWidth(0).Shrink(1).SingleLine().FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
					ui.Box(c).Size(12, 12).Children(func() {
						if a.modelID == m.ID {
							icon(c, "check", 12)
						}
					})
				}).OnClick(func() { a.activateModelRow(rows, i) })
			}).Grow(1).MinHeight(0).FillWidth().Gap(2).Focusable()
			if !a.modelSearchVisible() {
				list.AutoFocus().KeepFocus()
			}
			for _, key := range []ui.Key{ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd, ui.KeyPageUp, ui.KeyPageDown} {
				list.OnShortcut(0, key, func() {
					switch key {
					case ui.KeyUp:
						a.modelCursor = max(0, a.modelCursor-1)
					case ui.KeyDown:
						a.modelCursor = min(len(rows)-1, a.modelCursor+1)
					case ui.KeyHome:
						a.modelCursor = 0
					case ui.KeyEnd:
						a.modelCursor = len(rows) - 1
					case ui.KeyPageUp:
						a.modelCursor = max(0, a.modelCursor-7)
					case ui.KeyPageDown:
						a.modelCursor = min(len(rows)-1, a.modelCursor+7)
					}
					a.modelList.ScrollIntoView(a.modelCursor)
				})
			}
			list.OnShortcut(0, ui.KeyEnter, func() { a.activateModelRow(rows, a.modelCursor) })
		})
	}
	row := ui.Row(c).FillWidth().Height(leftHeight).AlignItems(ui.Start)
	if modelsOnLeft {
		row.Justify(ui.End)
	}
	row.Children(func() {
		providers()
		models()
	})
}

func (a *composerController) modelSearchVisible() bool {
	providerID := a.modelProvider
	if providerID == "" {
		providerID = a.selectedProvider().ID
	}
	count := 0
	for _, model := range a.state.Models {
		if model.ProviderID == providerID {
			count++
		}
	}
	return a.modelSearchOpen || count > 7
}

func (a *composerController) hasModelOptions() bool {
	effort := a.reasoningChoice()
	for _, option := range a.selectedModel().Options {
		if option.ID != effort.option.ID {
			return true
		}
	}
	return false
}

func (a *composerController) modelSearchInput(c *ui.Context) {
	palette := a.modelPopupPalette(c)
	ui.Row(c).Height(40).Shrink(0).FillWidth().Padding(0, 10).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(palette.Border).Children(func() {
		ui.Box(c).Size(14, 14).TextColor(palette.Muted).Children(func() { icon(c, "magnifer", 14) })
		ui.TextInputBase(c.Key("model-search"), &a.modelQuery).Bind(&a.modelSearch).Label(tr(c, a.controlLabel("Search models"))).Placeholder(tr(c, "Search models…")).PlaceholderColor(palette.Faint).CaretWidth(2).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).AutoFocus().OnChange(func() { a.modelCursor = 0; a.modelList.ScrollIntoView(0) }).OnTextKey(func(k ui.TextKey) bool {
			a.modelQuery = k.Text()
			rows := a.modelRows()
			if len(rows) > 0 && (a.modelCursor < 0 || a.modelCursor >= len(rows)) {
				a.modelCursor = 0
			}
			switch k.Key {
			case ui.KeyDown, ui.KeyUp:
				if len(rows) > 0 {
					delta := 1
					if k.Key == ui.KeyUp {
						delta = -1
					}
					a.modelCursor = (a.modelCursor + delta + len(rows)) % len(rows)
					a.modelList.ScrollIntoView(a.modelCursor)
				}
				return true
			case ui.KeyEnter:
				a.activateModelRow(rows, a.modelCursor)
				return true
			}
			return false
		})
	})
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
}

func (a *composerController) modelRows() []modelRow {
	var rows []modelRow
	query := strings.ToLower(strings.TrimSpace(a.modelQuery))
	providerID := a.modelProvider
	if providerID == "" {
		providerID = a.selectedProvider().ID
	}
	for _, provider := range a.state.Providers {
		if !a.modelProviderOffered(provider) {
			continue
		}
		for _, model := range a.state.Models {
			if model.ProviderID != provider.ID {
				continue
			}
			if provider.ID != providerID {
				continue
			}
			matches := true
			for _, word := range strings.Fields(query) {
				if !strings.Contains(strings.ToLower(model.Label+" "+model.Description+" "+provider.Name), word) {
					matches = false
					break
				}
			}
			if matches {
				rows = append(rows, modelRow{model, provider})
			}
		}
	}
	return rows
}

func (a *composerController) modelGroupButton(c *ui.Context, id, label string) ui.Element {
	current := a.modelProvider
	if current == "" {
		current = a.selectedProvider().ID
	}
	b := controlPromptChoice(c.Key("model-provider:"+id), current == id).FillWidth().Label(label).Children(func() {
		for _, provider := range a.state.Providers {
			if provider.ID == id {
				providerLogo(c, provider.Logo, 16)
				break
			}
		}
		ui.Text(c, label).SingleLine().MinWidth(0).Shrink(1)
		if a.selectedModel().ProviderID == id {
			icon(c, "check", 12)
		}
		ui.Box(c).Grow(1)
		icon(c, "alt-arrow-right", 12)
	})
	wasHovered := ui.Local(b, "provider-hover", func() bool { return false })
	hovered := b.Hovered()
	if hovered && !*wasHovered && current != id {
		a.browseModelProvider(id)
		// The models card can precede this rail when it opens on the left.
		c.Invalidate()
	}
	*wasHovered = hovered
	b.OnClick(func() {
		a.browseModelProvider(id)
		if a.modelSearchVisible() {
			a.modelSearch.Focus(c)
		}
	})
	return b
}

func (a *composerController) browseModelProvider(id string) {
	if a.modelProvider == id {
		return
	}
	a.modelProvider = id
	a.modelQuery = ""
	a.modelCursor = 0
	a.modelList.ScrollIntoView(0)
}

func (a *composerController) openModelSettings() {
	a.modelMenu, a.effortMenu = false, false
	if a.state.Runtime == nil {
		a.navigate("settings/Providers")
		return
	}
	a.navigate("settings/Model settings")
}
