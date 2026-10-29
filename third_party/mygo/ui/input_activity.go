package ui

import "github.com/egoist/mygo/internal/platform"

// InputModality identifies the latest modality of input received by this UI
// engine. It does not identify the system's global or physical input modality.
type InputModality uint8

const (
	InputModalityUnknown InputModality = iota
	InputModalityMouse
	InputModalityKeyboard
)

// InputActivitySnapshot is a copy of activity received by one window's UI
// engine after InputActivity or Handle.OwnsPrimaryPress first subscribes,
// before its inspector, widgets or shortcuts route or consume it.
// Events handled earlier by the platform, a native menu, an input method or a
// web view may never reach this engine and are not counted.
//
// Modality changes to Keyboard only for KeyPressed, and to Mouse only for
// PointerMove or PointerDown. Releases, modifier changes and scrolling leave
// it unchanged. Each serial counts received events of that kind, including
// repeated keys and same-position moves; internal hover updates, re-layout,
// generated widget releases and frames do not count as new input.
type InputActivitySnapshot struct {
	Modality InputModality
	// PointerDown reports a held button among the received surface pointer
	// presses. It is independent of whether any widget owns the press. A
	// surface blur clears it without inventing an UpSerial or modality change;
	// it is not a query of physical button state elsewhere in the OS.
	PointerDown bool
	// PrimaryDown reports the primary (button0) part of PointerDown. It lets
	// controls distinguish a primary press from other held pointer buttons.
	PrimaryDown bool
	// MoveButtonKnown marks MoveButton as the last actual PointerMove's
	// reported held-button fact. It is independent of received Down/Up history;
	// check Known before interpreting 0/1/2/3/4/-1 as primary/secondary/middle/
	// first extended/second extended/none. Other platforms may leave it unknown.
	MoveButtonKnown bool
	MoveButton      int
	MoveSerial      uint64
	DownSerial      uint64
	UpSerial        uint64
	ScrollSerial    uint64
	KeyDownSerial   uint64
}

type inputActivityState struct {
	snapshot InputActivitySnapshot
	buttons  uint64
}

// InputActivity returns a plain snapshot of the input received by the UI
// engine since its first subscription. Events before that subscription have
// no history, so the query establishing that subscription returns zero. It
// consumes no event. Reading it subscribes this window to a new frame after
// subsequent counted input or a blur that clears PointerDown, so
// the view can observe activity even when no widget otherwise needs a frame.
// It requests no recurring frame or timer.
func (c *context) InputActivity() InputActivitySnapshot {
	return c.rt.subscribeInputActivity().snapshot
}

func (rt *engine) subscribeInputActivity() *inputActivityState {
	if activity, ok := rt.inputActivity.(*inputActivityState); ok {
		return activity
	}
	activity := &inputActivityState{}
	rt.inputActivity = activity
	rt.inputPressEpoch = 1
	rt.observeInput = func(ev platform.SurfaceEvent) { rt.recordInputActivity(activity, ev) }
	rt.observePrimaryPress = func(target *state, epoch uint64) {
		target.primaryDownEpoch = epoch
	}
	return activity
}

// OwnsPrimaryPress reports whether this handle's committed control received
// primary press ownership and no pointer-button release has arrived since.
// It does not require the pointer to remain inside the control. A later press
// of another button may replace the engine's current press target while this
// historical primary pending fact remains true; any received button release,
// surface blur, known PointerMove without primary held, hidden control or
// closed window makes it false. A known move only invalidates ownership; it
// does not manufacture a received release or alter PrimaryDown's history.
//
// It observes the engine's actual target: a handler that took the press, or
// the interactive target selected by normal routing. It does not mark the
// control interactive, install a handler, consume input or infer ownership
// for ancestors. Like InputActivity it starts a window's opt-in subscription;
// events before that first subscription have no history.
func (r Handle) OwnsPrimaryPress(c *Context) bool {
	rt := c.runtime()
	if rt == nil {
		return false
	}
	if !rt.subscribeInputActivity().snapshot.PrimaryDown {
		return false
	}
	b := (&r).binding(rt, false)
	if b == nil || b.closed || b.id == 0 {
		return false
	}
	target := rt.states[b.id]
	return target != nil && target.rt == rt && target.primaryDownEpoch != 0 && target.primaryDownEpoch == rt.inputPressEpoch
}

func (rt *engine) recordInputActivity(activity *inputActivityState, ev platform.SurfaceEvent) {
	a := &activity.snapshot
	changed := true
	switch ev.Kind {
	case platform.PointerMove:
		a.Modality = InputModalityMouse
		a.MoveSerial++
		a.MoveButton, a.MoveButtonKnown = decodeTaggedMoveButton(ev.Button)
		if a.MoveButtonKnown && a.MoveButton != 0 {
			rt.invalidatePrimaryPress()
		}
	case platform.PointerDown:
		a.Modality = InputModalityMouse
		a.DownSerial++
		// The surface contract's buttons are 0 (primary), 1 (secondary)
		// and 2 (middle). Keep their held state independently; shifting is
		// guarded so malformed backend input cannot panic the UI engine.
		if ev.Button >= 0 && ev.Button < 64 {
			activity.buttons |= uint64(1) << uint(ev.Button)
		}
		a.PointerDown = activity.buttons != 0
		a.PrimaryDown = activity.buttons&1 != 0
	case platform.PointerUp:
		a.UpSerial++
		rt.invalidatePrimaryPress()
		if ev.Button >= 0 && ev.Button < 64 {
			activity.buttons &^= uint64(1) << uint(ev.Button)
		}
		a.PointerDown = activity.buttons != 0
		a.PrimaryDown = activity.buttons&1 != 0
	case platform.PointerScroll:
		a.ScrollSerial++
	case platform.KeyPressed:
		a.Modality = InputModalityKeyboard
		a.KeyDownSerial++
	case platform.SurfaceBlur:
		changed = a.PointerDown
		rt.invalidatePrimaryPress()
		activity.buttons = 0
		a.PointerDown = false
		a.PrimaryDown = false
	default:
		changed = false
	}
	if changed {
		rt.requestFrame()
	}
}

func (rt *engine) invalidatePrimaryPress() {
	rt.inputPressEpoch++
	if rt.inputPressEpoch == 0 {
		// Zero means no recorded primary ownership, including recycled state.
		rt.inputPressEpoch = 1
	}
}

// decodeTaggedMoveButton decodes a Windows raw button mask only for an explicitly
// tagged actual Move. Parsing is opt-in; unused apps only forward the mask.
func decodeTaggedMoveButton(button int) (int, bool) {
	mask := -128 - button
	if mask < 0 || mask & ^0x73 != 0 {
		return button, false
	}
	switch {
	case mask&0x01 != 0:
		return 0, true
	case mask&0x02 != 0:
		return 1, true
	case mask&0x10 != 0:
		return 2, true
	case mask&0x20 != 0:
		return 3, true
	case mask&0x40 != 0:
		return 4, true
	default:
		return -1, true
	}
}
