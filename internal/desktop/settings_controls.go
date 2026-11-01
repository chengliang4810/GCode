package desktop

import (
	"strings"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type buttonVariant uint8

const (
	buttonPlain buttonVariant = iota
	buttonSecondary
	buttonPrimary
	buttonDanger
	// buttonCaptionClose is the title bar's close control: the system's
	// close-button treatment rather than a theme accent.
	buttonCaptionClose
)

// Windows caption controls use the system's close colors: a red backplate
// with a white glyph while hovered or pressed.
var (
	captionCloseBackplate = ui.Hex("#E81123")
	captionCloseGlyph     = ui.Hex("#FFFFFF")
)

// Component constructors own appearance. Callers may supply content, layout,
// stable identity and behavior, but must not restyle the returned control.
func controlButton(c *ui.Context, label string, variant buttonVariant) ui.Element {
	s, t := design.Controls(), c.Theme()
	b := ui.ButtonBase(c).Height(max(s.ButtonHeight, design.TextSize(c, s.FontSize)*1.5+s.ButtonPaddingY*2)).
		Padding(s.ButtonPaddingY, s.ButtonPaddingX).
		Radius(s.ButtonRadius).Gap(s.ButtonGap).
		FontSize(design.TextSize(c, s.FontSize)).Label(label).AlignItems(ui.Center).Justify(ui.Center)
	switch variant {
	case buttonPrimary:
		b.Background(t.Accent).TextColor(t.AccentText)
		if b.Hovered() {
			b.Background(t.AccentHover)
		}
		if b.Pressed() {
			b.Background(t.AccentPressed)
		}
	case buttonSecondary:
		b.Background(t.SurfaceHover).Border(s.BorderWidth, t.Border)
		if b.Hovered() {
			b.Background(t.SurfacePressed)
		}
	case buttonDanger:
		b.TextColor(t.Danger)
		if b.Hovered() {
			b.Background(t.Danger.Alpha(.1))
		}
	case buttonCaptionClose:
		if b.Hovered() {
			b.Background(captionCloseBackplate).TextColor(captionCloseGlyph)
		}
		if b.Pressed() {
			b.Background(captionCloseBackplate.Alpha(.8)).TextColor(captionCloseGlyph.Alpha(.8))
		}
	default:
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
		if b.Pressed() {
			b.Background(t.SurfacePressed)
		}
	}
	return b
}

func textButton(c *ui.Context, label string) ui.Element {
	return controlButton(c, label, buttonPlain).Children(func() { ui.Text(c, label).SingleLine() })
}

func primaryButton(c *ui.Context, label string) ui.Element {
	return controlButton(c, label, buttonPrimary).Children(func() { ui.Text(c, label).SingleLine() })
}

func secondaryButton(c *ui.Context, label string) ui.Element {
	return controlButton(c, label, buttonSecondary).Children(func() { ui.Text(c, label).SingleLine() })
}

func dangerButton(c *ui.Context, label string) ui.Element {
	return controlButton(c, label, buttonDanger).Children(func() { ui.Text(c, label).SingleLine() })
}

func buttonWithIcon(c *ui.Context, label, name string, variant buttonVariant) ui.Element {
	return controlButton(c, label, variant).Children(func() {
		icon(c, name, design.Controls().IconSize)
		ui.Text(c, label).SingleLine()
	})
}

func controlIconButton(c *ui.Context, label string) ui.Element {
	return controlIconButtonVariant(c, label, buttonPlain)
}

func controlIconButtonVariant(c *ui.Context, label string, variant buttonVariant) ui.Element {
	s := design.Controls()
	// Toolbar hit targets are logical pixels; text scaling must not outgrow
	// the shell's caption slots. The glyph still follows the UI font scale.
	size := s.IconButtonSize
	return controlButton(c, label, variant).MinHeight(0).Size(size, size).Padding(0).Tooltip(label)
}

func iconButton(c *ui.Context, name, label string) ui.Element {
	return controlIconButton(c, label).
		Children(func() { icon(c, name, design.Controls().IconSize) })
}

// windowCloseButton is the title bar's close control wherever the window
// hides its native buttons. It shares the 28px target, 16px glyph and radius
// of the title bar's other icon buttons, and adds the system's red close
// treatment on hover and press.
func windowCloseButton(c *ui.Context, label string) ui.Element {
	return controlIconButtonVariant(c, label, buttonCaptionClose).
		Children(func() { icon(c, "caption-close", design.Controls().IconSize) })
}

func inlineIconButton(c *ui.Context, name, label string) ui.Element {
	size := design.Controls().InlineIconButtonSize
	return controlIconButton(c, label).Size(size, size).
		Children(func() { icon(c, name, design.Controls().IconSize) })
}

func updateIconButton(c *ui.Context, label string) ui.Element {
	s, t := design.Controls(), c.Theme()
	button := controlIconButton(c, label).Radius(s.RoundIconButtonRadius).
		Background(t.SurfaceHover)
	if button.Hovered() || button.Pressed() {
		button.Background(t.SurfacePressed)
	}
	return button.Children(func() {
		size := s.IconSize
		ui.Box(c).Size(size, size).Radius(size/2).Border(s.BorderWidth, t.Text).
			Center().Children(func() { icon(c, "arrow-up", size*.75) })
	})
}

// controlReorderRow makes the whole row a drag source, matching the sidebar
// where a conversation row itself is dragged. Pressing the row keeps the
// keyboard focus so dragging never submits an inline draft.
func controlReorderRow(c *ui.Context, row ui.Element, value any, disabled bool) ui.Element {
	if disabled {
		return row
	}
	row = row.KeepFocus()
	row.Drag(value)
	if row.Dragging() {
		row.Opacity(0.4)
	}
	return row
}

func usageIconButton(c *ui.Context, label string) ui.Element {
	t := c.Theme()
	return controlIconButton(c, label).Children(func() {
		size := design.Controls().IconSize
		ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
			for i, height := range []float32{.45, .85, .6} {
				p.Fill(ui.Rect{X: r.X + r.W*(.16+float32(i)*.3), Y: r.Y + r.H*(1-height),
					W: r.W * .08, H: r.H * height}, t.Text, r.W*.04)
			}
		})
	})
}

