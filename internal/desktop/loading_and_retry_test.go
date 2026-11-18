package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// A detail load that settles inside the reveal delay keeps the resting layout,
// so a cached or fast switch never flashes a spinner.
func TestLoadingPlaceholderWaitsForTheRevealDelay(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		a.chatView(c)
	}, 900, 700)
	// The panes read the app clock for the delay, so it has to be the frame's
	// clock for Advance to mean anything.
	a.now = tt.Now
	delete(a.state.Messages, "chat-0")
	delete(a.sessionDetailsLoading, "chat-0")
	delete(a.sessionDetailsLoadingSince, "chat-0")
	a.markSessionDetailsLoading([]string{"chat-0"})
	tt.Frame()
	if tt.HasText("Loading conversation…") {
		t.Fatal("a fresh load showed its spinner before the reveal delay")
	}
	tt.Advance(100 * time.Millisecond)
	if tt.HasText("Loading conversation…") {
		t.Fatal("the loading state appeared before the reveal delay")
	}
	tt.Advance(100 * time.Millisecond)
	if !tt.HasText("Loading conversation…") {
		t.Fatalf("the loading state never appeared after the reveal delay: %v", tt.Texts())
	}
	// The settled load leaves the pane as it was.
	a.sessionDetailsLoaded("chat-0")
	tt.Frame()
	if tt.HasText("Loading conversation…") {
		t.Fatal("a settled load kept the loading state")
	}
}

// recordingService keeps the commands the UI dispatches while delegating every
// call to the fixture backend.
type recordingService struct {
	contracts.Service
	executed []contracts.Command
}

func (s *recordingService) Execute(ctx context.Context, cmd contracts.Command) error {
	s.executed = append(s.executed, cmd)
	return s.Service.Execute(ctx, cmd)
}

func retryFixture(t *testing.T, status string) (*App, *recordingService, contracts.Message) {
	t.Helper()
	a := testApp(t, "default")
	a.selectSession("chat-0")
	recorder := &recordingService{Service: a.service}
	a.service = recorder
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == "chat-0" {
			a.state.Sessions[i].LastOutcome = status
		}
	}
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "audit-run", SessionID: "chat-0", Status: status, StartedAt: time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)})
	return a, recorder, activityFixture()
}

// The footer offers a retry only for the reply whose own turn failed or was
// interrupted, and the click dispatches the engine's idempotent retry.
func TestRetryFooterDispatchesRetryForFailedReply(t *testing.T) {
	for _, status := range []string{runStatusFailed, runStatusInterrupted} {
		t.Run(status, func(t *testing.T) {
			a, recorder, message := retryFixture(t, status)
			a.palette = design.Palette("rcode-light", false)
			tt := transcriptTester(t, a, &message, 760, 900, false)
			if _, ok := tt.Find("Retry response"); !ok {
				t.Fatalf("a %s reply has no retry action: %v", status, tt.Texts())
			}
			if err := tt.Click("Retry response"); err != nil {
				t.Fatal(err)
			}
			if len(recorder.executed) != 1 {
				t.Fatalf("retry dispatched %d commands", len(recorder.executed))
			}
			if got := recorder.executed[0]; got.Kind != "message.retry" || got.SessionID != "chat-0" {
				t.Fatalf("retry dispatched %+v", got)
			}
		})
	}
}

// A completed reply keeps the footer it always had: copy and the date.
func TestRetryFooterAbsentForCompletedReply(t *testing.T) {
	a, recorder, message := retryFixture(t, "completed")
	a.palette = design.Palette("rcode-light", false)
	tt := transcriptTester(t, a, &message, 760, 900, false)
	if tt.HasText("Retry response") {
		t.Fatal("a completed reply offers a retry")
	}
	if err := tt.Click("Copy message"); err != nil {
		t.Fatal(err)
	}
	if len(recorder.executed) != 0 {
		t.Fatalf("the completed footer dispatched %+v", recorder.executed)
	}
}

// A stop request already sent is the button's own label, so the reader sees
// that the run is stopping before it ends.
func TestStoppingLabelReflectsPendingStop(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.palette = design.Palette("rcode-light", false)
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "live-run", SessionID: a.sessionID, Status: runStatusRunning, StartedAt: time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		a.chatView(c)
	}, 900, 700)
	if !tt.HasText("Stop response") {
		t.Fatalf("a running turn lost its stop action: %v", tt.Texts())
	}
	a.state.Runs[len(a.state.Runs)-1].Status = runStatusStopping
	tt.Frame()
	if !tt.HasText("Stopping…") {
		t.Fatalf("the stop action did not report the pending stop: %v", tt.Texts())
	}
	if tt.HasText("Stop response") {
		t.Fatal("the stop action still asks for a stop that was already sent")
	}
}
