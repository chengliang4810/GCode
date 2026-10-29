package ui

import (
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func receivedActivity(rt *engine) InputActivitySnapshot {
	if activity, ok := rt.inputActivity.(*inputActivityState); ok {
		return activity.snapshot
	}
	return InputActivitySnapshot{}
}

func TestInputActivityModalityAndSerials(t *testing.T) {
	var seen InputActivitySnapshot
	tt := coreNewTester(func(c *context) {
		seen = c.InputActivity()
		coreBox(c).Size(100, 100)
	}, 200, 200)
	if seen != (InputActivitySnapshot{}) {
		t.Fatalf("initial activity = %+v", seen)
	}
	for _, test := range []struct {
		name string
		act  func()
		want InputActivitySnapshot
	}{
		{"key down", func() { tt.KeyDown(KeyA) }, InputActivitySnapshot{Modality: InputModalityKeyboard, KeyDownSerial: 1}},
		{"key up", func() { tt.KeyUp(KeyA) }, InputActivitySnapshot{Modality: InputModalityKeyboard, KeyDownSerial: 1}},
		{"modifiers", func() { tt.HoldModifiers(Ctrl | Shift) }, InputActivitySnapshot{Modality: InputModalityKeyboard, KeyDownSerial: 1}},
		{"wheel", func() { tt.Wheel(20, 20, 0, 10) }, InputActivitySnapshot{Modality: InputModalityKeyboard, ScrollSerial: 1, KeyDownSerial: 1}},
		{"move restores mouse", func() { tt.Move(20, 20) }, InputActivitySnapshot{Modality: InputModalityMouse, MoveSerial: 1, ScrollSerial: 1, KeyDownSerial: 1}},
		{"same position is still received input", func() { tt.Move(20, 20) }, InputActivitySnapshot{Modality: InputModalityMouse, MoveSerial: 2, ScrollSerial: 1, KeyDownSerial: 1}},
		{"press", func() { tt.Press(20, 20) }, InputActivitySnapshot{Modality: InputModalityMouse, PointerDown: true, PrimaryDown: true, MoveSerial: 2, DownSerial: 1, ScrollSerial: 1, KeyDownSerial: 1}},
		{"key while held", func() { tt.KeyDown(KeyB) }, InputActivitySnapshot{Modality: InputModalityKeyboard, PointerDown: true, PrimaryDown: true, MoveSerial: 2, DownSerial: 1, ScrollSerial: 1, KeyDownSerial: 2}},
		{"release keeps keyboard modality", func() { tt.Release(20, 20) }, InputActivitySnapshot{Modality: InputModalityKeyboard, MoveSerial: 2, DownSerial: 1, UpSerial: 1, ScrollSerial: 1, KeyDownSerial: 2}},
		{"mouse restores again", func() { tt.Move(21, 20) }, InputActivitySnapshot{Modality: InputModalityMouse, MoveSerial: 3, DownSerial: 1, UpSerial: 1, ScrollSerial: 1, KeyDownSerial: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.act()
			if seen != test.want {
				t.Fatalf("activity = %+v, want %+v", seen, test.want)
			}
		})
	}
	before := seen
	tt.Frame()
	tt.Frame()
	tt.Advance(time.Hour)
	if seen != before {
		t.Fatalf("frames or time invented input: %+v -> %+v", before, seen)
	}
}

func TestInputActivityMultipleRawButtonsAndHiddenOwner(t *testing.T) {
	show := true
	var seen InputActivitySnapshot
	tt := coreNewTester(func(c *context) {
		seen = c.InputActivity()
		if show {
			coreButtonBase(c).Size(100, 100)
		}
	}, 200, 200)
	tt.Press(20, 20)
	show = false
	tt.Frame()
	if !seen.PointerDown || !seen.PrimaryDown || seen.DownSerial != 1 || seen.UpSerial != 0 || tt.rt.pressed != nil {
		t.Fatalf("hiding widget changed raw activity or retained widget ownership: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 2, X: 20, Y: 20})
	tt.Release(20, 20)
	if !seen.PointerDown || seen.PrimaryDown || seen.DownSerial != 2 || seen.UpSerial != 1 {
		t.Fatalf("primary release lost the middle button: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 2, X: 20, Y: 20})
	if seen.PointerDown || seen.PrimaryDown || seen.UpSerial != 2 || seen.MoveSerial != 0 {
		t.Fatalf("release did not finish raw hold or internal pointerMove counted as input: %+v", seen)
	}
	// A repeated down is still an event, but does not require two physical
	// releases for the same button. Secondary and primary stay independent.
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 1})
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 1})
	if !seen.PointerDown || seen.PrimaryDown || seen.DownSerial != 4 {
		t.Fatalf("secondary press pretended to hold primary: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 0})
	tt.send(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 1})
	if !seen.PointerDown || !seen.PrimaryDown || seen.DownSerial != 5 || seen.UpSerial != 3 {
		t.Fatalf("secondary release lost primary: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 0})
	if seen.PointerDown || seen.PrimaryDown || seen.UpSerial != 4 {
		t.Fatalf("primary remained held: %+v", seen)
	}
}

func TestInputActivityBlurDoesNotInventReleaseOrModality(t *testing.T) {
	var seen InputActivitySnapshot
	var delivered []InputKind
	tt := coreNewTester(func(c *context) {
		seen = c.InputActivity()
		coreBox(c).Size(100, 100).Focusable().HandleInput(func(ev InputEvent) bool {
			delivered = append(delivered, ev.Kind)
			return true
		})
	}, 200, 200)
	tt.Press(20, 20)
	before := seen
	tt.SetFocused(false)
	if seen.PointerDown || seen.PrimaryDown || seen.UpSerial != before.UpSerial || seen.DownSerial != before.DownSerial || seen.MoveSerial != before.MoveSerial || seen.Modality != before.Modality || !reflect.DeepEqual(delivered, []InputKind{InputPointerMove, InputPointerDown, InputPointerUp}) {
		t.Fatalf("blur forged raw release/modality or changed delivery: activity %+v, delivered %v", seen, delivered)
	}
	cleared := seen
	tt.SetFocused(true)
	tt.Frame()
	if seen != cleared {
		t.Fatalf("focus/frame forged raw activity: %+v -> %+v", cleared, seen)
	}
}

func TestInputActivityStartsAtSubscriptionAndDoesNotPoll(t *testing.T) {
	read := false
	var seen InputActivitySnapshot
	tt := coreNewTester(func(c *context) {
		if read {
			seen = c.InputActivity()
		}
		coreBox(c).Size(100, 100)
	}, 200, 200)
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: 20, Y: 20})
	if tt.FramePending() {
		t.Fatal("unused API requested a frame for an otherwise idle pointer")
	}
	tt.Press(20, 20)
	read = true
	tt.Frame()
	if seen != (InputActivitySnapshot{}) {
		t.Fatalf("first read fabricated pre-subscription history: %+v", seen)
	}
	tt.Release(20, 20)
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: 20, Y: 20})
	if !tt.FramePending() {
		t.Fatal("reading API did not request a frame for same-position activity")
	}
	tt.settle()
	if seen.MoveSerial != 1 || tt.FramePending() {
		t.Fatalf("counted input lost or recurring frame requested: %+v", seen)
	}
	seen.Modality, seen.MoveSerial = InputModalityKeyboard, 999
	tt.Frame()
	if seen.Modality != InputModalityMouse || seen.MoveSerial != 1 {
		t.Fatal("mutating returned snapshot changed engine state")
	}
	tt.Advance(time.Hour)
	if tt.FramePending() || seen.MoveSerial != 1 {
		t.Fatal("activity observation polled or invented input")
	}
}