// controlReorderTarget decorates a row with an insertion indicator and reports
// a typed drop. The caller validates the list scope and persists the move.
func controlReorderTarget[T any](c *ui.Context, row ui.Element, accepts func(T) bool) (value T, after, dropped bool) {
	_, y, _ := row.PointerPosition()
	after = y >= row.Bounds().H/2
	if dragging, over := ui.DragOver[T](row); over && accepts(dragging) {
		color, width := c.Theme().Accent, design.Controls().ReorderIndicatorWidth
		bottom := after
		row.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if bottom {
				r.Y += r.H - width
			}
			r.H = width
			p.Fill(r, color, 0)
		})
	}
	value, dropped = ui.Drop[T](row)
	return value, after, dropped && accepts(value)
}

func controlTextInput(c *ui.Context, value *string) ui.Element {
	s := design.Controls()
	return ui.TextInput(c, value).Height(max(s.InputHeight, design.TextSize(c, s.FontSize)*1.5+s.InputPaddingY*2)).
		FontSize(design.TextSize(c, s.FontSize)).Radius(s.InputRadius).
		Padding(s.InputPaddingY, s.InputPaddingX).
		Border(s.BorderWidth, c.Theme().Border)
}

func controlTextArea(c *ui.Context, value *string) ui.Element {
	s := design.Controls()
	return ui.TextArea(c, value).FontSize(design.TextSize(c, s.FontSize)).Radius(s.InputRadius).
		Padding(s.InputPaddingY, s.InputPaddingX).
		Border(s.BorderWidth, c.Theme().Border)
}

// A secret field uses the same input and accessory icon sizes as other fields.
// The accessory keeps input focus, so revealing a draft cannot trigger a save.
func controlSecretInput(c *ui.Context, value *string, show, canToggle bool, input func(ui.Element), toggle func()) {
	s := design.Controls()
	ui.Row(c).FillWidth().Gap(s.ButtonGap).AlignItems(ui.Center).Children(func() {
		e := controlTextInput(c.Key("value"), value).Grow(1).MinWidth(0)
		if !show {
			e.Password()
		}
		input(e)
		name, label := "eye", "Show API key"
		if show {
			name, label = "eye-closed", "Hide API key"
		}
		iconButton(c.Key("reveal"), name, tr(c, label)).KeepFocus().Disabled(!canToggle).OnClick(toggle)
	})
}

