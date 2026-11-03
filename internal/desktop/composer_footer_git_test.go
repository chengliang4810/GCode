package desktop

import (
	"context"
	"errors"
	"fmt"
	"gcode/internal/assets"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/mock"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func footerGitTestApp(t *testing.T, scenario string) *App {
	t.Helper()
	if scenario == "" {
		return testApp(t, "default")
	}
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	backend, err := mock.New(mock.Options{Instant: true, GitScenario: scenario})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	a.ensureComposerFooterRefs(false)
	return a
}

func footerGitTester(a *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		a.palette = design.Palette("rcode-dark", true)
		c.SetTheme(a.palette.UI())
		c.Root().Font("Geist").FontSize(13).TextColor(a.palette.Text)
		design.ApplyReferenceTypography(c)
		ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
			ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Children(func() {
				a.composerFooterGit(c)
			})
		})
	}, 840, 560)
}

func footerGitClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}

func TestComposerFooterNewWorktreeMenuAndSend(t *testing.T) {
	a := footerGitTestApp(t, "")
	initial := len(a.state.Projects[0].Worktrees)
	tt := ui.NewTester(a.View, 1320, 880)
	checkout, _ := tt.Find("Select checkout")
	ref, _ := tt.Find("Select ref")
	if checkout.X >= ref.X || checkout.H != 20 || ref.H != 20 {
		t.Fatalf("source checkout/ref order or height lost: checkout=%+v ref=%+v", checkout, ref)
	}
	footerGitClick(t, tt, "Select checkout")
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if a.footerGit.Kind != contracts.CheckoutNew || a.footerGit.CheckoutOpen {
		t.Fatal("checkout keyboard action did not choose New worktree")
	}
	footerGitClick(t, tt, "Select ref")
	footerGitClick(t, tt, "Ref fieldnotes/settings-search")
	if a.state.Projects[0].Branch != "main" {
		t.Fatal("choosing a new-worktree base switched the current checkout")
	}
	footerGitClick(t, tt, "Message")
	tt.Type("Create a fresh checkout from the chosen base")
	tt.Key(0, ui.KeyEnter)
	session := a.currentSession()
	if session.ID == "" || session.WorktreeID == "" || session.CWD == a.state.Projects[0].Path || session.SourceContext == nil {
		t.Fatalf("send did not bind authoritative new checkout: %+v error=%s", session, a.errorText)
	}
	if len(a.state.Projects[0].Worktrees) != initial+1 {
		t.Fatal("New worktree reused an existing last item")
	}
	for _, tree := range a.state.Projects[0].Worktrees {
		if tree.ID == session.WorktreeID && (tree.BaseRef != "fieldnotes/settings-search" || tree.Path != session.CWD) {
			t.Fatalf("send lost selected base or actual cwd: %+v", tree)
		}
	}
	if _, ok := tt.Find("Select checkout"); ok {
		t.Fatal("created session still exposes an editable checkout")
	}
}

func TestComposerFooterCurrentRefPreservesProjectDirectory(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.state.Projects[0].Path += "/selected-subdirectory"
	_, _, git := a.syncComposerFooterGit()
	if !git {
		t.Fatal("fixture lost Git detection")
	}
	scope := a.footerGit.Scope
	a.state.RepoRefs[scope.Key()] = contracts.RepoRefs{
		Scope: scope, State: contracts.LoadReady,
		Rows: []contracts.RepoRef{
			{Name: "main", Current: true, WorktreePath: "/repository"},
		},
	}
	plan := a.composerFooterCheckoutPlan()
	if plan == nil || plan.Kind != contracts.CheckoutCurrent || plan.Path != "" || plan.Branch != "" {
		t.Fatalf("current ref would replace the selected project directory: %+v", plan)
	}
	a.pickComposerFooterRef(a.state.RepoRefs[scope.Key()].Rows[0])
	if plan := a.composerFooterCheckoutPlan(); plan.Branch != "main" {
		t.Fatalf("explicit branch selection lost its validation: %+v", plan)
	}
	a.state.Projects[0].GitDetected = false
	if plan := a.composerFooterCheckoutPlan(); plan != nil {
		t.Fatalf("non-Git project supplied a checkout plan: %+v", plan)
	}
}

