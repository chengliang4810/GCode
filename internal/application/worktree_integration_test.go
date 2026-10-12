package application

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gcode/internal/appmodel"
	"gcode/internal/worktrees"
)

func TestServiceWorktreeDefaultBranchAndRestart(t *testing.T) {
	ctx := context.Background()
	repo := applicationWorktreeRepository(t, true)
	dataDir := t.TempDir()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, repo); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		_ = service.Close(ctx)
		t.Fatalf("project snapshot = %#v, error = %v", snapshot.Projects, err)
	}
	projectID := snapshot.Projects[0].ID
	canonicalRepo := applicationWorktreeCanonicalPath(t, repo)

	// The ordinary session path is the registered project and never creates a
	// manager worktree unless the per-session option explicitly enables it.
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: projectID, Title: "default"}); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	defaultSession := applicationWorktreeSession(t, snapshot, "default")
	if defaultSession.Worktree || defaultSession.Cwd != canonicalRepo || defaultSession.Branch != "" {
		_ = service.Close(ctx)
		t.Fatalf("default session unexpectedly uses worktree: %#v", defaultSession)
	}

	branches, err := service.ListBranches(ctx, projectID)
	if err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	wantBranches := []string{"feature-base", "main"}
	if !worktreeEqualStrings(branches, wantBranches) {
		_ = service.Close(ctx)
		t.Fatalf("local branches = %#v, want %#v", branches, wantBranches)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{
		ProjectID:  projectID,
		Title:      "isolated",
		Worktree:   true,
		BaseBranch: "feature-base",
	}); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	isolated := applicationWorktreeSession(t, snapshot, "isolated")
	if !isolated.Worktree || isolated.Cwd == canonicalRepo || isolated.Branch == "" {
		_ = service.Close(ctx)
		t.Fatalf("isolated session binding = %#v", isolated)
	}
	if got := applicationWorktreeCanonicalPath(t, applicationWorktreeGit(t, isolated.Cwd, "rev-parse", "--show-toplevel")); got != isolated.Cwd {
		_ = service.Close(ctx)
		t.Fatalf("worktree root = %q, snapshot cwd = %q", got, isolated.Cwd)
	}
	if got := applicationWorktreeGit(t, isolated.Cwd, "branch", "--show-current"); got != isolated.Branch {
		_ = service.Close(ctx)
		t.Fatalf("worktree branch = %q, snapshot branch = %q", got, isolated.Branch)
	}
	if got := applicationWorktreeGit(t, isolated.Cwd, "rev-parse", "--abbrev-ref", "HEAD"); got != "gcode/"+isolated.ID {
		_ = service.Close(ctx)
		t.Fatalf("worktree branch from session id = %q, session = %q", got, isolated.ID)
	}
	if _, err := os.Stat(filepath.Join(isolated.Cwd, "feature-base.txt")); err != nil {
		_ = service.Close(ctx)
		t.Fatalf("base branch file missing from worktree: %v", err)
	}
	if got := string(applicationWorktreeReadFile(t, filepath.Join(isolated.Cwd, "README.md"))); got != "initial\n" {
		_ = service.Close(ctx)
		t.Fatalf("tracked source modification copied into worktree: %q", got)
	}
	if _, err := os.Stat(filepath.Join(isolated.Cwd, "source-uncommitted.txt")); !errors.Is(err, os.ErrNotExist) {
		_ = service.Close(ctx)
		t.Fatalf("source uncommitted file copied into worktree: %v", err)
	}
	if got := applicationWorktreeGit(t, canonicalRepo, "branch", "--show-current"); got != "main" {
		_ = service.Close(ctx)
		t.Fatalf("source branch changed = %q", got)
	}
	if _, err := os.Stat(filepath.Join(canonicalRepo, "source-uncommitted.txt")); err != nil {
		_ = service.Close(ctx)
		t.Fatalf("source uncommitted file was changed or removed: %v", err)
	}
	if got := string(applicationWorktreeReadFile(t, filepath.Join(canonicalRepo, "README.md"))); got != "source modified\n" {
		_ = service.Close(ctx)
		t.Fatalf("source tracked modification was changed: %q", got)
	}
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(isolated.Cwd); err != nil {
		t.Fatalf("Close removed worktree: %v", err)
	}

	reopened, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	restartedSnapshot, err := reopened.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restarted := applicationWorktreeSession(t, restartedSnapshot, "isolated")
	if restarted.ID != isolated.ID || restarted.Cwd != isolated.Cwd || restarted.Branch != isolated.Branch || !restarted.Worktree {
		t.Fatalf("restarted worktree session = %#v, before = %#v", restarted, isolated)
	}
	if defaultAfterRestart := applicationWorktreeSession(t, restartedSnapshot, "default"); defaultAfterRestart.Worktree || defaultAfterRestart.Cwd != canonicalRepo {
		t.Fatalf("default session changed after restart = %#v", defaultAfterRestart)
	}
	if err := reopened.RemoveWorktree(ctx, restarted.ID); err != nil {
		t.Fatalf("remove restored worktree = %v", err)
	}
	if _, err := os.Stat(restarted.Cwd); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restored worktree remains after explicit remove: %v", err)
	}
	t.Logf("service worktree created at %s on %s from %s and restored after Close/reopen", isolated.Cwd, isolated.Branch, "feature-base")
}

