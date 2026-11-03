package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func notificationFixture() contracts.Snapshot {
	return contracts.Snapshot{
		Sessions: []contracts.Session{{ID: "session", Title: "My session", Status: contracts.Working}},
		Runs:     []contracts.Run{{ID: "run", SessionID: "session", Status: "running"}},
		Settings: contracts.Settings{Notifications: contracts.NotificationSettings{
			Enabled: true, Sound: true, Completion: true, Questions: true, Errors: true,
		}},
	}
}

func notificationTestController(snapshot contracts.Snapshot) *desktopNotifications {
	n := newDesktopNotifications(snapshot)
	n.queue = make(chan desktopNotice, 64)
	return n
}

func TestDesktopNotificationsCompletionEdgesAndInitialBaseline(t *testing.T) {
	snapshot := notificationFixture()
	snapshot.Runs[0].Status = "completed"
	snapshot.Sessions[0].Status = contracts.Idle
	snapshot.Sessions[0].LastOutcome = "completed"
	n := notificationTestController(snapshot)
	n.Observe(snapshot, false)
	if len(n.queue) != 0 {
		t.Fatal("startup replayed a completed historical run")
	}
	snapshot.Runs = append(snapshot.Runs, contracts.Run{ID: "new-run", SessionID: "session", Status: "completed"})
	n.Observe(snapshot, false)
	n.Observe(snapshot, false)
	if len(n.queue) != 1 {
		t.Fatalf("new completed run produced %d notices", len(n.queue))
	}
	got := <-n.queue
	if got.Kind != notificationCompleted || got.Body != "My session" || !got.Desktop || !got.Sound {
		t.Fatalf("completion notice: %+v", got)
	}
}

func TestDesktopNotificationsIndependentSettings(t *testing.T) {
	for _, test := range []struct {
		name                         string
		prefs                        contracts.NotificationSettings
		focused, desktop, sound, any bool
	}{
		{"disabled", contracts.NotificationSettings{}, false, false, false, false},
		{"background banner", contracts.NotificationSettings{Enabled: true}, false, true, false, true},
		{"foreground suppressed", contracts.NotificationSettings{Enabled: true}, true, false, false, false},
		{"foreground allowed", contracts.NotificationSettings{Enabled: true, WhenFocused: true}, true, true, false, true},
		{"sound without desktop", contracts.NotificationSettings{Sound: true, Completion: true}, true, false, true, true},
		{"completion category off", contracts.NotificationSettings{Sound: true, Questions: true, Errors: true}, false, false, false, false},
		{"master sound off", contracts.NotificationSettings{Completion: true}, false, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := notificationFixture()
			snapshot.Settings.Notifications = test.prefs
			n := notificationTestController(snapshot)
			snapshot.Runs[0].Status = "completed"
			snapshot.Sessions[0].Status = contracts.Idle
			n.Observe(snapshot, test.focused)
			if (len(n.queue) == 1) != test.any {
				t.Fatalf("notice count: %d", len(n.queue))
			}
			if test.any {
				got := <-n.queue
				if got.Desktop != test.desktop || got.Sound != test.sound {
					t.Fatalf("notice routing: %+v", got)
				}
			}
		})
	}
}

func TestDesktopNotificationsQuestionsErrorsAndInterruptions(t *testing.T) {
	snapshot := notificationFixture()
	n := notificationTestController(snapshot)
	snapshot.Sessions[0].Status = contracts.AwaitingInput
	snapshot.Messages = map[string][]contracts.Message{"session": {{Blocks: []contracts.Block{
		{Question: &contracts.Question{ID: "question-1", Pages: []contracts.QuestionPage{{Question: "Choose", Header: "Choose"}}}},
	}}}}
	n.Observe(snapshot, false)
	n.Observe(snapshot, false)
	snapshot.Messages["session"][0].Blocks[0].Question.ID = "question-2"
	n.Observe(snapshot, false)
	if len(n.queue) != 2 {
		t.Fatalf("distinct unanswered questions: %d", len(n.queue))
	}
	for range 2 {
		if got := <-n.queue; got.Kind != notificationQuestion || !got.Sound {
			t.Fatalf("question routing: %+v", got)
		}
	}
	snapshot.Sessions[0].Status = contracts.Errored
	snapshot.Runs[0].Status = "failed"
	n.Observe(snapshot, false)
	n.Observe(snapshot, false)
	if len(n.queue) != 1 || (<-n.queue).Kind != notificationError {
		t.Fatal("run failure and session error were not deduplicated")
	}
	snapshot.Sessions[0].Status = contracts.Working
	snapshot.Runs[0].Status = "running"
	n.Observe(snapshot, false)
	snapshot.Sessions[0].Status = contracts.Idle
	snapshot.Sessions[0].LastOutcome = "interrupted"
	snapshot.Runs[0].Status = "interrupted"
	n.Observe(snapshot, false)
	if len(n.queue) != 0 {
		t.Fatal("intentional interruption produced a completion/error alert")
	}
}

