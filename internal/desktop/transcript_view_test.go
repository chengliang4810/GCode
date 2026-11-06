package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/linkui"
	"github.com/egoist/mygo/ui"
)

// transcriptMessageForTest renders one message the way the transcript list
// does: as its projected rows inside one selection scope.
func (a *App) transcriptMessageForTest(c *ui.Context, message contracts.Message) {
	cache := &transcriptCache{}
	messages := cache.entriesFor(0, message.SessionID, []contracts.Message{message}, a.state.Settings.General.CompactMode)
	rows := cache.conversationFor(messages, a.state.Settings.General.CompactMode)
	pane := transcriptPane{cache: cache, scope: "test", session: message.SessionID}
	root := ui.Column(c).FillWidth().Selectable()
	selection := linkui.BeginSelection(root)
	if a.transcriptSelections == nil {
		a.transcriptSelections = map[string]*linkui.Selection{}
	}
	a.transcriptSelections[pane.scope] = selection
	previous := a.linkSelection
	a.linkSelection = selection
	defer func() { a.linkSelection = previous }()
	root.Children(func() {
		for i := range rows {
			a.transcriptRowView(c, pane, messages, rows, i, transcriptRowLayout{})
		}
	})
	selection.Menu(c, root)
}

// A code block shows the tabs of its source at the editor's tab stops,
// rather than collapsing them to nothing.
func TestTranscriptCodeBlockShowsTabs(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.state.Messages[a.sessionID] = []contracts.Message{
		{ID: "m", SessionID: a.sessionID, Role: "assistant", Blocks: []contracts.Block{
			{ID: "b", Kind: "text", Text: "```go\nfunc main() {\n\tif x {\n\t\tpanic(1)\n\t}\n}\n```"},
		}},
	}
	a.palette = design.Palette("rcode", false)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		a.transcriptMessageForTest(c, a.state.Messages[a.sessionID][0])
	}, 900, 600)
	if !tt.HasText("    if x {") || !tt.HasText("        panic(1)") {
		t.Fatalf("code block tabs are missing: %q", tt.Texts())
	}
}