func TestComposerFooterReuseWorktreeAndReadonlySession(t *testing.T) {
	a := footerGitTestApp(t, "")
	initial := len(a.state.Projects[0].Worktrees)
	tt := ui.NewTester(a.View, 1320, 880)
	footerGitClick(t, tt, "Select ref")
	footerGitClick(t, tt, "Ref fieldnotes/workspace")
	if plan := a.composerFooterCheckoutPlan(); plan.Kind != contracts.CheckoutReuse || plan.Path != "~/worktrees/fieldnotes-workspace" {
		t.Fatalf("worktree ref did not create typed reuse plan: %+v", plan)
	}
	footerGitClick(t, tt, "Message")
	tt.Type("Use the existing checkout")
	tt.Key(0, ui.KeyEnter)
	session := a.currentSession()
	if session.CWD != "~/worktrees/fieldnotes-workspace" || session.WorktreeID != "workspace-tree" || len(a.state.Projects[0].Worktrees) != initial {
		t.Fatalf("reuse created or moved checkout: %+v", session)
	}
	if _, ok := tt.Find("Session checkout"); !ok || !tt.HasText("Worktree") {
		t.Fatal("readonly session did not project cwd-derived worktree kind")
	}
	a.pickComposerFooterCheckout(contracts.CheckoutNew)
	a.pickComposerFooterRef(contracts.RepoRef{Name: "main", Current: true})
	if a.composerFooterCheckoutPlan() != nil || a.currentSession().CWD != session.CWD {
		t.Fatal("existing session admitted a draft checkout plan")
	}
	a.sessionID = "row-not-yet-arrived"
	if a.composerFooterCheckoutPlan() != nil {
		t.Fatal("selected session ID without a row became a new checkout")
	}
}

func TestComposerFooterRefSwitchErrorRetainsMenuAndRetry(t *testing.T) {
	a := footerGitTestApp(t, "switch-error")
	tt := footerGitTester(a)
	footerGitClick(t, tt, "Select ref")
	footerGitClick(t, tt, "Ref fieldnotes/settings-search")
	if !a.footerGit.RefOpen || a.footerGit.SwitchError == "" || a.footerGit.Branch != "" || a.errorText != "" {
		t.Fatalf("failed switch lost menu/old ref or leaked error: %+v", a.footerGit)
	}
	if !tt.HasText("Local changes would be overwritten by checkout") || a.state.Projects[0].Branch != "main" {
		t.Fatal("switch failure changed checkout or omitted Git error")
	}
	footerGitClick(t, tt, "Ref main")
	if a.footerGit.RefOpen || a.footerGit.SwitchError != "" {
		t.Fatal("a subsequent current-ref selection did not recover")
	}

	errApp := footerGitTestApp(t, "refs-error")
	errTester := footerGitTester(errApp)
	footerGitClick(t, errTester, "Select ref")
	if _, ok := errTester.Find("Retry refs"); !ok || !errTester.HasText("Could not load refs") {
		t.Fatal("refs error did not expose explicit retry")
	}
}

type footerGitRecordingService struct {
	contracts.Service
	commands []contracts.Command
	err      error
}

func (s *footerGitRecordingService) Execute(ctx context.Context, command contracts.Command) error {
	s.commands = append(s.commands, command.Clone())
	if command.Kind == "git.refs.load" && s.err != nil {
		return s.err
	}
	return s.Service.Execute(ctx, command)
}

