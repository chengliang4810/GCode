package desktop

import (
	"context"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// sendRecordingService keeps the sends the app dispatches, so a test can read
// the delivery policy they carry.
type sendRecordingService struct {
	contracts.Service
	commands []contracts.Command
}

func (s *sendRecordingService) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "message.send" {
		s.commands = append(s.commands, command.Clone())
	}
	return s.Service.Execute(ctx, command)
}

func setSessionStatus(a *App, status contracts.SessionStatus) {
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].Status = status
		}
	}
}

// A send during a live response follows General.ActiveTurnSendBehavior: steer
// by default, queue when the user asks for it. An idle session sends directly
// whatever the setting says.
func TestActiveTurnSendFollowsTheSetting(t *testing.T) {
	for _, tc := range []struct {
		name, behavior string
		working        bool
		delivery       string
	}{
		{name: "steer by default", behavior: "", working: true, delivery: "steer"},
		{name: "queue when chosen", behavior: "queue", working: true, delivery: ""},
		{name: "idle sends directly", behavior: "steer", working: false, delivery: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.state.Settings.General.ActiveTurnSendBehavior = tc.behavior
			if tc.working {
				setSessionStatus(a, contracts.Working)
			}
			service := &sendRecordingService{Service: a.service}
			a.service = service
			if !a.send("A correction for the run") {
				t.Fatal("the send was refused")
			}
			if len(service.commands) != 1 {
				t.Fatalf("dispatched %d sends, want 1", len(service.commands))
			}
			if got := service.commands[0].Send.Delivery; got != tc.delivery {
				t.Fatalf("delivery = %q, want %q", got, tc.delivery)
			}
		})
	}
}

// The working trailer says which of the two the send is doing.
func TestWorkingTrailerNamesTheSteer(t *testing.T) {
	a := testApp(t, "default")
	a.pendingSends = map[string]primaryPendingSend{"chat-0": {Queued: true, Steer: true}}
	if word, _, visible := a.workingState("chat-0"); !visible || word != "Steering the running response" {
		t.Fatalf("trailer = %q visible=%v", word, visible)
	}
	a.pendingSends["chat-0"] = primaryPendingSend{Queued: true}
	if word, _, _ := a.workingState("chat-0"); word != "Queued — will send automatically" {
		t.Fatalf("queued trailer = %q", word)
	}
}

// The action button names what the key will do: Stop only with an empty
// composer, and the busy send reads as the selected behavior.
func TestComposerButtonReflectsTheActiveTurnSetting(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	setSessionStatus(a, contracts.Working)
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if _, ok := tt.Find("Stop response"); !ok {
		t.Fatal("an empty composer during a run must offer Stop")
	}
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("A correction")
	tt.Frame()
	if _, ok := tt.Find("Steer the running response"); !ok {
		t.Fatal("the default busy send must offer Steer")
	}
	a.state.Settings.General.ActiveTurnSendBehavior = "queue"
	tt.Frame()
	if _, ok := tt.Find("Queue for the next turn"); !ok {
		t.Fatal("the queue setting must offer Queue")
	}
}
