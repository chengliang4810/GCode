package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"strings"
)

// composerSendMode is what the action button does this frame, as the
// reference's SendButtonMode: Stop only with an empty composer, and a send
// during a run steers or queues as the setting says.
type composerSendMode uint8

const (
	composerSend composerSendMode = iota
	composerSteer
	composerQueue
	composerStop
)

// One native input/action surface for both owners. UI state and callbacks bind
// to the captured composer; the root App owns backend/settings work throughout.
type composerSurfaceProps struct {
	keyPrefix                                     string
	draft                                         *string
	input, surface                                *ui.Handle
	controls                                      *composerController
	inputLabel, attachLabel, sendLabel, stopLabel string
	steerLabel, queueLabel                        string
	sendMode                                      composerSendMode
	canSend, running, autofocus                   bool
	hero                                          bool
	// handoff carries the blank-canvas route the composer is leaving while
	// its dock clock runs; zero Amount means the pill is at rest.
	handoff                composerHandoff
	widthHint              float32
	renderStaged           func()
	onSend                 func(string) bool
	onStop, onChooseAttach func()
	onPaste                func(ui.Services) bool
	onFilesDrop            func([]string)
	onMention              func(workbench.FileDrag)
	stopWithEscape         bool
	focusPending           *bool
}

func (a *App) nativeComposerSurface(c *ui.Context, p composerSurfaceProps) {
	prompt := a.preparePromptEditor(p)
	radius := float32(22)
	if p.hero {
		radius = 26
	}
	composer := ui.Column(c).Key("native-composer").FillWidth().Shrink(0).Radius(radius)
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
	font := ui.Font{Family: design.Font(a.state.Settings.Appearance.Font), Size: design.TextSize(c, design.TextBase)}
	lineHeight := font.Size * 1.625
	compactHeight := composerCompactHeight - 2
	// Center the line in the fixed single-row box. Fixed 12 px top/bottom
	// padding leaves too little room once the interface font reaches 16 px.
	compactPaddingY := max(0, (compactHeight-lineHeight)/2)
	displayText := prompt.doc.Text().String()
	if layout.hasDisplayText && layout.committedText == *p.draft {
		displayText = layout.displayText
	}
	textWidth, _ := ui.ShapeRichText(promptMeasureSpans(prompt, displayText), font, 0).Size()
	expanded, settle := layout.update(c.Now(), textWidth, composerCompactCapacity(width, controlWidth), composerHasNewline(displayText))
	expanded = expanded || p.hero
	if settle > 0 {
		c.After(settle)
	}
	// The blank canvas forces the tall input; an established thread uses the
	// flip's committed mode. These are the two ends the dock handoff eases
	// between, so the pill keeps one layout while only its box morphs.
	inputFloor := float32(60)
	if p.hero {
		inputFloor = 76
	}
	inputHeight := min(260, max(inputFloor, prompt.state.ContentHeight+20))
	natural := composerCompactHeight + 2
	if expanded {
		natural = inputHeight + composerActionsHeight + 2
	}
	morph := p.handoff.Amount > 0 && p.handoff.Height > 0
	var geometry composerMorphGeometry
	if morph {
		geometry = composerMorphGeometryOf(
			composerRouteStyle{hero: p.hero, expanded: expanded},
			composerRouteStyle{hero: p.handoff.Hero, expanded: p.handoff.Expanded, height: p.handoff.Height},
			p.handoff.Amount, natural, compactPaddingY, controlWidth,
		)
		radius = geometry.radius
		composer.Radius(radius).Height(geometry.height).Clip()
	}
	// Committed bounds arrive after layout. One following frame learns the
	// controls' real width after a resize or label change, without a worker.
	controlKey := p.controls.modelLabel() + p.controls.reasoning
	if layout.lastWidth != width || layout.lastWidthHint != p.widthHint || layout.lastDraft != *p.draft || layout.lastControls != controlKey {
		c.AnimationFrame()
		layout.lastWidth, layout.lastDraft, layout.lastControls = width, *p.draft, controlKey
		layout.lastWidthHint = p.widthHint
	}
	inset, actionBottom := float32(8), float32(7.5)
	if expanded {
		inset, actionBottom = 12, 8
	}
	if morph {
		inset, actionBottom = geometry.inset, geometry.actionBottom
	}
	// The frosted surface goes on after the morph settles the pill's shape:
	// its tint and hairline follow the radius the pill has this frame.
	composer = a.frostedComposer(c, composer, radius)
	composer.Children(func() {
		if p.renderStaged != nil {
			p.renderStaged()
		}
		// Both modes retain this body and the same keyed input. Only positions
		// change, preserving the native editor's IME, undo, focus and selection.
		body := ui.Column(c).Key("body").FillWidth().Shrink(0)
		switch {
		case morph:
			// Mid-handoff the box is explicit; controls pinned to its bottom
			// stay put while the top edge sweeps past the text.
			body.Height(max(0, geometry.height-2))
		case !expanded:
			body.Height(compactHeight)
		}
		body.Children(func() {
			c.Key(p.keyPrefix + "input")
			services := c.Services()
			options := richtext.EditorOptions{
				Label: tr(c, p.inputLabel), Placeholder: tr(c, "Do anything…"), Font: font, NoMarkdown: true,
				Theme:          &richtext.Theme{Text: a.palette.Text, Muted: a.palette.Faint, FixedLineHeight: lineHeight},
				State:          &prompt.state,
				OnKey:          func(key ui.InputEvent, composing bool) bool { return a.promptKey(p, prompt, key, composing) },
				ContentPadding: [4]float32{compactPaddingY, 8, compactPaddingY, 8},
			}
			options.OnPaste = func() bool {
				if p.onPaste != nil && p.onPaste(services) {
					return true
				}
				text := services.ReadClipboard()
				for _, part := range contracts.ParsePrompt(text) {
					if part.Reference != nil {
						prompt.doc.Insert(a.promptText(text))
						return true
					}
				}
				return false
			}
			switch {
			case morph:
				options.ContentPadding = geometry.content
			case expanded:
				options.ContentPadding = [4]float32{16, 16, 4, 16}
			}
			input := richtext.EditorBase(c, prompt.doc, options).Bind(p.input).MinWidth(0).FocusRing(false)
			a.promptInput(c, input, p, prompt)
			switch {
			case morph:
				input.Absolute().Top(0).Left(geometry.left).Right(geometry.right).Height(geometry.input).Padding(
					geometry.padding[0], geometry.padding[1], geometry.padding[2], geometry.padding[3],
				).Shrink(0)
			case expanded:
				input.FillWidth().Height(inputHeight).Padding(16, 16, 4, 16).Shrink(0)
			default:
				input.Absolute().Top(0).Left(36).Right(44+controlWidth).Height(compactHeight).Padding(compactPaddingY, 8)
			}
			// A plain native display snapshot includes preedit without writing it
			// into the draft. Re-evaluate after this input has consumed its edit.
			shown := prompt.state.DisplayText
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
			if expanded && !morph {
				ui.Box(c).Key("action-space").Height(composerActionsHeight).Shrink(0)
			}
			c.Key(p.keyPrefix + "attach")
			attach := ui.ButtonBase(c).Absolute().Left(inset).Bottom(actionBottom+2).Size(28, 28).Radius(14).Label(tr(c, p.attachLabel)).Tooltip(tr(c, p.attachLabel)).Center().TextColor(a.palette.Muted)
			if attach.Hovered() {
				attach.Background(a.palette.Hover)
			}
			attach.Children(func() { icon(c, "paperclip", 18) }).OnClick(p.onChooseAttach)
			ui.Row(c).Key("controls").Bind(&layout.controls).Absolute().Right(inset + 28 + 8).Bottom(actionBottom).Height(32).MinWidth(0).MaxWidth(width * .45).Gap(4).AlignItems(ui.Center).Children(func() {
				p.controls.modelTrigger(c)
				p.controls.reasoningTrigger(c)
			})
			label := p.sendLabel
			switch p.sendMode {
			case composerStop:
				label = p.stopLabel
			case composerSteer:
				label = p.steerLabel
			case composerQueue:
				label = p.queueLabel
			}
			c.Key(p.keyPrefix + "send")
			button := ui.ButtonBase(c).Absolute().Right(inset).Bottom(actionBottom+2).Size(28, 28).Radius(14).Label(tr(c, label)).Tooltip(tr(c, label)).Center().Background(a.palette.Text).TextColor(a.palette.Background)
			button.Children(func() {
				if p.sendMode == composerStop {
					ui.Box(c).Size(11, 11).Radius(3).Background(a.palette.Background)
				} else {
					icon(c, "arrow-up", 14)
				}
			})
			button.Disabled(p.sendMode != composerStop && !p.canSend).OnClick(func() {
				if p.sendMode == composerStop {
					p.onStop()
				} else {
					p.onSend(*p.draft)
				}
			})
		})
	})
	a.promptPanel(c, composer, p, prompt)
	if p.onFilesDrop != nil {
		composer.OnFilesDrop(p.onFilesDrop)
	}
	if composer.FileDragOver() {
		composer.Border(1, a.palette.Accent)
	}
	if file, ok := ui.Drop[workbench.FileDrag](composer); ok && p.onMention != nil {
		p.onMention(file)
	}
	if p.keyPrefix == "" {
		// The handoff leaves from this route's own geometry. The measured box
		// is a frame old, so the height this frame committed is kept beside
		// it: a draft typed and sent within one frame batch still hands off
		// its tall pill instead of the previous, shorter one.
		a.composerPillBox = composer.LayoutBounds()
		a.composerPillHeight = max(natural, a.composerPillBox.H)
		a.composerExpanded = expanded || p.hero
	}
}

func composerHasContent(draft string, staged int) bool {
	return strings.TrimSpace(draft) != "" || staged > 0
}
