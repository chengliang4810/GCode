package desktop

import (
	"context"
	"errors"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestModEnterSettingControlsSubmission(t *testing.T) {
	a := testApp(t, "default")
	initialSessions := len(a.state.Sessions)
	a.state.Settings.General.SendBehavior = "modEnter"
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep this as a draft")
	tt.Key(0, ui.KeyEnter)
	if a.sessionID != "" || len(a.state.Sessions) != initialSessions {
		t.Fatal("plain Enter submitted despite modifier+Enter setting")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	if a.sessionID == "" {
		t.Fatal("modifier+Enter did not submit")
	}
}

func TestSessionPanelsAttachmentsAndWorkspaceStayScoped(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "first"
	a.attachments = []contracts.Attachment{{ID: "first-attachment", Name: "README.md"}}
	a.files = true
	a.right = true
	a.terminal = true
	a.selectSession("chat-6")
	if a.draft != "" || len(a.attachments) != 0 || a.files || a.right || a.terminal {
		t.Fatal("previous session view leaked into a new session")
	}
	if a.workspace.ProjectID != "rcode" || a.bottom.ProjectID != "rcode" {
		t.Fatal("workspace did not follow session project")
	}
	a.draft = "second"
	a.selectSession("chat-0")
	if a.draft != "first" || len(a.attachments) != 1 || !a.files || !a.right || !a.terminal {
		t.Fatal("session view did not restore")
	}
	if a.branch != a.currentSession().Branch || a.workspace.ProjectID != "fieldnotes" {
		t.Fatal("session branch/project was not restored")
	}
	a.newConversation()
	if a.sessionID != "" || a.draft != "" || len(a.attachments) != 0 {
		t.Fatal("new session retained an earlier draft")
	}
}

type failingSendService struct{ contracts.Service }

func (s failingSendService) Execute(ctx context.Context, cmd contracts.Command) error {
	if cmd.Kind == "message.send" {
		return errors.New("send failed")
	}
	return s.Service.Execute(ctx, cmd)
}

func TestFailedNewSendRestoresComposer(t *testing.T) {
	a := testApp(t, "default")
	a.service = failingSendService{a.service}
	a.draft = "keep this prompt"
	a.attachments = []contracts.Attachment{{ID: "draft-file", Name: "draft.md"}}
	a.send(a.draft)
	if a.pendingNew || a.draft != "keep this prompt" || len(a.attachments) != 1 {
		t.Fatal("failed send lost draft or kept pending selection")
	}
}

func TestDispatchFreezesQueuedCommand(t *testing.T) {
	a := testApp(t, "default")
	a.win = &mygo.Window{}
	defer func() { a.win = nil }()
	s := a.state.Settings
	a.dispatch(contracts.Command{Kind: "settings.update", Settings: &s})
	s.Shortcuts[0].Keys = "changed after enqueue"
	s.Appearance.FontSize = 20
	queued := <-a.jobs
	if queued.Settings.Shortcuts[0].Keys == "changed after enqueue" || queued.Settings.Appearance.FontSize == 20 {
		t.Fatal("queued command retained caller-owned memory")
	}
}

func TestFinalSettingsResponseReconcilesConcurrentResourceMutation(t *testing.T) {
	a := testApp(t, "default")
	a.settingsPending = 1
	desired := copySettings(a.state.Settings)
	desired.Appearance.Accent = "orange"
	a.desiredSettings = &desired
	snapshot := a.state
	snapshot.Revision++
	snapshot.Settings = copySettings(desired)
	snapshot.Settings.Appearance.Background = "new-background.png"
	a.apply(snapshot)
	if a.state.Settings.Appearance.Background != "" {
		t.Fatal("optimistic update did not remain stable while pending")
	}
	a.commandFinished(contracts.Command{Kind: "settings.update"}, nil)
	a.apply(snapshot)
	if a.state.Settings.Appearance.Background != "new-background.png" || a.state.Settings.Appearance.Accent != "orange" {
		t.Fatal("equal-revision completion lost an authoritative backend preference")
	}
}
