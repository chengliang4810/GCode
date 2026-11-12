package desktop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

func TestQueueRowsStayInsideComposerWidth(t *testing.T) {
	for _, width := range []int{700, 1320, 2048} {
		for _, dark := range []bool{false, true} {
			for _, empty := range []bool{false, true} {
				t.Run(fmt.Sprintf("width=%d/dark=%t/empty=%t", width, dark, empty), func(t *testing.T) {
					a, _ := queueRowApp(t)
					a.state.Settings.Appearance.ThemeID = "rcode-light"
					if dark {
						a.state.Settings.Appearance.ThemeID = "rcode-dark"
					}
					if empty {
						a.state.Messages[a.sessionID] = nil
						a.state.Revision++
					}
					var surface ui.Rect
					tt := ui.NewTester(func(c *ui.Context) {
						a.View(c)
						surface = a.composerSurface.Bounds(c)
					}, width, 880)
					tt.Advance(time.Second)
					tt.Frame()
					for _, label := range []string{"First queued message", "Second queued message", "Send now (interrupt)", "Edit queued message", "Remove queued message"} {
						bounds, ok := tt.Find(label)
						if !ok {
							t.Fatalf("queue control missing: %s", label)
						}
						if bounds.X < surface.X || bounds.X+bounds.W > surface.X+surface.W || bounds.Y+bounds.H > surface.Y {
							t.Fatalf("%s=%+v must stay above and within composer=%+v", label, bounds, surface)
						}
					}
					writeModelSurfaceEvidence(t, fmt.Sprintf("queue-width-%d-dark-%t-empty-%t", width, dark, empty), tt, map[string]any{"composer": surface})
				})
			}
		}
	}
}

// queueCommandRecorder keeps the queue commands the app dispatches so a test
// can read the rows they name.
type queueCommandRecorder struct {
	contracts.Service
	commands []contracts.Command
}

func (s *queueCommandRecorder) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "queue.send" || command.Kind == "queue.move" {
		s.commands = append(s.commands, command.Clone())
	}
	return s.Service.Execute(ctx, command)
}

func queueRowApp(t *testing.T) (*App, *queueCommandRecorder) {
	t.Helper()
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Runtime = &contracts.RuntimeInfo{}
	a.state.Queue = []contracts.QueuedMessage{
		{ID: "queue-1", SessionID: "chat-0", Text: "First queued message"},
		{ID: "queue-2", SessionID: "chat-0", Text: "Second queued message"},
	}
	recorder := &queueCommandRecorder{Service: a.service}
	a.service = recorder
	return a, recorder
}

// Send now is a distinct queue-row action: it names the row it interrupts the
// run with, rather than steering the row into the current turn.
func TestQueueRowSendsNow(t *testing.T) {
	a, recorder := queueRowApp(t)
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if _, ok := tt.Find("Send now (interrupt)"); !ok {
		t.Fatal("the queue row has no Send now action")
	}
	if err := tt.Click("Send now (interrupt)"); err != nil {
		t.Fatal(err)
	}
	if len(recorder.commands) != 1 || recorder.commands[0].Kind != "queue.send" || recorder.commands[0].ID != "queue-1" {
		t.Fatalf("queue commands = %+v", recorder.commands)
	}
}

// The queue row drags as a whole: dropping a row over the lower half of
// another moves it after that row, and the command names both rows.
func TestQueueRowDragReorders(t *testing.T) {
	a, recorder := queueRowApp(t)
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	source, ok := tt.Find("First queued message")
	if !ok {
		t.Fatal("the first queue row is not laid out")
	}
	target, ok := tt.Find("Second queued message")
	if !ok {
		t.Fatal("the second queue row is not laid out")
	}
	tt.Press(source.X+source.W/2, source.Y+source.H/2)
	tt.Move(target.X+target.W/2, target.Y+target.H*.8)
	tt.Frame()
	tt.Release(target.X+target.W/2, target.Y+target.H*.8)
	tt.Frame()
	if len(recorder.commands) != 1 || recorder.commands[0].Kind != "queue.move" {
		t.Fatalf("queue commands = %+v", recorder.commands)
	}
	move := *recorder.commands[0].Reorder
	if move.SourceID != "queue-1" || move.TargetID != "queue-2" || !move.After {
		t.Fatalf("move = %+v", move)
	}
}