func controlCheckbox(c *ui.Context, checked *bool, label string) ui.Element {
	s, t := design.Controls(), c.Theme()
	row := ui.CheckboxBase(c, checked).Label(label).Gap(s.ButtonGap).
		FontSize(design.TextSize(c, s.FontSize)).FocusRing(false)
	on, focused, hovered := *checked, row.FocusVisible(), row.Hovered()
	size, radius := s.CheckboxSize, s.CheckboxRadius
	row.Children(func() {
		box := ui.Box(c).Size(size, size).Radius(radius).Shrink(0)
		if on {
			box.Background(t.Accent)
		} else {
			border := t.Border.Mix(t.Text, .25)
			if hovered {
				border = t.Accent
			}
			box.Background(t.Background).Border(s.BorderWidth, border)
		}
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if on {
				path := (&ui.Path{}).MoveTo(r.X+r.W*.22, r.Y+r.H*.52).
					LineTo(r.X+r.W*.42, r.Y+r.H*.71).LineTo(r.X+r.W*.78, r.Y+r.H*.31)
				p.StrokePath(path, r.W/8, t.AccentText)
			}
			if focused {
				p.FocusRing(r, [4]float32{radius, radius, radius, radius})
			}
		})
		if label != "" {
			ui.Text(c, label)
		}
	})
	return row
}

// controlCheckboxOption is the shared bordered checkbox used by option groups.
func controlCheckboxOption(c *ui.Context, checked *bool, label string) ui.Element {
	s, t := design.Controls(), c.Theme()
	return controlCheckbox(c, checked, label).
		Padding(s.OptionPaddingY, s.OptionPaddingX).
		Radius(s.OptionRadius).Border(s.BorderWidth, t.Border).Background(t.Surface)
}

func controlSelect[T comparable](c *ui.Context, value *T) ui.SelectParts[T] {
	sel := ui.SelectBase(c, value)
	controlSelectTrigger(c, sel.Trigger)
	return sel
}

func controlSelectTrigger(c *ui.Context, trigger ui.Element) ui.Element {
	s, t := design.Controls(), c.Theme()
	return trigger.MinWidth(s.SelectMinWidth).Height(max(s.InputHeight, design.TextSize(c, s.FontSize)*1.5+s.InputPaddingY*2)).
		FontSize(design.TextSize(c, s.FontSize)).Padding(0, s.InputPaddingX).
		Gap(s.ButtonGap).Radius(s.InputRadius).
		Background(t.Surface).Border(s.BorderWidth, t.Border).AlignItems(ui.Center)
}

func controlMenuButton(c *ui.Context, label string, build func(*ui.Menu)) ui.Element {
	return controlSelectTrigger(c, ui.ButtonBase(c)).Label(label).Menu(build).Children(func() {
		ui.Text(c, label).Grow(1).MinWidth(0).SingleLine()
		icon(c, "alt-arrow-down", 13)
	})
}

func controlStepSlider(c *ui.Context, value *float64, lo, hi, step float64) ui.Element {
	// Native tick marks need seven theme spaces even when the text is small.
	height := max(design.Controls().SliderHeight, c.Theme().Space(7))
	return ui.StepSlider(c, value, lo, hi, step).Height(height)
}

// controlHorizontalScrollbar gives native keyboard, focus, range and drag
// semantics to the table's floating scrollbar.
func controlHorizontalScrollbar(c *ui.Context, value *float64, maximum, viewport float32) ui.Element {
	s, t := design.Controls(), c.Theme()
	thumb := max(s.ScrollbarMinThumb, viewport*viewport/max(float32(1), viewport+maximum))
	fraction := float32(*value) / max(float32(1), maximum)
	bar := ui.SliderBase(c, value, 0, float64(max(float32(1), maximum))).Step(1).
		Height(s.ScrollbarHeight).Padding(0, thumb/2).FocusRing(false)
	bar.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y + (r.H-8)/2, W: r.W, H: 8}, t.Text.Alpha(.04), 4)
		x := r.X + max(float32(0), r.W-thumb)*min(float32(1), max(float32(0), fraction))
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-10)/2, W: min(thumb, r.W), H: 10}, t.Text.Alpha(.25), 5)
	})
	return bar
}

