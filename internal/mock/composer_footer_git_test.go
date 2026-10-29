package mock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestComposerFooterNewCheckoutCreatesUniqueWorktreesFromHEAD(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	before := footerGitSnapshot(t, s)
	project := footerGitProject(t, before, "fieldnotes")
	seenIDs, seenPaths := map[string]bool{}, map[string]bool{}
	for _, tree := range project.Worktrees {
		seenIDs[tree.ID], seenPaths[tree.Path] = true, true
	}
	for range 2 {
		run, err := s.Send(context.Background(), contracts.SendRequest{ProjectID: project.ID, DeviceID: "macbook", Text: "Inspect isolated checkout", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew}})
		if err != nil {
			t.Fatal(err)
		}
		after := footerGitSnapshot(t, s)
		session := footerGitSession(t, after, run.SessionID)
		if session.DeviceID != project.DeviceID || seenIDs[session.WorktreeID] || seenPaths[session.CWD] {
			t.Fatalf("new checkout reused a prior checkout or caller host: %+v", session)
		}
		created := false
		for _, tree := range footerGitProject(t, after, project.ID).Worktrees {
			if tree.ID == session.WorktreeID {
				created = tree.Path == session.CWD && tree.Branch == session.Branch && tree.BaseRef == "HEAD" && strings.HasPrefix(tree.Branch, "zeron/")
			}
		}
		if !created || session.SourceContext == nil || session.SourceContext.CheckoutID != session.WorktreeID || session.SourceContext.RepoRoot != session.CWD {
			t.Fatalf("host did not publish a fresh HEAD-based checkout: %+v", session)
		}
		seenIDs[session.WorktreeID], seenPaths[session.CWD] = true, true
	}
	after := footerGitSnapshot(t, s)
	if got := len(footerGitProject(t, after, project.ID).Worktrees); got != len(project.Worktrees)+2 {
		t.Fatalf("worktree count = %d, want %d", got, len(project.Worktrees)+2)
	}
}

func TestComposerFooterReuseAndCurrentCheckoutUseAuthoritativePaths(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, cwd, branch, worktree string
		plan                        contracts.CheckoutPlan
	}{
		{name: "reuse selected worktree", cwd: "~/worktrees/fieldnotes-workspace", branch: "fieldnotes/workspace", worktree: "workspace-tree", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: "~/worktrees/fieldnotes-workspace", Branch: "fieldnotes/workspace", WorktreeID: "workspace-tree"}},
		{name: "reuse resolves optional identity", cwd: "~/worktrees/fieldnotes-workspace", branch: "fieldnotes/workspace", worktree: "workspace-tree", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: "~/worktrees/fieldnotes-workspace", Branch: "fieldnotes/workspace"}},
		{name: "current checkout", cwd: "~/projects/fieldnotes", branch: "main", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{TokenInterval: time.Hour})
			before := footerGitSnapshot(t, s)
			run, err := s.Send(context.Background(), contracts.SendRequest{ProjectID: "fieldnotes", DeviceID: "macbook", Text: "Inspect checkout", Checkout: &test.plan})
			if err != nil {
				t.Fatal(err)
			}
			after := footerGitSnapshot(t, s)
			session := footerGitSession(t, after, run.SessionID)
			if session.CWD != test.cwd || session.Branch != test.branch || session.WorktreeID != test.worktree || session.DeviceID != "local" {
				t.Fatalf("resolved checkout = %+v", session)
			}
			if !reflect.DeepEqual(footerGitProject(t, before, "fieldnotes"), footerGitProject(t, after, "fieldnotes")) {
				t.Fatal("current/reuse changed the project's worktree inventory")
			}
			if session.SourceContext == nil || session.SourceContext.RepoRoot != test.cwd || session.SourceContext.CWD != test.cwd || session.SourceContext.Branch != test.branch {
				t.Fatalf("missing authoritative source observation: %+v", session.SourceContext)
			}
		})
	}
}

func TestComposerFooterCloudCheckoutRequiresRepositoryAndOwnsHost(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		cloudOrigin bool
	}{
		{name: "authorized repository"},
		{name: "project already lives on Cloud", cloudOrigin: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{TokenInterval: time.Hour})
			s.mu.Lock()
			for i := range s.state.Projects {
				if s.state.Projects[i].ID == "fieldnotes" && test.cloudOrigin {
					s.state.Projects[i].DeviceID = "cloud-project-origin"
				}
			}
			if !test.cloudOrigin {
				s.state.Integrations.Cloud.Repositories = []contracts.CloudRepository{{ID: "reachable", Name: "zeronsh/fieldnotes", Authorized: true}}
			}
			s.mu.Unlock()
			before := footerGitSnapshot(t, s)
			seen := map[string]bool{}
			for _, branch := range []string{"", "fieldnotes/settings-search"} {
				run, err := s.Send(context.Background(), contracts.SendRequest{ProjectID: "fieldnotes", Text: "Inspect Cloud checkout", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutCloud, Branch: branch}})
				if err != nil {
					t.Fatal(err)
				}
				after := footerGitSnapshot(t, s)
				session := footerGitSession(t, after, run.SessionID)
				wantBranch := branch
				if wantBranch == "" {
					wantBranch = "main"
				}
				if session.DeviceID != "cloud-"+session.ID || seen[session.DeviceID] || session.DeviceID == "cloud-project-origin" || session.CWD != "/workspace/Fieldnotes" || session.Branch != wantBranch {
					t.Fatalf("Cloud conversation did not own its machine: %+v", session)
				}
				seen[session.DeviceID] = true
				if session.SourceContext == nil || session.SourceContext.CheckoutID != "checkout:"+session.ID || session.SourceContext.RepoRoot != session.CWD {
					t.Fatalf("Cloud source identity = %+v", session.SourceContext)
				}
				if !slices.ContainsFunc(after.Devices, func(device contracts.Device) bool {
					return device.ID == session.DeviceID && device.Status == "connected"
				}) {
					t.Fatal("Cloud host was not published in device state")
				}
			}
			after := footerGitSnapshot(t, s)
			if len(after.Devices) != len(before.Devices)+2 || !reflect.DeepEqual(before.Projects, after.Projects) {
				t.Fatal("Cloud send reused a machine or changed local project worktrees")
			}
		})
	}
}

