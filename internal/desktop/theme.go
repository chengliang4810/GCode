package desktop

import "github.com/egoist/mygo/ui"

// palette keeps the workbench surfaces close to neutral. Accent colors carry
// interaction and state only; inactive structure is separated by one-pixel
// rules and small changes in luminance.
type palette struct {
	dark          bool
	background    ui.Color
	sidebar       ui.Color
	surface       ui.Color
	surfaceAlt    ui.Color
	surfaceRaised ui.Color
	selection     ui.Color
	border        ui.Color
	text          ui.Color
	muted         ui.Color
	accent        ui.Color
	accentSoft    ui.Color
	success       ui.Color
	warning       ui.Color
	danger        ui.Color
	code          ui.Color
	codeText      ui.Color
	diffAdd       ui.Color
	diffDel       ui.Color
}

func newPalette(dark bool) palette {
	if dark {
		return palette{
			dark:          true,
			background:    ui.Hex("#090909"),
			sidebar:       ui.Hex("#080808"),
			surface:       ui.Hex("#0d0d0d"),
			surfaceAlt:    ui.Hex("#181818"),
			surfaceRaised: ui.Hex("#191919"),
			selection:     ui.Hex("#242424"),
			border:        ui.RGBA(255, 255, 255, 0.085),
			text:          ui.Hex("#e7e7e7"),
			muted:         ui.Hex("#a0a0a0"),
			accent:        ui.Hex("#5b9cff"),
			accentSoft:    ui.RGBA(91, 156, 255, 0.15),
			success:       ui.Hex("#3dbb8a"),
			warning:       ui.Hex("#e7b86c"),
			danger:        ui.Hex("#ed6b76"),
			code:          ui.Hex("#111111"),
			codeText:      ui.Hex("#d9d9d5"),
			diffAdd:       ui.RGBA(52, 211, 153, 0.13),
			diffDel:       ui.RGBA(248, 113, 113, 0.13),
		}
	}
	return palette{
		background:    ui.Hex("#f6f6f4"),
		sidebar:       ui.Hex("#ededeb"),
		surface:       ui.Hex("#fbfbf9"),
		surfaceAlt:    ui.Hex("#f0f0ee"),
		surfaceRaised: ui.Hex("#e6e6e3"),
		selection:     ui.Hex("#dcdcd8"),
		border:        ui.RGBA(25, 25, 22, 0.13),
		text:          ui.Hex("#1c1c1a"),
		muted:         ui.Hex("#666660"),
		accent:        ui.Hex("#4b67c8"),
		accentSoft:    ui.RGBA(75, 103, 200, 0.12),
		success:       ui.Hex("#217d58"),
		warning:       ui.Hex("#94620e"),
		danger:        ui.Hex("#b33c43"),
		code:          ui.Hex("#f0f0ed"),
		codeText:      ui.Hex("#2b2b28"),
		diffAdd:       ui.RGBA(22, 135, 93, 0.11),
		diffDel:       ui.RGBA(189, 62, 70, 0.11),
	}
}

func (p palette) theme() *ui.Theme {
	base := ui.LightTheme()
	if p.dark {
		base = ui.DarkTheme()
	}
	base.Background = p.background
	base.Surface = p.surface
	base.SurfaceHover = p.surfaceAlt
	base.SurfacePressed = p.surfaceAlt
	base.Border = p.border
	base.Text = p.text
	base.TextMuted = p.muted
	base.Accent = p.accent
	base.AccentHover = p.accent.Mix(ui.RGB(0, 0, 0), 0.08)
	base.AccentPressed = p.accent.Mix(ui.RGB(0, 0, 0), 0.16)
	base.AccentText = ui.RGB(255, 255, 255)
	base.Danger = p.danger
	base.Warning = p.warning
	base.Success = p.success
	base.Radius = 8
	base.Spacing = 4
	return base
}

func (p palette) statusColor(state appState) ui.Color {
	switch state {
	case stateRunning, stateWaiting:
		return p.accent
	case stateFailed:
		return p.danger
	case stateDone:
		return p.success
	default:
		return p.muted
	}
}

type appState uint8

const (
	stateIdle appState = iota
	stateRunning
	stateWaiting
	stateFailed
	stateDone
)
