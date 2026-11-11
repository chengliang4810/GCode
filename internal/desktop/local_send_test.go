package desktop

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func holdSendJobs(t *testing.T, a *App) {
	t.Helper()
	a.win = &mygo.Window{}
	t.Cleanup(func() { a.win = nil })
}

func nextSendJob(t *testing.T, a *App) contracts.Command {
	t.Helper()
	select {
	case job := <-a.jobs:
		a.win = nil
		if job.Kind != "message.send" {
			t.Fatalf("expected send, got %s", job.Kind)
		}
		return job.Command
	default:
		t.Fatal("send was not dispatched")
		return contracts.Command{}
	}
}

func requestMessages(a *App, sessionID, requestID string) []contracts.Message {
	return slices.DeleteFunc(slices.Clone(a.projectedMessages(sessionID)), func(m contracts.Message) bool {
		return m.RequestID != requestID
	})
}

func TestLocalSendShowsBubbleBeforeCoreAccepts(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{900, 1320} {
			for _, session := range []string{"", "chat-0"} {
				t.Run(fmt.Sprintf("%s/%d/%s", theme, width, session), func(t *testing.T) {
					a := testApp(t, "default")
					if session != "" {
						a.selectSession(session)
					}
					a.state.Settings.Appearance.Mode = theme
					a.draft = "Immediately visible user message"
					a.attachments = []contracts.Attachment{{ID: "file", Name: "notes.txt", MIME: "text/plain"}}
					before := len(a.state.Messages[session])
					holdSendJobs(t, a)
					if !a.send(a.draft) {
						t.Fatal("send refused")
					}
					command := nextSendJob(t, a)
					if a.draft != "" || len(a.attachments) != 0 || len(a.state.Messages[session]) != before {
						t.Fatal("send did not clear composer, or changed core state")
					}
					tt := ui.NewTester(a.View, width, 880)
					// No elapsed time or service execution: the very first
					// post-send build must contain the bubble and attachment.
					if !tt.HasText(command.Send.Text) || !tt.HasText("notes.txt") {
						t.Fatal("first frame omitted the pending bubble or attachment")
					}
					bubbles := requestMessages(a, session, command.Send.RequestID)
					if len(bubbles) != 1 || len(bubbles[0].Blocks) != 2 {
						t.Fatalf("pending message = %+v", bubbles)
					}
					snapshot := contracts.CloneSnapshot(a.state)
					snapshot.Revision++
					a.apply(snapshot)
					if len(requestMessages(a, session, command.Send.RequestID)) != 1 {
						t.Fatal("unrelated snapshot erased the local bubble")
					}
				})
			}
		}
	}
}

func TestLocalSendConfirmationDoesNotDuplicate(t *testing.T) {
	for _, newSession := range []bool{false, true} {
		for _, ackFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("new=%t/ackFirst=%t", newSession, ackFirst), func(t *testing.T) {
				a := testApp(t, "default")
				if !newSession {
					a.selectSession("chat-0")
				}
				holdSendJobs(t, a)
				a.draft = "Repeated text"
				if !a.send(a.draft) {
					t.Fatal("send refused")
				}
				command := nextSendJob(t, a)
				req := command.Send
				snapshot := contracts.CloneSnapshot(a.state)
				snapshot.Revision++
				sessionID := req.SessionID
				if newSession {
					sessionID = "created"
					snapshot.Sessions = append(snapshot.Sessions, contracts.Session{ID: sessionID})
				}
				snapshot.Runs = append(snapshot.Runs, contracts.Run{ID: "accepted", RequestID: req.RequestID, SessionID: sessionID})
				message := contracts.Message{ID: "accepted-user", RequestID: req.RequestID, SessionID: sessionID, Role: "user",
					Blocks: []contracts.Block{{ID: "accepted-text", Kind: "text", Text: req.Text}}}
				snapshot.Messages[sessionID] = append(snapshot.Messages[sessionID], message)
				if ackFirst {
					a.commandFinished(command, nil)
					if len(requestMessages(a, req.SessionID, req.RequestID)) != 1 {
						t.Fatal("acknowledgment erased bubble before the snapshot")
					}
				}
				a.applyEvent(contracts.Event{Kind: "run.started", RequestID: req.RequestID, SessionID: sessionID, RunID: "accepted", Snapshot: &snapshot})
				if !ackFirst {
					a.commandFinished(command, nil)
				}
				if len(requestMessages(a, sessionID, req.RequestID)) != 1 || a.localSendByRequest(req.RequestID) != nil {
					t.Fatal("confirmation duplicated or retained the local bubble")
				}
			})
		}
	}
}

