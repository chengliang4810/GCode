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
	controlsWidth                                 float32
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
	composer := ui.Column(c).FillWidth().MinHeight(116).Radius(22).Background(a.palette.Input).Border(1, a.palette.Border)
	if p.surface != nil {
		composer.Bind(p.surface)
	}
	composer.Children(func() {
		if p.renderStaged != nil {
			p.renderStaged()
		}
		input := ui.TextAreaBase(c, p.draft).Bind(p.input).Label(p.inputLabel).Placeholder("Do anything…").Lines(2, 8).Padding(16, 18, 8, 18).FontSize(design.UISize(c, 14)).FixedLineHeight(design.UISize(c, 22))
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
		input.OnTextKey(func(key ui.TextKey) bool {
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
		ui.Row(c).Padding(0, 12, 12, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { iconButton(c, "paperclip", p.attachLabel).Shrink(0).OnClick(p.onChooseAttach) })
			row := ui.Row(c).MinWidth(0).Shrink(1).Gap(4).AlignItems(ui.Center)
			if p.controlsWidth > 0 {
				row.MaxWidth(p.controlsWidth)
			}
			row.Children(func() { p.controls.runControls(c); p.controls.modelTrigger(c) })
			glyph, label := "arrow-up", p.sendLabel
			if p.running {
				glyph, label = "stop", p.stopLabel
			}
			button := iconButton(c, glyph, label).Size(28, 28).Shrink(0).Radius(14).Background(a.palette.Text).TextColor(a.palette.Background)
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