func TestComposerFooterCloudHostCopiesCapabilitiesWithoutAccountsOrPreferences(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	s.mu.Lock()
	s.state.Integrations.Cloud.Repositories = []contracts.CloudRepository{{Name: "zeronsh/fieldnotes", Authorized: true}}
	for i := range s.state.Integrations.Providers {
		provider := &s.state.Integrations.Providers[i]
		if provider.TargetID == "cloud-1" {
			provider.Installed, provider.Enabled = provider.ProviderID == "openai", provider.ProviderID == "openai"
			provider.AccountNote, provider.AccountsError, provider.Warning, provider.Error = "source account note", "source account error", "source warning", "source install error"
			provider.AccountsState, provider.UpdatePolicy = contracts.LoadError, "auto"
			provider.LoginOptions = []contracts.ProviderLoginOption{{ID: "source-login", Label: "Cloud login", Kind: "browser"}}
		}
	}
	s.state.Integrations.Accounts = append(s.state.Integrations.Accounts, contracts.ProviderAccount{ID: "template-cloud-account", ProviderID: "openai", TargetID: "cloud-1", InUse: true, Switchable: true, ReportsUsage: true, UsageState: contracts.LoadReady, Usage: []contracts.UsageWindow{{Label: "Template only", UsedPercent: 87}}})
	s.state.Integrations.Login = &contracts.ProviderLogin{ID: "template-login", ProviderID: "openai", TargetID: "cloud-1", State: "waiting"}
	s.mu.Unlock()
	before := footerGitSnapshot(t, s)
	template := map[string]contracts.ProviderDetails{}
	for _, provider := range before.Integrations.Providers {
		if provider.TargetID == "cloud-1" {
			template[provider.ProviderID] = provider
		}
	}
	if len(template) == 0 || !template["openai"].Installed || template["claude"].Enabled {
		t.Fatal("fixture needs available and explicitly disabled Cloud capabilities")
	}
	var targets []string
	for range 2 {
		run, err := s.Send(context.Background(), contracts.SendRequest{ProjectID: "fieldnotes", Text: "Inspect Cloud provider capabilities", ModelID: "gpt-5.4", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutCloud}})
		if err != nil {
			t.Fatal(err)
		}
		after := footerGitSnapshot(t, s)
		target := footerGitSession(t, after, run.SessionID).DeviceID
		targets = append(targets, target)
		seen := map[string]bool{}
		for _, provider := range after.Integrations.Providers {
			if provider.TargetID != target {
				continue
			}
			want, ok := template[provider.ProviderID]
			if !ok || seen[provider.ProviderID] {
				t.Fatalf("new Cloud host has foreign or duplicate provider: %+v", provider)
			}
			seen[provider.ProviderID] = true
			if provider.Installed != want.Installed || provider.Enabled != want.Enabled || provider.CanInstall != want.CanInstall || provider.DollarSkills != want.DollarSkills || provider.SeparateSlash != want.SeparateSlash || provider.NativeSubagents != want.NativeSubagents || provider.CanSwitchSubagents != want.CanSwitchSubagents || !reflect.DeepEqual(provider.LoginOptions, want.LoginOptions) {
				t.Fatalf("new Cloud host lost template capabilities: got=%+v want=%+v", provider, want)
			}
			if provider.AccountNote != "" || provider.AccountsError != "" || provider.Warning != "" || provider.Error != "" || provider.AccountsState != contracts.LoadReady || provider.UpdatePolicy != "off" {
				t.Fatalf("new Cloud host inherited account state or user preferences: %+v", provider)
			}
		}
		if len(seen) != len(template) {
			t.Fatalf("new Cloud target %q has %d providers, want %d", target, len(seen), len(template))
		}
		for _, account := range after.Integrations.Accounts {
			if account.TargetID == target {
				t.Fatalf("new Cloud host borrowed another target's account or usage: %+v", account)
			}
		}
		if err := s.Execute(context.Background(), contracts.Command{Kind: "provider.account.switch", Integration: &contracts.IntegrationRequest{TargetID: target, ProviderID: "openai", AccountID: "template-cloud-account"}}); err == nil {
			t.Fatal("new Cloud target switched an account owned by the template target")
		}
	}
	after := footerGitSnapshot(t, s)
	if !reflect.DeepEqual(after.Integrations.Accounts, before.Integrations.Accounts) || !reflect.DeepEqual(after.Integrations.Login, before.Integrations.Login) || !reflect.DeepEqual(after.Settings, before.Settings) {
		t.Fatal("creating Cloud capability targets changed accounts, login, or user settings")
	}
	for _, provider := range after.Integrations.Providers {
		if provider.TargetID == "cloud-1" && !reflect.DeepEqual(provider, template[provider.ProviderID]) {
			t.Fatal("new Cloud host creation changed its source template")
		}
	}
	copy := footerGitSnapshot(t, s)
	for i := range copy.Integrations.Providers {
		provider := &copy.Integrations.Providers[i]
		if provider.TargetID == targets[0] || provider.TargetID == "cloud-1" {
			provider.Enabled, provider.Installed = !provider.Enabled, !provider.Installed
			provider.LoginOptions[0].Label = "caller mutation"
		}
	}
	if got := footerGitSnapshot(t, s); !reflect.DeepEqual(got.Integrations, after.Integrations) {
		t.Fatal("mutating returned Cloud capabilities contaminated backend, source template, or another target")
	}
}

