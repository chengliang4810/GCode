// Package linkui supplies native, selectable links shared by conversation and
// document views. Owners resolve destinations and perform all navigation.
package linkui

import (
	"strings"
	"time"
	"unicode/utf16"

	"gcode/internal/design"
	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui"
)

// Destination is presentation data resolved by the owner without UI-thread IO.
type Destination struct {
	Raw, Tooltip, Label string
	Kind                linktarget.Kind
	KnownFile, Local    bool
}

// Config keeps opening policy and persistence in the caller.
type Config struct {
	Resolve                    func(raw string) (Destination, bool)
	Open, OpenExternal, Reveal func(raw string)
	Internal                   bool
	SetInternal                func(bool)
	Selection                  *Selection
}

type stateKey struct{}

type linkState struct {
	anchor, card     ui.Handle
	wait, leave      time.Time
	shown, dismissed bool
	down, key        uint64
	primary, dragged bool
	pressX, pressY   float32
}

// Link preserves each label span as selectable text. Clicking and keyboard
// activation use native text interaction; dragging a selection is not a click.
func Link(c *ui.Context, spans []ui.Span, destination Destination, config Config) ui.Element {
	return link(c, spans, destination, config, true)
}

func link(c *ui.Context, spans []ui.Span, d Destination, cfg Config, underline bool) ui.Element {
	spans = append([]ui.Span(nil), spans...)
	var label strings.Builder
	for i := range spans {
		spans[i].Color = c.Theme().Accent
		spans[i].Weight = max(500, spans[i].Weight)
		spans[i].Underline, spans[i].WavyUnderline = false, false
		spans[i].Background, spans[i].Radius = ui.Color{}, 0
		label.WriteString(spans[i].Text)
	}
	element := ui.RichText(c, spans...).Role(ui.RoleLink).Focusable().Selectable().
		Cursor(ui.CursorPointer).Label(label.String()).Description(targetText(d))
	st := ui.Local(element, stateKey{}, func() linkState { return linkState{} })
	element.Bind(&st.anchor)
	primary := st.anchor.OwnsPrimaryPress(c)
	if primary {
		x, y, _ := c.Root().PointerPosition()
		if !st.primary {
			st.pressX, st.pressY, st.dragged = x, y, false
		} else if dx, dy := x-st.pressX, y-st.pressY; dx*dx+dy*dy >= 16 {
			st.dragged = true
		}
	}
	st.primary = primary
	keyboard := c.InputActivity().Modality == ui.InputModalityKeyboard
	element.OnClick(func() {
		st.shown, st.dismissed = false, true
		if st.dragged && !keyboard {
			st.dragged = false
			return
		}
		open(d, cfg, false)
	})
	buildMenu := linkMenu(c, cfg)
	element.ContextMenu(func(menu *ui.Menu) {
		st.shown, st.dismissed = false, true
		buildMenu(menu, d)
	})
	hover := element.Hovered()
	if underline && hover {
		// Inline text does not run DrawOver in MyGo. A non-interactive overlay
		// paints its decoration; Block below shapes the whole paragraph so
		// links wrapping after preceding words retain exact line positions.
		bounds := st.anchor.Bounds(c)
		font := ui.Font{Family: c.Theme().Font, Size: c.Theme().FontSize, Weight: 500}
		drawSpans := append([]ui.Span(nil), spans...)
		color := c.Theme().Accent
		ui.Overlay(c, func() {
			ui.Box(c).Absolute().PassThrough().AttachAtPoint(bounds.X, bounds.Y, 0).
				Size(bounds.W, bounds.H).Draw(func(p *ui.Painter, _ ui.Rect) {
				drawUnderline(p, bounds, drawSpans, font, 0, 0, utf16Length(label.String()), color)
			})
		})
	}
	tooltip(c, element, st, targetText(d))
	return element
}

func targetText(d Destination) string {
	if d.Tooltip != "" {
		return d.Tooltip
	}
	return d.Raw
}