func TestComposerFooterRejectedLoadDoesNotRetryEveryFrame(t *testing.T) {
	a := footerGitTestApp(t, "")
	delete(a.state.RepoRefs, a.footerGit.Scope.Key())
	a.footerGit.Requested = false
	recorder := &footerGitRecordingService{Service: a.service, err: errors.New("Repository target is unavailable")}
	a.service = recorder
	tt := footerGitTester(a)
	for range 4 {
		tt.Frame()
	}
	if len(recorder.commands) != 1 || a.footerGit.RefError == "" {
		t.Fatalf("RPC rejection caused render retries: commands=%d error=%q", len(recorder.commands), a.footerGit.RefError)
	}
	footerGitClick(t, tt, "Select ref")
	if !tt.HasText("Repository target is unavailable") {
		t.Fatal("RPC rejection did not render in the ref menu")
	}
	n := len(recorder.commands)
	footerGitClick(t, tt, "Retry refs")
	if len(recorder.commands) != n+1 {
		t.Fatal("explicit retry did not request refs")
	}
}

func TestComposerFooterRefreshesSavedRefsOnEntryAndEveryOpen(t *testing.T) {
	a := footerGitTestApp(t, "")
	recorder := &footerGitRecordingService{Service: a.service}
	a.service = recorder
	a.footerGit.Requested = false
	before := a.state.RepoRefs[a.footerGit.Scope.Key()].Generation
	tt := footerGitTester(a)
	assertLoads := func(want int) {
		t.Helper()
		if len(recorder.commands) != want {
			t.Fatalf("loads=%d want=%d", len(recorder.commands), want)
		}
		for _, command := range recorder.commands {
			if command.Kind != "git.refs.load" || !command.Git.Force {
				t.Fatalf("saved Ready refs were reused instead of refreshed: %+v", command)
			}
		}
	}
	assertLoads(1)
	if a.state.RepoRefs[a.footerGit.Scope.Key()].Generation <= before {
		t.Fatal("entry retained the persisted refs generation")
	}
	for range 3 {
		tt.Frame()
	}
	assertLoads(1)
	footerGitClick(t, tt, "Select ref")
	assertLoads(2)
	tt.Key(0, ui.KeyEscape)
	footerGitClick(t, tt, "Select ref")
	assertLoads(3)
	footerGitClick(t, tt, "Ref main")
	if a.footerGit.Branch != "main" {
		t.Fatal("explicit ref choice was not recorded")
	}
	a.newConversation()
	tt.Frame()
	assertLoads(4)
	if a.footerGit.Branch != "" {
		t.Fatal("new draft inherited the preceding draft's pinned branch")
	}
}

func TestComposerFooterRefreshDoesNotDuplicateInFlightOperation(t *testing.T) {
	for _, switching := range []bool{false, true} {
		t.Run(fmt.Sprint(switching), func(t *testing.T) {
			a := footerGitTestApp(t, "")
			recorder := &footerGitRecordingService{Service: a.service}
			a.service = recorder
			scope := a.footerGit.Scope
			resource := a.state.RepoRefs[scope.Key()]
			resource.State = contracts.LoadLoading
			if switching {
				resource.State, resource.Switching = contracts.LoadReady, "topic"
			}
			a.state.RepoRefs[scope.Key()] = resource
			a.footerGit.Requested = false
			a.ensureComposerFooterRefs(false)
			a.ensureComposerFooterRefs(true)
			if len(recorder.commands) != 0 {
				t.Fatal("opening a picker duplicated an active repository operation")
			}
		})
	}
}

