package desktop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/i18n"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestNewSideChatEntryOpensIndependentComposerAndSends(t *testing.T) {
	for _, language := range []string{i18n.Chinese, i18n.English} {
		t.Run(language, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.General.Language = language
			a.saveSettings()
			a.selectSession("chat-0")
			a.draft = "Keep the central draft"
			a.expanded["session-info-user"], a.expanded["session-info"] = true, true
			a.setPanel(1, true)
			before := len(a.state.Sessions)
			mainMessages := len(a.state.Messages["chat-0"])
			tt := ui.NewTester(a.View, 1600, 1000)
			if _, ok := tt.Find("New side chat"); ok {
				t.Fatal("session information retained the removed creation action")
			}
			if err := tt.Click(a.t("Side chat")); err != nil {
				t.Fatal(err)
			}
			var child contracts.Session
			for _, session := range a.state.Sessions {
				if session.ParentSessionID == "chat-0" && session.SideChat {
					child = session
					break
				}
			}
			if child.ID == "" || len(a.state.Sessions) != before+1 || a.branchCreation != nil || a.pendingNew {
				t.Fatal("side creation did not settle independently")
			}
			label := a.t("Message to ") + child.Title
			if _, ok := tt.Find(label); !ok || a.workspace.ActiveKind() != "session" || a.sessionID != "chat-0" {
				t.Fatal("new side chat did not open beside its parent")
			}
			if !tt.Focused(label) {
				t.Fatal("new side composer did not receive focus")
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Type("Check this independently")
			tt.Key(0, ui.KeyEnter)
			if a.draft != "Keep the central draft" || a.sessionID != "chat-0" || len(a.state.Messages["chat-0"]) != mainMessages {
				t.Fatal("side send changed the parent conversation")
			}
			user, assistant := false, false
			for _, message := range a.state.Messages[child.ID] {
				if message.Role == "assistant" {
					assistant = true
				}
				for _, block := range message.Blocks {
					if message.Role == "user" && block.Text == "Check this independently" {
						user = true
					}
				}
			}
			if !user || !assistant || a.sideComposer(child.ID).Draft != "" {
				t.Fatal("new side chat did not complete the existing typed send/reply flow")
			}
		})
	}
}

func TestSideChatLauncherLocationsAndIndependentTabs(t *testing.T) {
	for _, language := range []string{i18n.Chinese, i18n.English} {
		for _, appearance := range []string{"light", "dark"} {
			for _, width := range []int{900, 1600} {
				t.Run(fmt.Sprintf("%s/%s/%d", language, appearance, width), func(t *testing.T) {
					a := testApp(t, "default")
					a.state.Settings.General.Language = language
					a.state.Settings.Appearance.Mode = appearance
					a.saveSettings()
					a.selectSession("chat-0")
					a.draft = "Parent draft"
					a.setPanel(1, true)
					tt := ui.NewTester(a.View, width, 900)
					if err := tt.Click(a.t("Side chat")); err != nil {
						t.Fatal(err)
					}
					var first string
					for _, session := range a.state.Sessions {
						if session.SideChat && session.ParentSessionID == "chat-0" {
							first = session.ID
							break
						}
					}
					if first == "" || len(a.state.Messages[first]) != 0 {
						t.Fatal("new side chat did not start with an empty visible transcript")
					}
					a.sideComposer(first).Draft = "First side draft"
					tt.Frame()
					if err := tt.Click(a.t("New surface")); err != nil {
						t.Fatal(err)
					}
					if err := tt.ChooseMenuItem(a.t("Side chat")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if a.workspace.TabCount() != 2 || a.sessionID != "chat-0" || a.draft != "Parent draft" || a.sideComposer(first).Draft != "First side draft" {
						t.Fatal("plus-menu creation reused a child or changed another composer's draft")
					}
					a.expanded["session-info-user"], a.expanded["session-info"] = true, true
					tt.Frame()
					if _, ok := tt.Find("New side chat"); ok {
						t.Fatal("removed creation action returned in session information")
					}
					tt.Frame()
				})
			}
		}
	}
}

func queueBranchCreation(t *testing.T, a *App) contracts.Command {
	t.Helper()
	// Hold just dispatch in its native queue; deliver ACKs/snapshots explicitly.
	a.win = &mygo.Window{}
	a.createRelatedSession(a.sessionID, true)
	a.win = nil
	select {
	case command := <-a.jobs:
		return command
	default:
		t.Fatal("branch command was not queued")
		return contracts.Command{}
	}
}

func TestSideCreationMatchesRequestAcrossEventAndAckOrder(t *testing.T) {
	for _, ackFirst := range []bool{false, true} {
		name := "event-first"
		if ackFirst {
			name = "ack-first"
		}
		t.Run(name, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.draft = "Parent draft"
			command := queueBranchCreation(t, a)
			if err := a.service.Execute(a.ctx, contracts.Command{Kind: "session.sidechat", SessionID: "chat-0", RequestID: "another-client"}); err != nil {
				t.Fatal(err)
			}
			unrelated, _ := a.service.Snapshot(a.ctx)
			a.apply(unrelated)
			if a.branchCreation == nil || a.workspace.TabCount() != 0 {
				t.Fatal("an unrelated child claimed the pending request")
			}
			if err := a.service.Execute(a.ctx, command); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := a.service.Snapshot(a.ctx)
			if ackFirst {
				a.commandFinished(command, nil)
			}
			a.apply(snapshot)
			if !ackFirst {
				a.commandFinished(command, nil)
			}
			a.creationCommandApplied(command, nil)
			a.apply(snapshot)
			tt := ui.NewTester(a.View, 1600, 1000)
			if _, ok := tt.Find("Message to Side chat"); !ok || a.workspace.TabCount() != 1 || a.sessionID != "chat-0" || a.draft != "Parent draft" || a.branchCreation != nil {
				t.Fatal("correlated creation did not open exactly one side chat")
			}
		})
	}
}

