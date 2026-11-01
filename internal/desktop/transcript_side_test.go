package desktop

import (
	"context"
	"errors"
	"gcode/internal/assets"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/mock"
	"github.com/egoist/mygo/ui"
)

type sideRecordingService struct {
	contracts.Service
	commands  []contracts.Command
	sendErr   error
	updateErr error
	stopOnly  bool
}

func (s *sideRecordingService) Execute(ctx context.Context, command contracts.Command) error {
	s.commands = append(s.commands, command.Clone())
	if command.Kind == "message.send" && s.sendErr != nil {
		return s.sendErr
	}
	if command.Kind == "session.update" && s.updateErr != nil {
		return s.updateErr
	}
	if command.Kind == "message.stop" && s.stopOnly {
		return nil
	}
	return s.Service.Execute(ctx, command)
}

func sideSession(t *testing.T, a *App, id string) contracts.Session {
	t.Helper()
	for _, session := range a.state.Sessions {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("missing side session %s", id)
	return contracts.Session{}
}

func sideViewTester(t *testing.T) (*App, *ui.Tester, contracts.Session) {
	t.Helper()
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.draft = "Preserve the central draft"
	child := sideSession(t, a, "showcase-agent")
	a.workspace.OpenSession(child.ID)
	tt := ui.NewTester(a.View, 1600, 1000)
	return a, tt, child
}

func TestSideComposerSendsToChildAndHandlesIME(t *testing.T) {
	a, tt, child := sideViewTester(t)
	recorder := &sideRecordingService{Service: a.service}
	a.service = recorder
	mainCount, childCount := len(a.state.Messages[a.sessionID]), len(a.state.Messages[child.ID])
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Compose("你好", 2)
	tt.Key(0, ui.KeyEnter)
	if len(recorder.commands) != 0 {
		t.Fatal("IME composition Enter sent a message")
	}
	tt.Compose("", 0)
	tt.Type("Review the fixture fields")
	tt.Key(0, ui.KeyEnter)
	if len(a.state.Messages[a.sessionID]) != mainCount || len(a.state.Messages[child.ID]) <= childCount {
		t.Fatal("side send reached the wrong conversation")
	}
	if a.draft != "Preserve the central draft" || a.sideComposer(child.ID).Draft != "" || a.sideComposer(child.ID).Pending != nil {
		t.Fatal("send lost central draft or failed to settle side draft")
	}
	var request *contracts.SendRequest
	for _, command := range recorder.commands {
		if command.Kind == "message.send" {
			request = command.Send
		}
	}
	if request == nil || request.SessionID != child.ID || request.ModelID != child.ModelID || request.ProjectID != child.ProjectID || request.DeviceID != child.DeviceID || request.Text != "Review the fixture fields" {
		t.Fatalf("side request ownership %+v", request)
	}
	if !tt.Focused("Message to " + child.Title) {
		t.Fatal("successful side send lost input focus")
	}
}

// The conversation and the pane share one composer line: the side strip
// reserves the same status strip the central composer carries below it.
func TestSideComposerSharesTheConversationComposerLine(t *testing.T) {
	_, tt, child := sideViewTester(t)
	main, ok := tt.Find("Send message")
	if !ok {
		t.Fatal("conversation send button missing")
	}
	side, ok := tt.Find("Send message to " + child.Title)
	if !ok {
		t.Fatal("side send button missing")
	}
	if delta := main.Y - side.Y; delta > 1 || delta < -1 {
		t.Fatalf("composers sit on different lines: conversation %+v, side %+v", main, side)
	}
}

func TestSideComposerRespectsSendShortcutModes(t *testing.T) {
	for _, test := range []struct {
		name, behavior string
		mods           ui.Modifiers
	}{
		{"shift enter", "shiftEnter", ui.Shift}, {"mod enter", "modEnter", ui.Cmd},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, tt, child := sideViewTester(t)
			a.state.Settings.General.SendBehavior = test.behavior
			tt.Frame()
			if err := tt.Click("Message to " + child.Title); err != nil {
				t.Fatal(err)
			}
			tt.Type("First line")
			before := len(a.state.Messages[child.ID])
			tt.Key(0, ui.KeyEnter)
			if len(a.state.Messages[child.ID]) != before || !strings.Contains(a.sideComposer(child.ID).Draft, "\n") {
				t.Fatal("plain Enter ignored multiline shortcut mode")
			}
			tt.Type("Second line")
			tt.Key(test.mods, ui.KeyEnter)
			if len(a.state.Messages[child.ID]) <= before {
				t.Fatal("configured shortcut did not send")
			}
		})
	}
}

func TestSideComposerFailureKeepsDraftAndRetry(t *testing.T) {
	a, tt, child := sideViewTester(t)
	recorder := &sideRecordingService{Service: a.service, sendErr: errors.New("The request could not be completed.")}
	a.service = recorder
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep my side draft")
	tt.Key(0, ui.KeyEnter)
	state := a.sideComposer(child.ID)
	if state.Draft != "Keep my side draft" || state.Pending != nil || state.Failure == "" {
		t.Fatal("failed side send discarded its draft or stayed pending")
	}
	if a.errorText != "" || a.draft != "Preserve the central draft" {
		t.Fatal("side failure mutated central composer/dialog ownership")
	}
	recorder.sendErr = nil
	if err := tt.Click("Send message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	if state.Draft != "" || state.Failure != "" {
		t.Fatal("retry did not settle the same side composer")
	}
}