func TestDesktopNotificationsAvoidSubagentFloodAndReconnectDuplicates(t *testing.T) {
	snapshot := notificationFixture()
	snapshot.Sessions[0].ParentSessionID = "parent"
	snapshot.Sessions[0].SubagentType = "explorer"
	n := notificationTestController(snapshot)
	snapshot.Runs[0].Status = "completed"
	n.Observe(snapshot, false)
	if len(n.queue) != 0 {
		t.Fatal("child run produced its own completion notification")
	}
	for range 2 {
		n.ObserveEvent(contracts.Event{Kind: "connection.disconnected"}, snapshot.Settings, false)
	}
	n.ObserveEvent(contracts.Event{Kind: "connection.connected"}, snapshot.Settings, false)
	n.ObserveEvent(contracts.Event{Kind: "connection.disconnected"}, snapshot.Settings, false)
	if len(n.queue) != 2 {
		t.Fatalf("disconnect episodes produced %d notices", len(n.queue))
	}
}

func TestDesktopNotificationsUserSideChatsAndForks(t *testing.T) {
	for _, conversation := range []struct {
		name string
		side bool
	}{{name: "side chat", side: true}, {name: "fork"}} {
		t.Run(conversation.name, func(t *testing.T) {
			for _, event := range []struct {
				name string
				kind notificationKind
			}{
				{"completion", notificationCompleted},
				{"question", notificationQuestion},
				{"failure", notificationError},
			} {
				t.Run(event.name, func(t *testing.T) {
					snapshot := notificationFixture()
					snapshot.Sessions[0].ParentSessionID = "user-parent"
					snapshot.Sessions[0].SideChat = conversation.side
					n := notificationTestController(snapshot)
					switch event.kind {
					case notificationCompleted:
						snapshot.Sessions[0].Status = contracts.Idle
						snapshot.Runs[0].Status = "completed"
					case notificationQuestion:
						snapshot.Sessions[0].Status = contracts.AwaitingInput
					case notificationError:
						snapshot.Sessions[0].Status = contracts.Errored
						snapshot.Runs[0].Status = "failed"
					}
					n.Observe(snapshot, false)
					n.Observe(snapshot, false)
					if len(n.queue) != 1 {
						t.Fatalf("user conversation produced %d notices", len(n.queue))
					}
					if got := <-n.queue; got.Kind != event.kind {
						t.Fatalf("notice kind = %s, want %s", got.Kind, event.kind)
					}
				})
			}
		})
	}
}

type waitingNotificationSink struct {
	started chan struct{}
}

func (sink waitingNotificationSink) Deliver(ctx context.Context, _ desktopNotice) error {
	close(sink.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestDesktopNotificationsCloseCancelsInFlightDelivery(t *testing.T) {
	snapshot := notificationFixture()
	n := newDesktopNotifications(snapshot)
	sink := waitingNotificationSink{started: make(chan struct{})}
	n.Start(context.Background(), sink)
	t.Cleanup(n.Close)
	snapshot.Runs[0].Status = "completed"
	n.Observe(snapshot, false)
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("delivery worker did not start")
	}
	n.Close()
	select {
	case <-n.done:
	case <-time.After(time.Second):
		t.Fatal("notification worker leaked after window closure")
	}
}

type recordingNotificationSink struct {
	notices chan desktopNotice
}

func (sink recordingNotificationSink) Deliver(_ context.Context, notice desktopNotice) error {
	sink.notices <- notice
	return nil
}

func TestDesktopNotificationsAppliedBusinessSnapshotReachesDelivery(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.state.Settings.Notifications = contracts.NotificationSettings{Enabled: true, Sound: true, Completion: true}
	active := app.state
	active.Sessions = []contracts.Session{{ID: "notification-session", Title: "Actual applied session", Status: contracts.Working}}
	active.Runs = []contracts.Run{{ID: "notification-run", SessionID: "notification-session", Status: "running"}}
	app.notifications = newDesktopNotifications(active)
	sink := recordingNotificationSink{notices: make(chan desktopNotice, 1)}
	app.notifications.Start(app.ctx, sink)
	t.Cleanup(app.notifications.Close)
	finished := active
	finished.Revision++
	finished.Runs = []contracts.Run{{ID: "notification-run", SessionID: "notification-session", Status: "completed"}}
	finished.Sessions = []contracts.Session{{ID: "notification-session", Title: "Actual applied session", Status: contracts.Idle, LastOutcome: "completed"}}
	app.apply(finished)
	app.apply(finished)
	select {
	case notice := <-sink.notices:
		if notice.Kind != notificationCompleted || notice.Body != "Actual applied session" || !notice.Sound || !notice.Desktop {
			t.Fatalf("business snapshot delivery: %+v", notice)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted business snapshot did not reach the delivery sink")
	}
	app.notifications.Close()
	select {
	case <-app.notifications.done:
	case <-time.After(time.Second):
		t.Fatal("delivery worker did not stop")
	}
	if len(sink.notices) != 0 {
		t.Fatal("reapplying the same business revision repeated a notification")
	}
}