func controlSelectPopup(c *ui.Context, panel ui.Element) ui.Element {
	s := design.Controls()
	return panel.Padding(s.PopupPadding).Radius(s.PopupRadius).
		Background(c.Theme().Background).Border(s.BorderWidth, c.Theme().Border)
}

func controlSelectItem(c *ui.Context, item ui.Element) ui.Element {
	// Item inherits the shared trigger's font; option paddings remain in one
	// definition even for numeric, string and enum selects.
	s := design.Controls()
	item.Padding(s.OptionPaddingY, s.OptionPaddingX).
		Radius(s.OptionRadius)
	if item.Highlighted() {
		item.Background(c.Theme().SurfaceHover)
	}
	return item
}

func controlRowButton(c *ui.Context) ui.Element {
	s := design.Controls()
	b := ui.ButtonBase(c).MinHeight(s.ButtonHeight).
		Padding(s.ButtonPaddingY, s.ButtonPaddingX).
		Radius(s.ButtonRadius).FontSize(design.TextSize(c, s.FontSize)).
		AlignItems(ui.Center).Justify(ui.Start)
	if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	return b
}

func controlPromptChoice(c *ui.Context, selected bool) ui.Element {
	button := controlRowButton(c).Gap(design.Controls().ButtonGap)
	if selected {
		button.Background(c.Theme().SurfaceHover)
	}
	return button
}

func controlNavButton(c *ui.Context, label string, selected bool) ui.Element {
	s := design.Controls()
	b := ui.ButtonBase(c).Height(max(s.NavigationHeight, design.TextSize(c, s.FontSize)*1.5)).Padding(0, s.NavigationPaddingX).
		Gap(s.NavigationGap).Radius(s.InputRadius).
		FontSize(design.TextSize(c, s.FontSize)).Label(label).AlignItems(ui.Center).Justify(ui.Start)
	if selected {
		b.Background(c.Theme().SurfacePressed)
	} else if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	return b
}

func controlSidebarRow(c *ui.Context, label string, selected bool) ui.Element {
	s := design.Sidebar()
	b := ui.ButtonBase(c).Label(label).FillWidth().MinWidth(0).
		Height(max(s.RowHeight, design.TextSize(c, design.TextBase)*1.5+8)).
		Padding(0, s.RowPaddingX).Gap(8).Radius(s.RowRadius).
		FontSize(design.TextSize(c, design.TextBase)).FontWeight(400).LineHeight(1.5).
		AlignItems(ui.Center).Justify(ui.Start).TextColor(c.Theme().Text)
	if selected {
		b.Background(c.Theme().SurfacePressed)
	} else if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	return b
}

func controlSidebarProjectRow(c *ui.Context, label string, selected, offline bool) ui.Element {
	row := controlSidebarRow(c, label, selected).TextColor(c.Theme().TextMuted)
	if offline {
		row.Opacity(.5)
	}
	return row
}

func controlSidebarMoreRow(c *ui.Context, label string) ui.Element {
	return controlSidebarRow(c, label, false).
		Padding(0, design.Sidebar().ConversationTextInset).
		Background(ui.Transparent).TextColor(c.Theme().TextMuted)
}

func controlSidebarEmptyRow(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).FontSize(design.TextSize(c, design.TextBase)).
		LineHeight(1.5).TextColor(c.Theme().TextMuted).
		Padding(8, design.Sidebar().ConversationTextInset)
}

// Sidebar rows render one fixed compact line; details live in the hover card.
type sidebarConversationStyle struct {
	Selected bool
}

func controlSidebarConversationRow(c *ui.Context, label string, style sidebarConversationStyle) ui.Element {
	height := design.Sidebar().RowHeight +
		max(0, design.TextSize(c, design.TextBase)-design.TextBase)*1.5
	// Keep the shared row's center alignment: the content row is as tall as
	// the row itself, so stretching it would pin it under the top padding
	// and read low.
	return controlSidebarRow(c, label, style.Selected).Height(height).Padding(4, 8).
		Justify(ui.Start)
}

// Sidebar rows reveal their full context in one shared hover card: the title
// plus icon-labelled lines for chat count, location, branch and time.
type sidebarHoverInfo struct {
	Icon, Text string
	Mono       bool
}

