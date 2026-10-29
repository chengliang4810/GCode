package desktop

import (
	"context"
	"errors"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestDeletedSelectedSessionReturnsToCanvasAndCanSend(t *testing.T) {
	a := testApp(t, "default")
	a.draft = "Canvas draft before opening a session"
	a.selectSession("chat-0")
	a.draft = "Deleted session draft"
	a.right, a.files, a.rightExpanded = true, true, true
	a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
	if a.sessionID != "" || a.draft != "Canvas draft before opening a session" {
		t.Fatalf("deleted selection did not restore canvas: id=%q draft=%q", a.sessionID, a.draft)
	}
	tt := ui.NewTester(a.View, 1320, 880)
	if _, ok := tt.Find("Toggle right pane"); ok {
		t.Fatal("deleted selection retained session chrome")
	}
	if _, ok := tt.Find("Context usage"); ok {
		t.Fatal("deleted session context leaked onto the canvas")
	}
	if !a.send("A new conversation after deletion") || a.sessionID == "" || a.sessionID == "chat-0" || a.errorText != "" {
		t.Fatalf("new canvas send targeted deleted session: id=%q error=%q", a.sessionID, a.errorText)
	}
}

func TestSnapshotSelectionReconciliationUsesAcceptedState(t *testing.T) {
	for _, tc := range []struct {
		name             string
		revisionDelta    int
		missingInCurrent bool
		removeIncoming   bool
		archiveIncoming  bool
		wantCanvas       bool
	}{
		{name: "newer removes selected", revisionDelta: 1, removeIncoming: true, wantCanvas: true},
		{name: "equal cannot remove present selected", removeIncoming: true},
		{name: "older cannot remove present selected", revisionDelta: -1, removeIncoming: true},
		{name: "unchanged frame heals missing selection", missingInCurrent: true, wantCanvas: true},
		{name: "older frame cannot resurrect missing selection", revisionDelta: -1, missingInCurrent: true, wantCanvas: true},
		{name: "archived but present remains selected", revisionDelta: 1, archiveIncoming: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			snapshot, err := a.service.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Revision = uint64(int64(a.state.Revision) + int64(tc.revisionDelta))
			if tc.removeIncoming {
				snapshot.Sessions = slices.DeleteFunc(snapshot.Sessions, func(s contracts.Session) bool { return s.ID == "chat-0" })
			}
			if tc.archiveIncoming {
				for i := range snapshot.Sessions {
					if snapshot.Sessions[i].ID == "chat-0" {
						snapshot.Sessions[i].Archived = true
					}
				}
			}
			if tc.missingInCurrent {
				a.state.Sessions = slices.DeleteFunc(a.state.Sessions, func(s contracts.Session) bool { return s.ID == "chat-0" })
			}
			a.apply(snapshot)
			if (a.sessionID == "") != tc.wantCanvas {
				t.Fatalf("selection=%q, canvas=%v", a.sessionID, tc.wantCanvas)
			}
		})
	}
}

func TestDeletedSelectionPreservesSettingsAndPullRequests(t *testing.T) {
	for _, route := range []string{"settings/Appearance", "pull-requests"} {
		t.Run(route, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.OpenRoute(route)
			tt := ui.NewTester(a.View, 1320, 880)
			beforeRoute, beforeKind := a.route, a.workspace.ActiveKind()
			a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
			tt.Frame()
			if a.sessionID != "" || a.route != beforeRoute {
				t.Fatalf("deletion changed page or retained missing selection: id=%q route=%q", a.sessionID, a.route)
			}
			if route == "pull-requests" {
				if a.workspace.ActiveKind() != beforeKind || !tt.HasText("Search pull requests") {
					t.Fatal("clearing chat selection replaced the active pull-request surface")
				}
				a.OpenRoute("chat")
				tt.Frame()
				if _, ok := tt.Find("Message"); !ok {
					t.Fatal("returning from pull requests did not restore the canvas")
				}
			} else if a.section != "Appearance" || !tt.HasText("Appearance") {
				t.Fatal("clearing chat selection lost the settings section")
			}
		})
	}
}

