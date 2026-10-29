package contracts

import (
	"testing"
	"time"
)

func TestCheckoutPlanValidateVariants(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		plan    CheckoutPlan
		wantErr bool
	}{
		{name: "current without a reported branch", plan: CheckoutPlan{Kind: CheckoutCurrent}},
		{name: "current with branch", plan: CheckoutPlan{Kind: CheckoutCurrent, Branch: "main"}},
		{name: "reuse existing worktree", plan: CheckoutPlan{Kind: CheckoutReuse, Path: "/wt/topic", Branch: "topic"}},
		{name: "reuse with known identity", plan: CheckoutPlan{Kind: CheckoutReuse, Path: "/wt/topic", Branch: "topic", WorktreeID: "wt-topic"}},
		{name: "new with base", plan: CheckoutPlan{Kind: CheckoutNew, Base: "main"}},
		{name: "new with unloaded refs uses HEAD", plan: CheckoutPlan{Kind: CheckoutNew}},
		{name: "cloud default branch", plan: CheckoutPlan{Kind: CheckoutCloud}},
		{name: "cloud picked branch", plan: CheckoutPlan{Kind: CheckoutCloud, Branch: "topic"}},
		{name: "missing kind", plan: CheckoutPlan{}, wantErr: true},
		{name: "unknown kind", plan: CheckoutPlan{Kind: "other"}, wantErr: true},
		{name: "current cannot specify new base", plan: CheckoutPlan{Kind: CheckoutCurrent, Base: "main"}, wantErr: true},
		{name: "current cannot override cwd", plan: CheckoutPlan{Kind: CheckoutCurrent, Path: "/wt/topic"}, wantErr: true},
		{name: "current cannot select worktree identity", plan: CheckoutPlan{Kind: CheckoutCurrent, WorktreeID: "wt-topic"}, wantErr: true},
		{name: "reuse requires path", plan: CheckoutPlan{Kind: CheckoutReuse, Branch: "topic"}, wantErr: true},
		{name: "reuse requires branch", plan: CheckoutPlan{Kind: CheckoutReuse, Path: "/wt/topic"}, wantErr: true},
		{name: "reuse cannot create from base", plan: CheckoutPlan{Kind: CheckoutReuse, Path: "/wt/topic", Branch: "topic", Base: "main"}, wantErr: true},
		{name: "new branch comes from host", plan: CheckoutPlan{Kind: CheckoutNew, Branch: "already-created"}, wantErr: true},
		{name: "new cannot reuse path", plan: CheckoutPlan{Kind: CheckoutNew, Path: "/wt/topic"}, wantErr: true},
		{name: "new cannot reuse identity", plan: CheckoutPlan{Kind: CheckoutNew, WorktreeID: "wt-topic"}, wantErr: true},
		{name: "cloud cannot use local base", plan: CheckoutPlan{Kind: CheckoutCloud, Base: "main"}, wantErr: true},
		{name: "cloud cannot reuse local path", plan: CheckoutPlan{Kind: CheckoutCloud, Path: "/wt/topic"}, wantErr: true},
		{name: "cloud cannot reuse local identity", plan: CheckoutPlan{Kind: CheckoutCloud, WorktreeID: "wt-topic"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.plan.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate(%+v) = %v, want error %v", test.plan, err, test.wantErr)
			}
		})
	}
}

