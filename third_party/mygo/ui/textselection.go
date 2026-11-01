package ui

import (
	"runtime"
	"strings"
	"unicode/utf8"
)

// textPoint identifies a rune of a paragraph by its stable element ID.
type textPoint struct {
	id uint64
	at int
}

// textSpan is one paragraph's slice of a selection, with the text it held
// when the selection took it. A virtualized list builds only the rows near
// its view, so a selection that reached across more keeps its spans: copying
// and painting still have the text of the paragraphs it scrolled away.
type textSpan struct {
	id       uint64
	from, to int
	text     string
}

// selected returns the runes of the span's text that it holds.
func (s textSpan) selected() string {
	runes := []rune(s.text)
	from := min(max(s.from, 0), len(runes))
	to := min(max(s.to, from), len(runes))
	return string(runes[from:to])
}

// textSelection is one selection across the paragraphs of a container.
// Each paragraph keeps its own layout and editor; syncTextSelection projects
// the shared endpoints onto those editors for painting.
type textSelection struct {
	scope         uint64
	anchor, caret textPoint
	start         [2]textPoint
	unit          int
	dragging      bool
	desiredX      float32
	hasDesired    bool
	// spans is the selection resolved into paragraphs, in document order,
	// with each paragraph's text kept. The endpoints alone cannot paint or
	// copy a selection whose paragraphs a list has scrolled out of its
	// build, so the spans are what the frame works from.
	spans []textSpan
	// forward is which side of the spans the caret is on, settled while the
	// anchor was still built. Once the anchor is not, it says which end a
	// drag extends, as extendTextSpans does.
	forward, hasForward bool
}

// prepareSelectable waits until inline children have finished their text,
// so rebuilding a paragraph does not reset its selection to its own text.
func (rt *engine) prepareSelectable(e *node) {
	e.prepareSelectable()
	for ch := e.first; ch != nil; ch = ch.next {
		rt.prepareSelectable(ch)
	}
}

func (e *node) prepareSelectable() {
	if e.kind != kindText {
		return
	}
	if e.isInline() {
		e.st.textScope = 0
		return
	}
	scope := uint64(0)
	if e.flags&(flagClickable|flagFocusable|flagUnselectable) == 0 && e.inputFn == nil {
		for p := e.parent; p != nil; p = p.parent {
			if p.kind == kindBox && p.flags&flagSelectable != 0 {
				scope = p.id
				break
			}
			if p.flags&(flagClickable|flagFocusable|flagEditable|flagUnselectable) != 0 || p.inputFn != nil {
				break // a control owns its text
			}
		}
	}
	if old := e.st.textScope; old != 0 && old != scope {
		if e.c.rt.selection.scope == old && e.c.rt.textSelectionIncludes(e.id) {
			e.c.rt.selection = textSelection{}
		}
		if ed := e.st.editor; ed != nil {
			ed.anchor, ed.caret = 0, 0
		}
	}
	e.st.textScope = scope
	if scope != 0 {
		e.flags |= flagSelectable
	}
	if e.flags&flagSelectable == 0 {
		return
	}
	ed := e.st.editor
	if ed == nil {
		ed = newEditor(e.c.rt.now)
		ed.readOnly, ed.multiline = true, true
		e.st.editor = ed
	}
	if ed.source != e.text || len(ed.buf.paras) == 0 {
		// Text only added at the end, as a reply being written, keeps
		// the selection and its offsets.
		grew := len(ed.buf.paras) > 0 && strings.HasPrefix(e.text, ed.source)
		// A paragraph the list stopped building comes back as a new state:
		// the spans the selection kept hold its text, and the same text
		// rebuilt is not a change that invalidates them.
		changed := ed.source != "" && ed.source != e.text
		if changed && !grew && scope != 0 && e.c.rt.selection.scope == scope && e.c.rt.textSelectionIncludes(e.id) {
			e.c.rt.selection = textSelection{}
		}
		caret, anchor := ed.caret, ed.anchor
		ed.source = e.text
		ed.setText(e.text)
		ed.caret, ed.anchor = 0, 0
		if grew {
			ed.caret, ed.anchor = caret, anchor
			e.c.rt.refreshTextSpan(e.id, e.text)
		}
	}
	if e.c.rt.focused == e.id || len(ed.queue) > 0 {
		ed.process(e.c, e)
	}
}