func TestComposerFooterStandardGithubAuthorizationReachesCanonicalProject(t *testing.T) {
	t.Parallel()
	for _, teardown := range []string{"cloud.github.disconnect", "cloud.delete"} {
		t.Run(teardown, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{Instant: true})
			footerGitExecute(t, s, contracts.Command{Kind: "cloud.enable"})
			footerGitExecute(t, s, contracts.Command{Kind: "cloud.github.start"})
			footerGitExecute(t, s, contracts.Command{Kind: "cloud.github.poll"})
			connected := footerGitSnapshot(t, s)
			project := footerGitProject(t, connected, "fieldnotes")
			cloud := connected.Integrations.Cloud
			if cloud.State != "ready" || cloud.Github.State != "connected" || !cloud.Github.Authorized {
				t.Fatalf("standard GitHub connection did not become ready: %+v", cloud)
			}
			if !slices.ContainsFunc(cloud.Repositories, func(repo contracts.CloudRepository) bool {
				return repo.ID == project.ID && repo.Authorized && repo.Name == project.CloudRepository
			}) {
				t.Fatalf("GitHub completion did not expose the project's canonical repository: %+v", cloud.Repositories)
			}
			footerGitExecute(t, s, contracts.Command{Kind: "cloud.authorize", Enabled: true, Integration: &contracts.IntegrationRequest{ProviderID: "github"}})
			footerGitExecute(t, s, contracts.Command{Kind: "cloud.repositories.choose"})
			selected := footerGitSnapshot(t, s)
			for _, id := range []string{"fieldnotes", "zeron"} {
				want := footerGitProject(t, selected, id)
				if want.CloudRepository == "" || !slices.ContainsFunc(selected.Integrations.Cloud.Repositories, func(repo contracts.CloudRepository) bool {
					return repo.ID == want.ID && repo.Authorized && repo.Name == want.CloudRepository
				}) {
					t.Fatalf("repository chooser did not preserve strict project identity for %q: %+v", id, selected.Integrations.Cloud.Repositories)
				}
			}
			scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: selected.Integrations.Cloud.DeviceID, RepoPath: project.Path, CloudRepository: project.CloudRepository, DefaultBranch: project.DefaultBranch}
			footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
			refs := footerGitSnapshot(t, s).RepoRefs[scope.Key()]
			if refs.State != contracts.LoadReady || len(refs.Rows) == 0 || refs.Scope != scope {
				t.Fatalf("authorized Cloud scope could not load refs: %+v", refs)
			}
			for _, row := range refs.Rows {
				if row.WorktreePath != "" || row.Current != (row.Name == project.DefaultBranch) {
					t.Fatalf("Cloud branch list used local checkout/worktree tags: %+v", refs.Rows)
				}
			}
			request := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{ProjectID: project.ID, Text: "Inspect authorized Cloud checkout", ModelID: "gpt-5.4", ReasoningLevel: "Medium", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutCloud, Branch: project.DefaultBranch}}}
			beforeSend := footerGitSnapshot(t, s)
			footerGitExecute(t, s, request)
			afterSend := footerGitSnapshot(t, s)
			if len(afterSend.Sessions) != len(beforeSend.Sessions)+1 || len(afterSend.Devices) != len(beforeSend.Devices)+1 {
				t.Fatal("standard authorization did not create one Cloud conversation and host")
			}
			created := afterSend.Sessions[0]
			if created.ProjectID != project.ID || created.DeviceID != "cloud-"+created.ID || created.DeviceID == scope.TargetID || created.CWD != "/workspace/"+project.Name || created.Branch != project.DefaultBranch || created.SourceContext == nil {
				t.Fatalf("standard authorization did not reach an owned Cloud checkout: %+v", created)
			}
			footerGitExecute(t, s, contracts.Command{Kind: teardown})
			disconnected := footerGitSnapshot(t, s)
			if len(disconnected.Integrations.Cloud.Repositories) != 0 || disconnected.Integrations.Cloud.Github.State != "disconnected" {
				t.Fatal("Cloud/GitHub teardown left authorized repository access")
			}
			if err := s.Execute(context.Background(), contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}}); err == nil {
				t.Fatal("disconnected Cloud repository still accepted refs requests")
			}
			if err := s.Execute(context.Background(), request); err == nil {
				t.Fatal("disconnected Cloud repository accepted a new conversation")
			}
			afterReject := footerGitSnapshot(t, s)
			if len(afterReject.Sessions) != len(disconnected.Sessions) || len(afterReject.Devices) != len(disconnected.Devices) || !reflect.DeepEqual(afterReject.Projects, disconnected.Projects) {
				t.Fatal("rejected Cloud send left a conversation, host, or worktree behind")
			}
		})
	}
}

