package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// A new turn grows under a reader who sits at the end: the transcript keeps
// the end in view, so the history above it is pushed up instead of the new
// turn landing below the fold. Scrolling away stops the follow, which the
// "Back to latest" button (chat.go) then offers to resume.
func TestTranscriptPushesHistoryUpOnNewTurn(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	msgs := a.state.Messages["chat-0"]
	for i := 0; i < 24; i++ {
		msgs = append(msgs, contracts.Message{ID: fmt.Sprintf("fill-%d", i), SessionID: "chat-0", Role: "assistant", TurnID: "fill",
			Blocks: []contracts.Block{{ID: fmt.Sprintf("fill-block-%d", i), Kind: "text", Text: fmt.Sprintf("FILL LINE %02d", i)}}})
	}
	a.state.Messages["chat-0"] = msgs
	a.state.Revision++
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Advance(time.Second)
	tt.Frame()
	marker, ok := tt.Find("FILL LINE 22")
	if !ok {
		t.Fatal("the history did not fill the viewport")
	}
	if !a.messages.AtEnd() {
		t.Fatal("an untouched transcript must sit at its end")
	}
	a.state.Messages["chat-0"] = append(a.state.Messages["chat-0"], contracts.Message{
		ID: "check-user", SessionID: "chat-0", Role: "user", TurnID: "check",
		Blocks: []contracts.Block{{ID: "check-block", Kind: "text", Text: "CHECK_PUSH_UP"}},
	})
	a.state.Revision++
	for i := 0; i < 60; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	after, ok := tt.Find("FILL LINE 22")
	if !ok {
		t.Fatal("the history scrolled away entirely instead of moving up")
	}
	if after.Y >= marker.Y {
		t.Fatalf("the history did not move up: before y=%v after y=%v", marker.Y, after.Y)
	}
	if _, ok := tt.Find("CHECK_PUSH_UP"); !ok {
		t.Fatal("the new turn is not in view")
	}
	if !a.messages.AtEnd() {
		t.Fatal("following the end stopped without the reader scrolling away")
	}
}

func TestTranscriptStreamingKeepsHistoryAnchor(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{900, 1320} {
			t.Run(fmt.Sprintf("dark=%t/width=%d", dark, width), func(t *testing.T) {
				a := testApp(t, "transcript")
				a.selectSession("chat-0")
				a.state.Settings.Appearance.ReduceMotion = true
				a.expanded["session-info-user"], a.expanded["session-info"] = true, false
				a.palette = design.Palette("rcode", dark)
				history := railFixtureMessages(32)
				reply := &history[len(history)-1]
				reply.Blocks = []contracts.Block{{ID: "active-text", Kind: "text", Text: "FINAL REPLY"}}
				reply.Streaming = true
				a.state.Messages[a.sessionID] = history
				a.state.Revision++
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(a.palette.UI())
					a.View(c)
				}, width, 720)
				tt.Frame()
				a.messages.ScrollTo(13*15, ui.Start)
				tt.Frame()
				first, _ := a.messages.Visible()
				if a.messages.AtEnd() {
					t.Fatal("fixture did not leave the end")
				}
				a.applyEvent(contracts.Event{
					Kind: "message.delta", Revision: a.state.Revision + 1, SessionID: a.sessionID,
					MessageID: reply.ID, BlockKind: "text", Text: " STREAMED APPEND",
				})
				tt.Frame()
				if after, _ := a.messages.Visible(); a.messages.AtEnd() || after != first {
					t.Fatalf("streaming moved the reading position: row %d -> %d", first, after)
				}
				cache := &a.transcriptCache
				hits := a.conversationMatches(cache, cache.entries, cache.flat, "STREAMED APPEND")
				if len(hits) != 1 || hits[0] != len(cache.flat)-1 {
					t.Fatalf("search did not see streamed text at the correct row: %v", hits)
				}
				a.messages.ScrollToEnd()
				tt.Advance(time.Second)
				tt.Frame()
				if !tt.HasText("FINAL REPLY STREAMED APPEND") {
					t.Fatal("returning to the end showed stale streamed text")
				}
			})
		}
	}
}
