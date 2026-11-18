package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestInternalQueueInputsStayHiddenOnAllConversationSurfaces(t *testing.T) {
	for _, source := range []string{"subagent", "hook"} {
		for _, delivery := range []string{"steer", "queue"} {
			for _, mode := range []string{"light", "dark"} {
				for _, width := range []int{900, 1320} {
					t.Run(fmt.Sprintf("%s/%s/%s/%d", source, delivery, mode, width), func(t *testing.T) {
						a := steeringApp(t)
						a.state.Settings.Appearance.Mode = mode
						const text = "internal runtime input must not be displayed"
						a.state.Queue = append(a.state.Queue, contracts.QueuedMessage{ID: "internal", SessionID: "chat-0", Source: source, Delivery: delivery, Text: text})
						for _, m := range a.projectedMessages("chat-0") {
							for _, b := range m.Blocks {
								if b.Text == text {
									t.Fatal("internal result projected as a message")
								}
							}
						}
						tt := ui.NewTester(a.View, width, 880)
						if tt.HasText(text) {
							t.Fatal("internal result displayed in primary conversation")
						}
						side := ui.NewTester(func(c *ui.Context) { a.sideQueuedMessages(c, "chat-0", &sideComposerState{}) }, width, 880)
						if side.HasText(text) {
							t.Fatal("internal result displayed in side queue")
						}
						if len(a.state.Queue) != 3 || a.state.Queue[2].Source != source {
							t.Fatal("hiding the input changed the runtime queue")
						}
						if !tt.HasText("Correct the live answer") || !tt.HasText("Waits for the next turn") {
							t.Fatal("hiding runtime input hid a user's message")
						}
					})
				}
			}
		}
	}
}