func TestComposerFooterRefreshWaitsForQueuedRequestSnapshot(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.win = &mygo.Window{}
	defer func() { a.win = nil }()
	a.footerGit.Requested = false
	a.ensureComposerFooterRefs(false)
	command := <-a.jobs
	a.ensureComposerFooterRefs(true)
	if len(a.jobs) != 0 {
		t.Fatal("menu open duplicated a refresh still waiting in the service queue")
	}
	a.composerFooterGitCommandFinished(command, nil)
	a.ensureComposerFooterRefs(true)
	if len(a.jobs) != 0 {
		t.Fatal("command receipt was mistaken for an updated repository snapshot")
	}
	resource := a.state.RepoRefs[a.footerGit.Scope.Key()]
	resource.Generation++
	resource.State = contracts.LoadLoading
	a.state.RepoRefs[a.footerGit.Scope.Key()] = resource
	a.ensureComposerFooterRefs(true)
	if len(a.jobs) != 0 {
		t.Fatal("published loading snapshot admitted a duplicate")
	}
	resource.State = contracts.LoadReady
	a.state.RepoRefs[a.footerGit.Scope.Key()] = resource
	a.ensureComposerFooterRefs(true)
	if len(a.jobs) != 1 {
		t.Fatal("completed refresh prevented the next explicit refresh")
	}
}

func TestComposerFooterDefaultPlansDoNotPinCachedBranch(t *testing.T) {
	a := footerGitTestApp(t, "")
	scope := a.footerGit.Scope
	a.state.RepoRefs[scope.Key()] = contracts.RepoRefs{Scope: scope, State: contracts.LoadReady,
		Rows: []contracts.RepoRef{{Name: "obsolete-cached-branch", Current: true}}}
	if plan := a.composerFooterCheckoutPlan(); plan.Kind != contracts.CheckoutCurrent || plan.Branch != "" {
		t.Fatalf("default current checkout pinned an unverified cached branch: %+v", plan)
	}
	a.footerGit.Kind = contracts.CheckoutNew
	if plan := a.composerFooterCheckoutPlan(); plan.Base != "" {
		t.Fatalf("default new worktree pinned an unverified cached HEAD: %+v", plan)
	}
}

func TestComposerFooterRefreshKeepsExplicitWorktreeDestination(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.pickComposerFooterRef(contracts.RepoRef{Name: "fieldnotes/workspace", WorktreePath: "~/worktrees/fieldnotes-workspace"})
	scope := a.footerGit.Scope
	for _, status := range []contracts.LoadStatus{contracts.LoadLoading, contracts.LoadError} {
		t.Run(string(status), func(t *testing.T) {
			resource := a.state.RepoRefs[scope.Key()]
			resource.State = status
			a.state.RepoRefs[scope.Key()] = resource
			plan := a.composerFooterCheckoutPlan()
			if plan.Kind != contracts.CheckoutReuse || plan.Path != "~/worktrees/fieldnotes-workspace" || plan.Branch != "fieldnotes/workspace" {
				t.Fatalf("refresh retargeted an explicit worktree selection: %+v", plan)
			}
		})
	}
}

func TestComposerFooterRefreshKeepsCurrentKeyboardRow(t *testing.T) {
	a := footerGitTestApp(t, "")
	tt := footerGitTester(a)
	footerGitClick(t, tt, "Select ref")
	scope := a.footerGit.Scope
	resource := a.state.RepoRefs[scope.Key()]
	resource.Generation++
	resource.Rows = append([]contracts.RepoRef{{Name: "added-before-current"}}, resource.Rows...)
	a.state.RepoRefs[scope.Key()] = resource
	tt.Frame()
	rows := footerFilteredRefs(resource, "")
	if rows[a.footerGit.Cursor].Name != "main" {
		t.Fatalf("refresh moved keyboard choice to another branch: %+v", rows[a.footerGit.Cursor])
	}
	tt.Key(0, ui.KeyEnter)
	if a.footerGit.Branch != "main" || a.state.Projects[0].Branch != "main" {
		t.Fatal("Enter after refresh switched to an unchosen branch")
	}
}