func TestServiceWorktreeOperationsProtectAndCloseExplicitly(t *testing.T) {
	ctx := context.Background()
	repo := applicationWorktreeRepository(t, false)
	dataDir := t.TempDir()
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(ctx)
	if err := service.AddProject(ctx, repo); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("project snapshot = %#v, error = %v", snapshot.Projects, err)
	}
	projectID := snapshot.Projects[0].ID
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{ProjectID: projectID, Title: "operations", Worktree: true, BaseBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session := applicationWorktreeSession(t, snapshot, "operations")

	if err := os.WriteFile(filepath.Join(session.Cwd, "pending.txt"), []byte("pending\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = service.RemoveWorktree(ctx, session.ID)
	if !errors.Is(err, worktrees.ErrSessionWorktreeDirty) {
		t.Fatalf("remove dirty worktree = %v, want %v", err, worktrees.ErrSessionWorktreeDirty)
	}
	if _, statErr := os.Stat(filepath.Join(session.Cwd, "pending.txt")); statErr != nil {
		t.Fatalf("dirty removal lost pending file: %v", statErr)
	}
	if err := os.Remove(filepath.Join(session.Cwd, "pending.txt")); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(session.Cwd, "feature.txt"), []byte("feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = service.MergeWorktree(ctx, session.ID, "main")
	if !errors.Is(err, worktrees.ErrSessionWorktreeDirty) {
		t.Fatalf("merge dirty worktree = %v, want %v", err, worktrees.ErrSessionWorktreeDirty)
	}
	if _, statErr := os.Stat(session.Cwd); statErr != nil {
		t.Fatalf("failed merge removed worktree: %v", statErr)
	}
	applicationWorktreeGit(t, session.Cwd, "add", "feature.txt")
	applicationWorktreeGit(t, session.Cwd, "commit", "-qm", "feature")

	// A dirty source checkout rejects merge after the feature has been
	// committed, and the worktree remains available for retry.
	if err := os.WriteFile(filepath.Join(repo, "source-pending.txt"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = service.MergeWorktree(ctx, session.ID, "main")
	if err == nil || !strings.Contains(err.Error(), "source repository contains uncommitted") {
		t.Fatalf("merge dirty source = %v", err)
	}
	if _, statErr := os.Stat(session.Cwd); statErr != nil {
		t.Fatalf("source-dirty merge removed worktree: %v", statErr)
	}
	if err := os.Remove(filepath.Join(repo, "source-pending.txt")); err != nil {
		t.Fatal(err)
	}

	if err := service.MergeWorktree(ctx, session.ID, "main"); err != nil {
		t.Fatal(err)
	}
	if got := string(applicationWorktreeReadFile(t, filepath.Join(repo, "feature.txt"))); got != "feature\n" {
		t.Fatalf("merged feature = %q", got)
	}
	if err := service.RemoveWorktree(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(session.Cwd); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("explicit remove left worktree: %v", statErr)
	}
	finalSnapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finalSession := applicationWorktreeSession(t, finalSnapshot, "operations")
	if !finalSession.Worktree || !strings.Contains(finalSession.Error, "worktree 已删除") {
		t.Fatalf("removed session state = %#v", finalSession)
	}
	t.Logf("dirty operations were rejected; merge and explicit remove completed for %s", session.ID)
}

func applicationWorktreeRepository(t *testing.T, withFeatureBase bool) string {
	t.Helper()
	repo := t.TempDir()
	applicationWorktreeGit(t, repo, "init", "-q", "-b", "main")
	applicationWorktreeGit(t, repo, "config", "user.email", "gcode@example.invalid")
	applicationWorktreeGit(t, repo, "config", "user.name", "GCode Integration Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	applicationWorktreeGit(t, repo, "add", "README.md")
	applicationWorktreeGit(t, repo, "commit", "-qm", "initial")
	if withFeatureBase {
		applicationWorktreeGit(t, repo, "checkout", "-qb", "feature-base")
		if err := os.WriteFile(filepath.Join(repo, "feature-base.txt"), []byte("feature base\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		applicationWorktreeGit(t, repo, "add", "feature-base.txt")
		applicationWorktreeGit(t, repo, "commit", "-qm", "feature base")
		applicationWorktreeGit(t, repo, "checkout", "main")
		if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("source modified\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "source-uncommitted.txt"), []byte("source only\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return applicationWorktreeCanonicalPath(t, repo)
}

func applicationWorktreeGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %v: %v\n%s", directory, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func applicationWorktreeCanonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(abs)
}

func applicationWorktreeReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func applicationWorktreeSession(t *testing.T, snapshot appmodel.Snapshot, title string) appmodel.Session {
	t.Helper()
	for _, session := range snapshot.Sessions {
		if session.Title == title {
			return session
		}
	}
	t.Fatalf("session %q not found in %#v", title, snapshot.Sessions)
	return appmodel.Session{}
}

func worktreeEqualStrings(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
