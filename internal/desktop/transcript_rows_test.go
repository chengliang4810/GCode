package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func rowsFixture() ([]contracts.Message, func(contracts.Message) []transcriptRow) {
	entries := []contracts.Message{
		{ID: "u1", Role: "user", Blocks: []contracts.Block{{ID: "u1b", Kind: "text", Text: "hello"}}},
		{ID: "a1", Role: "assistant", Blocks: []contracts.Block{
			{ID: "a1b1", Kind: "text", Text: "first"},
			{ID: "a1b2", Kind: "tool", Tool: &contracts.ToolCall{ID: "tool", Kind: contracts.ToolExec, Status: "completed"}},
			{ID: "a1b3", Kind: "text", Text: "last"},
		}},
		{ID: "a2", Role: "assistant"},
	}
	return entries, func(entry contracts.Message) []transcriptRow {
		return transcriptRows(entry, false)
	}
}

// The list is a row per block, not a row per message: the reference tiles the
// transcript from block rows, and a message's footer rides its last row.
func TestConversationRowsSplitAMessageIntoBlockRows(t *testing.T) {
	entries, rowsOf := rowsFixture()
	rows := conversationRows(entries, rowsOf)
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.Key] {
			t.Fatalf("row key %q repeats", row.Key)
		}
		seen[row.Key] = true
	}
	if !rows[0].User || !rows[0].Last || rows[0].TopGap != 0 {
		t.Fatalf("first row %+v", rows[0])
	}
	if rows[1].User || rows[1].Block != 0 || rows[1].TopGap != transcriptTurnGap || rows[1].Last {
		t.Fatalf("first block row %+v", rows[1])
	}
	if rows[2].TopGap != transcriptActivityGap || rows[3].TopGap != transcriptActivityGap {
		t.Fatalf("activity gaps %v/%v", rows[2].TopGap, rows[3].TopGap)
	}
	if !rows[3].Last {
		t.Fatal("the entry's footer is not on its last row")
	}
	if !rows[4].Bare || !rows[4].Last || rows[4].TopGap != transcriptTurnGap {
		t.Fatalf("an entry without rows %+v", rows[4])
	}
}

// A virtualized transcript builds the rows in view, not every block of every
// reply: that is what keeps a long conversation's frames cheap.
func TestTranscriptListBuildsOnlyTheRowsInView(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	blocks := make([]contracts.Block, 0, 60)
	for i := range 60 {
		blocks = append(blocks, contracts.Block{ID: fmt.Sprintf("b%02d", i), Kind: "text", Text: fmt.Sprintf("Paragraph %02d of the reply", i)})
	}
	a.state.Messages[a.sessionID] = []contracts.Message{{ID: "long", SessionID: a.sessionID, Role: "assistant", Blocks: blocks}}
	a.palette = design.Palette("rcode", false)
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		a.chatView(c)
	}, 900, 600)
	// The conversation follows its end, so the tail's rows are the built ones.
	if !tt.HasText("Paragraph 59 of the reply") {
		t.Fatal("the last block is missing")
	}
	if tt.HasText("Paragraph 00 of the reply") {
		t.Fatal("a block far above the view was built")
	}
	a.messages.ScrollTo(0, ui.Start)
	tt.Frame()
	if !tt.HasText("Paragraph 00 of the reply") {
		t.Fatal("scrolling to the start did not build the first block")
	}
	if tt.HasText("Paragraph 59 of the reply") {
		t.Fatal("the list kept building rows it scrolled away")
	}
}

// A selection made across block rows still copies every paragraph after the
// list scrolls those rows out of its build: the selection holds the text it
// resolved, so a virtualized transcript cannot lose what the user selected.
func TestTranscriptSelectionCopiesAfterItsRowsScrollOut(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	blocks := make([]contracts.Block, 0, 60)
	for i := range 60 {
		blocks = append(blocks, contracts.Block{ID: fmt.Sprintf("b%02d", i), Kind: "text", Text: fmt.Sprintf("Paragraph %02d of the reply", i)})
	}
	a.state.Messages[a.sessionID] = []contracts.Message{{ID: "long", SessionID: a.sessionID, Role: "assistant", Blocks: blocks}}
	a.palette = design.Palette("rcode", false)
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		a.chatView(c)
	}, 900, 600)
	a.messages.ScrollTo(0, ui.Start)
	tt.Frame()
	first, ok := tt.Find("Paragraph 00 of the reply")
	if !ok {
		t.Fatal("the first block is missing")
	}
	last, ok := tt.Find("Paragraph 02 of the reply")
	if !ok {
		t.Fatal("the third block is missing")
	}
	tt.Press(first.X+1, first.Y+first.H/2)
	tt.Move(last.X+last.W-1, last.Y+last.H/2)
	tt.Release(last.X+last.W-1, last.Y+last.H/2)
	a.messages.ScrollToEnd()
	tt.Frame()
	if tt.HasText("Paragraph 01 of the reply") {
		t.Fatal("the selected rows are still built, so nothing was tested")
	}
	tt.Key(ui.Cmd, ui.KeyC)
	want := "Paragraph 00 of the reply\nParagraph 01 of the reply\nParagraph 02 of the reply"
	if got := tt.Clipboard(); got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// The message's footer rides its last row, and the pointer being anywhere in
// the message's rows is what reveals it: rows are separate list items, so one
// row answering for the message would hide it while the pointer is above.
func TestTranscriptFooterFollowsTheMessageRows(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.state.Messages[a.sessionID] = []contracts.Message{{ID: "reply", SessionID: a.sessionID, Role: "assistant", Blocks: []contracts.Block{
		{ID: "b1", Kind: "text", Text: "First block of the reply"},
		{ID: "b2", Kind: "text", Text: "Second block of the reply"},
		{ID: "b3", Kind: "text", Text: "Third block of the reply"},
	}}}
	a.palette = design.Palette("rcode", false)
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		a.chatView(c)
	}, 900, 600)
	first, ok := tt.Find("First block of the reply")
	if !ok {
		t.Fatal("the first block is missing")
	}
	tt.Move(first.X+2, first.Y+first.H/2)
	tt.Frame()
	if !a.transcriptEntryHovered(a.sessionID, "reply") {
		t.Fatal("hovering the first row did not reach the message's footer")
	}
	tt.Move(4, 4)
	tt.Frame()
	if a.transcriptEntryHovered(a.sessionID, "reply") {
		t.Fatal("the footer stayed revealed after the pointer left")
	}
}

// Find moves to the row holding the match, not to the message around it.
func TestConversationMatchesNameTheRowOfTheMatch(t *testing.T) {
	a := testApp(t, "transcript")
	entries, rowsOf := rowsFixture()
	entries[1].Blocks[2].Text = "needle here"
	rows := conversationRows(entries, rowsOf)
	var cache transcriptCache
	matches := a.conversationMatches(&cache, entries, rows, "needle")
	if len(matches) != 1 || matches[0] != 3 {
		t.Fatalf("matches = %v, want the third block row", matches)
	}
}