func TestSideCompletionDoesNotEraseNewerDraftOrAttachment(t *testing.T) {
	a := testApp(t, "transcript")
	state := a.sideComposer("showcase-agent")
	state.Draft = "Newer draft"
	state.Attachments = []contracts.Attachment{{ID: "submitted"}, {ID: "new"}}
	state.Pending = &sidePendingSend{RequestID: "request-1", Draft: "Submitted draft", Attachments: []contracts.Attachment{{ID: "submitted"}}}
	if a.sideCommandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: "showcase-agent", RequestID: "older-request"}}, nil) {
		t.Fatal("stale request took current draft")
	}
	a.commandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: "showcase-agent", RequestID: "request-1"}}, nil)
	if state.Draft != "Newer draft" || len(state.Attachments) != 1 || state.Attachments[0].ID != "new" || state.Pending != nil {
		t.Fatal("async send completion erased a later edit")
	}
}

func TestSideComposerStopAndEscapeOnlyTargetChild(t *testing.T) {
	a, tt, child := sideViewTester(t)
	a.state.Settings.General.StopWithEscape = true
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID || a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	recorder := &sideRecordingService{Service: a.service, stopOnly: true}
	a.service = recorder
	tt.Frame()
	if err := tt.Click("Stop response in " + child.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if len(recorder.commands) != 2 {
		t.Fatalf("stop commands %+v", recorder.commands)
	}
	for _, command := range recorder.commands {
		if command.Kind != "message.stop" || command.SessionID != child.ID {
			t.Fatalf("side stop leaked to central session: %+v", command)
		}
	}
}

func TestSideTabsKeepDraftAndReleaseHiddenFocus(t *testing.T) {
	a, tt, child := sideViewTester(t)
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("First tab draft")
	other := sideSession(t, a, "chat-1")
	a.workspace.OpenSession(other.ID)
	tt.Frame()
	if tt.Focused("Message to " + child.Title) {
		t.Fatal("hidden side input kept focus")
	}
	if err := tt.Click("Message to " + other.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("Second tab draft")
	a.workspace.OpenSession(child.ID)
	tt.Frame()
	if a.sideComposer(child.ID).Draft != "First tab draft" || a.sideComposer(other.ID).Draft != "Second tab draft" {
		t.Fatal("side drafts mixed across tabs")
	}
	a.workspace.CloseActive()
	tt.Frame()
	if tt.Focused("Message to " + child.Title) {
		t.Fatal("closed side input retained focus")
	}
	a.workspace.OpenSession(child.ID)
	tt.Frame()
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "First tab draft" {
		t.Fatalf("reopened native editor restored %q", tt.Clipboard())
	}

}

func TestSideNativePickerStagesOnlyChildAttachments(t *testing.T) {
	a, tt, child := sideViewTester(t)
	path := filepath.Join(t.TempDir(), "side-note.txt")
	if err := os.WriteFile(path, []byte("Inspect the schema"), 0600); err != nil {
		t.Fatal(err)
	}
	a.pickAttachmentFiles = func() ([]string, error) { return []string{path}, nil }
	if err := tt.Click("Attach files to " + child.Title); err != nil {
		t.Fatal(err)
	}
	state := a.sideComposer(child.ID)
	if len(state.Attachments) != 1 || state.Attachments[0].Name != "side-note.txt" || len(a.attachments) != 0 || state.Importing != 0 {
		t.Fatal("native picker staged attachment in wrong composer")
	}
	if err := tt.Click("Remove side attachment: side-note.txt"); err != nil {
		t.Fatal(err)
	}
	if len(state.Attachments) != 0 {
		t.Fatal("side attachment removal did not apply")
	}
}

func TestSideRunningSessionQueuesAndEditsItsOwnMessage(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	backend, err := mock.New(mock.Options{Scenario: "live", TokenInterval: time.Hour, Now: func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	useEnglishSettings(t, backend)
	a, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	a.selectSession("chat-1")
	a.draft = "Preserve central draft"
	child := sideSession(t, a, "chat-0")
	a.workspace.OpenSession(child.ID)
	tt := ui.NewTester(a.View, 1600, 1000)
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("Queue the next check")
	tt.Key(0, ui.KeyEnter)
	if len(a.state.Queue) != 1 || a.state.Queue[0].SessionID != child.ID || a.sideComposer(child.ID).Draft != "" {
		t.Fatal("running side session did not queue its own message")
	}
	if err := tt.Click("Edit queued side message"); err != nil {
		t.Fatal(err)
	}
	if a.sideComposer(child.ID).Draft != "Queue the next check" || len(a.state.Queue) != 0 || a.draft != "Preserve central draft" {
		t.Fatal("queue edit crossed composer ownership")
	}
}
