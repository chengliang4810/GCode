package contracts

import "testing"

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

func TestComposerFooterCommandCloneOwnsNestedPayloads(t *testing.T) {
	t.Parallel()
	t.Run("git request", func(t *testing.T) {
		t.Parallel()
		scope := RepoRefScope{ProjectID: "project", TargetID: "host", RepoPath: "/repo"}
		original := Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope, Force: true}}
		frozen := original.Clone()
		if frozen.Git == original.Git {
			t.Fatal("cloned git command shares mutable request")
		}
		original.Git.Force = false
		original.Git.Scope.TargetID = "changed-by-caller"
		if !frozen.Git.Force || frozen.Git.Scope != scope {
			t.Fatalf("caller mutation reached frozen command: %+v", frozen.Git)
		}
		frozen.Git.Ref = "worker-ref"
		if original.Git.Ref != "" {
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
	for _, test := range []struct {
		name    string
		command Command
		wantErr bool
	}{
		{name: "list refs", command: Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope, Force: true}}},
		{name: "switch local ref", command: Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: scope, Ref: "topic"}}},
		{name: "send unresolved new worktree base", command: Command{Kind: "message.send", Send: &SendRequest{Checkout: &CheckoutPlan{Kind: CheckoutNew}}}},
		{name: "git payload on send", command: Command{Kind: "message.send", Git: &GitRequest{Scope: scope}}, wantErr: true},
		{name: "list missing payload", command: Command{Kind: "git.refs.load"}, wantErr: true},
		{name: "switch missing payload", command: Command{Kind: "git.ref.switch"}, wantErr: true},
		{name: "list missing scope", command: Command{Kind: "git.refs.load", Git: &GitRequest{}}, wantErr: true},
		{name: "list cannot carry ref", command: Command{Kind: "git.refs.load", Git: &GitRequest{Scope: scope, Ref: "topic"}}, wantErr: true},
		{name: "switch requires ref", command: Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: scope}}, wantErr: true},
		{name: "switch rejects blank ref", command: Command{Kind: "git.ref.switch", Git: &GitRequest{Scope: scope, Ref: " \t"}}, wantErr: true},
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
		{name: "background adjustment", add: func(command *Command) { command.Adjustment = &BackgroundAdjustment{} }},
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
