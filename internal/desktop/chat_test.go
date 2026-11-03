package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestChatFailureOnlyAppearsInTranscript(t *testing.T) {
	for _, theme := range []string{"rcode-light", "rcode-dark"} {
		t.Run(theme, func(t *testing.T) {
			a := testApp(t, "transcript")
			a.selectSession("chat-0")
			a.palette = design.Palette(theme, theme == "rcode-dark")
			for i := range a.state.Sessions {
				if a.state.Sessions[i].ID == a.sessionID {
					a.state.Sessions[i].Status = contracts.Errored
				}
			}
			const failure = "Run failed: model request failed (HTTP 405)"
			a.state.Messages[a.sessionID] = []contracts.Message{{
				ID: "failure", SessionID: a.sessionID, Role: "assistant",
				Blocks: []contracts.Block{{Kind: "text", Text: failure}},
			}}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				a.chatView(c)
			}, 900, 600)
			for _, text := range tt.Texts() {
				if text == "Run failed" {
					t.Fatal("redundant failure status is still visible")
				}
			}
			if !tt.HasText(failure) {
				t.Fatalf("transcript failure detail was removed: %v", tt.Texts())
			}
		})
	}
}
