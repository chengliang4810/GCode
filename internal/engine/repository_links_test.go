package engine

import (
	"context"
	"testing"

	"gcode/internal/contracts"
)

func TestGitInspectionPublishesPullRequestsLink(t *testing.T) {
	repository := temporaryGitRepository(t)
	runGitTest(t, repository, "remote", "add", "origin", "git@github.com:example/project.git")
	repo, err := inspectGit(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	core := &Engine{state: contracts.Snapshot{Projects: []contracts.Project{{ID: "project", Path: repository}}}}
	core.applyGitProjectLocked("project", repo)
	if got := core.state.Projects[0].PullRequestsURL; got != "https://github.com/example/project/pulls" {
		t.Fatalf("got %q", got)
	}
	runGitTest(t, repository, "remote", "remove", "origin")
	repo, err = inspectGit(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	core.applyGitProjectLocked("project", repo)
	if core.state.Projects[0].PullRequestsURL != "" {
		t.Fatal("removed origin retained a stale PR URL")
	}
}