func TestLocalSendFailureRetainsContentAndRetryPreservesNewDraft(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	holdSendJobs(t, a)
	a.draft = "Original failed message"
	a.attachments = []contracts.Attachment{{ID: "original", Name: "original.txt", MIME: "text/plain"}}
	a.send(a.draft)
	command := nextSendJob(t, a)
	a.draft = "New draft after sending"
	a.attachments = []contracts.Attachment{{ID: "new", Name: "new.txt", MIME: "text/plain"}}
	a.commandFinished(command, errors.New("delivery failed"))
	tt := ui.NewTester(a.View, 1320, 880)
	if !tt.HasText(command.Send.Text) || !tt.HasText("Send failed") {
		t.Fatal("failure discarded the message or its status")
	}
	if _, ok := tt.Find("Retry"); !ok || a.errorText != "" {
		t.Fatal("failure row has no retry action, or is covered by a dialog")
	}
	holdSendJobs(t, a)
	a.retryLocalSend(command.Send.RequestID)
	retry := nextSendJob(t, a)
	if retry.Send.RequestID != command.Send.RequestID || retry.Send.Text != command.Send.Text ||
		!slices.Equal(retry.Send.Attachments, command.Send.Attachments) {
		t.Fatal("retry did not preserve the original request")
	}
	if a.draft != "New draft after sending" || len(a.attachments) != 1 || a.attachments[0].ID != "new" {
		t.Fatal("retry overwrote the newer draft or attachment")
	}
}

func TestLocalSendQueueAndIdenticalTextUseRequestIdentity(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	holdSendJobs(t, a)
	a.send("Identical")
	first := nextSendJob(t, a)
	holdSendJobs(t, a)
	a.send("Identical")
	second := nextSendJob(t, a)
	snapshot := contracts.CloneSnapshot(a.state)
	snapshot.Revision++
	snapshot.Queue = append(snapshot.Queue, contracts.QueuedMessage{
		ID: "queue-accepted", RequestID: first.Send.RequestID, SessionID: a.sessionID, Text: "Identical",
	})
	a.apply(snapshot)
	if len(requestMessages(a, a.sessionID, first.Send.RequestID)) != 0 ||
		len(requestMessages(a, a.sessionID, second.Send.RequestID)) != 1 {
		t.Fatal("queue acknowledgment matched the wrong identical message")
	}
	a.commandFinished(first, nil)
	a.commandFinished(second, errors.New("second failed"))
	if send := a.localSendByRequest(second.Send.RequestID); send == nil || send.failure == "" {
		t.Fatal("older acknowledgment discarded a later send")
	}
}

func TestLocalSendCreationKeepsBubbleUntilHistoryLoads(t *testing.T) {
	a := testApp(t, "default")
	holdSendJobs(t, a)
	a.send("New conversation message")
	command := nextSendJob(t, a)
	a.commandFinished(command, nil)
	snapshot := contracts.CloneSnapshot(a.state)
	snapshot.Revision++
	snapshot.Sessions = append(snapshot.Sessions, contracts.Session{ID: "created"})
	snapshot.Runs = append(snapshot.Runs, contracts.Run{ID: "new-run", RequestID: command.Send.RequestID, SessionID: "created"})
	a.apply(snapshot)
	if len(requestMessages(a, "created", command.Send.RequestID)) != 1 {
		t.Fatal("new-session metadata removed the bubble before scoped history")
	}
	a.sessionID = "" // Starting another canvas must abandon the old projection.
	a.newConversation()
	a.commandFinished(command, errors.New("late error"))
	a.reconcileLocalSends()
	if len(a.localSends) != 0 || a.draft != "" {
		t.Fatal("late failure crossed canvas generations")
	}
}

