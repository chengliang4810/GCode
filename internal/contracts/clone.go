package contracts

import (
	"maps"
	"slices"

	"gcode/internal/model"
)

// CloneSnapshot deliberately shares only immutable strings and time values.
// This avoids serializing thousands of historical messages on each UI update.
func CloneSnapshot(v Snapshot) Snapshot {
	v.Plugins = v.Plugins.Clone()
	v.UserResources = v.UserResources.Clone()
	if v.Runtime != nil {
		info := *v.Runtime
		v.Runtime = &info
	}
	v.CustomProviders = slices.Clone(v.CustomProviders)
	for i := range v.CustomProviders {
		v.CustomProviders[i].Provider = model.CloneProvider(v.CustomProviders[i].Provider)
		v.CustomProviders[i].ModelChecks = CloneModelChecks(v.CustomProviders[i].ModelChecks)
		v.CustomProviders[i].ModelFetch = CloneModelFetch(v.CustomProviders[i].ModelFetch)
	}
	v.Integrations = cloneIntegrations(v.Integrations)
	v.ThemeLibrary = slices.Clone(v.ThemeLibrary)
	for i := range v.ThemeLibrary {
		v.ThemeLibrary[i].VariantIDs = slices.Clone(v.ThemeLibrary[i].VariantIDs)
	}
	if v.ThemeImport != nil {
		p := *v.ThemeImport
		p.Variants = slices.Clone(p.Variants)
		p.Warnings = slices.Clone(p.Warnings)
		v.ThemeImport = &p
	}
	v.Projects = slices.Clone(v.Projects)
	for i := range v.Projects {
		v.Projects[i].Branches = slices.Clone(v.Projects[i].Branches)
		v.Projects[i].Worktrees = slices.Clone(v.Projects[i].Worktrees)
	}
	v.Devices = slices.Clone(v.Devices)
	v.Sessions = slices.Clone(v.Sessions)
	v.SessionGroups = slices.Clone(v.SessionGroups)
	for i := range v.Sessions {
		v.Sessions[i].Goal = v.Sessions[i].Goal.Clone()
	}
	cloneComposerFooterGitSnapshots(&v)
	messages := make(map[string][]Message, len(v.Messages))
	for key, ms := range v.Messages {
		out := slices.Clone(ms)
		for i := range out {
			out[i].Blocks = CloneBlocks(out[i].Blocks)
		}
		messages[key] = out
	}
	v.Messages = messages
	if v.ContextUsage != nil {
		usage := make(map[string]ContextUsage, len(v.ContextUsage))
		for id, context := range v.ContextUsage {
			usage[id] = context.Clone()
		}
		v.ContextUsage = usage
	}
	v.Models = slices.Clone(v.Models)
	for i := range v.Models {
		v.Models[i].ReasoningLevels = slices.Clone(v.Models[i].ReasoningLevels)
		v.Models[i].Options = slices.Clone(v.Models[i].Options)
		for j := range v.Models[i].Options {
			v.Models[i].Options[j].Choices = slices.Clone(v.Models[i].Options[j].Choices)
			v.Models[i].Options[j].ChoiceLabels = maps.Clone(v.Models[i].Options[j].ChoiceLabels)
		}
	}
	v.Providers = slices.Clone(v.Providers)
	for i := range v.Providers {
		v.Providers[i].Models = slices.Clone(v.Providers[i].Models)
	}
	v.Settings.Shortcuts = slices.Clone(v.Settings.Shortcuts)
	v.Settings.Subagents = CloneSubagentProfiles(v.Settings.Subagents)
	v.Settings.Hooks = CloneHookRules(v.Settings.Hooks)
	v.Settings.ModelPicker.Options = maps.Clone(v.Settings.ModelPicker.Options)
	v.Settings.ModelPicker.Selections = maps.Clone(v.Settings.ModelPicker.Selections)
	v.Files = slices.Clone(v.Files)
	v.Changes = slices.Clone(v.Changes)
	for i := range v.Changes {
		v.Changes[i].Hunks = slices.Clone(v.Changes[i].Hunks)
		for j := range v.Changes[i].Hunks {
			v.Changes[i].Hunks[j].Lines = slices.Clone(v.Changes[i].Hunks[j].Lines)
		}
	}
	v.Commits = slices.Clone(v.Commits)
	for i := range v.Commits {
		v.Commits[i].Parents = slices.Clone(v.Commits[i].Parents)
		v.Commits[i].Refs = slices.Clone(v.Commits[i].Refs)
	}
	v.Terminals = slices.Clone(v.Terminals)
	v.Queue = slices.Clone(v.Queue)
	for i := range v.Queue {
		v.Queue[i].Annotations = slices.Clone(v.Queue[i].Annotations)
		for j := range v.Queue[i].Annotations {
			v.Queue[i].Annotations[j] = v.Queue[i].Annotations[j].Bounded()
		}
		v.Queue[i].Attachments = slices.Clone(v.Queue[i].Attachments)
	}
	v.Runs = slices.Clone(v.Runs)
	v.ModelRequests = slices.Clone(v.ModelRequests)
	for i := range v.Runs {
		if v.Runs[i].FinishedAt != nil {
			t := *v.Runs[i].FinishedAt
			v.Runs[i].FinishedAt = &t
		}
	}
	v.Account.Usage = slices.Clone(v.Account.Usage)
	v.Account.Invoices = slices.Clone(v.Account.Invoices)
	v.Appshots = slices.Clone(v.Appshots)
	v.Attachments = slices.Clone(v.Attachments)
	return v
}