// collectSelectable follows build order, independently of painting order
// (absolute children paint last). Inline children are already in their
// paragraph's text, and must not be copied a second time.
func (rt *engine) collectSelectable(e *node, hidden bool) {
	hidden = hidden || e.flags&(flagInvisible|flagInert) != 0 || e.hidden
	if !hidden && e.kind == kindText && !e.isInline() && e.st.flags&flagDisabled == 0 && e.flags&flagSelectable != 0 {
		rt.texts = append(rt.texts, e.st)
	}
	for ch := e.first; ch != nil; ch = ch.next {
		rt.collectSelectable(ch, hidden)
	}
}

func (rt *engine) textPointIndex(p textPoint) int {
	for i, s := range rt.texts {
		if s.id == p.id && s.textScope == rt.selection.scope {
			return i
		}
	}
	return -1
}

func (rt *engine) compareTextPoints(a, b textPoint) int {
	i, j := rt.textPointIndex(a), rt.textPointIndex(b)
	if i != j {
		return i - j
	}
	return a.at - b.at
}

// refreshTextSpan keeps a span's text with the paragraph it holds, as a
// reply being written grows under it.
func (rt *engine) refreshTextSpan(id uint64, text string) {
	for i := range rt.selection.spans {
		if rt.selection.spans[i].id != id {
			continue
		}
		span := &rt.selection.spans[i]
		if span.to >= utf8.RuneCountInString(span.text) {
			span.to = utf8.RuneCountInString(text)
		}
		span.text = text
		return
	}
}

// textSpanAt returns the span holding paragraph id, if the selection has one.
func (rt *engine) textSpanAt(id uint64) (textSpan, bool) {
	for _, span := range rt.selection.spans {
		if span.id == id {
			return span, true
		}
	}
	return textSpan{}, false
}

// textSelectionSelected reports whether the selection holds any text. A
// click that never moved selects nothing, as does one whose whole range is
// an empty paragraph.
func (rt *engine) textSelectionSelected() bool {
	for _, span := range rt.selection.spans {
		if span.to > span.from {
			return true
		}
	}
	return false
}

// textSelectionHasPoint reports whether p lies inside the selection. A
// span's paragraph need not be built: the spans keep what a list scrolled
// out of its build.
func (rt *engine) textSelectionHasPoint(p textPoint) bool {
	span, ok := rt.textSpanAt(p.id)
	return ok && p.at >= span.from && p.at <= span.to
}

func (rt *engine) textSelectionIncludes(id uint64) bool {
	_, ok := rt.textSpanAt(id)
	return ok
}

// resolveTextSpans returns the selection between a and b as one span per
// paragraph in document order, with the text each paragraph held. Empty
// paragraphs strictly between the endpoints keep their line, as the copy
// joins every span with a newline.
func (rt *engine) resolveTextSpans(a, b textPoint) []textSpan {
	i, j := rt.textPointIndex(a), rt.textPointIndex(b)
	if i < 0 || j < 0 {
		return nil
	}
	if i > j {
		a, b, i, j = b, a, j, i
	}
	var spans []textSpan
	for k := i; k <= j; k++ {
		p := rt.texts[k]
		if p.textScope != rt.selection.scope || p.editor == nil {
			continue
		}
		from, to := 0, p.editor.buf.n
		if k == i {
			from = min(a.at, to)
		}
		if k == j {
			to = min(b.at, to)
		}
		from = min(max(from, 0), to)
		spans = append(spans, textSpan{id: p.id, from: from, to: to, text: p.editor.source})
	}
	return spans
}