func TestSessionCheckoutPRKeyRequiresConversationSource(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*Session)
		valid  bool
	}{
		{name: "source cwd may be a repository subdirectory", valid: true},
		{name: "legacy fields do not choose the source", change: func(session *Session) {
			session.Branch, session.WorktreeID, session.CWD = "unrelated", "old-checkout", "/old-repo"
		}, valid: true},
		{name: "source branch is trimmed", change: func(session *Session) { session.SourceContext.Branch = " \tfeature/source\n" }, valid: true},
		{name: "legacy branch alone is insufficient", change: func(session *Session) { session.SourceContext = nil }},
		{name: "missing host", change: func(session *Session) { session.DeviceID = "" }},
		{name: "missing checkout identity", change: func(session *Session) { session.SourceContext.CheckoutID = "" }},
		{name: "missing repository root", change: func(session *Session) { session.SourceContext.RepoRoot = "" }},
		{name: "missing source branch", change: func(session *Session) { session.SourceContext.Branch = "" }},
		{name: "blank source branch", change: func(session *Session) { session.SourceContext.Branch = " \t\n" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			session := footerSourceSession()
			if test.change != nil {
				test.change(&session)
			}
			got, valid := SessionCheckoutPRKey(session)
			if valid != test.valid {
				t.Fatalf("SessionCheckoutPRKey = (%+v, %v), want valid %v", got, valid, test.valid)
			}
			if test.valid && got != footerSourceKey() {
				t.Fatalf("key = %+v, want conversation source %+v", got, footerSourceKey())
			}
		})
	}
}

func TestChangeRequestForSessionMatchesCompleteSourceIdentity(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name           string
		changeSession  func(*Session)
		changeSnapshot func(*CheckoutPRStatus)
		missing        bool
		wantPR         bool
	}{
		{name: "repository root matches despite source cwd subdirectory", wantPR: true},
		{name: "legacy branch cannot replace source branch", changeSnapshot: func(status *CheckoutPRStatus) { status.Key.Branch = "legacy-branch" }},
		{name: "no source context", changeSession: func(session *Session) { session.SourceContext = nil }},
		{name: "no snapshot", missing: true},
		{name: "different host", changeSession: func(session *Session) { session.DeviceID = "other-host" }},
		{name: "different checkout", changeSession: func(session *Session) { session.SourceContext.CheckoutID = "other-checkout" }},
		{name: "different repository", changeSession: func(session *Session) { session.SourceContext.RepoRoot = "/other-repo" }},
		{name: "different source branch", changeSession: func(session *Session) { session.SourceContext.Branch = "other-branch" }},
		{name: "snapshot host is revalidated", changeSnapshot: func(status *CheckoutPRStatus) { status.Key.TargetID = "other-host" }},
		{name: "snapshot checkout is revalidated", changeSnapshot: func(status *CheckoutPRStatus) { status.Key.CheckoutID = "other-checkout" }},
		{name: "empty snapshot checkout is rejected", changeSnapshot: func(status *CheckoutPRStatus) { status.Key.CheckoutID = "" }},
		{name: "snapshot root is revalidated", changeSnapshot: func(status *CheckoutPRStatus) { status.Key.RepoRoot = "/other-repo" }},
		{name: "snapshot branch is revalidated", changeSnapshot: func(status *CheckoutPRStatus) { status.Key.Branch = "other-branch" }},
		{name: "head and observation time do not change PR identity", changeSession: func(session *Session) {
			session.SourceContext.HeadSHA = "new-head"
			session.SourceContext.ObservedAt = time.Unix(9000, 0)
		}, wantPR: true},
		{name: "successful none has no PR", changeSnapshot: func(status *CheckoutPRStatus) { status.Summary = nil }},
		{name: "same source error retains successful PR", changeSnapshot: func(status *CheckoutPRStatus) {
			status.State, status.Error = LoadError, "rate limited"
		}, wantPR: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			session := footerSourceSession()
			if test.changeSession != nil {
				test.changeSession(&session)
			}
			want := ChangeRequestSummary{Provider: "github", Number: 42, Title: "Source PR", URL: "https://github.com/acme/repo/pull/42", State: "open", BaseRef: "main", HeadRef: "feature/source"}
			status := CheckoutPRStatus{Key: footerSourceKey(), State: LoadReady, Summary: &want}
			if test.changeSnapshot != nil {
				test.changeSnapshot(&status)
			}
			// Deliberately keep the requested map key even when the stored
			// identity changes: lookup by map key alone must not expose a PR.
			snapshots := map[string]CheckoutPRStatus{}
			if !test.missing {
				snapshots[footerSourceKey().Key()] = status
			}
			got := ChangeRequestForSession(session, snapshots)
			if !test.wantPR {
				if got != nil {
					t.Fatalf("exposed mismatched PR: %+v", *got)
				}
				return
			}
			if got == nil || *got != want {
				t.Fatalf("PR = %+v, want %+v", got, want)
			}
		})
	}
}