func TestComposerFooterCachedRowsStayVisibleWhileRefreshIsQueuedLoadingOrFailed(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.win = &mygo.Window{}
	defer func() { a.win = nil }()
	tt := footerGitTester(a)
	footerGitClick(t, tt, "Select ref")
	if len(a.jobs) != 1 {
		t.Fatal("opening the picker did not immediately queue a refresh")
	}
	assertCachedRow := func() {
		t.Helper()
		if _, ok := tt.Find("Ref fieldnotes/settings-search"); !ok {
			t.Fatal("refresh hid the previous successful ref list")
		}
	}
	assertCachedRow() // No host loading snapshot has arrived yet.
	scope := a.footerGit.Scope
	resource := a.state.RepoRefs[scope.Key()]
	resource.State, resource.Generation = contracts.LoadLoading, resource.Generation+1
	a.state.RepoRefs[scope.Key()] = resource
	tt.Frame()
	assertCachedRow()
	footerGitClick(t, tt, "Search refs")
	tt.Type("settings")
	assertCachedRow()
	if _, ok := tt.Find("Ref main"); ok {
		t.Fatal("cached refs ignored the search query")
	}
	resource.State, resource.Error = contracts.LoadError, "Refresh fixture unavailable"
	a.state.RepoRefs[scope.Key()] = resource
	tt.Frame()
	assertCachedRow()
	if _, ok := tt.Find("Retry refs"); !ok || !tt.HasText(resource.Error) {
		t.Fatal("failed refresh hid the cache or lost its retry/error")
	}
	footerGitClick(t, tt, "Retry refs")
	assertCachedRow()
	if len(a.jobs) != 2 {
		t.Fatal("retry did not queue a new refresh")
	}
}

func TestComposerFooterRefreshPreservesNavigatedCursorByRefName(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.win = &mygo.Window{}
	defer func() { a.win = nil }()
	a.footerGit.Kind = contracts.CheckoutNew
	tt := footerGitTester(a)
	footerGitClick(t, tt, "Select ref")
	tt.Key(0, ui.KeyDown)
	scope := a.footerGit.Scope
	resource := a.state.RepoRefs[scope.Key()]
	chosen := footerFilteredRefs(resource, "")[a.footerGit.Cursor].Name
	if chosen == "main" {
		t.Fatal("fixture did not navigate away from the current ref")
	}
	resource.Generation++
	resource.Rows = append([]contracts.RepoRef{{Name: "inserted-before-the-cursor"}}, resource.Rows...)
	a.state.RepoRefs[scope.Key()] = resource
	tt.Frame()
	if got := footerFilteredRefs(resource, "")[a.footerGit.Cursor].Name; got != chosen {
		t.Fatalf("refresh shifted the cursor from %q to %q", chosen, got)
	}
	tt.Key(0, ui.KeyEnter)
	if a.footerGit.Branch != chosen {
		t.Fatal("Enter selected a different branch after refresh")
	}
}

func TestComposerFooterCachedSelectionWaitsForRefreshBeforeSwitching(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(fmt.Sprint(removed), func(t *testing.T) {
			a := footerGitTestApp(t, "")
			a.win = &mygo.Window{}
			defer func() { a.win = nil }()
			tt := footerGitTester(a)
			footerGitClick(t, tt, "Select ref")
			footerGitClick(t, tt, "Ref fieldnotes/settings-search")
			if a.footerGit.PendingRef != "fieldnotes/settings-search" || len(a.jobs) != 1 {
				t.Fatal("cached selection raced the queued refresh")
			}
			a.draft = "Keep pending branch draft"
			if a.send(a.draft) || a.draft != "Keep pending branch draft" {
				t.Fatal("send ran on the old branch before pending selection completed")
			}
			scope := a.footerGit.Scope
			resource := a.state.RepoRefs[scope.Key()]
			resource.Generation++
			if removed {
				resource.Rows = []contracts.RepoRef{{Name: "main", Current: true}}
			}
			a.state.RepoRefs[scope.Key()] = resource
			tt.Frame()
			if a.footerGit.PendingRef != "" {
				t.Fatal("completed refresh retained a pending selection")
			}
			refresh := <-a.jobs
			if refresh.Kind != "git.refs.load" {
				t.Fatal("first operation was not the refresh")
			}
			if removed {
				if len(a.jobs) != 0 || a.footerGit.SwitchError == "" || a.footerGit.Branch != "" {
					t.Fatal("removed cached ref switched to a different branch or lost its error")
				}
				return
			}
			if len(a.jobs) != 1 {
				t.Fatal("completed refresh did not release the chosen branch switch")
			}
			switchCommand := <-a.jobs
			if switchCommand.Kind != "git.ref.switch" || switchCommand.Git.Ref != "fieldnotes/settings-search" {
				t.Fatalf("wrong deferred operation: %+v", switchCommand)
			}
		})
	}
}