func TestLateSideCreationDoesNotTakeOverAnotherConversation(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	command := queueBranchCreation(t, a)
	a.selectSession("chat-1")
	a.draft = "Newer draft"
	if err := a.service.Execute(a.ctx, command); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := a.service.Snapshot(a.ctx)
	a.commandFinished(command, nil)
	a.apply(snapshot)
	a.creationCommandApplied(command, nil)
	if a.sessionID != "chat-1" || a.draft != "Newer draft" || a.workspace.TabCount() != 0 || a.branchCreation != nil {
		t.Fatal("late creation took over the selected conversation")
	}
}

type failedBranchService struct{ contracts.Service }

func (s failedBranchService) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "session.sidechat" {
		return errors.New("branch unavailable")
	}
	return s.Service.Execute(ctx, command)
}

func TestSideCreationFailureCanRetry(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	backend := a.service
	a.service = failedBranchService{backend}
	a.createRelatedSession("chat-0", true)
	if a.branchCreation != nil || a.pendingNew || a.errorText != "branch unavailable" {
		t.Fatal("creation error did not release the pending state")
	}
	a.service = backend
	a.createRelatedSession("chat-0", true)
	if a.branchCreation != nil || a.workspace.ActiveKind() != "session" {
		t.Fatal("retry did not open the side chat")
	}
}

func TestExistingSideChatEntryKeepsParentSelected(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.draft = "Parent draft"
	a.expanded["session-info-user"], a.expanded["session-info"] = true, true
	child := sideSession(t, a, "showcase-agent")
	tt := ui.NewTester(a.View, 1600, 1000)
	if err := tt.Click(child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if _, ok := tt.Find("Message to " + child.Title); !ok || a.sessionID != "chat-0" || a.draft != "Parent draft" {
		t.Fatal("existing side entry navigated over the main conversation")
	}
}

func TestForkEntryOpensCopiedConversationAndRestoresParentDraft(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "Parent draft"
	a.expanded["session-info-user"], a.expanded["session-info"] = true, true
	count := len(a.state.Messages["chat-0"])
	tt := ui.NewTester(a.View, 1600, 1000)
	if err := tt.Click("Fork chat"); err != nil {
		t.Fatal(err)
	}
	if a.sessionID == "chat-0" || a.sessionID == "" || len(a.state.Messages[a.sessionID]) != count || a.branchCreation != nil || a.pendingNew {
		t.Fatal("fork did not open its copied conversation")
	}
	a.selectSession("chat-0")
	if a.draft != "Parent draft" {
		t.Fatal("fork discarded the parent draft")
	}
}

type failedBranchSnapshotService struct{ contracts.Service }

func (s failedBranchSnapshotService) Snapshot(context.Context) (contracts.Snapshot, error) {
	return contracts.Snapshot{}, errors.New("snapshot unavailable")
}

func TestSideCreationSnapshotFailureDoesNotStayPending(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.service = failedBranchSnapshotService{a.service}
	a.createRelatedSession("chat-0", true)
	if a.branchCreation != nil || a.workspace.TabCount() != 0 || a.errorText != "snapshot unavailable" {
		t.Fatal("snapshot failure left the entry pending or opened stale content")
	}
}