func TestComposerFooterCloudRepositoryMatchesCanonicalName(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, cloudState string
		repository       contracts.CloudRepository
		cloudOrigin      bool
		allowed          bool
	}{
		{name: "canonical name ignores ASCII case", cloudState: "ready", repository: contracts.CloudRepository{ID: "fieldnotes", Name: "ZeRoNsH/FiElDnOtEs", Authorized: true}, allowed: true},
		{name: "canonical-looking ID cannot replace wrong name", cloudState: "ready", repository: contracts.CloudRepository{ID: "zeronsh/fieldnotes", Name: "other-owner/fieldnotes", Authorized: true}},
		{name: "matching name still requires authorization", cloudState: "ready", repository: contracts.CloudRepository{ID: "fieldnotes", Name: "zeronsh/fieldnotes"}},
		{name: "Cloud off rejects stale authorized rows", cloudState: "off", repository: contracts.CloudRepository{ID: "fieldnotes", Name: "zeronsh/fieldnotes", Authorized: true}},
		{name: "Cloud provisioning rejects stale authorized rows", cloudState: "provisioning", repository: contracts.CloudRepository{ID: "fieldnotes", Name: "zeronsh/fieldnotes", Authorized: true}},
		{name: "Cloud-origin project needs no local repository authorization", cloudState: "off", cloudOrigin: true, allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{TokenInterval: time.Hour})
			s.mu.Lock()
			s.state.Integrations.Cloud.State = test.cloudState
			s.state.Integrations.Cloud.Repositories = []contracts.CloudRepository{test.repository}
			if test.cloudOrigin {
				for i := range s.state.Projects {
					if s.state.Projects[i].ID == "fieldnotes" {
						s.state.Projects[i].DeviceID = "cloud-project-origin"
					}
				}
			}
			s.mu.Unlock()
			before := footerGitSnapshot(t, s)
			project := footerGitProject(t, before, "fieldnotes")
			scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: "cloud-1", RepoPath: project.Path, CloudRepository: project.CloudRepository, DefaultBranch: project.DefaultBranch}
			refsErr := s.Execute(context.Background(), contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
			run, sendErr := s.Send(context.Background(), contracts.SendRequest{ProjectID: project.ID, Text: "Inspect repository authorization", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutCloud}})
			if (refsErr == nil) != test.allowed || (sendErr == nil) != test.allowed {
				t.Fatalf("allowed=%v, refs=%v send=%v", test.allowed, refsErr, sendErr)
			}
			after := footerGitSnapshot(t, s)
			if !test.allowed {
				if !reflect.DeepEqual(before, after) {
					t.Fatal("rejected repository identity created resources or published refs")
				}
				return
			}
			if got := after.RepoRefs[scope.Key()]; got.State != contracts.LoadLoading {
				t.Fatalf("authorized scope did not start its refs request: %+v", got)
			}
			if session := footerGitSession(t, after, run.SessionID); session.DeviceID != "cloud-"+session.ID || session.ProjectID != project.ID {
				t.Fatalf("authorized repository did not create an owned Cloud host: %+v", session)
			}
		})
	}
}

func TestComposerFooterCheckoutFailuresDoNotCreateConversations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, scenario string
		plan           contracts.CheckoutPlan
		setup          func(*Server)
	}{
		{name: "worktree creation failure", scenario: "worktree-error", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutNew}},
		{name: "missing new base", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutNew, Base: "deleted-ref"}},
		{name: "reuse wrong path", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: "/missing", Branch: "fieldnotes/workspace", WorktreeID: "workspace-tree"}},
		{name: "reuse wrong identity", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: "~/worktrees/fieldnotes-workspace", Branch: "fieldnotes/workspace", WorktreeID: "other-tree"}},
		{name: "reuse wrong branch", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: "~/worktrees/fieldnotes-workspace", Branch: "main", WorktreeID: "workspace-tree"}},
		{name: "Cloud unreachable repository", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutCloud}},
		{name: "Cloud unauthorized repository", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutCloud}, setup: func(s *Server) {
			s.state.Integrations.Cloud.Repositories = []contracts.CloudRepository{{Name: "zeronsh/fieldnotes", Authorized: false}}
		}},
		{name: "non Git new worktree", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, setup: func(s *Server) {
			for i := range s.state.Projects {
				if s.state.Projects[i].ID == "fieldnotes" {
					s.state.Projects[i].GitDetected = false
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{GitScenario: test.scenario, TokenInterval: time.Hour})
			if test.setup != nil {
				s.mu.Lock()
				test.setup(s)
				s.mu.Unlock()
			}
			before := footerGitSnapshot(t, s)
			if _, err := s.Send(context.Background(), contracts.SendRequest{ProjectID: "fieldnotes", Text: "Do not partially create", Checkout: &test.plan}); err == nil {
				t.Fatal("invalid checkout succeeded")
			}
			if after := footerGitSnapshot(t, s); !reflect.DeepEqual(before, after) {
				t.Fatal("failed checkout partially created a conversation, worktree, or host")
			}
		})
	}
}

func TestComposerFooterExistingSendLocksTargetButUpdatesRunConfiguration(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	before := footerGitSnapshot(t, s)
	owned := footerGitSession(t, before, "chat-0")
	request := contracts.SendRequest{SessionID: owned.ID, ProjectID: "zeron", DeviceID: "unknown-host", Branch: "wrong-ref", WorktreeID: "wrong-tree", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, Text: "Inspect existing checkout", ModelID: "gpt-5.4", ReasoningLevel: "Medium"}
	if _, err := s.Send(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	after := footerGitSnapshot(t, s)
	got := footerGitSession(t, after, owned.ID)
	if got.ProjectID != owned.ProjectID || got.DeviceID != owned.DeviceID || got.CWD != owned.CWD || got.WorktreeID != owned.WorktreeID || got.Branch != owned.Branch || !reflect.DeepEqual(got.SourceContext, owned.SourceContext) {
		t.Fatalf("existing conversation migrated: before=%+v after=%+v", owned, got)
	}
	if got.ModelID != "gpt-5.4" || got.ReasoningLevel != "Medium" || !reflect.DeepEqual(before.Projects, after.Projects) || len(before.Devices) != len(after.Devices) {
		t.Fatal("run configuration was locked with the checkout, or draft checkout created resources")
	}
	request.Checkout, request.ModelID, request.ReasoningLevel = &contracts.CheckoutPlan{Kind: contracts.CheckoutCloud}, "claude-sonnet-4-6", "Low"
	queued, err := s.Send(context.Background(), request)
	if err != nil || queued.Status != "queued" {
		t.Fatalf("queued send = %+v, %v", queued, err)
	}
	after = footerGitSnapshot(t, s)
	if len(after.Queue) != 1 {
		t.Fatalf("queue = %+v", after.Queue)
	}
	row := after.Queue[0]
	if row.DeviceID != owned.DeviceID || row.Branch != owned.Branch || row.WorktreeID != owned.WorktreeID || row.ModelID != "claude-sonnet-4-6" || row.ReasoningLevel != "Low" {
		t.Fatalf("queued send lost target lock or editable run configuration: %+v", row)
	}
}

func TestComposerFooterSelectedSessionWithoutRowDoesNotBecomeNewDraft(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	before := footerGitSnapshot(t, s)
	if _, err := s.Send(context.Background(), contracts.SendRequest{SessionID: "row-not-synced", ProjectID: "fieldnotes", Text: "Keep selected conversation identity", Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew}}); err == nil {
		t.Fatal("selected session without a row was treated as a new conversation")
	}
	if !reflect.DeepEqual(before, footerGitSnapshot(t, s)) {
		t.Fatal("missing selected session created draft resources")
	}
}