func TestCanvasTargetSurvivesSessionBrowsingAndRemoval(t *testing.T) {
	for _, projectID := range []string{"backend", ""} {
		t.Run("project="+projectID, func(t *testing.T) {
			a := testApp(t, "default")
			a.projectID, a.deviceID = projectID, "remote"
			a.selectSession("chat-0")
			a.selectSession("chat-6")
			a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-6"})
			if a.sessionID != "" || a.projectID != projectID || a.deviceID != "remote" {
				t.Fatalf("canvas target overwritten by session browsing: id=%q project=%q device=%q", a.sessionID, a.projectID, a.deviceID)
			}
		})
	}
}

func TestCanvasTargetProjectRemovalUsesSourceDisplayOrder(t *testing.T) {
	for _, tc := range []struct {
		name, pickedDevice, wantProject, wantDevice string
		projects                                    []contracts.Project
	}{
		{name: "device before global", pickedDevice: "remote", wantProject: "z", wantDevice: "remote", projects: []contracts.Project{{ID: "a", Name: "Alpha", DeviceID: "local"}, {ID: "z", Name: "Zulu", DeviceID: "remote"}}},
		{name: "global order without local preference", pickedDevice: "missing", wantProject: "a", wantDevice: "remote", projects: []contracts.Project{{ID: "z", Name: "Zulu", DeviceID: "local"}, {ID: "a", Name: "alpha", DeviceID: "remote"}}},
		{name: "case insensitive name then ID", pickedDevice: "remote", wantProject: "a", wantDevice: "remote", projects: []contracts.Project{{ID: "z", Name: "alpha", DeviceID: "remote"}, {ID: "a", Name: "Alpha", DeviceID: "remote"}}},
		{name: "all projects removed", pickedDevice: "remote", wantDevice: "remote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.projectID, a.deviceID = "backend", tc.pickedDevice
			a.selectSession("chat-0")
			snapshot, err := a.service.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Revision = a.state.Revision + 1
			snapshot.Projects = tc.projects
			snapshot.Sessions = slices.DeleteFunc(snapshot.Sessions, func(s contracts.Session) bool { return s.ID == "chat-0" })
			a.apply(snapshot)
			if a.projectID != tc.wantProject || a.deviceID != tc.wantDevice || a.workspace.ProjectID != tc.wantProject || a.bottom.ProjectID != tc.wantProject {
				t.Fatalf("wrong restored target: project=%q device=%q workspace=%q bottom=%q", a.projectID, a.deviceID, a.workspace.ProjectID, a.bottom.ProjectID)
			}
		})
	}
}

func TestInvalidSessionHistoryReturnsToRememberedCanvas(t *testing.T) {
	a := testApp(t, "default")
	a.projectID, a.deviceID = "backend", "remote"
	a.draft = "Remember the canvas"
	a.navigate("chat/chat-0")
	a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
	a.navigate("settings/Appearance")
	a.back(-1)
	if a.sessionID != "" || a.route != "chat" || a.projectID != "backend" || a.deviceID != "remote" || a.draft != "Remember the canvas" {
		t.Fatalf("history reinstated deleted target: id=%q project=%q device=%q draft=%q route=%q", a.sessionID, a.projectID, a.deviceID, a.draft, a.route)
	}
	a.OpenRoute("chat/unknown")
	if a.sessionID != "" {
		t.Fatal("unknown direct route created a dangling selection")
	}
}

func TestLateFailedSendCannotResurrectDeletedSession(t *testing.T) {
	for _, failureFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete then failure", true: "failure then delete"}[failureFirst], func(t *testing.T) {
			a := testApp(t, "default")
			a.draft = "Canvas is independent"
			a.selectSession("chat-0")
			a.pendingSends = map[string]primaryPendingSend{"chat-0": {RequestID: "r11-send"}}
			command := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: "chat-0", RequestID: "r11-send", Text: "Old failed message"}}
			if failureFirst {
				a.commandFinished(command, errors.New("delivery failed"))
				if a.draft != "Old failed message" {
					t.Fatal("current failed send lost its recovery draft")
				}
			}
			a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
			a.commandFinished(command, errors.New("late delivery failure"))
			if _, ok := a.sessionStates["chat-0"]; ok {
				t.Fatal("completion recreated deleted view state")
			}
			if _, ok := a.drafts["chat-0"]; ok {
				t.Fatal("completion recreated deleted draft")
			}
			if a.draft != "Canvas is independent" || a.sessionID != "" {
				t.Fatal("completion replaced current canvas")
			}
		})
	}
}