// updateTextSpans resolves the selection's spans against this frame's built
// paragraphs. While both endpoints are in view, everything between them was
// built too, so the range is resolved whole: that is what lets a drag
// contract or reverse. Once a list scrolls an endpoint out of its build, the
// spans resolved earlier are what keeps the selection, and a live drag
// extends them through extendTextSpans.
func (rt *engine) updateTextSpans() {
	t := &rt.selection
	ai, ci := rt.textPointIndex(t.anchor), rt.textPointIndex(t.caret)
	if ai >= 0 && ci >= 0 && textInView(rt.texts[ai]) && textInView(rt.texts[ci]) {
		if t.dragging {
			t.forward, t.hasForward = ai <= ci, true
		}
		t.spans = rt.resolveTextSpans(t.anchor, t.caret)
		return
	}
	if t.dragging && t.hasForward {
		rt.extendTextSpans()
	}
}

// textInView reports whether a paragraph's row showed it in this frame. The
// rows a list builds beyond its view are laid out but clipped out, and the
// paragraphs between two paragraphs in view are always built.
func textInView(s *state) bool {
	return s.vw > 0 && s.vh > 0
}

// extendTextSpans carries a selection whose anchor a list scrolled out of
// its build to where the drag has reached now. The paragraphs still built on
// both sides are where the spans and the build overlap: the spans before
// them keep the text they hold, and the rest resolves up to the caret.
func (rt *engine) extendTextSpans() {
	t := &rt.selection
	built := make(map[uint64]bool, len(rt.texts))
	for _, s := range rt.texts {
		if s.textScope == t.scope {
			built[s.id] = true
		}
	}
	if t.forward {
		for k, span := range t.spans {
			if !built[span.id] {
				continue
			}
			rest := rt.resolveTextSpans(textPoint{span.id, span.from}, t.caret)
			if rest == nil {
				return
			}
			merged := make([]textSpan, 0, k+len(rest))
			t.spans = append(append(merged, t.spans[:k]...), rest...)
			return
		}
		return
	}
	for k := len(t.spans) - 1; k >= 0; k-- {
		span := t.spans[k]
		if !built[span.id] {
			continue
		}
		rest := rt.resolveTextSpans(t.caret, textPoint{span.id, span.to})
		if rest == nil {
			return
		}
		merged := make([]textSpan, 0, len(rest)+len(t.spans)-k-1)
		t.spans = append(append(merged, rest...), t.spans[k+1:]...)
		return
	}
}

func (rt *engine) textSelectionVisible(s *state) bool {
	if !rt.windowFocus() || s.flags&flagDisabled != 0 {
		return false
	}
	if s.textScope == 0 {
		return rt.focused == s.id
	}
	f := rt.states[rt.focused]
	return s.textScope == rt.selection.scope && f != nil && f.textScope == s.textScope && f.flags&flagDisabled == 0
}

func (rt *engine) syncTextSelection() {
	for _, s := range rt.texts {
		if s.textScope != 0 {
			s.editor.anchor, s.editor.caret = 0, 0
		}
	}
	if rt.selection.scope == 0 {
		return
	}
	if rt.states[rt.selection.scope] == nil {
		// The container itself is gone: never let a recycled state inherit
		// the selection.
		rt.selection = textSelection{}
		return
	}
	if rt.selection.dragging && rt.pressed != nil {
		rt.moveTextSelection(rt.pointerX, rt.pointerY)
	}
	rt.updateTextSpans()
	// Project the spans onto the paragraphs built this frame. Both are in
	// document order, so one walk of each pairs them; a spanned paragraph
	// that a list no longer builds kept its text in its span, and paints
	// again when the list builds it back.
	at := 0
	for _, span := range rt.selection.spans {
		for at < len(rt.texts) {
			s := rt.texts[at]
			at++
			if s.textScope != rt.selection.scope || s.editor == nil || s.id != span.id {
				// A paragraph before the span, or an empty one the span
				// left out from between two others.
				continue
			}
			s.editor.anchor = min(span.from, s.editor.buf.n)
			s.editor.caret = min(span.to, s.editor.buf.n)
			break
		}
	}
}

// resolveTextSelectionNow resolves the selection against the paragraphs
// built now, without waiting for the frame that follows.
func (rt *engine) resolveTextSelectionNow() {
	t := &rt.selection
	if rt.textPointIndex(t.anchor) < 0 || rt.textPointIndex(t.caret) < 0 {
		return
	}
	t.spans = rt.resolveTextSpans(t.anchor, t.caret)
}