func TestInputActivitySeesConsumedKeysWithoutChangingRouting(t *testing.T) {
	var seen InputActivitySnapshot
	var delivered []InputKind
	shortcutHits := 0
	tt := coreNewTester(func(c *context) {
		seen = c.InputActivity()
		if c.Shortcut(Super, KeyK) {
			shortcutHits++
		}
		coreBox(c).Size(100, 100).Focusable().AutoFocus().HandleInput(func(ev InputEvent) bool {
			delivered = append(delivered, ev.Kind)
			return true
		})
	}, 200, 200)
	tt.Key(0, KeyA)
	if seen.Modality != InputModalityKeyboard || seen.KeyDownSerial != 1 || !reflect.DeepEqual(delivered, []InputKind{InputKeyDown, InputKeyUp}) {
		t.Fatalf("consumed key changed routing or vanished from activity: %+v %v", seen, delivered)
	}
	delivered = nil
	tt.Key(Super, KeyK)
	if shortcutHits != 1 || seen.KeyDownSerial != 2 || !reflect.DeepEqual(delivered, []InputKind{InputKeyUp}) {
		t.Fatalf("shortcut key changed routing or activity: hits %d activity %+v events %v", shortcutHits, seen, delivered)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyA, Repeat: true})
	if seen.KeyDownSerial != 3 {
		t.Fatalf("repeated received key was not counted once: %+v", seen)
	}
}