func TestComposerFooterRefsForceRefreshRetainsRowsAndExplicitRetry(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{Instant: true, TokenInterval: 10 * time.Millisecond})
	scope := footerGitScope(footerGitProject(t, footerGitSnapshot(t, s), "fieldnotes"))
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
	ready := footerGitSnapshot(t, s).RepoRefs[scope.Key()]
	if ready.State != contracts.LoadReady || len(ready.Rows) == 0 || !slices.ContainsFunc(ready.Rows, func(row contracts.RepoRef) bool { return row.Name == "fieldnotes/workspace" && row.WorktreePath != "" }) {
		t.Fatalf("refs did not report current/worktree resources: %+v", ready)
	}
	s.mu.Lock()
	s.opts.Instant, s.opts.GitScenario = false, "refs-error"
	err := s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}})
	loading := cloneSnapshot(s.state).RepoRefs[scope.Key()]
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if loading.State != contracts.LoadLoading || !reflect.DeepEqual(ready.Rows, loading.Rows) {
		t.Fatal("forced refresh discarded visible rows")
	}
	footerGitWaitJobs(t, s)
	failed := footerGitSnapshot(t, s).RepoRefs[scope.Key()]
	if failed.State != contracts.LoadError || failed.Error == "" || !reflect.DeepEqual(ready.Rows, failed.Rows) {
		t.Fatalf("failed refresh lost stale rows: %+v", failed)
	}
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
	if got := footerGitSnapshot(t, s).RepoRefs[scope.Key()]; !reflect.DeepEqual(got, failed) {
		t.Fatal("ordinary render-style load retried an error")
	}
	s.mu.Lock()
	s.opts.Instant, s.opts.GitScenario = true, ""
	s.mu.Unlock()
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}})
	if got := footerGitSnapshot(t, s).RepoRefs[scope.Key()]; got.State != contracts.LoadReady || got.Error != "" || got.Generation != ready.Generation+2 {
		t.Fatalf("explicit retry did not recover: %+v", got)
	}
}

func TestComposerFooterCloudRefsInvalidatedInFlightCanReloadAfterReconnect(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{Instant: true, TokenInterval: 10 * time.Millisecond})
	for _, command := range []contracts.Command{
		{Kind: "cloud.enable"},
		{Kind: "cloud.github.start"},
		{Kind: "cloud.github.poll"},
		{Kind: "cloud.authorize", Enabled: true, Integration: &contracts.IntegrationRequest{ProviderID: "github"}},
		{Kind: "cloud.repositories.choose"},
	} {
		footerGitExecute(t, s, command)
	}
	connected := footerGitSnapshot(t, s)
	project := footerGitProject(t, connected, "fieldnotes")
	scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: connected.Integrations.Cloud.DeviceID, RepoPath: project.Path, CloudRepository: project.CloudRepository, DefaultBranch: project.DefaultBranch}
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
	ready := footerGitSnapshot(t, s).RepoRefs[scope.Key()]
	if ready.State != contracts.LoadReady || len(ready.Rows) == 0 {
		t.Fatal("fixture needs visible Cloud refs before refresh")
	}
	// The old job must observe disconnection before its timer can finish.
	s.mu.Lock()
	s.opts.Instant = false
	loadErr := s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}})
	disconnectErr := s.applyLocked(contracts.Command{Kind: "cloud.github.disconnect"})
	if disconnectErr == nil {
		s.publishLocked("cloud.github.disconnect", "", "", "", "")
	}
	disconnectedRevision := s.state.Revision
	s.mu.Unlock()
	if loadErr != nil || disconnectErr != nil {
		t.Fatalf("load/disconnect = %v, %v", loadErr, disconnectErr)
	}
	footerGitWaitJobs(t, s)
	settled := footerGitSnapshot(t, s)
	invalidated := settled.RepoRefs[scope.Key()]
	if invalidated.State != "" || invalidated.Switching != "" || invalidated.Scope != scope || !reflect.DeepEqual(invalidated.Rows, ready.Rows) {
		t.Errorf("invalidated Cloud request did not settle to retryable idle with held rows: %+v", invalidated)
	}
	if settled.Revision <= disconnectedRevision {
		t.Error("invalidated Cloud completion did not publish its resource change")
	}
	s.mu.Lock()
	s.opts.Instant = true
	s.mu.Unlock()
	for _, command := range []contracts.Command{
		{Kind: "cloud.github.start"},
		{Kind: "cloud.github.poll"},
		{Kind: "cloud.authorize", Enabled: true, Integration: &contracts.IntegrationRequest{ProviderID: "github"}},
		{Kind: "cloud.repositories.choose"},
		{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}},
	} {
		footerGitExecute(t, s, command)
	}
	if got := footerGitSnapshot(t, s).RepoRefs[scope.Key()]; got.State != contracts.LoadReady || got.Generation <= invalidated.Generation || !reflect.DeepEqual(got.Rows, ready.Rows) {
		t.Fatalf("ordinary load after reconnection stayed stuck on obsolete loading: %+v", got)
	}
}

