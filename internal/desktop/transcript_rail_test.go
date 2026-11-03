package desktop

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestMessageRailProjectsUserRowsAndOwnReply(t *testing.T) {
	entries := []contracts.Message{
		{ID: "system", Role: "system", Blocks: []contracts.Block{{Kind: "text", Text: "instructions"}}},
		{ID: "u1", Role: "user", Blocks: []contracts.Block{{Kind: "text", Text: "  first\nprompt "}}},
		{ID: "thinking", Role: "assistant", Blocks: []contracts.Block{{ID: "thought", Kind: "reasoning", Text: "hidden thought"}}},
		{ID: "u2", Role: "user", Blocks: []contracts.Block{{Kind: "attachment", Attachment: &contracts.Attachment{Name: "截图.png"}}}},
		{ID: "a2", Role: "assistant", Blocks: []contracts.Block{
			{ID: "work", Kind: "reasoning", Text: "not a reply"},
			{ID: "reply", Kind: "text", Text: "  second\nreply🙂 "},
			{ID: "more", Kind: "text", Text: "later paragraph"},
		}},
		{ID: "u3", Role: "user"},
	}
	rows := conversationRows(entries, func(entry contracts.Message) []transcriptRow { return transcriptRows(entry, false) })
	ticks := messageRailTicks(entries, rows)
	if len(ticks) != 3 || ticks[0].Prompt != "first prompt" || ticks[0].Reply != "" {
		t.Fatalf("user ticks or reply boundary: %+v", ticks)
	}
	if ticks[1].Prompt != "截图.png" || ticks[1].Reply != "second reply🙂" {
		t.Fatalf("attachment or reply preview: %+v", ticks[1])
	}
	if ticks[2].Row != len(rows)-1 || ticks[2].Row == len(entries)-1 {
		t.Fatalf("rail targets message indices instead of block rows: %+v", ticks[2])
	}
}

func TestMessageRailPreviewText(t *testing.T) {
	for _, test := range []struct {
		name, text string
		limit      int
		want       string
	}{
		{"whitespace", " \n 中文\t🙂  reply \n", 20, "中文 🙂 reply"},
		{"bounded Unicode", "中文🙂很好", 4, "中文🙂…"},
		{"ellipsis replaces space", "abc def ghi", 5, "abc…"},
		{"empty limit", "abc", 0, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := railPreviewText(test.text, test.limit)
			if got != test.want || !utf8.ValidString(got) {
				t.Fatalf("preview = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMessageRailBucketsCoverLongConversation(t *testing.T) {
	for _, test := range []struct {
		name         string
		count        int
		height       float32
		wantCapacity int
	}{
		{"three marks", 3, 880, 3},
		{"long conversation", 137, 880, 12},
		{"tall window stays compact", 137, 2000, 12},
		{"short viewport", 137, 100, 4},
		{"no room", 137, 0, 1},
		{"empty", 0, 880, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			buckets := messageRailBuckets(test.count, test.height)
			if len(buckets) != test.wantCapacity {
				t.Fatalf("marks = %d, want %d", len(buckets), test.wantCapacity)
			}
			end := 0
			for _, bucket := range buckets {
				if bucket.Start != end || bucket.End <= bucket.Start {
					t.Fatalf("non-contiguous buckets: %+v", buckets)
				}
				end = bucket.End
			}
			if end != test.count {
				t.Fatalf("outline stops at %d of %d prompts", end, test.count)
			}
		})
	}
}

func railFixtureMessages(turns int) []contracts.Message {
	var messages []contracts.Message
	for turn := range turns {
		userID := fmt.Sprintf("rail-user-%02d", turn)
		messages = append(messages, contracts.Message{ID: userID, SessionID: "chat-0", Role: "user",
			Blocks: []contracts.Block{{ID: userID + ":text", Kind: "text", Text: fmt.Sprintf("Prompt %02d", turn)}}})
		reply := contracts.Message{ID: fmt.Sprintf("rail-reply-%02d", turn), SessionID: "chat-0", Role: "assistant"}
		for block := range 12 {
			reply.Blocks = append(reply.Blocks, contracts.Block{ID: fmt.Sprintf("%s:%02d", reply.ID, block), Kind: "text",
				Text: fmt.Sprintf("Reply %02d paragraph %02d", turn, block)})
		}
		messages = append(messages, reply)
	}
	return messages
}

func messageRailTester(t *testing.T, width int, dark bool, turns int) (*App, *ui.Tester) {
	t.Helper()
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.state.Messages[a.sessionID] = railFixtureMessages(turns)
	a.palette = design.Palette("rcode", dark)
	a.draft = "KEEP RAIL DRAFT"
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		a.chatView(c)
	}, width, 600)
	tt.Frame()
	return a, tt
}

func TestMessageRailNavigationPreviewAndFollow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			a, tt := messageRailTester(t, 900, dark, 3)
			firstLabel, secondLabel := "Go to message 1: Prompt 00", "Go to message 2: Prompt 01"
			firstMark, found := tt.Find(firstLabel)
			if !found || firstMark.X != design.MessageRail().Left || firstMark.H != design.MessageRail().SlotHeight {
				t.Fatalf("compact left rail missing or misplaced: %+v", firstMark)
			}
			first, _ := a.messages.Visible()
			if activeMessageRailTick(a.transcriptCache.rail, first) != 2 {
				t.Fatal("latest reading position did not activate its mark")
			}
			before, _ := tt.Find("Message")
			tt.Move(firstMark.X+3, firstMark.Y+firstMark.H/2)
			if !tt.HasText("Reply 00 paragraph 00") || !tt.HasText("Prompt 00") {
				t.Fatal("hover preview lacks its prompt and reply")
			}
			if err := tt.Click(firstLabel); err != nil {
				t.Fatal(err)
			}
			first, _ = a.messages.Visible()
			if first != a.transcriptCache.rail[0].Row || !tt.HasText("Reply 00 paragraph 00") {
				t.Fatal("click did not navigate from the tail to the first prompt")
			}
			tt.Move(850, 550)
			firstMark, _ = tt.Find(firstLabel)
			secondMark, _ := tt.Find(secondLabel)
			one := tt.Image().RGBAAt(int(firstMark.X+2), int(firstMark.Y+firstMark.H/2))
			two := tt.Image().RGBAAt(int(secondMark.X+2), int(secondMark.Y+secondMark.H/2))
			if one == two {
				t.Fatal("active and inactive marks have the same rendered ink")
			}
			backgroundOne := tt.Image().RGBAAt(int(firstMark.X+firstMark.W-2), int(firstMark.Y+firstMark.H/2))
			backgroundTwo := tt.Image().RGBAAt(int(secondMark.X+secondMark.W-2), int(secondMark.Y+secondMark.H/2))
			if backgroundOne != backgroundTwo {
				t.Fatal("the active mark gained a filled button backplate")
			}
			// Native buttons preserve keyboard navigation as well as pointer input.
			tt.Key(0, ui.KeyTab)
			if !tt.Focused(secondLabel) {
				t.Fatal("rail marks are not keyboard reachable in conversation order")
			}
			tt.Key(0, ui.KeyEnter)
			first, _ = a.messages.Visible()
			position, _ := tt.Find("Prompt 01")
			if first < a.transcriptCache.rail[1].Row-1 || first > a.transcriptCache.rail[1].Row ||
				position.Y > transcriptListTopPadding+transcriptTurnGap+20 {
				t.Fatalf("keyboard activation did not align the second prompt: first=%d prompt=%+v", first, position)
			}
			tt.Move(850, 550)
			firstMark, _ = tt.Find(firstLabel)
			secondMark, _ = tt.Find(secondLabel)
			afterFirst := tt.Image().RGBAAt(int(firstMark.X+2), int(firstMark.Y+firstMark.H/2))
			afterSecond := tt.Image().RGBAAt(int(secondMark.X+2), int(secondMark.Y+secondMark.H/2))
			if afterFirst != two || afterSecond != one {
				t.Fatal("a previous row at the clip edge kept the wrong mark highlighted")
			}
			if err := tt.Click("Back to latest"); err != nil {
				t.Fatal(err)
			}
			if !a.messages.AtEnd() {
				t.Fatal("returning to latest did not restore following")
			}
			after, _ := tt.Find("Message")
			if a.draft != "KEEP RAIL DRAFT" || before != after {
				t.Fatalf("outline navigation moved the composer or changed its draft: %+v -> %+v", before, after)
			}
		})
	}
}