func TestComposerFooterCommandCloneOwnsNestedPayloads(t *testing.T) {
	t.Parallel()
	t.Run("git request and source", func(t *testing.T) {
		t.Parallel()
		source := footerSourceKey()
		original := Command{Kind: "git.pr.refresh", Git: &GitRequest{Source: &source, Force: true}}
		frozen := original.Clone()
		if frozen.Git == original.Git || frozen.Git.Source == original.Git.Source {
			t.Fatal("cloned git command shares mutable request or source")
		}
		original.Git.Force = false
		original.Git.Source.TargetID = "changed-by-caller"
		if !frozen.Git.Force || *frozen.Git.Source != footerSourceKey() {
			t.Fatalf("caller mutation reached frozen command: %+v", frozen.Git)
		}
		frozen.Git.Source.Branch = "changed-by-worker"
		frozen.Git.Ref = "worker-ref"
		if original.Git.Source.Branch != "feature/source" || original.Git.Ref != "" {
			t.Fatal("worker mutation reached caller command")
		}
	})
	t.Run("send request and checkout plan", func(t *testing.T) {
		t.Parallel()
		original := Command{Kind: "message.send", Send: &SendRequest{Text: "Start", Checkout: &CheckoutPlan{Kind: CheckoutNew, Base: "main"}}}
		frozen := original.Clone()
		if frozen.Send == original.Send || frozen.Send.Checkout == original.Send.Checkout {
			t.Fatal("cloned send command shares mutable request or checkout plan")
		}
		original.Send.Text, original.Send.Checkout.Base = "Caller edit", "caller-base"
		if frozen.Send.Text != "Start" || frozen.Send.Checkout.Base != "main" {
			t.Fatalf("caller mutation reached frozen send: %+v", frozen.Send)
		}
		frozen.Send.Text, frozen.Send.Checkout.Kind = "Worker edit", CheckoutCurrent
		if original.Send.Text != "Caller edit" || original.Send.Checkout.Kind != CheckoutNew {
			t.Fatal("worker mutation reached caller send")
		}
	})
}