func TestComposerFooterDraftSwitchFailureAndSingleFlight(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{Instant: true, GitScenario: "switch-error", TokenInterval: 10 * time.Millisecond})
	project := footerGitProject(t, footerGitSnapshot(t, s), "fieldnotes")
	scope := footerGitScope(project)
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
	footerGitExecute(t, s, contracts.Command{Kind: "git.ref.switch", Git: &contracts.GitRequest{Scope: scope, Ref: "fieldnotes/settings-search"}})
	failed := footerGitSnapshot(t, s)
	if got := failed.RepoRefs[scope.Key()]; got.SwitchError == "" || got.Switching != "" || footerGitProject(t, failed, project.ID).Branch != project.Branch {
		t.Fatalf("failed switch changed checkout or lost error: %+v", got)
	}
	s.mu.Lock()
	s.opts.Instant, s.opts.GitScenario = false, ""
	err := s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.ref.switch", Git: &contracts.GitRequest{Scope: scope, Ref: "fieldnotes/settings-search"}})
	pending := cloneSnapshot(s.state).RepoRefs[scope.Key()]
	secondErr := s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.ref.switch", Git: &contracts.GitRequest{Scope: scope, Ref: "main"}})
	refreshErr := s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}})
	afterRefresh := cloneSnapshot(s.state).RepoRefs[scope.Key()]
	s.mu.Unlock()
	if err != nil || refreshErr != nil {
		t.Fatalf("switch/refresh = %v, %v", err, refreshErr)
	}
	if pending.Switching != "fieldnotes/settings-search" || pending.SwitchError != "" {
		t.Fatalf("switch did not enter pending state: %+v", pending)
	}
	if secondErr == nil {
		t.Fatal("second concurrent switch was accepted")
	}
	if afterRefresh.Generation != pending.Generation || afterRefresh.Switching != pending.Switching {
		t.Fatal("refresh cancelled the in-flight switch")
	}
	footerGitWaitJobs(t, s)
	after := footerGitSnapshot(t, s)
	got := after.RepoRefs[scope.Key()]
	if got.Switching != "" || got.SwitchError != "" || footerGitProject(t, after, project.ID).Branch != "fieldnotes/settings-search" || !slices.ContainsFunc(got.Rows, func(row contracts.RepoRef) bool { return row.Name == "fieldnotes/settings-search" && row.Current }) {
		t.Fatalf("successful retry did not update current checkout: %+v", got)
	}
	if err := s.Execute(context.Background(), contracts.Command{Kind: "git.ref.switch", Git: &contracts.GitRequest{Scope: scope, Ref: "fieldnotes/workspace"}}); err == nil {
		t.Fatal("local switch stole a ref from an existing worktree")
	}
}

func TestComposerFooterGitAsyncRejectsStaleGenerations(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"refs", "PR"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{TokenInterval: 10 * time.Millisecond})
			initial := footerGitSnapshot(t, s)
			scope := footerGitScope(footerGitProject(t, initial, "fieldnotes"))
			key := footerGitPRKey(t, footerGitSession(t, initial, "chat-0"))
			// Hold the mutex across both generations, so the old timer cannot
			// finish before the newer synchronous request is authoritative.
			s.mu.Lock()
			var err error
			if kind == "refs" {
				command := contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}}
				err = s.loadOrSwitchRefsLocked(command)
				s.opts.Instant = true
				if err == nil {
					err = s.loadOrSwitchRefsLocked(command)
				}
				s.opts.GitScenario = "refs-error"
			} else {
				err = s.refreshCheckoutPRLocked(key, true)
				s.opts.Instant = true
				if err == nil {
					err = s.refreshCheckoutPRLocked(key, true)
				}
				s.opts.GitScenario = "pr-error"
			}
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			fresh := footerGitSnapshot(t, s)
			footerGitWaitJobs(t, s)
			after := footerGitSnapshot(t, s)
			if kind == "refs" {
				if got := after.RepoRefs[scope.Key()]; got.State != contracts.LoadReady || got.Generation != 2 || !reflect.DeepEqual(got, fresh.RepoRefs[scope.Key()]) {
					t.Fatalf("stale refs callback overwrote newer success: %+v", got)
				}
			} else if got := after.CheckoutPR[key.Key()]; got.State != contracts.LoadReady || got.Summary == nil || !reflect.DeepEqual(got, fresh.CheckoutPR[key.Key()]) {
				t.Fatalf("stale PR callback overwrote newer success: %+v", got)
			}
		})
	}
}

func TestComposerFooterGitAsyncKeepsRepositoryScopesSeparate(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{TokenInterval: 10 * time.Millisecond})
	initial := footerGitSnapshot(t, s)
	first := footerGitScope(footerGitProject(t, initial, "fieldnotes"))
	second := footerGitScope(footerGitProject(t, initial, "backend"))
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: first}})
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: second}})
	footerGitWaitJobs(t, s)
	after := footerGitSnapshot(t, s)
	for _, scope := range []contracts.RepoRefScope{first, second} {
		got := after.RepoRefs[scope.Key()]
		project := footerGitProject(t, after, scope.ProjectID)
		if got.State != contracts.LoadReady || got.Scope != scope || len(got.Rows) != len(project.Branches) {
			t.Fatalf("scope returned another repository's rows: %+v", got)
		}
		for _, row := range got.Rows {
			if !slices.Contains(project.Branches, row.Name) {
				t.Fatalf("foreign ref %q in scope %+v", row.Name, scope)
			}
		}
	}
	heldRows := after.RepoRefs[first.Key()].Rows
	// Change the project's path while the old path's response is pending.
	// That late response must not complete under the new resource key.
	s.mu.Lock()
	err := s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: first, Force: true}})
	for i := range s.state.Projects {
		if s.state.Projects[i].ID == first.ProjectID {
			s.state.Projects[i].Path = "~/moved/fieldnotes"
			s.state.Projects[i].Branches = []string{"new-root-only"}
		}
	}
	newScope := first
	newScope.RepoPath = "~/moved/fieldnotes"
	s.opts.Instant = true
	if err == nil {
		err = s.loadOrSwitchRefsLocked(contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: newScope}})
	}
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	fresh := footerGitSnapshot(t, s).RepoRefs[newScope.Key()]
	footerGitWaitJobs(t, s)
	after = footerGitSnapshot(t, s)
	if got := after.RepoRefs[newScope.Key()]; !reflect.DeepEqual(got, fresh) || len(got.Rows) != 1 || got.Rows[0].Name != "new-root-only" {
		t.Fatalf("old path response contaminated new repository scope: %+v", got)
	}
	if got := after.RepoRefs[first.Key()]; got.State != "" || got.Switching != "" || got.Scope != first || !reflect.DeepEqual(got.Rows, heldRows) {
		t.Fatalf("obsolete path request did not return to idle with its own held rows: %+v", got)
	}
}