func TestInputActivitySeesEditorKeysAndStaysPerWindow(t *testing.T) {
	value := ""
	var editorActivity, otherActivity InputActivitySnapshot
	editor := coreNewTester(func(c *context) {
		editorActivity = c.InputActivity()
		coreTextInput(c, &value).AutoFocus()
	}, 200, 100)
	other := coreNewTester(func(c *context) { otherActivity = c.InputActivity() }, 200, 100)
	editor.TypeKey(0, KeyA, "a")
	other.Frame()
	if value != "a" || editorActivity.Modality != InputModalityKeyboard || editorActivity.KeyDownSerial != 1 || otherActivity != (InputActivitySnapshot{}) {
		t.Fatalf("editor routing/activity/window isolation: value %q editor %+v other %+v", value, editorActivity, otherActivity)
	}
	other.Move(10, 10)
	editor.Frame()
	if otherActivity.Modality != InputModalityMouse || otherActivity.MoveSerial != 1 || editorActivity.MoveSerial != 0 || editorActivity.Modality != InputModalityKeyboard {
		t.Fatalf("window activity leaked: editor %+v other %+v", editorActivity, otherActivity)
	}
}

func TestInputActivityIgnoresNonActivityEvents(t *testing.T) {
	var seen InputActivitySnapshot
	tt := coreNewTester(func(c *context) { seen = c.InputActivity() }, 200, 200)
	tt.KeyDown(KeyA)
	before := seen
	for _, kind := range []platform.SurfaceEventKind{
		platform.KeyReleased, platform.ModifiersChanged, platform.TextInput,
		platform.TextComposition, platform.SurfaceCommand, platform.PointerLeave,
		platform.PointerZoom, platform.SurfaceResize, platform.SurfaceShown,
		platform.SurfaceBlur, platform.SurfaceFocus,
	} {
		tt.send(platform.SurfaceEvent{Kind: kind, Key: platform.KeyA, Scale: 1})
		if seen != before {
			t.Fatalf("event %v invented counted input or changed modality: %+v", kind, seen)
		}
	}
}

func newPrimaryOwnershipTester() (*Tester, *Handle, *Handle) {
	a, b := &Handle{}, &Handle{}
	tt := NewTester(func(c *Context) {
		c.InputActivity()
		Row(c).Children(func() {
			ButtonBase(c).Label("A").Size(80, 80).Bind(a)
			ButtonBase(c).Label("B").Size(80, 80).Bind(b)
		})
	}, 200, 100)
	return tt, a, b
}

func TestOwnsPrimaryPressKeepsPrimaryTargetAcrossOtherButtonDown(t *testing.T) {
	tt, a, b := newPrimaryOwnershipTester()
	// Native input can arrive several times before one build. The middle
	// press replaces rt.pressed; it is not another primary pending owner.
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 0, X: 20, Y: 20})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 2, X: 100, Y: 20})
	if !a.OwnsPrimaryPress(tt.rt.public) || b.OwnsPrimaryPress(tt.rt.public) || tt.rt.pressed != tt.rt.states[b.binding(tt.rt, false).id] {
		t.Fatal("secondary target replaced or impersonated the actual primary target")
	}
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: 190, Y: 90})
	if !a.OwnsPrimaryPress(tt.rt.public) {
		t.Fatal("moving outside discarded primary pending ownership")
	}
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 2, X: 190, Y: 90})
	if a.OwnsPrimaryPress(tt.rt.public) || b.OwnsPrimaryPress(tt.rt.public) || !receivedActivity(tt.rt).PrimaryDown {
		t.Fatal("any-button release failed to invalidate pending while primary remains held")
	}
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 0})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 0, X: 20, Y: 20})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 0, X: 20, Y: 20})
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 0, X: 100, Y: 20})
	if a.OwnsPrimaryPress(tt.rt.public) || !b.OwnsPrimaryPress(tt.rt.public) {
		t.Fatal("down/up/down in one frame did not keep the final actual target")
	}
	tt.Frame()
	tt.Advance(time.Hour)
	if !b.OwnsPrimaryPress(tt.rt.public) || a.OwnsPrimaryPress(tt.rt.public) {
		t.Fatal("rebuilds or cached time changed actual pending ownership")
	}
}

