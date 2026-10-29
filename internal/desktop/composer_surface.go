package desktop

import (
	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
	"strings"
)

// One native input/action surface for both owners. UI state and callbacks bind
// to the captured composer; the root App owns backend/settings work throughout.
type composerSurfaceProps struct {
	draft                                         *string
	input, surface                                *ui.Handle
	controls                                      *composerController
	inputLabel, attachLabel, sendLabel, stopLabel string
	canSend, running, autofocus                   bool
	hero                                          bool
	widthHint                                     float32
	renderStaged                                  func()
	onSend                                        func(string) bool
	onStop, onChooseAttach                        func()
	onPaste                                       func(ui.Services) bool
	onFilesDrop                                   func([]string)
	onMention                                     func(workbench.FileDrag)
	stopWithEscape                                bool
	focusPending                                  *bool
}

func (a *App) nativeComposerSurface(c *ui.Context, p composerSurfaceProps) {
	radius := float32(22)
	if p.hero {
		radius = 26
	}
	composer := ui.Column(c).Key("native-composer").FillWidth().Shrink(0).Radius(radius).Background(a.palette.Input).Border(1, a.palette.Border)
	if p.surface != nil {
		composer.Bind(p.surface)
	}
	layout := ui.Local(composer, "layout", func() composerLayoutState { return composerLayoutState{} })
	width := composer.Bounds().W
	if p.widthHint > 0 && (width <= 0 || layout.lastWidthHint != p.widthHint) {
		width = p.widthHint
	}
	if width <= 0 {
		width = 736
	}
	controlWidth := layout.controls.Bounds(c).W
	if controlWidth <= 0 {
		controlWidth = min(width*.45, 300)
	}
	controlWidth = min(controlWidth, width*.45)
	font := ui.Font{Family: design.Font(a.state.Settings.Appearance.Font), Size: design.UISize(c, composerTextSize)}
	displayText := *p.draft
	if layout.hasDisplayText && layout.committedText == *p.draft {
		displayText = layout.displayText
	}
	textWidth, _ := ui.ShapeText(displayText, font, 0).Size()
	expanded, settle := layout.update(c.Now(), textWidth, composerCompactCapacity(width, controlWidth), composerHasNewline(displayText))
	expanded = expanded || p.hero
	if settle > 0 {
		c.After(settle)
	}
	// Committed bounds arrive after layout. One following frame learns the
	// controls' real width after a resize or label change, without a worker.
	controlKey := p.controls.modelLabel() + p.controls.reasoning + string(p.controls.accessMode)
	if p.controls.plan {
		controlKey += ":plan"
	}
	if layout.lastWidth != width || layout.lastWidthHint != p.widthHint || layout.lastDraft != *p.draft || layout.lastControls != controlKey {
		c.AnimationFrame()
		layout.lastWidth, layout.lastDraft, layout.lastControls = width, *p.draft, controlKey
		layout.lastWidthHint = p.widthHint
	}
	inset, actionBottom := float32(8), float32(7.5)
	if expanded {
		inset, actionBottom = 12, 8
	}
	composer.Children(func() {
		if p.renderStaged != nil {
			p.renderStaged()
		}
		// Both modes retain this body and the same keyed input. Only positions
		// change, preserving the native editor's IME, undo, focus and selection.
		body := ui.Column(c).Key("body").FillWidth().Shrink(0)
		if !expanded {
			body.Height(composerCompactHeight - 2)
		}
		body.Children(func() {
			c.Key("input")
			input := ui.TextAreaBase(c, p.draft).Bind(p.input).Label(tr(c, p.inputLabel)).Placeholder(tr(c, "Do anything…")).PlaceholderColor(a.palette.Faint).CaretWidth(2).MinWidth(0).FontSize(font.Size).FixedLineHeight(composerLineHeight)
			if expanded {
				floor := float32(60)
				if p.hero {
					floor = 76
				}
				// Let the native editor own wrapped/preedit content height,
				// including fallback-font leading, while clamping its box.
				input.FillWidth().Lines(1, 100).MinHeight(floor).MaxHeight(260).Padding(16, 16, 4, 16).Shrink(0)
			} else {
				input.Absolute().Top(0).Left(36).Right(44+controlWidth).Height(47).Padding(12, 8)
			}
			// A plain native display snapshot includes preedit without writing it
			// into the draft. Re-evaluate after this input has consumed its edit.
			shown := input.InputDisplayText()
			if shown != displayText {
				c.AnimationFrame()
			}
			layout.displayText, layout.committedText, layout.hasDisplayText = shown, *p.draft, true
			if p.autofocus {
				input.AutoFocus()
			}
			if p.focusPending != nil && *p.focusPending {
				p.input.Focus(c)
				*p.focusPending = false
			}
			services := c.Services()
			if p.onPaste != nil {
				input.OnPaste(func(_ string) bool { return p.onPaste(services) })
			}
			composing := input.Composing()
			input.OnTextKey(func(key ui.TextKey) bool {
				// Keep an IME confirmation from sending an already nonempty
				// draft. Capture data, never the build-scoped input element.
				if composing && key.Key == ui.KeyEnter {
					return true
				}
				behavior := a.state.Settings.General.SendBehavior
				shift := behavior == "shiftEnter" || behavior == "Shift+Enter"
				mod := behavior == "modEnter" || behavior == "mod-enter"
				if key.Key == ui.KeyEnter && ((!shift && !mod && key.Mods == 0) || (shift && key.Mods == ui.Shift) || (mod && key.Mods == ui.Cmd)) {
					if p.onSend(key.Text()) {
						key.SetText(*p.draft)
					}
					return true
				}
				return false
			})
			if p.stopWithEscape && p.onStop != nil {
				p.input.OnShortcut(c, 0, ui.KeyEscape, p.onStop)
			}
			if expanded {
				ui.Box(c).Key("action-space").Height(composerActionsHeight).Shrink(0)
			}
			c.Key("attach")
			attach := ui.ButtonBase(c).Absolute().Left(inset).Bottom(actionBottom+2).Size(28, 28).Radius(14).Label(tr(c, p.attachLabel)).Tooltip(tr(c, p.attachLabel)).Center().TextColor(a.palette.Muted)
			if attach.Hovered() {
				attach.Background(a.palette.Hover)
			}
			attach.Children(func() { icon(c, "paperclip", 18) }).OnClick(p.onChooseAttach)
			ui.Row(c).Key("controls").Bind(&layout.controls).Absolute().Right(inset + 28 + 8).Bottom(actionBottom).Height(32).MinWidth(0).MaxWidth(width * .45).Gap(4).AlignItems(ui.Center).Children(func() {
				p.controls.runControls(c)
				p.controls.modelTrigger(c)
			})
			label := p.sendLabel
			if p.running {
				label = p.stopLabel
			}
			c.Key("send")
			button := ui.ButtonBase(c).Absolute().Right(inset).Bottom(actionBottom+2).Size(28, 28).Radius(14).Label(tr(c, label)).Tooltip(tr(c, label)).Center().Background(a.palette.Text).TextColor(a.palette.Background)
			button.Children(func() {
				if p.running {
					ui.Box(c).Size(11, 11).Radius(3).Background(a.palette.Background)
				} else {
					icon(c, "arrow-up", 14)
				}
			})
			button.Disabled(!p.running && !p.canSend).OnClick(func() {
				if p.running {
					p.onStop()
				} else {
					p.onSend(*p.draft)
				}
			})
		})
	})
	if p.onFilesDrop != nil {
		composer.OnFilesDrop(p.onFilesDrop)
	}
	if composer.FileDragOver() {
		composer.Border(1, a.palette.Accent)
	}
	if file, ok := ui.Drop[workbench.FileDrag](composer); ok && p.onMention != nil {
		p.onMention(file)
	}
}

func composerHasContent(draft string, staged int) bool {
	return strings.TrimSpace(draft) != "" || staged > 0
}
