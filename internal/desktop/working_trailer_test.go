package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestWorkingTrailerFollowsSessionAndParksAfterCompletion(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	started := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return started.Add(17 * time.Second) }
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == "chat-0" {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "status-test", SessionID: "chat-0", StartedAt: started, Status: "running"})
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.View(c) }, 1320, 880)
	if !tt.HasText("Activity status: chat-0") || !tt.HasText("17s") {
		t.Fatal("working indicator was not attached to the transcript tail")
	}
	if _, _, visible := a.workingState("chat-1"); visible {
		t.Fatal("another conversation inherited the active indicator")
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == "chat-0" {
			a.state.Sessions[i].Status = contracts.Idle
		}
	}
	tt.Frame()
	if tt.HasText("Activity status: chat-0") {
		t.Fatal("completed run retained a working indicator")
	}
	tt.Advance(time.Second)
	settled := builds
	tt.Advance(3 * time.Second)
	if builds != settled {
		t.Fatal("removed working indicator kept requesting frames")
	}
}

func TestWorkingSendingBridgeAndQueueStayInTheirConversation(t *testing.T) {
	a := testApp(t, "default")
	a.pendingSends = map[string]primaryPendingSend{"chat-0": {RequestID: "pending", StartedAt: a.now()}}
	word, elapsed, visible := a.workingState("chat-0")
	if !visible || word != "Sending…" || elapsed != "" {
		t.Fatal("pending send displayed a stale run timer")
	}
	if _, _, visible := a.workingState("chat-1"); visible {
		t.Fatal("pending send crossed conversations")
	}
	a.pendingSends["chat-0"] = primaryPendingSend{RequestID: "queued", Queued: true}
	word, _, _ = a.workingState("chat-0")
	if word != "Queued — will send automatically" {
		t.Fatal("queued bridge missing")
	}
	a.commandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: "chat-0", RequestID: "older"}}, nil)
	if _, ok := a.pendingSends["chat-0"]; !ok {
		t.Fatal("an old acknowledgment cleared a newer send")
	}
	a.commandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: "chat-0", RequestID: "queued"}}, nil)
	if _, _, visible := a.workingState("chat-0"); visible {
		t.Fatal("acknowledged idle send retained a bridge")
	}
}