func open(d Destination, cfg Config, forceInternal bool) {
	if d.Kind == linktarget.Web && !cfg.Internal && !forceInternal {
		if cfg.OpenExternal != nil {
			cfg.OpenExternal(d.Raw)
		}
		return
	}
	if cfg.Open != nil {
		cfg.Open(d.Raw)
	}
}

func tooltip(c *ui.Context, anchor ui.Element, st *linkState, text string) {
	if text == "" {
		return
	}
	now := c.Now()
	activity := c.InputActivity()
	focused, hovered := anchor.FocusVisible(), anchor.Hovered()
	x, y, inWindow := c.Root().PointerPosition()
	card := st.card.Bounds(c)
	overCard := st.shown && inWindow && x >= card.X && y >= card.Y &&
		x < card.X+card.W && y < card.Y+card.H
	if !hovered && !overCard && !focused {
		st.wait = time.Time{}
		if st.shown {
			if st.leave.IsZero() {
				st.leave = now.Add(180 * time.Millisecond)
			}
			if now.Before(st.leave) {
				c.After(st.leave.Sub(now))
			} else {
				st.shown, st.dismissed = false, false
			}
		} else {
			st.dismissed = false
		}
	} else {
		st.leave = time.Time{}
	}
	if activity.DownSerial != st.down || activity.KeyDownSerial != st.key && !focused {
		st.shown, st.dismissed = false, true
	}
	st.down, st.key = activity.DownSerial, activity.KeyDownSerial
	if !st.dismissed && !activity.PrimaryDown {
		switch {
		case focused:
			st.shown = true
		case hovered:
			if st.wait.IsZero() {
				st.wait = now.Add(600 * time.Millisecond)
			}
			if !now.Before(st.wait) {
				st.shown = true
			} else {
				c.After(st.wait.Sub(now))
			}
		}
	}
	if !st.shown {
		return
	}
	size := design.TextSize(c, design.TextSmall)
	lineHeight := size * 1.5
	maxWidth := min(float32(360), max(float32(40), c.Root().Bounds().W-40))
	maxHeight := min(24*lineHeight, max(2*lineHeight, c.Root().Bounds().H-40))
	ui.Overlay(c, func() {
		tip := ui.Box(c).Bind(&st.card).Absolute().Role(ui.RoleTooltip).
			Label(text).MaxWidth(maxWidth).Padding(8, 10).Radius(6).
			Border(design.Controls().BorderWidth, c.Theme().Border).
			Background(c.Theme().Surface.Over(c.Theme().Background)).TextColor(c.Theme().TextMuted).
			FontSize(size).FixedLineHeight(lineHeight).
			AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 6, 0)
		if tip.OverlayShortcut(0, ui.KeyEscape) {
			st.shown, st.dismissed = false, true
		}
		tip.Children(func() {
			// An interactive card lets the pointer leave the label to scroll
			// the full destination. It never truncates the target string.
			ui.Scroll(c).MaxHeight(maxHeight).MaxWidth(maxWidth - 20).Children(func() {
				ui.Text(c, text)
			})
		})
	})
}

func utf16Length(text string) int { return len(utf16.Encode([]rune(text))) }

func drawUnderline(p *ui.Painter, bounds ui.Rect, spans []ui.Span, font ui.Font, lineHeight float32, start, end int, color ui.Color) {
	layout := ui.ShapeRichTextWith(spans, font, ui.TextShape{Width: bounds.W, FixedLineHeight: lineHeight})
	for _, rect := range layout.SelectionRects(ui.TextInputRange{Start: start, End: end}) {
		y := bounds.Y + rect.Y + rect.H - 2
		for x := bounds.X + rect.X; x < bounds.X+rect.X+rect.W; x += 3 {
			p.Fill(ui.Rect{X: x, Y: y, W: 1, H: 1}, color, .5)
		}
	}
}
