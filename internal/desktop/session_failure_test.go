package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/mock"
)

// A failed conversation keeps its marker until the user reaches it: the
// transcript already shows the failure, so the row and the project that owns
// it stop marking it as unseen once it is open.
func TestOpeningAFailedConversationAcknowledgesItsFailure(t *testing.T) {
	a, acknowledged := failureApp(t)
	failed := failedConversation(t, a)
	a.setSelectedSession(failed.ID, false)
	if ids := acknowledged.sessions(); len(ids) != 1 || ids[0] != failed.ID {
		t.Fatalf("acknowledged %v, want %s", ids, failed.ID)
	}
	if status := conversationStatus(t, a, failed.ID); status != contracts.Idle {
		t.Fatalf("a seen failure stayed %s", status)
	}
}

// The failure of the conversation on screen is read as soon as it shows up, and
// only once: a repeated frame never asks the core again.
func TestFailureOnScreenIsAcknowledgedOnce(t *testing.T) {
	a, acknowledged := failureApp(t)
	failed := failedConversation(t, a)
	a.sessionID = failed.ID
	a.acknowledgeVisibleFailures()
	a.acknowledgeVisibleFailures()
	if ids := acknowledged.sessions(); len(ids) != 1 || ids[0] != failed.ID {
		t.Fatalf("acknowledged %v, want %s", ids, failed.ID)
	}
}

// A failure in a conversation the user has not opened keeps its marker.
func TestUnseenFailureKeepsItsMarker(t *testing.T) {
	a, acknowledged := failureApp(t)
	failed := failedConversation(t, a)
	for _, session := range a.state.Sessions {
		if session.ID != failed.ID {
			a.sessionID = session.ID
			break
		}
	}
	a.acknowledgeVisibleFailures()
	if ids := acknowledged.sessions(); len(ids) != 0 {
		t.Fatalf("acknowledged %v without opening it", ids)
	}
	if status := conversationStatus(t, a, failed.ID); status != contracts.Errored {
		t.Fatalf("an unseen failure became %s", status)
	}
}

// failureApp opens the failed-conversation fixture and counts the commands the
// desktop asks the core to settle.
func failureApp(t *testing.T) (*App, *acknowledgedSessions) {
	t.Helper()
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }
	backend, err := mock.New(mock.Options{Scenario: "error", Instant: true, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	acknowledged := &acknowledgedSessions{Service: backend}
	app, err := NewWithOptions(acknowledged, Options{Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app, acknowledged
}

// acknowledgedSessions records the failures the desktop reported as seen.
type acknowledgedSessions struct {
	contracts.Service
	seen []string
}

func (s *acknowledgedSessions) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "session.acknowledge" {
		s.seen = append(s.seen, command.SessionID)
	}
	return s.Service.Execute(ctx, command)
}

func (s *acknowledgedSessions) sessions() []string {
	return append([]string{}, s.seen...)
}

func failedConversation(t *testing.T, a *App) contracts.Session {
	t.Helper()
	for _, session := range a.state.Sessions {
		if session.Status == contracts.Errored {
			return session
		}
	}
	t.Fatal("fixture has no failed conversation")
	return contracts.Session{}
}

func conversationStatus(t *testing.T, a *App, id string) contracts.SessionStatus {
	t.Helper()
	for _, session := range a.state.Sessions {
		if session.ID == id {
			return session.Status
		}
	}
	t.Fatalf("conversation %s is missing", id)
	return contracts.Idle
}