func TestComposerFooterPRRefreshRetainsErrorAndClearsSuccessfulNone(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{Instant: true})
	initial := footerGitSnapshot(t, s)
	session := footerGitSession(t, initial, "chat-0")
	key := footerGitPRKey(t, session)
	before := initial.CheckoutPR[key.Key()]
	if before.Summary == nil {
		t.Fatal("fixture needs a successful PR observation")
	}
	s.mu.Lock()
	s.opts.GitScenario = "pr-error"
	s.mu.Unlock()
	footerGitExecute(t, s, contracts.Command{Kind: "git.pr.refresh", Git: &contracts.GitRequest{Source: &key, Force: true}})
	failed := footerGitSnapshot(t, s)
	got := failed.CheckoutPR[key.Key()]
	if got.State != contracts.LoadError || got.Error == "" || !reflect.DeepEqual(got.Summary, before.Summary) || contracts.ChangeRequestForSession(session, failed.CheckoutPR) == nil {
		t.Fatalf("error erased the same-source successful PR: %+v", got)
	}
	s.mu.Lock()
	s.opts.GitScenario = ""
	s.state.PullRequests = slices.DeleteFunc(s.state.PullRequests, func(pr contracts.PullRequest) bool {
		return pr.Repository == "zeronsh/fieldnotes" && pr.HeadBranch == key.Branch
	})
	s.mu.Unlock()
	footerGitExecute(t, s, contracts.Command{Kind: "git.pr.refresh", Git: &contracts.GitRequest{Source: &key, Force: true}})
	after := footerGitSnapshot(t, s)
	got = after.CheckoutPR[key.Key()]
	if got.State != contracts.LoadReady || got.Error != "" || got.Summary != nil || contracts.ChangeRequestForSession(session, after.CheckoutPR) != nil {
		t.Fatalf("successful no-match retained stale PR: %+v", got)
	}
}

func TestComposerFooterPRRefreshNeverBorrowsAnotherSourceKey(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*contracts.Session)
	}{
		{name: "host", change: func(session *contracts.Session) { session.DeviceID = "macbook" }},
		{name: "checkout", change: func(session *contracts.Session) { session.SourceContext.CheckoutID = "new-checkout" }},
		{name: "repository root", change: func(session *contracts.Session) { session.SourceContext.RepoRoot = "/new-root" }},
		{name: "branch", change: func(session *contracts.Session) { session.SourceContext.Branch = "other-branch" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, Options{Instant: true, GitScenario: "pr-error"})
			before := footerGitSnapshot(t, s)
			oldKey := footerGitPRKey(t, footerGitSession(t, before, "chat-0"))
			s.mu.Lock()
			test.change(&s.state.Sessions[s.sessionIndexLocked("chat-0")])
			s.mu.Unlock()
			session := footerGitSession(t, footerGitSnapshot(t, s), "chat-0")
			key := footerGitPRKey(t, session)
			footerGitExecute(t, s, contracts.Command{Kind: "git.pr.refresh", Git: &contracts.GitRequest{Source: &key, Force: true}})
			after := footerGitSnapshot(t, s)
			got := after.CheckoutPR[key.Key()]
			if got.State != contracts.LoadError || got.Key != key || got.Summary != nil || contracts.ChangeRequestForSession(session, after.CheckoutPR) != nil {
				t.Fatalf("new source borrowed a prior source's PR: %+v", got)
			}
			if !reflect.DeepEqual(after.CheckoutPR[oldKey.Key()], before.CheckoutPR[oldKey.Key()]) {
				t.Fatal("refresh of another source modified the former successful snapshot")
			}
		})
	}
}

func TestComposerFooterDeleteKeepsSharedPRUntilLastSourceOwner(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{Instant: true})
	before := footerGitSnapshot(t, s)
	original := footerGitSession(t, before, "chat-0")
	key := footerGitPRKey(t, original)
	s.mu.Lock()
	shared := clone(original)
	shared.ID = "same-checkout-owner"
	s.state.Sessions = append(s.state.Sessions, shared)
	s.state.Messages[shared.ID] = nil
	s.mu.Unlock()
	footerGitExecute(t, s, contracts.Command{Kind: "session.delete", ID: original.ID})
	afterFirst := footerGitSnapshot(t, s)
	if !reflect.DeepEqual(afterFirst.CheckoutPR[key.Key()], before.CheckoutPR[key.Key()]) || contracts.ChangeRequestForSession(shared, afterFirst.CheckoutPR) == nil {
		t.Fatal("deleting one source owner erased another conversation's PR")
	}
	footerGitExecute(t, s, contracts.Command{Kind: "session.delete", ID: shared.ID})
	afterLast := footerGitSnapshot(t, s)
	if _, remains := afterLast.CheckoutPR[key.Key()]; remains {
		t.Fatal("last source owner deletion retained its PR snapshot")
	}
	if err := s.Execute(context.Background(), contracts.Command{Kind: "git.pr.refresh", Git: &contracts.GitRequest{Source: &key, Force: true}}); err == nil {
		t.Fatal("deleted source accepted another refresh")
	}
	if _, remains := footerGitSnapshot(t, s).CheckoutPR[key.Key()]; remains {
		t.Fatal("refresh resurrected a deleted source")
	}
	for encoded, status := range before.CheckoutPR {
		if encoded != key.Key() && !reflect.DeepEqual(afterLast.CheckoutPR[encoded], status) {
			t.Fatal("source deletion changed an unrelated PR snapshot")
		}
	}
}