func TestOwnsPrimaryPressStartsItsOwnSubscriptionWithoutHistory(t *testing.T) {
	read := false
	var handle Handle
	owned := false
	tt := NewTester(func(c *Context) {
		if read {
			owned = handle.OwnsPrimaryPress(c)
		}
		ButtonBase(c).Size(80, 80).Bind(&handle)
	}, 200, 100)
	tt.Press(20, 20)
	if tt.rt.observeInput != nil || tt.rt.inputActivity != nil {
		t.Fatal("unused queries recorded input history")
	}
	read = true
	tt.Frame()
	if owned || tt.rt.observeInput == nil || receivedActivity(tt.rt) != (InputActivitySnapshot{}) {
		t.Fatal("OwnsPrimaryPress did not subscribe with an empty initial history")
	}
	tt.Release(20, 20)
	tt.Press(20, 20)
	if !owned || receivedActivity(tt.rt).DownSerial != 1 || receivedActivity(tt.rt).UpSerial != 1 {
		t.Fatal("Handle-only subscription did not observe subsequent actual primary ownership")
	}
	tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: 150, Y: 90})
	if !handle.OwnsPrimaryPress(tt.rt.public) {
		t.Fatal("owner query depended on pointer remaining inside")
	}
}

func TestOwnsPrimaryPressPreservesAncestorAndSiblingRouting(t *testing.T) {
	var ancestor, child, sibling Handle
	var delivered []InputKind
	clicks := 0
	tt := NewTester(func(c *Context) {
		// Querying an unbound plain child must not turn it interactive or
		// install an input handler that steals its ancestor's delivery.
		child.OwnsPrimaryPress(c)
		Row(c).Children(func() {
			Box(c).Size(80, 80).Bind(&ancestor).HandleInput(func(ev InputEvent) bool {
				delivered = append(delivered, ev.Kind)
				return true
			}).Children(func() { Box(c).Fill().Bind(&child) })
			ButtonBase(c).Size(80, 80).Bind(&sibling).OnClick(func() { clicks++ })
		})
	}, 200, 100)
	tt.Press(20, 20)
	if !ancestor.OwnsPrimaryPress(tt.rt.public) || child.OwnsPrimaryPress(tt.rt.public) || sibling.OwnsPrimaryPress(tt.rt.public) || !reflect.DeepEqual(delivered, []InputKind{InputPointerMove, InputPointerDown}) {
		t.Fatalf("queries changed ancestor routing/target: %v", delivered)
	}
	tt.Move(30, 20)
	tt.Release(30, 20)
	if !reflect.DeepEqual(delivered, []InputKind{InputPointerMove, InputPointerDown, InputPointerMove, InputPointerUp}) {
		t.Fatalf("queries changed moves/releases: %v", delivered)
	}
	delivered = nil
	tt.Press(100, 20)
	tt.Release(100, 20)
	if clicks != 1 || len(delivered) != 0 {
		t.Fatalf("sibling routing changed: clicks %d, ancestor %v", clicks, delivered)
	}
}

func TestOwnsPrimaryPressHiddenReboundAndWindowIsolation(t *testing.T) {
	show, replacement := true, false
	var handle Handle
	view := func(c *Context) {
		c.InputActivity()
		if show {
			key := "original"
			if replacement {
				key = "replacement"
			}
			ButtonBase(c.Key(key)).Size(80, 80).Bind(&handle)
		}
	}
	a, b := NewTester(view, 200, 100), NewTester(view, 200, 100)
	a.Press(20, 20)
	if !handle.OwnsPrimaryPress(a.rt.public) || handle.OwnsPrimaryPress(b.rt.public) {
		t.Fatal("primary pending leaked across a shared handle's windows")
	}
	replacement = true
	a.Frame()
	if handle.OwnsPrimaryPress(a.rt.public) {
		t.Fatal("rebinding revived another control's primary pending")
	}
	a.Release(20, 20)
	a.Press(20, 20)
	if !handle.OwnsPrimaryPress(a.rt.public) {
		t.Fatal("rebound control could not acquire its own primary press")
	}
	show = false
	a.Frame()
	show = true
	a.Frame()
	if handle.OwnsPrimaryPress(a.rt.public) {
		t.Fatal("hidden/reappearing control revived old primary pending")
	}
	a.Release(20, 20)
	a.Press(20, 20)
	public := a.rt.public
	a.rt.close()
	if handle.OwnsPrimaryPress(public) || handle.OwnsPrimaryPress(nil) {
		t.Fatal("closed or nil context retained ownership")
	}
}