func TestComposerFooterFailedCheckoutRestoresDraftAndRefreshes(t *testing.T) {
	a := footerGitTestApp(t, "")
	recorder := &footerGitRecordingService{Service: a.service}
	a.service = recorder
	a.pendingNew = true
	a.pendingSends = map[string]primaryPendingSend{"": {RequestID: "stale-selection", DraftEpoch: a.draftEpoch}}
	command := contracts.Command{Kind: "message.send", ProjectID: a.projectID, Send: &contracts.SendRequest{
		RequestID: "stale-selection", Text: "Keep this draft", Attachments: []contracts.Attachment{{Name: "draft.txt"}},
		Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: "old"},
	}}
	a.commandFinished(command, errors.New("selected checkout changed"))
	if a.pendingNew || a.draft != command.Send.Text || len(a.attachments) != 1 {
		t.Fatal("failed checkout lost the pending message or left send locked")
	}
	if len(recorder.commands) != 1 || recorder.commands[0].Kind != "git.refs.load" || !recorder.commands[0].Git.Force {
		t.Fatal("failed checkout did not refresh repository refs")
	}
}

func TestComposerFooterScopeAndGenerationRejectLateSwitch(t *testing.T) {
	a := footerGitTestApp(t, "")
	oldScope, oldSequence := a.footerGit.Scope, a.footerGit.Sequence
	a.footerGit.Kind = contracts.CheckoutNew
	a.projectID = "rcode"
	a.syncComposerFooterGit()
	if a.footerGit.HeldRef == nil || a.footerGit.HeldRef.Name != "main" {
		t.Fatal("project transition lost the held display ref")
	}
	if plan := a.composerFooterCheckoutPlan(); plan.Branch != "" || plan.Kind != contracts.CheckoutCurrent {
		t.Fatalf("display-only held ref escaped into new project plan: %+v", plan)
	}
	a.composerFooterGitCommandFinished(contracts.Command{Kind: "git.ref.switch", Version: oldSequence, Git: &contracts.GitRequest{Scope: oldScope}}, errors.New("late old project failure"))
	if a.footerGit.SwitchError != "" {
		t.Fatal("old project command overwrote the current menu")
	}
	a.projectID = "fieldnotes"
	a.syncComposerFooterGit()
	if a.footerGit.Kind != contracts.CheckoutNew || a.footerGit.Branch != "" {
		t.Fatal("project checkout-kind memory retained the wrong branch")
	}
	f := &a.footerGit
	resource := a.state.RepoRefs[f.Scope.Key()]
	f.Switching, f.SwitchBaseline = "fieldnotes/settings-search", resource.Generation
	a.syncComposerFooterGit()
	if f.Switching == "" {
		t.Fatal("old ready snapshot acknowledged a newer switch")
	}
	resource.Generation++
	resource.SwitchError = "dirty tree"
	a.state.RepoRefs[f.Scope.Key()] = resource
	a.syncComposerFooterGit()
	if f.Switching != "" || f.SwitchError != "dirty tree" || f.Branch != "" {
		t.Fatal("new generation did not preserve failed switch semantics")
	}
}