func TestLocalSendSideBubbleAndFailurePreserveNewerInput(t *testing.T) {
	a, tt, child := sideViewTester(t)
	holdSendJobs(t, a)
	state := a.sideComposer(child.ID)
	state.Draft = "Immediate side message"
	if !a.sendSideMessage(child, state, state.Draft) {
		t.Fatal("side send refused")
	}
	command := nextSendJob(t, a)
	tt.Frame()
	if state.Draft != "" || !tt.HasText(command.Send.Text) || len(requestMessages(a, child.ID, command.Send.RequestID)) != 1 {
		t.Fatal("side composer did not immediately hand content to the bubble")
	}
	state.Draft = "Newer side message"
	state.Attachments = []contracts.Attachment{{ID: "later"}}
	a.commandFinished(command, errors.New("side delivery failed"))
	tt.Frame()
	if state.Draft != "Newer side message" || len(state.Attachments) != 1 || !tt.HasText("Send failed") {
		t.Fatal("side failure lost a later edit or the failed bubble")
	}
}

func TestLocalSendSideSuccessKeepsIdenticalNewDraft(t *testing.T) {
	a := testApp(t, "transcript")
	child := sideSession(t, a, "showcase-agent")
	state := a.sideComposer(child.ID)
	holdSendJobs(t, a)
	state.Draft = "Same text typed again"
	state.Attachments = []contracts.Attachment{{ID: "same-file", Name: "same.txt"}}
	a.sendSideMessage(child, state, state.Draft)
	command := nextSendJob(t, a)
	state.Draft = command.Send.Text
	state.Attachments = slices.Clone(command.Send.Attachments)
	a.commandFinished(command, nil)
	if state.Draft != command.Send.Text || !slices.Equal(state.Attachments, command.Send.Attachments) {
		t.Fatal("completion erased an identical draft entered after sending")
	}
}

func TestLocalSendDeletedSessionRejectsLateFailure(t *testing.T) {
	a := testApp(t, "default")
	a.draft = "Independent canvas"
	a.selectSession("chat-0")
	holdSendJobs(t, a)
	a.send("Message in a deleted conversation")
	command := nextSendJob(t, a)
	snapshot := contracts.CloneSnapshot(a.state)
	snapshot.Revision++
	snapshot.Sessions = slices.DeleteFunc(snapshot.Sessions, func(s contracts.Session) bool { return s.ID == "chat-0" })
	delete(snapshot.Messages, "chat-0")
	a.apply(snapshot)
	a.commandFinished(command, errors.New("late failure"))
	if len(a.localSends) != 0 || a.draft != "Independent canvas" || a.errorText != "" {
		t.Fatal("deleted conversation's failure changed the new canvas")
	}
}

func TestLocalSendAckWithoutSnapshotKeepsBubble(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	holdSendJobs(t, a)
	a.send("Accepted, but the snapshot request failed")
	command := nextSendJob(t, a)
	a.commandFinished(command, nil)
	// No creationCommandApplied call: the worker could not fetch the command
	// snapshot. A later unrelated event must not erase the pending projection.
	snapshot := contracts.CloneSnapshot(a.state)
	snapshot.Revision++
	a.apply(snapshot)
	if len(requestMessages(a, a.sessionID, command.Send.RequestID)) != 1 {
		t.Fatal("successful acknowledgment without history erased the bubble")
	}
	// Commands may validly update state without adding a user message. Their
	// applied command snapshot still settles that temporary projection.
	a.creationCommandApplied(command, nil)
	if a.localSendByRequest(command.Send.RequestID) != nil {
		t.Fatal("applied command snapshot did not settle a message-less command")
	}
}
