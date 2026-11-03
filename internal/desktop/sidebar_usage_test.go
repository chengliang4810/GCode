package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSidebarUsageOpensApplicationStatisticsSettings(t *testing.T) {
	a := testApp(t, "default")
	now := a.now()
	messages := map[string][]contracts.Message{
		"one": {{ID: "one", ModelID: "provider/model-a", CreatedAt: now, Usage: contracts.Usage{InputTokens: 100, OutputTokens: 30, CachedTokens: 60}}},
		"two": {{ID: "two", ModelID: "provider/model-b", CreatedAt: now.AddDate(0, 0, -8), Usage: contracts.Usage{InputTokens: 50, OutputTokens: 20, CachedTokens: 10}}},
	}
	a.state.Messages = messages
	a.state.Runs = nil
	tt := ui.NewTester(a.View, 1320, 1200)
	if err := tt.Click(a.t("Usage statistics")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != "settings" || a.section != "Usage statistics" {
		t.Fatalf("usage entry did not open settings: %s/%s", a.route, a.section)
	}
	for _, value := range []string{"200", a.t("Token activity"), a.t("Daily model usage"), a.t("Model usage"), "provider/model-a"} {
		if !tt.HasText(value) {
			t.Fatalf("usage page is missing %q", value)
		}
	}
	if tt.HasText("provider/model-b") {
		t.Fatal("default 7-day chart included an older request")
	}
	if err := tt.Click(a.t("Last 30 days")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("provider/model-b") || !tt.HasText("200") {
		t.Fatal("30-day filter lost the older model or changed lifetime totals")
	}
	for _, label := range []string{"Weekly", "Cumulative", "Daily"} {
		if err := tt.Click(a.t(label)); err != nil {
			t.Fatal(err)
		}
	}
	a.now = func() time.Time { return now.AddDate(0, 0, 1) }
	a.refreshAppUsage()
	if !a.appUsage.generated.Equal(a.now()) {
		t.Fatal("refresh did not rebuild calendar bounds")
	}
	if err := tt.Click(a.t("Back to conversations")); err != nil {
		t.Fatal(err)
	}
	if a.route != "chat" {
		t.Fatal("cannot return to conversations from usage settings")
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