func TestOwnsPrimaryPressBlurCannotRevivePriorOwner(t *testing.T) {
	tt, a, b := newPrimaryOwnershipTester()
	tt.Press(20, 20)
	beforeUp := receivedActivity(tt.rt).UpSerial
	tt.SetFocused(false)
	tt.SetFocused(true)
	tt.Press(100, 20)
	if a.OwnsPrimaryPress(tt.rt.public) || !b.OwnsPrimaryPress(tt.rt.public) || receivedActivity(tt.rt).UpSerial != beforeUp {
		t.Fatal("blur/new primary revived old pending or forged a public UpSerial")
	}
}

func TestOwnsPrimaryPressOmittedKeptPageCannotRevive(t *testing.T) {
	router := NewRouter("/a")
	router.Transition = TransitionNone
	var a, b Handle
	tt := NewTester(func(c *Context) {
		c.InputActivity()
		router.View(c, func(route *Route) {
			if route.Path() == "/a" {
				ButtonBase(c).Size(80, 80).Bind(&a)
			} else {
				ButtonBase(c).Size(80, 80).Bind(&b)
			}
		})
	}, 200, 100)
	tt.Press(20, 20)
	oldID := a.binding(tt.rt, false).id
	router.Push("/b")
	tt.Frame()
	if tt.rt.states[oldID] == nil {
		t.Fatal("test did not retain an omitted page's state")
	}
	if a.OwnsPrimaryPress(tt.rt.public) {
		t.Fatal("omitted kept-alive control retained pending ownership")
	}
	router.Back()
	tt.Frame()
	if a.OwnsPrimaryPress(tt.rt.public) {
		t.Fatal("returning to kept-alive page revived old pending")
	}
}

func TestOwnsPrimaryPressNestedReleaseBeforeTargetDoesNotRevive(t *testing.T) {
	for _, when := range []string{"move before down", "taking down"} {
		t.Run(when, func(t *testing.T) {
			var handle Handle
			var tt *Tester
			armed := false
			fired := false
			tt = NewTester(func(c *Context) {
				c.InputActivity()
				Box(c).Size(80, 80).Focusable().Bind(&handle).HandleInput(func(ev InputEvent) bool {
					trigger := when == "move before down" && ev.Kind == InputPointerMove || when == "taking down" && ev.Kind == InputPointerDown
					if armed && !fired && trigger {
						fired = true
						tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerUp, Button: 2, X: 20, Y: 20})
					}
					return true
				})
			}, 200, 100)
			armed = true
			tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 0, X: 20, Y: 20})
			if !fired || !receivedActivity(tt.rt).PrimaryDown || receivedActivity(tt.rt).UpSerial != 1 || handle.OwnsPrimaryPress(tt.rt.public) {
				t.Fatalf("nested release revived primary pending: fired %v activity %+v", fired, receivedActivity(tt.rt))
			}
		})
	}
}

func TestInputActivityMoveButtonIsIndependentOfPressHistory(t *testing.T) {
	var seen InputActivitySnapshot
	tt := coreNewTester(func(c *context) {
		seen = c.InputActivity()
		coreBox(c).Size(100, 100)
	}, 200, 100)
	if seen.MoveButtonKnown {
		t.Fatal("initial move invented a native button observation")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: knownMoveButtonForTest(0, true), X: 20, Y: 20})
	if !seen.MoveButtonKnown || seen.MoveButton != 0 || seen.MoveSerial != 1 || seen.PrimaryDown || seen.PointerDown || seen.DownSerial != 0 || seen.UpSerial != 0 {
		t.Fatalf("known held move invented press history: %+v", seen)
	}
	tt.Press(20, 20)
	if !seen.PrimaryDown || seen.DownSerial != 1 || seen.MoveSerial != 1 || !seen.MoveButtonKnown || seen.MoveButton != 0 {
		t.Fatalf("received down changed the last actual Move fact: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: knownMoveButtonForTest(-1, true), X: 20, Y: 20})
	if !seen.MoveButtonKnown || seen.MoveButton != -1 || !seen.PrimaryDown || seen.DownSerial != 1 || seen.UpSerial != 0 {
		t.Fatalf("known empty move forged an Up or changed received-held history: %+v", seen)
	}
	tt.SetFocused(false)
	tt.SetFocused(true)
	tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: knownMoveButtonForTest(0, true), X: 20, Y: 20})
	if !seen.MoveButtonKnown || seen.MoveButton != 0 || seen.PrimaryDown || seen.PointerDown || seen.DownSerial != 1 || seen.UpSerial != 0 {
		t.Fatalf("post-blur known-held move was confused with historical press state: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: 2, X: 21, Y: 20})
	if seen.MoveButtonKnown || seen.PrimaryDown || seen.DownSerial != 1 || seen.UpSerial != 0 {
		t.Fatalf("unknown move retained a prior Known fact or invented press history: %+v", seen)
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: knownMoveButtonForTest(4, true), X: 22, Y: 20})
	moveSerial := seen.MoveSerial
	tt.Key(0, KeyA)
	tt.Wheel(22, 20, 0, 1)
	tt.Release(22, 20)
	if !seen.MoveButtonKnown || seen.MoveButton != 4 || seen.MoveSerial != moveSerial || seen.UpSerial != 1 || seen.DownSerial != 1 {
		t.Fatalf("non-Move activity replaced the latest actual Move fact: %+v", seen)
	}
}