// textSelectionAt finds the nearest visible paragraph in scope. Gaps,
// padding, and a pointer dragged outside the container still have an
// endpoint; the editor's hit test clamps it to that paragraph's text.
func (rt *engine) textSelectionAt(scope uint64, x, y float32) textPoint {
	// Respect the topmost hit, including a link inside a rich paragraph.
	for _, id := range rt.hitChain(x, y) {
		if s := rt.states[id]; s != nil && s.textScope == scope && s.editor != nil && s.flags&flagDisabled == 0 {
			return textHit(s, x, y)
		}
	}
	var nearest *state
	distance := inf
	for _, s := range rt.texts {
		if s.textScope != scope || s.vw <= 0 || s.vh <= 0 || !s.editor.laidOut() {
			continue
		}
		for _, line := range s.editor.layout.Lines {
			r := intersect(transformRect(s.world, Rect{s.x + s.editor.originX + line.X, s.y + s.editor.originY + line.Y, line.Width, line.Height}), Rect{s.vx, s.vy, s.vw, s.vh})
			if r.W <= 0 || r.H <= 0 {
				continue
			}
			dx, dy := max(r.X-x, x-r.X-r.W, 0), max(r.Y-y, y-r.Y-r.H, 0)
			if d := dx*dx + dy*dy; nearest == nil || d < distance {
				nearest, distance = s, d
			}
		}
	}
	if nearest == nil {
		return textPoint{}
	}
	return textHit(nearest, x, y)
}

func textHit(s *state, x, y float32) textPoint {
	ed := s.editor
	lx, ly := s.local(x, y)
	at := ed.hit(lx, ly)
	if ly < ed.originY {
		at = 0
	} else if ly >= ed.originY+ed.layout.Height {
		at = ed.buf.n
	}
	return textPoint{s.id, at}
}

// scrollTextSelection follows a drag near a scroll container's edges. The
// captured paragraph identifies the scroll container even outside its clip;
// after layout, syncTextSelection hit-tests the newly visible text.
func (rt *engine) scrollTextSelection() {
	if !rt.selection.dragging || rt.pressed == nil {
		return
	}
	const edge = 32
	for s := rt.pressed; s != nil; s = rt.states[s.parent] {
		if s.flags&flagScrollY == 0 {
			continue
		}
		var dy float32
		_, y := s.local(rt.pointerX, rt.pointerY)
		switch top, bottom := y, s.h-y; {
		case top < edge:
			dy = -min(edge-top, edge) / 2
		case bottom < edge:
			dy = min(edge-bottom, edge) / 2
		}
		if dy != 0 && scrollBy(s, 0, dy) {
			rt.animating = true
		}
		return
	}
}

func textUnit(s *state, at, unit int) (int, int) {
	switch unit {
	case 2:
		return s.editor.buf.wordAt(at)
	case 3:
		return s.editor.lineEdges(at)
	}
	return at, at
}

func (rt *engine) pressTextSelection(s *state, x, y float32, mods Modifiers, clicks int) bool {
	if s.flags&flagDisabled != 0 {
		return false
	}
	scope := s.textScope
	if s.editor == nil && s.flags&flagSelectable != 0 {
		scope = s.id
	}
	if scope == 0 {
		return false
	}
	for _, id := range rt.hitChain(x, y) {
		if id == scope {
			break
		}
		if p := rt.states[id]; p != nil && p.flags&flagUnselectable != 0 {
			return false
		}
	}
	p := rt.textSelectionAt(scope, x, y)
	if p.id == 0 {
		return false
	}
	t := &rt.selection
	if t.scope != scope || mods&Shift == 0 || rt.textPointIndex(t.anchor) < 0 {
		*t = textSelection{scope: scope, anchor: p, caret: p}
	}
	t.caret, t.unit, t.dragging, t.hasDesired = p, min(clicks, 3), true, false
	if t.unit > 1 {
		a, b := textUnit(rt.states[p.id], p.at, t.unit)
		t.anchor, t.caret = textPoint{p.id, a}, textPoint{p.id, b}
	}
	t.start = [2]textPoint{t.anchor, t.caret}
	rt.resolveTextSelectionNow()
	// A press in padding starts in the nearest paragraph, which keeps the
	// focus and pointer capture until the release.
	s.pressed = false
	s = rt.states[p.id]
	rt.focused, rt.focusVisible = s.id, false
	rt.pressed = s
	s.pressed, s.pressMods = true, mods
	s.editor.dragging = false
	return true
}

