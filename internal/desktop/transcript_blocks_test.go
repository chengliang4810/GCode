package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTranscriptHistoryTextDoesNotReplayDuringActiveTurn(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			a := testApp(t, "default")
			a.palette = design.Palette("rcode", dark)
			a.state.Settings.Appearance.MotionPreference = "full"
			const history = "Earlier completed narration must remain fully visible when scrolling back through a running task."
			messages := []contracts.Message{
				{ID: "earlier", SessionID: "chat-0", TurnID: "turn", Role: "assistant", Streaming: true,
					Blocks: []contracts.Block{{ID: "earlier-text", Kind: "text", Text: history}}},
				{ID: "current", SessionID: "chat-0", TurnID: "turn", Role: "assistant", Streaming: true,
					Blocks: []contracts.Block{{ID: "current-thought", Kind: "reasoning", Text: "Still working"}}},
			}
			var cache transcriptCache
			mounted := true
			tt := ui.NewTester(func(c *ui.Context) {
				a.configureMotion(c)
				c.SetTheme(a.palette.UI())
				a.prepareTranscriptMarkdown(c)
				entries := cache.entriesFor(1, "chat-0", messages, false)
				rows := cache.conversationFor(entries, false)
				ui.Column(c).Fill().Children(func() {
					if mounted {
						for i, row := range rows {
							if !row.Activity {
								c.Key(row.Key)
								a.transcriptRowView(c, transcriptPane{cache: &cache, scope: "main"}, entries, rows, i, transcriptRowLayout{})
							}
						}
					}
				})
			}, 720, 300)
			check := func(stage string) {
				t.Helper()
				if !tt.HasText(history) || tt.FramePending() {
					t.Fatalf("%s replayed historical text: pending=%t texts=%q", stage, tt.FramePending(), tt.Texts())
				}
			}
			check("initial mount")
			mounted = false
			tt.Frame()
			tt.Advance(time.Second)
			mounted = true
			tt.Frame()
			check("remount")
		})
	}
}

func TestTranscriptLiveTextRemountPrimesExistingContentAndRevealsOnlyNewText(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.MotionPreference = "full"
	const prefix = "This text was already received before its row entered the viewport."
	source, mounted := prefix, true
	tt := ui.NewTester(func(c *ui.Context) {
		a.configureMotion(c)
		a.prepareTranscriptMarkdown(c)
		ui.Column(c).Fill().Children(func() {
			if mounted {
				a.transcriptBlock(c, contracts.Message{SessionID: "chat-0", Streaming: true},
					contracts.Block{ID: "live-text", Kind: "text", Text: source})
			}
		})
	}, 720, 300)
	if !tt.HasText(prefix) || tt.FramePending() {
		t.Fatalf("initial content replayed: pending=%t text=%q", tt.FramePending(), tt.Texts())
	}
	source += " " + strings.Repeat("new words ", 50)
	tt.Frame()
	if !tt.FramePending() || len(strings.Join(tt.Texts(), "")) >= len(source)-1 {
		t.Fatal("new stream data lost its reveal animation")
	}
	mounted = false
	tt.Frame()
	tt.Advance(time.Second)
	mounted = true
	tt.Frame()
	if !tt.HasText(strings.TrimSpace(source)) || tt.FramePending() {
		t.Fatalf("returning to live text replayed its prefix: pending=%t text=%q", tt.FramePending(), tt.Texts())
	}
}