func TestOwnsPrimaryPressKnownMoveWithoutPrimaryInvalidatesPending(t *testing.T) {
	for _, test := range []struct {
		name   string
		button int
	}{{"none", -1}, {"secondary", 1}, {"middle", 2}, {"first extended", 3}, {"second extended", 4}} {
		t.Run(test.name, func(t *testing.T) {
			tt, a, b := newPrimaryOwnershipTester()
			tt.Press(20, 20)
			tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: knownMoveButtonForTest(test.button, true), X: 20, Y: 20})
			activity := receivedActivity(tt.rt)
			if a.OwnsPrimaryPress(tt.rt.public) || !activity.PrimaryDown || activity.UpSerial != 0 || activity.DownSerial != 1 {
				t.Fatalf("known non-primary move did not invalidate only pending: %+v", activity)
			}
			tt.Frame()
			if a.OwnsPrimaryPress(tt.rt.public) {
				t.Fatal("an extra build revived stale primary pending")
			}
			tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerDown, Button: 0, X: 100, Y: 20})
			if a.OwnsPrimaryPress(tt.rt.public) || !b.OwnsPrimaryPress(tt.rt.public) {
				t.Fatal("new primary press revived old owner or lost its own target")
			}
		})
	}
	for _, known := range []bool{false, true} {
		tt, a, _ := newPrimaryOwnershipTester()
		tt.Press(20, 20)
		button := -1
		if known {
			button = 0
		}
		tt.send(platform.SurfaceEvent{Kind: platform.PointerMove, Button: knownMoveButtonForTest(button, known), X: 20, Y: 20})
		if !a.OwnsPrimaryPress(tt.rt.public) {
			t.Fatal("unknown move or known primary-held move incorrectly cancelled pending")
		}
	}
}

// Only the private SurfaceEvent representation changes; the public activity
// assertions above keep the original known/unknown, ownership and serial rules.
func knownMoveButtonForTest(button int, known bool) int {
	if !known {
		return button
	}
	masks := map[int]int{-1: 0, 0: 1, 1: 2, 2: 16, 3: 32, 4: 64}
	return -128 - masks[button]
}

func TestInputActivityTaggedMoveMaskAllCombinations(t *testing.T) {
	bits := []int{1, 2, 16, 32, 64}
	for subset := 0; subset < 32; subset++ {
		mask, want := 0, -1
		for i, bit := range bits {
			if subset&(1<<i) != 0 {
				mask |= bit
				if want == -1 {
					want = i
				}
			}
		}
		for _, modifiers := range []int{0, 4, 8, 12} {
			tagged := -128 - ((mask | modifiers) & 0x73)
			got, known := decodeTaggedMoveButton(tagged)
			if !known || got != want {
				t.Fatalf("mask=%#x: got %d/%v want %d", mask|modifiers, got, known, want)
			}
		}
	}
	for _, button := range []int{-2, -1, 0, 1, 2, 3, 4, 64, 69, 256, -128 - 4, -128 - 8, -128 - 0x74} {
		got, known := decodeTaggedMoveButton(button)
		if known || got != button {
			t.Fatalf("ordinary or malformed %d became known %d/%v", button, got, known)
		}
	}
}