// dragInlineLinkSelection hands an inline link's moved press to its selectable
// paragraph. Until it crosses the normal drag threshold the link keeps click,
// keyboard focus and context-menu behavior. Only a RoleLink nested in text
// qualifies: ordinary buttons and other interactive inline controls keep their
// own gestures.
func (rt *engine) dragInlineLinkSelection() {
	link := rt.pressed
	if link == nil || rt.pressButton != 0 || link.role != RoleLink || !link.drag.far {
		return
	}
	var paragraph *state
	for p := rt.states[link.parent]; p != nil; p = rt.states[p.parent] {
		if p.editor != nil {
			if p.flags&(flagSelectable|flagDisabled|flagEditable) == flagSelectable && p.editor.readOnly {
				paragraph = p
			}
			break
		}
		if p.flags&(flagClickable|flagEditable|flagUnselectable) != 0 || p.input != nil {
			break
		}
	}
	if paragraph == nil {
		return
	}
	originX, originY := link.drag.ox, link.drag.oy
	link.pressed = false
	paragraph.pressed, paragraph.pressMods = true, link.pressMods
	paragraph.drag = link.drag
	rt.pressed = paragraph
	if !rt.pressTextSelection(paragraph, originX, originY, link.pressMods, rt.pressClicks) {
		rt.focused, rt.focusVisible = paragraph.id, false
		paragraph.editor.pressMods = link.pressMods
		x, y := paragraph.local(originX, originY)
		paragraph.editor.press(x, y, rt.pressClicks, 0)
	}
	rt.requestFrame()
}

func (rt *engine) moveTextSelection(x, y float32) {
	t := &rt.selection
	p := rt.textSelectionAt(t.scope, x, y)
	if p.id == 0 {
		return
	}
	if t.unit > 1 {
		a, b := textUnit(rt.states[p.id], p.at, t.unit)
		if rt.compareTextPoints(p, t.start[0]) < 0 {
			t.anchor, t.caret = t.start[1], textPoint{p.id, a}
		} else {
			t.anchor, t.caret = t.start[0], textPoint{p.id, b}
			if rt.compareTextPoints(t.caret, t.start[1]) < 0 {
				t.caret = t.start[1]
			}
		}
	} else {
		t.caret = p
	}
}

func (rt *engine) textSelectionCommand(s *state, name string) bool {
	if s.textScope == 0 {
		return false
	}
	if s.flags&flagDisabled != 0 {
		return true
	}
	switch name {
	case "selectAll":
		var first, last *state
		for _, p := range rt.texts {
			if p.textScope == s.textScope {
				if first == nil {
					first = p
				}
				last = p
			}
		}
		if first != nil {
			rt.selection = textSelection{scope: s.textScope, anchor: textPoint{first.id, 0}, caret: textPoint{last.id, last.editor.buf.n}}
			rt.resolveTextSelectionNow()
		}
	case "copy":
		// The keys may have moved the endpoints since the last frame: resolve
		// them now, so copying is what the user just selected. A caret that
		// never moved copies nothing, as a paragraph's blank line between two
		// others still copies its newline.
		rt.resolveTextSelectionNow()
		if rt.selection.scope != s.textScope || rt.selection.anchor == rt.selection.caret {
			break
		}
		// The spans hold the text, so copying still has the paragraphs a
		// list built for the selection and then scrolled out of its build.
		if sc := rt.states[s.textScope]; sc != nil && sc.copyData != nil {
			rt.host.writeClipboardData(sc.copyData(rt.selectedTexts(s.textScope)))
			break
		}
		var out strings.Builder
		for i, span := range rt.selection.spans {
			if i > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(span.selected())
		}
		rt.host.writeClipboard(out.String())
	}
	return true // other editing commands cannot change selectable text
}

