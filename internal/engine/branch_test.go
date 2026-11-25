package engine

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestBranchCopiesTranscriptAsTopLevelConversation(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = core.Close() }()
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	parent := core.state.Sessions[0]
	releaseParentView := pinSessionView(t, core, parent.ID)
	defer releaseParentView()
	parent.Pinned, parent.Archived, parent.Unread, parent.PinOrder = true, true, true, 4
	parent.Goal = &contracts.SessionGoal{ID: "parent-goal", Status: contracts.GoalPaused}
	parent.SubagentType, parent.RunningSubagents, parent.LastOutcome = "Explore", 2, "completed"
	core.state.Sessions[0] = parent
	core.history[parent.ID] = []model.Message{
		{Role: "user", Text: "parent request", UserInput: true, InputIDs: []string{"parent-input"}},
		{Role: "assistant", Calls: []model.Call{{ID: "Stable_Call", Name: "Read", Arguments: `{"file_path":"proof.txt"}`}}},
		{Role: "tool", CallID: "Stable_Call", Text: "parent proof"},
		{Role: "assistant", Text: "parent answer"},
	}
	core.context[parent.ID] = model.CloneMessages(core.history[parent.ID])
	core.state.Messages[parent.ID] = []contracts.Message{{ID: "parent-display", SessionID: parent.ID, Role: "assistant"}}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.branch", SessionID: parent.ID, RequestID: "branch-once", Name: "Parent branch"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	branch := snapshot.Sessions[0]
	releaseBranchView := pinSessionView(t, core, branch.ID)
	defer releaseBranchView()
	snapshot, err = core.Snapshot(ctx, parent.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branch.ID == parent.ID || branch.ParentSessionID != "" || branch.SideChat || !branch.TopLevel() {
		t.Fatalf("branch is not a conversation of its own: %+v", branch)
	}
	if branch.Title != "Parent branch" || branch.CreationRequestID != "branch-once" {
		t.Fatalf("branch did not keep its creation identity: %+v", branch)
	}
	// A branch is the user's own conversation, so it starts empty of the parent's
	// execution and presentation state while keeping the transcript.
	if branch.Goal != nil || branch.SubagentType != "" || branch.RunningSubagents != 0 || branch.Status != contracts.Idle ||
		branch.Pinned || branch.Archived || branch.Unread || branch.PinOrder != 0 || branch.LastOutcome != "" {
		t.Fatalf("branch inherited parent execution or presentation state: %+v", branch)
	}
	if branch.CWD != parent.CWD || branch.ProjectID != parent.ProjectID || branch.ModelID != parent.ModelID {
		t.Fatalf("branch did not inherit its workspace and model: %+v", branch)
	}
	if len(core.history[branch.ID]) != 4 || !reflect.DeepEqual(core.context[branch.ID], core.history[parent.ID]) {
		t.Fatal("branch lost the inherited model history or context")
	}
	messages := snapshot.Messages[branch.ID]
	if len(messages) != len(snapshot.Messages[parent.ID]) || len(messages) == 0 {
		t.Fatalf("branch transcript was not copied: %+v", messages)
	}
	for _, message := range messages {
		if message.SessionID != branch.ID || message.Streaming {
			t.Fatalf("branch reused mutable or streaming message identity: %+v", message)
		}
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.branch", SessionID: parent.ID, RequestID: "branch-once"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.Sessions) != 2 {
		t.Fatal("retrying a creation request created another branch")
	}
}

func TestBranchIntoNewWorktreeTakesOwnCheckout(t *testing.T) {
	repository := temporaryGitRepository(t)
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	before := waitForProjectFiles(t, core)
	project := before.Projects[0]
	if !project.GitDetected || len(project.Worktrees) != 1 {
		t.Fatalf("project Git detection failed: %+v", project)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", ProjectID: project.ID, Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	parent := snapshot.Sessions[0]
	if err = core.Execute(ctx, contracts.Command{Kind: "session.branch", SessionID: parent.ID,
		Name: "Worktree branch", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew, Base: "main"}}); err != nil {
		t.Fatal(err)
	}
	after, _ := core.Snapshot(ctx)
	branch := after.Sessions[0]
	if branch.ID == parent.ID || !branch.TopLevel() || branch.CWD == parent.CWD ||
		branch.WorktreeID == "" || branch.WorktreeID == parent.WorktreeID ||
		branch.Branch == parent.Branch || !strings.HasPrefix(branch.Branch, "feat/") {
		t.Fatalf("branch did not take its own worktree: %+v", branch)
	}
	if filepath.Dir(branch.CWD) != filepath.Join(store.Directory(), "worktrees", filepath.Base(repository)) {
		t.Fatalf("branch worktree is outside the managed data directory: %s", branch.CWD)
	}
	if branch.SourceContext == nil || branch.SourceContext.CWD != branch.CWD || branch.SourceContext.CheckoutID != branch.WorktreeID {
		t.Fatalf("branch did not capture its own source context: %+v", branch.SourceContext)
	}
	actual, err := gitOutput(ctx, branch.CWD, "symbolic-ref", "--short", "HEAD")
	if err != nil || actual != branch.Branch {
		t.Fatalf("branch checkout does not carry its branch: %q, %v", actual, err)
	}
	parentAfter, _ := core.Snapshot(ctx)
	for _, session := range parentAfter.Sessions {
		if session.ID == parent.ID && (session.CWD != parent.CWD || session.Branch != parent.Branch || session.WorktreeID != parent.WorktreeID) {
			t.Fatalf("branch moved its parent checkout: %+v", session)
		}
	}
	projects := parentAfter.Projects
	for _, candidate := range projects {
		if candidate.ID == project.ID && len(candidate.Worktrees) != 2 {
			t.Fatalf("the new worktree was not registered: %+v", candidate.Worktrees)
		}
	}
}

func TestBranchIntoNewWorktreeRequiresProject(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = core.Close() }()
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	parent := core.state.Sessions[0]
	err = core.Execute(ctx, contracts.Command{Kind: "session.branch", SessionID: parent.ID,
		Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew}})
	if err == nil || !strings.Contains(err.Error(), "choose a project") {
		t.Fatalf("a branch without a project prepared a worktree: %v", err)
	}
	snapshot, _ := core.Snapshot(ctx)
	if len(snapshot.Sessions) != 1 {
		t.Fatal("a rejected worktree branch created a conversation")
	}
}