func TestNewConversationRejectsOldCanvasSendFailure(t *testing.T) {
	a := testApp(t, "default")
	a.pendingNew = true
	a.pendingSends = map[string]primaryPendingSend{"": {RequestID: "old", DraftEpoch: a.draftEpoch}}
	a.newConversation()
	a.commandFinished(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{RequestID: "old", Text: "Old canvas"}}, errors.New("old failure"))
	if a.pendingNew || a.draft != "" || a.errorText != "" {
		t.Fatal("old completion changed new canvas generation")
	}
}

func TestPendingCreationDoesNotAdoptSideChatOrNavigate(t *testing.T) {
	for _, route := range []string{"chat", "settings/Appearance", "pull-requests", "chat/chat-0"} {
		t.Run(route, func(t *testing.T) {
			a := testApp(t, "default")
			a.pendingNew = true
			a.OpenRoute(route)
			beforeRoute, beforeID := a.route, a.sessionID
			snapshot, err := a.service.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Revision = a.state.Revision + 1
			snapshot.Sessions = append([]contracts.Session{{ID: "new-side", SideChat: true}, {ID: "new-child", ParentSessionID: "chat-0"}}, snapshot.Sessions...)
			a.apply(snapshot)
			if a.route != beforeRoute || a.sessionID != beforeID {
				t.Fatalf("unrelated creation changed selection: route=%q id=%q", a.route, a.sessionID)
			}
		})
	}
}

func queueCanvasSend(t *testing.T, a *App) contracts.Command {
	t.Helper()
	a.win = &mygo.Window{}
	ok := a.send("R11 queued new conversation")
	a.win = nil
	if !ok {
		t.Fatal("canvas send was rejected")
	}
	select {
	case command := <-a.jobs:
		return command
	default:
		t.Fatal("send did not enqueue a command")
		return contracts.Command{}
	}
}

func TestCreatedSessionAckSurvivesCoalescingAndNavigation(t *testing.T) {
	for _, route := range []string{"chat", "settings/Appearance", "pull-requests", "chat/chat-0"} {
		for _, completionFirst := range []bool{true, false} {
			t.Run(route+map[bool]string{true: "/completion first", false: "/event first"}[completionFirst], func(t *testing.T) {
				a := testApp(t, "default")
				command := queueCanvasSend(t, a)
				a.OpenRoute(route)
				beforeRoute, beforeID := a.route, a.sessionID
				ctx, cancel := context.WithCancel(a.ctx)
				defer cancel()
				events, err := a.service.Subscribe(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := a.service.Execute(a.ctx, command); err != nil {
					t.Fatal(err)
				}
				event := <-events
				if event.RequestID == command.Send.RequestID {
					t.Fatal("fixture did not coalesce away the transient creation ACK")
				}
				if completionFirst {
					a.commandFinished(command, nil)
				}
				a.applyEvent(event)
				if !completionFirst {
					a.commandFinished(command, nil)
				}
				if a.pendingNew || a.route != beforeRoute {
					t.Fatalf("creation did not settle without navigating: pending=%v route=%q", a.pendingNew, a.route)
				}
				if route == "chat" {
					if a.sessionID == "" || a.sessionID == "chat-0" {
						t.Fatal("durable request/run relation did not select the created session")
					}
				} else if a.sessionID != beforeID {
					t.Fatal("creation stole the current selection")
				}
				a.OpenRoute("chat")
				if !a.send("R11 can send after navigation during creation") {
					t.Fatal("completed creation kept the canvas blocked")
				}
			})
		}
	}
}

func TestHistoricalRequestIdentityCannotAdoptOldRun(t *testing.T) {
	a := testApp(t, "default")
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "historical-run", SessionID: "chat-0", RequestID: "primary::1"})
	command := queueCanvasSend(t, a)
	if command.Send.RequestID != "primary::1" {
		t.Fatal("fixture must exercise a persisted request-ID collision")
	}
	a.apply(a.state)
	a.applyEvent(contracts.Event{Kind: "run.started", RunID: "historical-run", SessionID: "chat-0", RequestID: command.Send.RequestID})
	if a.sessionID != "" || !a.pendingNew {
		t.Fatal("a historical run/ACK was adopted by a new app's first send")
	}
	if err := a.service.Execute(a.ctx, command); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.service.Snapshot(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.commandFinished(command, nil)
	a.apply(snapshot)
	if a.sessionID == "" || a.sessionID == "chat-0" || a.pendingNew {
		t.Fatal("the new run was not distinguished from the historical request")
	}
}