// selectedTexts returns the parts of paragraphs the selection of
// container scope holds.
func (rt *engine) selectedTexts(scope uint64) []SelectedText {
	if rt.selection.scope != scope {
		return nil
	}
	parts := make([]SelectedText, 0, len(rt.selection.spans))
	for _, span := range rt.selection.spans {
		parts = append(parts, SelectedText{ID: span.id, Text: span.text, Start: span.from, End: span.to})
	}
	return parts
}

func (rt *engine) neighboringText(p textPoint, direction int) *state {
	for i := rt.textPointIndex(p) + direction; i >= 0 && i < len(rt.texts); i += direction {
		if s := rt.texts[i]; s.textScope == rt.selection.scope {
			return s
		}
	}
	return nil
}

func (rt *engine) textSelectionKey(s *state, k keyEvent) bool {
	if s.textScope == 0 {
		return false
	}
	if s.flags&flagDisabled != 0 {
		return true
	}
	if k.mods == Cmd && (k.key == KeyA || k.key == KeyC) {
		name := "copy"
		if k.key == KeyA {
			name = "selectAll"
		}
		return rt.textSelectionCommand(s, name)
	}
	t := &rt.selection
	if t.scope != s.textScope || rt.textPointIndex(t.caret) < 0 {
		p := textPoint{s.id, s.editor.caret}
		*t = textSelection{scope: s.textScope, anchor: p, caret: p}
	}
	s = rt.states[t.caret.id]
	ed, at := s.editor, t.caret.at
	mods := k.mods &^ Shift
	if (runtime.GOOS == "darwin" && mods == Cmd && (k.key == KeyUp || k.key == KeyDown)) ||
		(mods == Ctrl && (k.key == KeyHome || k.key == KeyEnd)) {
		direction := 1
		if k.key == KeyUp || k.key == KeyHome {
			direction = -1
		}
		for next := rt.neighboringText(textPoint{s.id, 0}, direction); next != nil; next = rt.neighboringText(textPoint{s.id, 0}, direction) {
			s = next
		}
		at = s.editor.buf.n
		if direction < 0 {
			at = 0
		}
		t.caret, t.hasDesired = textPoint{s.id, at}, false
		return true
	}
	if k.key == KeyUp || k.key == KeyDown {
		x, y, h := ed.layout.Caret(at)
		if !t.hasDesired {
			t.desiredX, t.hasDesired = s.x+ed.originX+x, true
		}
		direction := 1
		if k.key == KeyUp {
			direction = -1
		}
		y += h/2 + float32(direction)*h
		if y < 0 || y >= ed.layout.Height {
			if next := rt.neighboringText(t.caret, direction); next != nil {
				s, ed = next, next.editor
				y = 0
				if direction < 0 {
					y = ed.layout.Height - 1
				}
			} else {
				at = ed.buf.n
				if direction < 0 {
					at = 0
				}
				t.caret = textPoint{s.id, at}
				return true
			}
		}
		t.caret = textPoint{s.id, ed.layout.IndexAt(t.desiredX-s.x-ed.originX, y)}
		return true
	}
	t.hasDesired = false
	if (k.key == KeyLeft && at == 0 || k.key == KeyRight && at == ed.buf.n) && !(runtime.GOOS == "darwin" && mods == Cmd) {
		direction := 1
		if k.key == KeyLeft {
			direction = -1
		}
		if next := rt.neighboringText(t.caret, direction); next != nil {
			at = 0
			if direction < 0 {
				at = next.editor.buf.n
			}
			t.caret = textPoint{next.id, at}
			return true
		}
	}
	ed.caret = at
	ed.key(&rt.c, s, editEvent{kind: editKey, mods: k.mods, key: k.key})
	t.caret = textPoint{s.id, ed.caret}
	return true
}

func (rt *engine) textSelectionMenu(s *state, x, y float32) bool {
	if s.textScope == 0 {
		return false
	}
	p := textHit(s, x, y)
	if !rt.textSelectionVisible(s) || !rt.textSelectionHasPoint(p) {
		rt.selection = textSelection{scope: s.textScope, anchor: p, caret: p}
		rt.resolveTextSelectionNow()
	}
	rt.focused, rt.focusVisible = s.id, false
	return true
}
