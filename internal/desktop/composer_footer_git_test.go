package desktop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/mock"
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
		a.palette = design.Palette("zeron-dark", true)
		c.SetTheme(a.palette.UI())
		c.Root().Font("Geist").FontSize(13).TextColor(a.palette.Text)
		design.ApplyReferenceTypography(c)
		ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
			ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Children(func() {
				a.composerFooterGit(c)
				a.composerFooterPRBadge(c)
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

func TestComposerFooterScopeAndGenerationRejectLateSwitch(t *testing.T) {
	a := footerGitTestApp(t, "")
	oldScope, oldSequence := a.footerGit.Scope, a.footerGit.Sequence
	a.footerGit.Kind = contracts.CheckoutNew
	a.projectID = "zeron"
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
	a.state.Integrations.Cloud.Repositories = []contracts.CloudRepository{{ID: "repo", Name: "zeronsh/fieldnotes", Authorized: true}}
	a.deviceID = "cloud-1"
	tt := footerGitTester(a)
	if plan := a.composerFooterCheckoutPlan(); plan.Kind != contracts.CheckoutCloud || a.footerGit.Scope.CloudRepository != "zeronsh/fieldnotes" {
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

func TestComposerFooterPRSourceMatchingHoverAndDestination(t *testing.T) {
	a := footerGitTestApp(t, "")
	a.selectSession("chat-0")
	tt := footerGitTester(a)
	const label = "Open pull request 412: Refine the workspace layout"
	badge, ok := tt.Find(label)
	if !ok || badge.H != 20 {
		t.Fatal("matching source did not expose 20px PR badge")
	}
	tt.Move(badge.X+badge.W/2, badge.Y+badge.H/2)
	tt.Advance(349 * time.Millisecond)
	if _, ok := tt.Find("Pull request details"); ok {
		t.Fatal("PR card appeared before 350ms")
	}
	tt.Advance(2 * time.Millisecond)
	if _, ok := tt.Find("Pull request details"); !ok {
		t.Fatal("PR card did not appear after 350ms")
	}
	a.state.Settings.General.PRDestination = "pullRequest"
	footerGitClick(t, tt, label)
	if a.workspace.ActiveKind() != "pr" || !a.right {
		t.Fatal("badge did not open the existing native PR destination")
	}
	a.state.Settings.General.PRDestination = "browser"
	footerGitClick(t, tt, label)
	if a.workspace.ActiveKind() != "browser" {
		t.Fatal("badge did not open the existing in-app browser destination")
	}
	for _, tc := range []struct {
		url, detail, preference, want string
	}{
		{"https://github.com/zeronsh/fieldnotes/pull/412", "pr-412", "pullRequest", "pullRequest"},
		{"https://github.com/zeronsh/fieldnotes/pull/412", "", "pullRequest", "external"},
		{"https://enterprise.example/pull/412", "pr-412", "pullRequest", "external"},
		{"https://github.com/zeronsh/fieldnotes/pull/412", "pr-412", "browser", "browser"},
		{"https://github.com/zeronsh/fieldnotes/pull/412", "pr-412", "external", "external"},
	} {
		if got := composerFooterPRDestination(contracts.ChangeRequestSummary{URL: tc.url, DetailID: tc.detail}, tc.preference); got != tc.want {
			t.Errorf("destination %+v = %q", tc, got)
		}
	}
	a.footerGit.PRRequested = "" // A legacy scalar branch/number cannot claim a PR.
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].SourceContext = nil
		}
	}
	tt.Frame()
	if _, ok := tt.Find(label); ok {
		t.Fatal("legacy PR number borrowed an unrelated checkout PR")
	}
}