func TestPRDeletionThenSurfaceRouteUsesCanvasProject(t *testing.T) {
	for _, route := range []string{"files", "history"} {
		t.Run(route, func(t *testing.T) {
			a := testApp(t, "default")
			a.projectID, a.deviceID = "backend", "remote"
			a.selectSession("chat-0")
			a.OpenRoute("pull-requests")
			a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
			a.OpenRoute(route)
			if a.chatWorkspacePending || a.workspace.ProjectID != "backend" || a.route != "chat" {
				t.Fatalf("surface route used removed scope: pending=%v project=%q route=%q", a.chatWorkspacePending, a.workspace.ProjectID, a.route)
			}
			kind := a.workspace.ActiveKind()
			a.OpenRoute("chat")
			if kind == "" || a.workspace.ActiveKind() != kind {
				t.Fatal("explicit Chat navigation discarded the just-opened canvas tab")
			}
		})
	}
}

func TestOldCanvasCreationCannotOverrideNewGeneration(t *testing.T) {
	a := testApp(t, "default")
	old := queueCanvasSend(t, a)
	a.newConversation()
	current := queueCanvasSend(t, a)
	if err := a.service.Execute(a.ctx, old); err != nil {
		t.Fatal(err)
	}
	oldSnapshot, err := a.service.Snapshot(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.apply(oldSnapshot)
	a.commandFinished(old, errors.New("old completion"))
	if a.sessionID != "" || !a.pendingNew || a.pendingNewRequestID != current.Send.RequestID || a.draft != "" || a.errorText != "" {
		t.Fatal("an old generation's completion changed the newer pending canvas")
	}
	if err := a.service.Execute(a.ctx, current); err != nil {
		t.Fatal(err)
	}
	currentSnapshot, err := a.service.Snapshot(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.commandFinished(current, nil)
	a.apply(currentSnapshot)
	var currentID string
	for _, run := range currentSnapshot.Runs {
		if run.RequestID == current.Send.RequestID {
			currentID = run.SessionID
		}
	}
	if currentID == "" || a.sessionID != currentID || a.pendingNew {
		t.Fatal("new generation did not adopt only its own created session")
	}
}

func TestCreationDeletedBeforeCompletionSettlesWithoutResurrection(t *testing.T) {
	a := testApp(t, "default")
	command := queueCanvasSend(t, a)
	if err := a.service.Execute(a.ctx, command); err != nil {
		t.Fatal(err)
	}
	created, err := a.service.Snapshot(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var run contracts.Run
	for _, candidate := range created.Runs {
		if candidate.RequestID == command.Send.RequestID {
			run = candidate
		}
	}
	if run.ID == "" {
		t.Fatal("backend did not publish durable request identity")
	}
	if err := a.service.Execute(a.ctx, contracts.Command{Kind: "session.delete", ID: run.SessionID}); err != nil {
		t.Fatal(err)
	}
	removed, err := a.service.Snapshot(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.commandFinished(command, nil)
	a.apply(removed)
	a.creationCommandApplied(command, nil)
	a.applyEvent(contracts.Event{Kind: "run.started", RequestID: command.Send.RequestID, SessionID: run.SessionID, RunID: run.ID, Snapshot: &created})
	if a.pendingNew || a.sessionID != "" || a.hasSession(run.SessionID) {
		t.Fatal("late ACK resurrected the removed created session or retained pending")
	}
}

func TestSyntheticCloudWithoutRemainingProjectsKeepsPickedHost(t *testing.T) {
	a := testApp(t, "default")
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == "fieldnotes" {
			a.state.Projects[i].DeviceID = "remote"
		}
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == "chat-0" {
			a.state.Sessions[i].DeviceID = "remote"
		}
	}
	a.deviceID = "cloud-1"
	a.selectSession("chat-0")
	snapshot := a.state
	snapshot.Revision++
	snapshot.Projects = nil
	snapshot.Sessions = slices.DeleteFunc(slices.Clone(snapshot.Sessions), func(s contracts.Session) bool { return s.ID == "chat-0" })
	a.apply(snapshot)
	if a.sessionID != "" || a.projectID != "" || a.deviceID != "remote" {
		t.Fatalf("synthetic Cloud reset the picked host when projects vanished: id=%q project=%q device=%q", a.sessionID, a.projectID, a.deviceID)
	}
}