func TestMessageRailVisibilityAndLongConversation(t *testing.T) {
	for _, test := range []struct {
		name         string
		width, turns int
		visible      bool
	}{
		{"one turn", 900, 1, false},
		{"narrow conversation", 767, 3, false},
		{"width threshold", 768, 3, true},
		{"long conversation", 900, 137, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, tt := messageRailTester(t, test.width, false, test.turns)
			if tt.HasText("Conversation outline") != test.visible {
				t.Fatalf("outline visibility at %d DIP with %d turns: %v", test.width, test.turns, tt.Texts())
			}
			count := 0
			for _, text := range tt.Texts() {
				if strings.HasPrefix(text, "Go to message ") {
					count++
				}
			}
			if test.visible && (count < 2 || count > design.MessageRail().MaxMarks) {
				t.Fatalf("outline footprint has %d marks", count)
			}
		})
	}
}

func TestMessageRailCacheRebuildsWithTranscriptRows(t *testing.T) {
	cache := transcriptCache{}
	messages := railFixtureMessages(3)
	entries := cache.entriesFor(1, "chat-0", messages, false)
	cache.conversationFor(entries, false)
	old := cache.rail
	cache.conversationFor(entries, false)
	if &cache.rail[0] != &old[0] {
		t.Fatal("outline was reprojected on an unchanged scroll frame")
	}
	messages[1].Blocks = append(messages[1].Blocks, contracts.Block{ID: "new-block", Kind: "text", Text: "new reply block"})
	entries = cache.entriesFor(2, "chat-0", messages, false)
	cache.conversationFor(entries, false)
	if cache.rail[1].Row != old[1].Row+1 {
		t.Fatal("outline retained message targets from an obsolete row projection")
	}
}

func TestMessageRailPointerInFullShell(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.state.Messages[a.sessionID] = railFixtureMessages(3)
	a.state.Settings.Appearance.ReduceMotion = true
	a.draft = "KEEP SHELL DRAFT"
	tt := ui.NewTester(a.View, 1320, 600)
	mark, found := tt.Find("Go to message 1: Prompt 00")
	if !found {
		t.Fatal("full shell did not mount the message rail")
	}
	tt.Move(mark.X+3, mark.Y+mark.H/2)
	if !tt.HasText("Reply 00 paragraph 00") {
		t.Fatalf("full shell blocks the rail hover: mark=%+v texts=%v", mark, tt.Texts())
	}
	if err := tt.Click("Go to message 1: Prompt 00"); err != nil {
		t.Fatal(err)
	}
	if first, _ := a.messages.Visible(); first != 0 {
		t.Fatalf("full shell blocks the rail pointer click: first=%d", first)
	}
	tt.SetSize(900, 600)
	if tt.HasText("Conversation outline") {
		t.Fatal("resizing the shell left the outline in a narrow conversation")
	}
	tt.SetSize(1320, 600)
	if !tt.HasText("Conversation outline") {
		t.Fatal("widening the shell did not restore the outline")
	}
}
