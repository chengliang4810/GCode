package desktop

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestWorkingRetryVisibilityThresholdAndReset(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.state.Settings.Appearance.Mode = appearance
			a.state.Settings.Appearance.ReduceMotion = true
			a.state.Settings.General.Language = "zh-CN"
			var index int
			for i := range a.state.Sessions {
				if a.state.Sessions[i].ID == "chat-0" {
					index = i
				}
			}
			session := &a.state.Sessions[index]
			session.Status = contracts.Working
			tt := ui.NewTester(func(c *ui.Context) { a.View(c) }, 900, 600)
			for _, attempt := range []int{1, 2, 3, 10} {
				session.ModelRetry = &model.RetryStatus{Attempt: attempt, MaxRetries: 10, HTTPStatus: 503}
				tt.Frame()
				caption := fmt.Sprintf("重新连接中... %d/10", attempt)
				if tt.HasText(caption) != (attempt >= 3) {
					t.Fatalf("attempt %d has wrong loading status", attempt)
				}
				tt.Advance(2 * time.Second)
				if attempt >= 3 && !tt.HasText(caption) {
					t.Fatal("retry label changed without a new attempt")
				}
			}
			session.ModelRetry = nil
			tt.Frame()
			if tt.HasText("重新连接中... 10/10") {
				t.Fatal("successful stream retained retry status")
			}
			session.ModelRetry = &model.RetryStatus{Attempt: 3, MaxRetries: 10}
			session.Status = contracts.Idle
			tt.Frame()
			if tt.HasText("重新连接中... 3/10") {
				t.Fatal("idle session displayed a stale retry")
			}
		})
	}
}

func TestSideRetryStatusIsIndependent(t *testing.T) {
	a, tt, child := sideViewTester(t)
	for i := range a.state.Sessions {
		session := &a.state.Sessions[i]
		if session.ID == a.sessionID {
			session.Status = contracts.Working
			session.ModelRetry = &model.RetryStatus{Attempt: 4, MaxRetries: 10}
		}
		if session.ID == child.ID {
			session.Status = contracts.Working
			session.ModelRetry = &model.RetryStatus{Attempt: 8, MaxRetries: 10}
		}
	}
	tt.Frame()
	if !tt.HasText("Reconnecting... 4/10") || !tt.HasText("Reconnecting... 8/10") {
		t.Fatal("main and side chat lost independent retry status")
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].ModelRetry = nil
		}
	}
	tt.Frame()
	if !tt.HasText("Reconnecting... 4/10") || tt.HasText("Reconnecting... 8/10") {
		t.Fatal("settling child retry affected its parent")
	}
}

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

func TestSideWorkingTrailerUsesChildRunAndSettlesWithIt(t *testing.T) {
	a, tt, child := sideViewTester(t)
	started := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return started.Add(17 * time.Second) }
	for i := range a.state.Sessions {
		switch a.state.Sessions[i].ID {
		case a.sessionID:
			a.state.Sessions[i].Status = contracts.Working
		case child.ID:
			a.state.Sessions[i].Status = contracts.Idle
		}
	}
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "main-run", SessionID: a.sessionID, StartedAt: started, Status: "running"})
	tt.Frame()
	if tt.HasText("Activity status: " + child.ID) {
		t.Fatal("idle child inherited the central conversation's activity")
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "child-run", SessionID: child.ID, StartedAt: started.Add(6 * time.Second), Status: "running"})
	tt.Frame()
	if !tt.HasText("Activity status: "+child.ID) || !tt.HasText("11s") {
		t.Fatal("side transcript did not show its own working trailer and timer")
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].Status = contracts.Idle
		}
	}
	tt.Frame()
	if tt.HasText("Activity status: "+child.ID) || !tt.HasText("Activity status: "+a.sessionID) {
		t.Fatal("settling the child affected the wrong activity trailer")
	}
}

func TestSidePendingTrailerSurvivesEmptyTranscriptAndStaleAcknowledgment(t *testing.T) {
	for _, test := range []struct {
		name, word string
		queued     bool
	}{
		{name: "sending", word: "Sending…"},
		{name: "queued", word: "Queued — will send automatically", queued: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, tt, child := sideViewTester(t)
			a.state.Messages[child.ID] = nil
			for i := range a.state.Sessions {
				if a.state.Sessions[i].ID == child.ID {
					a.state.Sessions[i].Status = contracts.Idle
				}
			}
			state := a.sideComposer(child.ID)
			state.Draft = "Keep this draft"
			state.Pending = &sidePendingSend{RequestID: "pending", Draft: state.Draft, Queued: test.queued}
			tt.Frame()
			if !tt.HasText("Activity status: "+child.ID) || !tt.HasText(test.word) {
				t.Fatal("pending side send was hidden by its empty transcript")
			}
			if _, elapsed, _ := a.workingState(child.ID); elapsed != "" {
				t.Fatal("pending side send inherited a previous turn's timer")
			}
			a.commandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: child.ID, RequestID: "older"}}, nil)
			tt.Frame()
			if !tt.HasText("Activity status: " + child.ID) {
				t.Fatal("stale acknowledgment dismissed the newer pending trailer")
			}
			a.commandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: child.ID, RequestID: "pending"}}, errors.New("Delivery failed"))
			tt.Frame()
			if tt.HasText("Activity status: "+child.ID) || state.Draft != "Keep this draft" || !tt.HasText("Delivery failed") {
				t.Fatal("failed side send retained progress or discarded its draft/error")
			}
		})
	}
}
