package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestInterruptedMessageEditor(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			a := testApp(t, "default")
			a.sessionID = "chat-0"
			if err := a.service.Execute(a.ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				SessionID: a.sessionID, Text: "Original question", RequestID: "stopped-request",
			}}); err != nil {
				t.Fatal(err)
			}
			a.dispatch(contracts.Command{Kind: "message.stop", SessionID: a.sessionID})
			a.palette = design.Palette("rcode-"+theme, theme == "dark")
			messages := a.state.Messages[a.sessionID]
			i := contracts.EditableTurnMessage(a.currentSession(), messages, a.state.Runs)
			if i < 0 {
				t.Fatal("mock did not stop on a user turn")
			}
			original := messages[i]
			a.draft = "Unrelated draft"
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				a.userMessage(c, original)
				a.interruptedMessageDialog(c)
			}, 360, 500)
			if err := tt.Click("Edit message"); err != nil {
				t.Fatal(err)
			}
			if a.interruptedEditor.text != "Original question" {
				t.Fatalf("incorrect prefilled text: %q", a.interruptedEditor.text)
			}
			if err := tt.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
			if a.interruptedEditor.open || a.state.Messages[a.sessionID][i].ID != original.ID {
				t.Fatal("cancel changed history")
			}
			if err := tt.Click("Edit message"); err != nil {
				t.Fatal(err)
			}
			a.interruptedEditor.text = "Revised input"
			if err := tt.Click("Replace and resend"); err != nil {
				t.Fatal(err)
			}
			if a.interruptedEditor.open || a.draft != "Unrelated draft" {
				t.Fatalf("unexpected editor or draft state: %+v", a.interruptedEditor)
			}
			messages = a.state.Messages[a.sessionID]
			if messages[i].ID == original.ID || messages[i].Blocks[0].Text != "Revised input" {
				t.Fatal("edited input did not replace the original row")
			}
		})
	}
}
