package workbench

import (
	"gcode/internal/design"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The source keeps independent tooltip tasks for each tab and its close button.
// Those tasks follow slot indices; the actual tab controls retain their existing
// resource identities. Only plain data and durable Handles cross builds here.
type tabTooltipState struct {
	scope    string
	activity ui.InputActivitySnapshot
	owners   []*tabTooltipOwner
	shown    int
}

type tabTooltipOwner struct {
	anchor, card ui.Handle
	phase        tabTooltipPhase
	text         string
	bounds       ui.Rect
	deadline     time.Time
	x, y         float32
	close        bool
	primaryDown  bool
}

type tabTooltipPhase uint8

const (
	tabTooltipIdle tabTooltipPhase = iota
	tabTooltipWaiting
	tabTooltipVisible
)

type tabTooltipPass struct {
	now               time.Time
	activity          ui.InputActivitySnapshot
	x, y              float32
	moved             bool
	pressedOrScrolled bool
	visible           *tabTooltipOwner
	previouslyShown   int
	nonPrimaryMove    bool
}

type tabTooltipCardKey struct {
	scope string
	owner int
}

func (s *tabTooltipState) begin(c *ui.Context, scope string, count int) tabTooltipPass {
	if s.scope != scope {
		// The existing application mounts a different tab-strip scope here.
		// This is not a claim that GPUI automatically resets a resource's tip.
		for _, owner := range s.owners {
			owner.clear()
			owner.primaryDown = false
		}
		s.scope = scope
	}
	previouslyShown := s.shown
	s.shown = -1
	if count*2 < len(s.owners) {
		// Reuse the bounded slot Handles when tabs return: the window keeps
		// each Handle binding until it closes. Inactive slots keep no task.
		for _, owner := range s.owners[count*2:] {
			owner.clear()
			owner.primaryDown = false
		}
	}
	for len(s.owners) < count*2 {
		s.owners = append(s.owners, &tabTooltipOwner{close: len(s.owners)%2 == 1})
	}
	activity := c.InputActivity()
	moved := activity.MoveSerial != s.activity.MoveSerial
	// Source capture clears pending on any MouseUp. Blur alone does not
	// erase a captured slot press, even though native ownership resets.
	if activity.UpSerial != s.activity.UpSerial {
		for _, owner := range s.owners {
			owner.primaryDown = false
		}
	}
	// Like the source window's last mouse position, this point survives a
	// plain window exit; its hit-test flag is not an extra cancellation.
	x, y, _ := c.Root().PointerPosition()
	pass := tabTooltipPass{
		now: c.Now(), activity: activity, x: x, y: y,
		moved:             moved,
		pressedOrScrolled: activity.DownSerial != s.activity.DownSerial || activity.ScrollSerial != s.activity.ScrollSerial,
		previouslyShown:   previouslyShown,
		nonPrimaryMove:    moved && !tabTooltipMoveHasPrimary(activity),
	}
	s.activity = activity
	return pass
}

func (s *tabTooltipState) observe(c *ui.Context, pass *tabTooltipPass, index int, close bool, text string, anchor ui.Element, hovered bool) {
	ownerIndex := index * 2
	if close {
		ownerIndex++
	}
	owner := s.owners[ownerIndex]
	// Read before binding: an omitted owner has no committed box, so returning
	// after an unmount cannot revive an old task or tooltip.
	bounds := owner.anchor.Bounds(c)
	// This reads the committed actual primary target before rebinding the
	// slot. It observes input without replacing any native event handler.
	// Keep the slot's pending press until any MouseUp, even if its resource
	// changes or movement leaves the original bounds in the meantime.
	pressed := owner.anchor.OwnsPrimaryPress(c)
	anchor.Bind(&owner.anchor)
	if pass.nonPrimaryMove && owner.primaryDown {
		// TooltipMove sees the old pending press before the source's generic
		// Move clears it. That event cancels WaitingForShow but cannot also
		// schedule another task; the next move may start a fresh dwell.
		owner.clear()
		owner.primaryDown = false
		return
	}
	if pressed {
		owner.primaryDown = true
	}
	if text == "" || bounds.W <= 0 || bounds.H <= 0 {
		owner.clear()
		return
	}
	keyboard := pass.activity.Modality == ui.InputModalityKeyboard
	if owner.phase == tabTooltipWaiting && pass.moved && (!hovered || keyboard || owner.primaryDown) {
		owner.clear()
	}
	if owner.phase == tabTooltipVisible {
		card := owner.card.Bounds(c)
		cardHovered := pass.previouslyShown == ownerIndex && tabTooltipContains(card, pass.x, pass.y)
		if keyboard || owner.primaryDown || !tabTooltipContains(owner.bounds, pass.x, pass.y) ||
			pass.pressedOrScrolled && !cardHovered {
			owner.clear()
		}
	}
	if owner.phase == tabTooltipIdle && pass.moved && hovered && !keyboard && !owner.primaryDown {
		owner.phase = tabTooltipWaiting
		// Match the source task's captured builder and prepaint bounds. A
		// still-eligible slot changing text does not restart or replace them.
		owner.text, owner.bounds = text, bounds
		delay := 350 * time.Millisecond
		if close {
			delay = 500 * time.Millisecond
		}
		owner.deadline = pass.now.Add(delay)
	}
	if owner.phase == tabTooltipWaiting {
		if pass.now.Before(owner.deadline) {
			c.After(owner.deadline.Sub(pass.now))
		} else if keyboard || owner.primaryDown || !tabTooltipContains(owner.bounds, pass.x, pass.y) {
			// Press and KeyDown do not cancel WaitingForShow themselves. The
			// original deadline still runs, then its prepaint check rejects it.
			owner.clear()
		} else {
			owner.phase = tabTooltipVisible
			owner.x, owner.y = pass.x, pass.y
		}
	}
	if owner.phase == tabTooltipVisible {
		// Source prepaint checks requests back-to-front. Parent detail may
		// show at350ms; the independently ready close tip wins at500ms.
		pass.visible = owner
	}
}

func tabTooltipMoveHasPrimary(activity ui.InputActivitySnapshot) bool {
	if activity.MoveButtonKnown {
		return activity.MoveButton == 0
	}
	return activity.PrimaryDown
}

func (s *tabTooltipOwner) clear() {
	s.phase, s.text, s.bounds, s.deadline = tabTooltipIdle, "", ui.Rect{}, time.Time{}
}

func tabTooltipContains(r ui.Rect, x, y float32) bool {
	return r.W > 0 && r.H > 0 && x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

func tabTooltipLineHeight(size, scale float32) float32 {
	// GPUI TextStyle's phi() and Window.pixel_snap use f32 arithmetic,
	// rounding half a device pixel toward zero before returning to DIPs.
	pixels := size * float32(1.618034) * scale
	return float32(math.Ceil(float64(pixels-.5))) / scale
}

func (s *State) renderTabTooltip(c *ui.Context, pass tabTooltipPass) {
	owner := pass.visible
	if owner == nil {
		return
	}
	for index, candidate := range s.tabTooltip.owners {
		if candidate == owner {
			s.tabTooltip.shown = index
			break
		}
	}
	p := s.palette()
	// Tooltip width stays in DIPs; text follows the interface caption role.
	width, size := float32(380), design.TextSize(c, design.TextCaption)
	if owner.close {
		width = 320
	}
	ui.Overlay(c, func() {
		ui.Box(c).Key(tabTooltipCardKey{s.scopeID, s.tabTooltip.shown}).Role(ui.RoleTooltip).PassThrough().
			Bind(&owner.card).MaxWidth(width).Padding(6, 9).Radius(6).Border(1, p.Border).
			Background(p.Input.Over(p.Background)).TextColor(p.Muted).Font(tabTooltipFontFamily()).FontWeight(400).
			FontSize(size).FixedLineHeight(tabTooltipLineHeight(size, c.DeviceScale())).AttachAtPoint(owner.x, owner.y, 1).
			Children(func() {
				text := ui.WithGlyphPlacement(ui.Text(c, owner.text), ui.GlyphPlacementWindowsClearTypeQuarter)
				if minWidth := tabTooltipLeafMinWidth(c, owner.text, size, width-20); minWidth > 0 {
					text.FontFeatures(tabTooltipLeafFeatures()).Lang(c.Locale()).MinWidth(minWidth)
				}
			})
	})
}