func controlSidebarHoverCard(c *ui.Context, title string, infos []sidebarHoverInfo) ui.Element {
	card := ui.Column(c).MinWidth(180).MaxWidth(300).Gap(6).Padding(12, 14).Radius(10).
		Background(c.Theme().Surface).Border(1, c.Theme().Border)
	card.Children(func() {
		ui.Text(c, title).SingleLine().FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
		for _, info := range infos {
			line := info
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if line.Icon != "" {
					icon(c, line.Icon, 13)
				}
				text := ui.Text(c, line.Text).SingleLine().FontSize(design.TextSize(c, design.TextSmall)).TextColor(c.Theme().TextMuted)
				if line.Mono {
					text.Font("Geist Mono")
				}
			})
		}
	})
	return card
}

func controlSidebarHeading(c *ui.Context, label string) ui.Element {
	s := design.Sidebar()
	b := ui.ButtonBase(c).Label(label).MinWidth(0).Grow(1).
		MinHeight(s.HeaderHeight).Padding(0, s.RowPaddingX).Gap(4).
		FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).
		LineHeight(1.5).TextColor(c.Theme().TextMuted.Alpha(.75)).
		AlignItems(ui.Center).Justify(ui.Start)
	if b.Hovered() || b.Focused() {
		b.TextColor(c.Theme().Text)
	}
	return b
}

func controlTileButton(c *ui.Context, selected bool) ui.Element {
	s := design.Controls()
	b := ui.ButtonBase(c).Padding(s.TilePadding).Radius(s.TileRadius).
		Border(1, c.Theme().Border).Justify(ui.Start)
	if selected {
		b.BorderColor(c.Theme().Accent).Background(c.Theme().SurfacePressed)
	} else if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	return b
}

func controlPreviewButton(c *ui.Context, label string) ui.Element {
	return ui.ButtonBase(c).Padding(0).Radius(design.Controls().TileRadius).
		Label(label).AlignItems(ui.Start).Justify(ui.Start)
}

func controlColorSwatch(c *ui.Context, label string, color ui.Color, selected bool) ui.Element {
	size := design.Controls().SwatchSize
	b := ui.ButtonBase(c).Size(size, size).Radius(size / 2).Background(color).Label(label).Tooltip(label)
	if selected {
		b.Border(2, c.Theme().Text)
	}
	return b
}

func controlResourceAvatar(c *ui.Context, glyph string) ui.Element {
	s := design.Controls()
	return ui.Box(c).Size(s.ResourceIconSize, s.ResourceIconSize).
		Shrink(0).Radius(s.TileRadius).Center().
		Background(c.Theme().Surface).TextColor(c.Theme().TextMuted).Children(func() {
		icon(c, glyph, s.IconSize)
	})
}

func controlResourceBadge(c *ui.Context, label string) ui.Element {
	s := design.Controls()
	return ui.Text(c, label).SingleLine().MinWidth(0).FontSize(design.TextSize(c, design.TextSmall)).
		Padding(s.BadgePaddingY, s.BadgePaddingX).
		Radius(s.OptionRadius).Background(c.Theme().SurfaceHover).TextColor(c.Theme().TextMuted)
}

// controlEffortSwatch previews a reasoning-slider preset: three equal stops
// in one small capsule, as the reference appearance select shows them.
func controlEffortSwatch(c *ui.Context, palette design.EffortPalette) ui.Element {
	s := design.Controls()
	swatch := ui.Box(c).Width(s.GradientSwatchWidth).Height(s.GradientSwatchHeight).
		Shrink(0).Radius(s.GradientSwatchHeight/2).Border(s.BorderWidth, c.Theme().Border).Clip()
	swatch.Children(func() {
		ui.Row(c).Fill().Children(func() {
			for _, color := range palette.Colors {
				ui.Box(c).Grow(1).FillHeight().Background(color)
			}
		})
	})
	return swatch
}

func controlAccentSwatch(c *ui.Context, label string, palette design.Tokens, selected, gradient bool) ui.Element {
	b := controlColorSwatch(c, label, palette.Accent, selected)
	if gradient {
		b.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
				{Color: palette.Glyph[0], At: 0},
				{Color: palette.Glyph[1], At: .5},
				{Color: palette.Glyph[2], At: 1},
			}}, r.H/2)
		})
	}
	return b
}

