package desktop

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Frozen composer.rs geometry. The dock's textarea floor is 16px shorter
// than the hero's (DockLayout::thread_height); both share the same upper cap.
const (
	composerCompactHeight = float32(49)
	composerActionsHeight = float32(42)
	composerFooterHeight  = float32(24)
	composerResizeSettle  = 150 * time.Millisecond
)

func composerFlip(expanded bool, textWidth, capacity float32, newline, resizing bool) bool {
	if newline || capacity < 200 {
		return true
	}
	if expanded {
		return resizing || textWidth >= capacity-32
	}
	return textWidth > capacity
}

func composerOuterWidth(panel, transcript float32, hero bool) float32 {
	if hero {
		return min(max(0, panel), 768)
	}
	return min(max(0, panel), max(0, transcript)+32)
}

// These widths stay independent of the committed mode. Reading an expanded
// input's larger viewport as compact capacity would otherwise oscillate.
func composerCompactCapacity(surface, controls float32) float32 {
	return max(0, surface-2-8-28-16-controls-8-28-8-8)
}

type composerLayoutState struct {
	expanded                bool
	measured                bool
	capacity                float32
	widthChangedAt          time.Time
	controls                ui.Handle
	lastWidth               float32
	lastWidthHint           float32
	lastDraft, lastControls string
	displayText             string
	committedText           string
	hasDisplayText          bool
}

func (s *composerLayoutState) update(now time.Time, textWidth, capacity float32, newline bool) (bool, time.Duration) {
	if s.measured && math.Abs(float64(capacity-s.capacity)) > .5 {
		s.widthChangedAt = now
	}
	s.measured, s.capacity = true, capacity
	remaining := time.Duration(0)
	if !s.widthChangedAt.IsZero() {
		remaining = max(0, composerResizeSettle-now.Sub(s.widthChangedAt))
	}
	s.expanded = composerFlip(s.expanded, textWidth, capacity, newline, remaining > 0)
	return s.expanded, remaining
}

func composerHasNewline(text string) bool { return strings.ContainsAny(text, "\r\n") }
