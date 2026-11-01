package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarUsageAggregatesRecordedConversations(t *testing.T) {
	messages := map[string][]contracts.Message{
		"one": {{Usage: contracts.Usage{InputTokens: 100, OutputTokens: 30, CachedTokens: 60}}},
		"two": {{Usage: contracts.Usage{InputTokens: 50, OutputTokens: 20, CachedTokens: 10}},
			{Usage: contracts.Usage{InputTokens: -1, OutputTokens: -1, CachedTokens: -1}}},
	}
	got := summarizeConversationUsage(messages)
	if got.Input != 150 || got.Output != 50 || got.Cached != 70 || got.Input+got.Output != 200 {
		t.Fatalf("cached input must not be counted twice: %+v", got)
	}
	a := testApp(t, "default")
	a.state.Messages = messages
	tt := ui.NewTester(a.View, 900, 600)
	if err := tt.Click(a.t("Usage statistics")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, value := range []string{"150", "50", "70", "200"} {
		if !tt.HasText(value) {
			t.Fatalf("usage dialog is missing %q", value)
		}
	}
	if err := tt.Click(a.t("Close dialog")); err != nil {
		t.Fatal(err)
	}
	if a.sidebarUI.usageOpen {
		t.Fatal("usage dialog did not close")
	}
}

func TestSidebarUsageCountsForkedExecutionOnce(t *testing.T) {
	messages := map[string][]contracts.Message{
		"parent": {
			{TurnID: "run-one", Usage: contracts.Usage{InputTokens: 100, OutputTokens: 30, CachedTokens: 60}},
			{TurnID: "run-one", Usage: contracts.Usage{InputTokens: 50, OutputTokens: 20, CachedTokens: 10}},
		},
		"fork": {
			{TurnID: "run-one", Usage: contracts.Usage{InputTokens: 100, OutputTokens: 30, CachedTokens: 60}},
			{TurnID: "run-one", Usage: contracts.Usage{InputTokens: 50, OutputTokens: 20, CachedTokens: 10}},
			{TurnID: "run-two", Usage: contracts.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	if usage := summarizeConversationUsage(messages); usage.Input != 160 || usage.Output != 55 || usage.Cached != 70 {
		t.Fatalf("forked history was counted again or a new step was lost: %+v", usage)
	}
}

func TestSidebarPullRequestsOpensCurrentProject(t *testing.T) {
	a := testApp(t, "default")
	project, found := a.footerGitProject()
	if !found {
		t.Fatal("fixture has no selected project")
	}
	const want = "https://github.com/example/current/pulls"
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == project.ID {
			a.state.Projects[i].GitDetected = true
			a.state.Projects[i].PullRequestsURL = want
		}
	}
	tt := ui.NewTester(a.View, 900, 600)
	if err := tt.Click(a.t("Pull requests")); err != nil {
		t.Fatal(err)
	}
	if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != want {
		t.Fatalf("opened URLs = %v", urls)
	}
}

func TestSidebarPullRequestsUsesCompletedHostMetadata(t *testing.T) {
	a := testApp(t, "default")
	a.sessionID = ""
	project, found := a.footerGitProject()
	if !found {
		t.Fatal("fixture has no selected project")
	}
	tt := ui.NewTester(a.View, 900, 600)
	scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: project.DeviceID, RepoPath: project.Path}
	a.sidebarUI.pullRequests = &scope
	next := contracts.CloneSnapshot(a.state)
	next.Revision++
	const want = "https://github.com/example/refreshed/pulls"
	for i := range next.Projects {
		if next.Projects[i].ID == project.ID {
			next.Projects[i].PullRequestsURL = want
		}
	}
	if next.RepoRefs == nil {
		next.RepoRefs = make(map[string]contracts.RepoRefs)
	}
	next.RepoRefs[scope.Key()] = contracts.RepoRefs{Scope: scope, State: contracts.LoadReady}
	a.apply(next)
	if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != want || a.sidebarUI.pullRequests != nil {
		t.Fatalf("completed host metadata was not opened once: %v", urls)
	}
	a.sidebarUI.pullRequests = &scope
	a.projectID = "another-project"
	a.finishSidebarPullRequests()
	if len(tt.OpenedURLs()) != 1 || a.sidebarUI.pullRequests != nil {
		t.Fatal("a late reply for the previous project opened a browser")
	}
}