func controlCodeButton(c *ui.Context, label, font string) ui.Element {
	return controlButton(c, label, buttonPlain).Font(font).FontSize(design.TextSize(c, design.TextXL)).
		Children(func() { ui.Text(c, label).SingleLine() })
}

// controlSwitch is a pure UI component. The caller owns its value and
// persistence callback, including provider, resource and ordinary settings.
func (a *App) controlSwitch(c *ui.Context, value *bool, label string, disabled bool, change func(bool)) ui.Element {
	s := design.Controls()
	b := ui.SwitchBase(c, value).Size(s.SwitchWidth, s.SwitchHeight).
		Label(label).Disabled(disabled).OnClick(func() {
		if change != nil {
			change(*value)
		}
	})
	position := float32(0)
	if *value {
		position = 1
	}
	position = b.AnimateWith("switch-travel", position, 180*time.Millisecond,
		func(v float32) float32 { return 1 - (1-v)*(1-v)*(1-v) })
	if a.state.Settings.Appearance.ReduceMotion ||
		strings.EqualFold(a.state.Settings.Appearance.MotionPreference, "system") && c.Preferences().ReduceMotion {
		position = 0
		if *value {
			position = 1
		}
	}
	frost := a.palette.Frosted || strings.EqualFold(a.state.Settings.Appearance.Surface, "Frosted")
	if strings.EqualFold(a.state.Settings.Appearance.Surface, "Opaque") {
		frost = false
	}
	palette := a.palette
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		// Normalized geometry preserves the existing settings switch shape
		// when shared dimensions or UI font size change.
		sx, sy := r.W/44.8, r.H/28.8
		rect := func(x, y, w, h float32) ui.Rect {
			return ui.Rect{X: r.X + x*sx, Y: r.Y + y*sy, W: w * sx, H: h * sy}
		}
		track := rect(0, 4, 44.8, 20.8)
		ink, coverage := ui.Hex("#000000"), float32(.1)
		if palette.Dark {
			ink, coverage = ui.Hex("#ffffff"), .18
		}
		if frost {
			coverage = .12
			if palette.Dark {
				coverage = .22
			}
		}
		off := ink.Alpha(coverage).Over(palette.Shell)
		on := palette.AccentStrong
		if palette.Dark {
			on = ui.Hex("#000000").Alpha(.14).Over(on)
		}
		color := off.Mix(on, position)
		light, shade := color, color
		if frost {
			light = ui.Hex("#ffffff").Alpha(.07).Over(color)
			shade = ui.Hex("#000000").Alpha(.09).Over(color)
		}
		p.FillGradient(track, ui.LinearGradient{From: light, To: shade, Angle: 180}, 10.4*sy)
		rim := palette.Border.Over(color).Mix(ui.Hex("#ffffff").Alpha(.16).Over(color), position)
		p.Stroke(track, rim, 10.4*sy, 1)
		p.Fill(rect(9, 10.8, 1.2, 7.2), ui.Hex("#ffffff").Alpha(.96*position), .6*sy)
		p.Stroke(rect(31.6, 10.8, 7.2, 7.2), ui.Hex("#ffffff").Alpha(.92*(1-position)), 3.6*sy, 1)
		x := float32(1.6) + 17.6*position
		thumb := ui.Hex("#ffffff").Alpha(.94).Over(palette.Shell)
		thumbLight, thumbShade := thumb, thumb
		if frost {
			thumbLight = ui.Hex("#ffffff").Alpha(.12).Over(thumb)
			thumbShade = ui.Hex("#000000").Alpha(.07).Over(thumb)
		}
		thumbRect := rect(x, 5.6, 24, 17.6)
		p.FillGradient(thumbRect, ui.LinearGradient{From: thumbLight, To: thumbShade, Angle: 180}, 8.8*sy)
		p.Stroke(thumbRect, ui.Hex("#000000").Alpha(.1).Over(thumb), 8.8*sy, 1)
		if frost && position > 0 {
			p.Fill(rect(x+7.2, 7.2, 9.6, 1), ui.Hex("#ffffff").Alpha(.45*position).Over(thumbLight), .5*sy)
		}
	})
	return b
}
