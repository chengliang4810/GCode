package desktop

import (
	"context"
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
	if _, ok := tt.Find("Cancel this correction"); !ok {
		t.Fatal("the steering message has no way to be cancelled")
	}
}

// Cancelling the correction removes its queue row, which is the only thing the
// core keeps until the boundary hands the input over.
func TestSteeringMessageCancelsTheCorrection(t *testing.T) {
	a := steeringApp(t)
	var dispatched []contracts.Command
	a.service = &sliceRecordingService{Service: a.service, recorded: &dispatched}
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if err := tt.Click("Cancel this correction"); err != nil {
		t.Fatal(err)
	}
	if len(dispatched) != 1 || dispatched[0].Kind != "queue.remove" || dispatched[0].ID != "queue-steer" {
		t.Fatalf("cancel dispatched %+v", dispatched)
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
	if _, ok := tt.Find("Cancel this correction"); ok {
		t.Fatal("the handed-over message kept its steering status")
	}
	if got := steeringCount(tt.Texts(), "Send now (interrupt)"); got != 0 {
		t.Fatalf("a handed-over correction left a queue row: %q", tt.Texts())
	}
}

// sliceRecordingService records the commands the app dispatches without
// replacing the backend the fixture built.
type sliceRecordingService struct {
	contracts.Service
	recorded *[]contracts.Command
}

func (s *sliceRecordingService) Execute(ctx context.Context, command contracts.Command) error {
	*s.recorded = append(*s.recorded, command.Clone())
	return s.Service.Execute(ctx, command)
}
