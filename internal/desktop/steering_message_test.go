package desktop

import (
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// steeringCount counts how many rendered texts carry the phrase.
func steeringCount(texts []string, phrase string) int {
	count := 0
	for _, text := range texts {
		count += strings.Count(text, phrase)
	}
	return count
}

func steeringApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Runtime = &contracts.RuntimeInfo{}
	start := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	a.state.Queue = []contracts.QueuedMessage{
		{ID: "queue-next", SessionID: "chat-0", Text: "Waits for the next turn", Delivery: "queue", CreatedAt: start},
		{ID: "queue-steer", SessionID: "chat-0", Text: "Correct the live answer", Delivery: "steer", CreatedAt: start.Add(time.Second)},
	}
	a.state.Revision++
	return a
}

// A steering input is already accepted and will take over at the next step
// boundary: it reads as the user's message in the conversation, never as a row
// waiting above the composer for the next turn.
func TestSteeringRowReadsAsAUserMessage(t *testing.T) {
	a := steeringApp(t)
	messages := a.projectedMessages("chat-0")
	var projected *contracts.Message
	for i := range messages {
		if messages[i].ID == steeringMessageID("queue-steer") {
			projected = &messages[i]
		}
	}
	if projected == nil || projected.Role != "user" || !strings.Contains(projected.Blocks[0].Text, "Correct the live answer") {
		t.Fatalf("steering row was not projected as a user message: %+v", messages)
	}
	if !projected.CreatedAt.Equal(a.state.Queue[1].CreatedAt) {
		t.Fatal("the projected message lost its sent time")
	}
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	texts := tt.Texts()
	// Only the ordinary row keeps the queue controls: the steering row is not
	// drawn above the composer at all.
	if got := steeringCount(texts, "Send now (interrupt)"); got != 1 {
		t.Fatalf("queue actions rendered %d times, want the ordinary row only: %q", got, texts)
	}
	if got := steeringCount(texts, "Remove queued message"); got != 1 {
		t.Fatalf("queue removal rendered %d times, want the ordinary row only: %q", got, texts)
	}
	// The correction reads as a message in the conversation, above the row that
	// still waits for the next turn.
	bubble, ok := tt.Find("Correct the live answer")
	if !ok {
		t.Fatalf("the correction has no message bubble: %q", texts)
	}
	row, ok := tt.Find("Waits for the next turn")
	if !ok {
		t.Fatalf("the ordinary queued row is missing: %q", texts)
	}
	if bubble.Y >= row.Y {
		t.Fatalf("the correction is not in the conversation: bubble=%+v queue row=%+v", bubble, row)
	}
	// The correction is already on its way into the running reply: the bubble
	// carries no cancel control and no queue action.
	if _, ok := tt.Find("Cancel this correction"); ok {
		t.Fatal("the sent correction still offers a cancel button")
	}
	if got := steeringCount(tt.Texts(), "Cancel this correction"); got != 0 {
		t.Fatalf("the sent correction rendered %d cancel labels: %q", got, tt.Texts())
	}
}

// Once the boundary hands the input over, the queue row is gone and the core's
// own message carries the text: the conversation still shows it exactly once.
func TestSteeringRowLeavesNoDuplicateAfterHandoff(t *testing.T) {
	a := steeringApp(t)
	a.state.Queue = nil
	a.state.Messages["chat-0"] = append(a.state.Messages["chat-0"], contracts.Message{
		ID: "queue-steer", SessionID: "chat-0", Role: "user",
		Blocks: []contracts.Block{{ID: "queue-steer:text", Kind: "text", Text: "Correct the live answer"}},
	})
	a.state.Revision++
	for _, message := range a.projectedMessages("chat-0") {
		if message.ID == steeringMessageID("queue-steer") {
			t.Fatal("the handed-over message is still projected from the queue")
		}
	}
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	// The conversation outline lists every prompt once: a duplicate projection
	// would add a second entry for the same correction.
	outlined := 0
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "Go to message ") && strings.Contains(text, "Correct the live answer") {
			outlined++
		}
	}
	if outlined != 1 {
		t.Fatalf("the handed-over correction has %d outline entries: %q", outlined, tt.Texts())
	}
	if got := steeringCount(tt.Texts(), "Send now (interrupt)"); got != 0 {
		t.Fatalf("a handed-over correction left a queue row: %q", tt.Texts())
	}
}