func TestComposerFooterCloudLossAndNonGit(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.state.Integrations.Cloud.Repositories = []contracts.CloudRepository{{ID: "repo", Name: "r-code/fieldnotes", Authorized: true}}
	a.deviceID = "cloud-1"
	tt := footerGitTester(a)
	if plan := a.composerFooterCheckoutPlan(); plan.Kind != contracts.CheckoutCloud || a.footerGit.Scope.CloudRepository != "r-code/fieldnotes" {
		t.Fatalf("reachable Cloud target did not resolve a Cloud plan: %+v", plan)
	}
	if _, ok := tt.Find("Select checkout"); ok {
		t.Fatal("Cloud draft exposed the local checkout menu")
	}
	a.state.Integrations.Cloud.Repositories = nil
	tt.Frame()
	if plan := a.composerFooterCheckoutPlan(); plan.Kind != contracts.CheckoutCurrent || a.footerGit.Branch != "" {
		t.Fatalf("Cloud loss retained a stale clone plan: %+v", plan)
	}
	a.state.Projects[0].GitDetected = false
	tt.Frame()
	if _, ok := tt.Find("Select ref"); ok {
		t.Fatal("non-Git project displayed a ref picker")
	}
}

func TestComposerFooterRefSearchFiltersBeforeSourceCap(t *testing.T) {
	a := footerGitTestApp(t, "")
	resource := a.state.RepoRefs[a.footerGit.Scope.Key()]
	resource.Rows = nil
	for i := range 305 {
		resource.Rows = append(resource.Rows, contracts.RepoRef{Name: fmt.Sprintf("topic-%03d", i)})
	}
	a.state.RepoRefs[a.footerGit.Scope.Key()] = resource
	a.state.Projects[0].Branches = nil
	for _, row := range resource.Rows {
		a.state.Projects[0].Branches = append(a.state.Projects[0].Branches, row.Name)
	}
	a = modelSnapshotApp(t, a.state)
	a.syncComposerFooterGit()
	a.footerGit.Kind = contracts.CheckoutNew
	tt := footerGitTester(a)
	footerGitClick(t, tt, "Select ref")
	if !tt.HasText("Showing 300 of 305 refs") {
		t.Fatal("frozen source's 300-row disclosure missing")
	}
	footerGitClick(t, tt, "Search refs")
	tt.Type("304")
	if _, ok := tt.Find("Ref topic-304"); !ok || tt.HasText("Showing 300 of 305 refs") {
		t.Fatal("search did not reach a ref beyond the unfiltered cap")
	}
	tt.Key(0, ui.KeyEnter)
	if a.footerGit.Branch != "topic-304" || a.footerGit.RefOpen {
		t.Fatal("filtered keyboard selection lost the underlying ref ID")
	}
}

func TestComposerFooterRefRankingAndReopenKeepsSelectedKeyboardRow(t *testing.T) {
	a := footerGitTestApp(t, "")
	resource := a.state.RepoRefs[a.footerGit.Scope.Key()]
	resource.Rows = []contracts.RepoRef{{Name: "feature/topic"}, {Name: "topic-one"}, {Name: "TOPIC-two"}, {Name: "main", Current: true}}
	a.state.RepoRefs[a.footerGit.Scope.Key()] = resource
	a.state.Projects[0].Branches = []string{"feature/topic", "topic-one", "TOPIC-two", "main"}
	a = modelSnapshotApp(t, a.state)
	a.syncComposerFooterGit()
	rows := footerFilteredRefs(resource, " TOPIC ")
	if len(rows) != 3 || rows[0].Name != "topic-one" || rows[1].Name != "TOPIC-two" || rows[2].Name != "feature/topic" {
		t.Fatalf("source trimmed, stable prefix/substring ranking lost: %+v", rows)
	}
	a.footerGit.Kind, a.footerGit.Branch = contracts.CheckoutNew, "TOPIC-two"
	tt := footerGitTester(a)
	footerGitClick(t, tt, "Select ref")
	if a.footerGit.Cursor != 2 {
		t.Fatal("reopening the ref menu highlighted a different branch")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	if a.footerGit.RefOpen || a.footerGit.Branch != "TOPIC-two" {
		t.Fatal("modified Enter selected a different reopened ref")
	}
}