func TestComposerFooterCommandValidateTypedPayloads(t *testing.T) {
	t.Parallel()
	scope := RepoRefScope{ProjectID: "project", TargetID: "host", RepoPath: "/repo"}
	source := footerSourceKey()
	for _, test := range []struct {
		name    string
		command Command
		wantErr bool
	}{
		{name: "list refs", command: Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope, Force: true}}},
		{name: "switch local ref", command: Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: scope, Ref: "topic"}}},
		{name: "refresh source PR", command: Command{Kind: "git.pr.refresh", Git: &GitRequest{Source: &source}}},
		{name: "send unresolved new worktree base", command: Command{Kind: "message.send", Send: &SendRequest{Checkout: &CheckoutPlan{Kind: CheckoutNew}}}},
		{name: "git payload on send", command: Command{Kind: "message.send", Git: &GitRequest{Scope: scope}}, wantErr: true},
		{name: "list missing payload", command: Command{Kind: "git.refs.load"}, wantErr: true},
		{name: "switch missing payload", command: Command{Kind: "git.ref.switch"}, wantErr: true},
		{name: "PR missing payload", command: Command{Kind: "git.pr.refresh"}, wantErr: true},
		{name: "list missing scope", command: Command{Kind: "git.refs.load", Git: &GitRequest{}}, wantErr: true},
		{name: "list cannot carry ref", command: Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope, Ref: "topic"}}, wantErr: true},
		{name: "list cannot carry PR source", command: Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope, Source: &source}}, wantErr: true},
		{name: "switch requires ref", command: Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: scope}}, wantErr: true},
		{name: "switch rejects blank ref", command: Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: scope, Ref: " \t"}}, wantErr: true},
		{name: "PR requires source", command: Command{Kind: "git.pr.refresh", Git: &GitRequest{}}, wantErr: true},
		{name: "PR cannot carry repository scope", command: Command{Kind: "git.pr.refresh", Git: &GitRequest{Scope: scope, Source: &source}}, wantErr: true},
		{name: "PR cannot carry switch ref", command: Command{Kind: "git.pr.refresh", Git: &GitRequest{Source: &source, Ref: "topic"}}, wantErr: true},
		{name: "send rejects invalid checkout variant", command: Command{Kind: "message.send", Send: &SendRequest{Checkout: &CheckoutPlan{Kind: CheckoutNew, Path: "/wt/old"}}}, wantErr: true},
		{name: "send payload on another command", command: Command{Kind: "session.create", Send: &SendRequest{Checkout: &CheckoutPlan{Kind: CheckoutCurrent}}}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.command.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate(%+v) = %v, want error %v", test.command, err, test.wantErr)
			}
		})
	}
	for _, test := range []struct {
		name   string
		change func(*CheckoutPRKey)
	}{
		{name: "host", change: func(key *CheckoutPRKey) { key.TargetID = "" }},
		{name: "checkout", change: func(key *CheckoutPRKey) { key.CheckoutID = "" }},
		{name: "repository root", change: func(key *CheckoutPRKey) { key.RepoRoot = "" }},
		{name: "branch", change: func(key *CheckoutPRKey) { key.Branch = "" }},
		{name: "nonblank branch", change: func(key *CheckoutPRKey) { key.Branch = " \t" }},
	} {
		t.Run("PR refresh requires "+test.name, func(t *testing.T) {
			t.Parallel()
			key := source
			test.change(&key)
			command := Command{Kind: "git.pr.refresh", Git: &GitRequest{Source: &key}}
			if err := command.Validate(); err == nil {
				t.Fatalf("accepted incomplete PR source: %+v", key)
			}
		})
	}
	t.Run("switch cannot target cloud branches", func(t *testing.T) {
		t.Parallel()
		cloudScope := scope
		cloudScope.CloudRepository = "acme/repo"
		if err := (Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: cloudScope, Ref: "topic"}}).Validate(); err == nil {
			t.Fatal("accepted local switch command for Cloud repository")
		}
	})
	for _, test := range []struct {
		name string
		add  func(*Command)
	}{
		{name: "integration", add: func(command *Command) { command.Integration = &IntegrationRequest{} }},
		{name: "send", add: func(command *Command) { command.Send = &SendRequest{} }},
		{name: "session", add: func(command *Command) { command.Session = &SessionUpdate{} }},
		{name: "settings", add: func(command *Command) { command.Settings = &Settings{} }},
		{name: "attachment", add: func(command *Command) { command.Attachment = &Attachment{} }},
		{name: "project create", add: func(command *Command) { command.ProjectCreate = &ProjectCreate{} }},
		{name: "project action", add: func(command *Command) { command.Action = &ProjectAction{} }},
		{name: "background adjustment", add: func(command *Command) { command.Adjustment = &BackgroundAdjustment{} }},
		{name: "comment", add: func(command *Command) { command.Comment = &CommentInput{} }},
		{name: "terminal size", add: func(command *Command) { command.TerminalSize = &TerminalSize{} }},
	} {
		t.Run("git cannot mix "+test.name+" payload", func(t *testing.T) {
			t.Parallel()
			command := Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope}}
			test.add(&command)
			if err := command.Validate(); err == nil {
				t.Fatal("accepted ambiguous git command payload")
			}
		})
	}
}

func footerSourceSession() Session {
	return Session{
		ID: "session", ProjectID: "project", DeviceID: "host", Branch: "legacy-branch", WorktreeID: "legacy-checkout", CWD: "/repo/subdir",
		SourceContext: &ConversationSourceContext{
			CheckoutID: "checkout", RepoRoot: "/repo", CWD: "/repo/subdir", Branch: "feature/source", HeadSHA: "original-head", ObservedAt: time.Unix(1000, 0),
		},
	}
}

func footerSourceKey() CheckoutPRKey {
	return CheckoutPRKey{TargetID: "host", CheckoutID: "checkout", RepoRoot: "/repo", Branch: "feature/source"}
}
