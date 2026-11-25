package desktop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

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
	a.createSideChat(a.sessionID)
	a.win = nil
	select {
	case job := <-a.jobs:
		return job.Command
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
	a.createSideChat("chat-0")
	if a.branchCreation != nil || a.pendingNew || a.errorText != "branch unavailable" {
		t.Fatal("creation error did not release the pending state")
	}
	a.service = backend
	a.createSideChat("chat-0")
	if a.branchCreation != nil || a.workspace.ActiveKind() != "session" {
		t.Fatal("retry did not open the side chat")
	}
}

func TestBranchCreationOpensCopiedTopLevelConversation(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "Parent draft"
	parent := sidebarSession(t, a, "chat-0")
	count := len(a.state.Messages[parent.ID])
	a.branchSession(parent, false)
	opened := sidebarSession(t, a, a.sessionID)
	if a.sessionID == parent.ID || a.sessionID == "" || len(a.state.Messages[opened.ID]) != count || a.branchCreation != nil || a.pendingNew {
		t.Fatal("branch did not open its copied conversation")
	}
	if !opened.TopLevel() || opened.SideChat || opened.ParentSessionID != "" {
		t.Fatalf("branch is not a conversation of its own: %+v", opened)
	}
	if !slices.ContainsFunc(a.visibleSessions(), func(session contracts.Session) bool { return session.ID == opened.ID }) {
		t.Fatal("branch did not appear in the conversation list")
	}
	if opened.Title != "Branch of "+parent.Title || opened.CreationRequestID == "" {
		t.Fatalf("branch did not keep its creation identity: %+v", opened)
	}
	a.selectSession(parent.ID)
	if a.draft != "Parent draft" {
		t.Fatal("branch discarded the parent draft")
	}
}

func TestSidebarBranchMenuOffersBothBranchActions(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	tt := ui.NewTester(a.View, 1320, 880)
	parent := sidebarSession(t, a, "chat-0")
	if err := rightClickAccessible(tt, ui.RoleButton, parent.Title); err != nil {
		t.Fatal(err)
	}
	labels := tt.Menu()
	if !slices.Contains(labels, a.t("Branch chat")) || !slices.Contains(labels, a.t("Branch chat in a new worktree")) {
		t.Fatalf("conversation menu lacks the branch actions: %v", labels)
	}
	if err := tt.ChooseMenuItem(a.t("Branch chat in a new worktree")); err != nil {
		t.Fatal(err)
	}
	opened := sidebarSession(t, a, a.sessionID)
	if opened.ID == parent.ID || !opened.TopLevel() || a.branchCreation != nil {
		t.Fatalf("worktree branch did not open as its own conversation: %+v", opened)
	}
	if opened.WorktreeID == "" || opened.WorktreeID == parent.WorktreeID || opened.Branch == "" || opened.Branch == parent.Branch {
		t.Fatalf("worktree branch did not take a new worktree and branch: %+v", opened)
	}
	if opened.ProjectID != parent.ProjectID {
		t.Fatalf("worktree branch left its project: %+v", opened)
	}
}

func TestSidebarBranchWorktreeNeedsGitRepository(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	parent := sidebarSession(t, a, "chat-0")
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == parent.ProjectID {
			a.state.Projects[i].GitDetected = false
		}
	}
	if a.canBranchSessionWorktree(parent) {
		t.Fatal("a project without Git offered the worktree variant")
	}
	tt := ui.NewTester(a.View, 1320, 880)
	if err := rightClickAccessible(tt, ui.RoleButton, parent.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(a.t("Branch chat in a new worktree")); err == nil {
		t.Fatal("the worktree variant was usable without Git")
	}
	tt.CloseMenu()
	tt.Frame()
	if err := rightClickAccessible(tt, ui.RoleButton, parent.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(a.t("Branch chat")); err != nil {
		t.Fatal(err)
	}
	if opened := sidebarSession(t, a, a.sessionID); opened.ID == parent.ID || opened.WorktreeID != parent.WorktreeID {
		t.Fatalf("plain branch changed its worktree: %+v", opened)
	}
}

type failedBranchSnapshotService struct{ contracts.Service }

func (s failedBranchSnapshotService) Snapshot(context.Context, ...string) (contracts.Snapshot, error) {
	return contracts.Snapshot{}, errors.New("snapshot unavailable")
}

func TestSideCreationSnapshotFailureDoesNotStayPending(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.service = failedBranchSnapshotService{a.service}
	a.createSideChat("chat-0")
	if a.branchCreation != nil || a.workspace.TabCount() != 0 || a.errorText != "snapshot unavailable" {
		t.Fatal("snapshot failure left the entry pending or opened stale content")
	}
}