func TestComposerFooterDeletedSourceCannotBeRevivedByLatePRRefresh(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{TokenInterval: 10 * time.Millisecond})
	initial := footerGitSnapshot(t, s)
	key := footerGitPRKey(t, footerGitSession(t, initial, "chat-0"))
	s.mu.Lock()
	err := s.refreshCheckoutPRLocked(key, true)
	deleteErr := s.applyLocked(contracts.Command{Kind: "session.delete", ID: "chat-0"})
	_, remainedAfterDelete := s.state.CheckoutPR[key.Key()]
	s.mu.Unlock()
	if err != nil || deleteErr != nil {
		t.Fatalf("refresh/delete = %v, %v", err, deleteErr)
	}
	if remainedAfterDelete {
		t.Error("delete retained the final owner's pending PR resource")
	}
	footerGitWaitJobs(t, s)
	if got, remains := footerGitSnapshot(t, s).CheckoutPR[key.Key()]; remains {
		t.Fatalf("late refresh retained or revived a deleted source: %+v", got)
	}
}

func TestComposerFooterLegacyGitMetadataMigrationRespectsFieldPresence(t *testing.T) {
	t.Parallel()
	base := newTestServer(t, Options{Instant: true})
	legacy := footerGitSnapshot(t, base)
	legacy.RepoRefs, legacy.CheckoutPR = nil, nil
	for i := range legacy.Sessions {
		legacy.Sessions[i].SourceContext = nil
	}
	for i := range legacy.Projects {
		if legacy.Projects[i].ID == "zeron" {
			legacy.Projects[i].GitDetected = false
		}
	}
	legacy.Projects = append(legacy.Projects, contracts.Project{ID: "user-project", Name: "User project", Path: "/user-project", DeviceID: "local", Branch: "main", Branches: []string{"main"}})
	data, err := json.Marshal(persisted{Snapshot: legacy, Next: 3000})
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	projects := envelope["snapshot"].(map[string]any)["projects"].([]any)
	for _, item := range projects {
		project := item.(map[string]any)
		if project["id"] == "fieldnotes" || project["id"] == "user-project" {
			delete(project, "gitDetected")
			delete(project, "cloudRepository")
			delete(project, "defaultBranch")
		}
	}
	data, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "legacy-footer.json")
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	restored := newTestServer(t, Options{StatePath: statePath})
	after := footerGitSnapshot(t, restored)
	known := footerGitProject(t, after, "fieldnotes")
	if !known.GitDetected || known.CloudRepository != "zeronsh/fieldnotes" || known.DefaultBranch != "main" {
		t.Errorf("missing known mock Git metadata was not migrated: %+v", known)
	}
	if got := footerGitProject(t, after, "zeron"); got.GitDetected {
		t.Error("explicit false GitDetected was overwritten by seed metadata")
	}
	if got := footerGitProject(t, after, "user-project"); got.GitDetected || got.CloudRepository != "" || got.DefaultBranch != "" {
		t.Errorf("unknown project inferred Git from its branch: %+v", got)
	}
	for _, session := range after.Sessions {
		if session.SourceContext != nil {
			t.Fatal("legacy scalar branch manufactured conversation-owned source context")
		}
	}
}

func TestComposerFooterGitSnapshotsAndSubscribersOwnNestedData(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, Options{Instant: true})
	scope := footerGitScope(footerGitProject(t, footerGitSnapshot(t, s), "fieldnotes"))
	footerGitExecute(t, s, contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}})
	before := footerGitSnapshot(t, s)
	first, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	left, right := <-first, <-second
	mutate := func(snapshot *contracts.Snapshot) {
		resource := snapshot.RepoRefs[scope.Key()]
		resource.Rows[0].Name, resource.Scope.TargetID = "mutated", "mutated"
		snapshot.RepoRefs[scope.Key()] = resource
		session := footerGitSession(t, *snapshot, "chat-0")
		key := footerGitPRKey(t, session)
		snapshot.CheckoutPR[key.Key()].Summary.Title = "mutated"
		session.SourceContext.Branch = "mutated"
		delete(snapshot.CheckoutPR, key.Key())
	}
	copy := footerGitSnapshot(t, s)
	mutate(&copy)
	mutate(left.Snapshot)
	if !reflect.DeepEqual(*right.Snapshot, before) || !reflect.DeepEqual(footerGitSnapshot(t, s), before) {
		t.Fatal("refs rows, PR summary, maps, or conversation source escaped snapshot ownership")
	}
}

func footerGitSnapshot(t *testing.T, s *Server) contracts.Snapshot {
	t.Helper()
	snapshot, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func footerGitSession(t *testing.T, snapshot contracts.Snapshot, id string) contracts.Session {
	t.Helper()
	for _, session := range snapshot.Sessions {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("missing session %q", id)
	return contracts.Session{}
}

func footerGitProject(t *testing.T, snapshot contracts.Snapshot, id string) contracts.Project {
	t.Helper()
	for _, project := range snapshot.Projects {
		if project.ID == id {
			return project
		}
	}
	t.Fatalf("missing project %q", id)
	return contracts.Project{}
}

func footerGitScope(project contracts.Project) contracts.RepoRefScope {
	return contracts.RepoRefScope{ProjectID: project.ID, TargetID: project.DeviceID, RepoPath: project.Path}
}

func footerGitPRKey(t *testing.T, session contracts.Session) contracts.CheckoutPRKey {
	t.Helper()
	key, ok := contracts.SessionCheckoutPRKey(session)
	if !ok {
		t.Fatalf("missing conversation source for %q", session.ID)
	}
	return key
}

func footerGitExecute(t *testing.T, s *Server, command contracts.Command) {
	t.Helper()
	if err := s.Execute(context.Background(), command); err != nil {
		t.Fatalf("%s: %v", command.Kind, err)
	}
}

func footerGitWaitJobs(t *testing.T, s *Server) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Fatal("Git jobs did not settle")
	}
}