func CloneBlocks(src []Block) []Block {
	out := slices.Clone(src)
	for i := range out {
		b := &out[i]
		if b.Annotation != nil {
			v := b.Annotation.Bounded()
			b.Annotation = &v
		}
		if b.Tool != nil {
			v := *b.Tool
			if v.ExitCode != nil {
				code := *v.ExitCode
				v.ExitCode = &code
			}
			if v.Diff != nil {
				change := *v.Diff
				change.Hunks = slices.Clone(change.Hunks)
				for hi := range change.Hunks {
					change.Hunks[hi].Lines = slices.Clone(change.Hunks[hi].Lines)
				}
				v.Diff = &change
			}
			if v.FinishedAt != nil {
				t := *v.FinishedAt
				v.FinishedAt = &t
			}
			b.Tool = &v
		}
		if b.Question != nil {
			v := *b.Question
			v.Options = slices.Clone(v.Options)
			v.OptionDescriptions = slices.Clone(v.OptionDescriptions)
			b.Question = &v
		}
		if b.Permission != nil {
			v := *b.Permission
			b.Permission = &v
		}
		b.Todos = slices.Clone(b.Todos)
		if b.Subagent != nil {
			v := *b.Subagent
			b.Subagent = &v
		}
		if b.Attachment != nil {
			v := *b.Attachment
			b.Attachment = &v
		}
		if b.Artifact != nil {
			v := *b.Artifact
			b.Artifact = &v
		}
	}
	return out
}

func cloneIntegrations(v IntegrationState) IntegrationState {
	v.Providers = slices.Clone(v.Providers)
	for i := range v.Providers {
		v.Providers[i].LoginOptions = slices.Clone(v.Providers[i].LoginOptions)
	}
	v.Accounts = slices.Clone(v.Accounts)
	for i := range v.Accounts {
		v.Accounts[i].Usage = slices.Clone(v.Accounts[i].Usage)
	}
	if v.Login != nil {
		x := *v.Login
		v.Login = &x
	}
	v.Cloud.Repositories = slices.Clone(v.Cloud.Repositories)
	v.Cloud.Connections = slices.Clone(v.Cloud.Connections)
	v.Cloud.Usage = slices.Clone(v.Cloud.Usage)
	v.Billing.Plans = slices.Clone(v.Billing.Plans)
	if v.Billing.Subscription != nil {
		x := *v.Billing.Subscription
		v.Billing.Subscription = &x
	}
	if v.Billing.Checkout != nil {
		x := *v.Billing.Checkout
		v.Billing.Checkout = &x
	}
	return v
}

func cloneComposerFooterGitSnapshots(v *Snapshot) {
	v.RepoRefs = maps.Clone(v.RepoRefs)
	for key, resource := range v.RepoRefs {
		resource.Rows = slices.Clone(resource.Rows)
		v.RepoRefs[key] = resource
	}
	for i := range v.Sessions {
		if v.Sessions[i].SourceContext != nil {
			copy := *v.Sessions[i].SourceContext
			v.Sessions[i].SourceContext = &copy
		}
	}
}
