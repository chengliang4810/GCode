package worktrees

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManagerCreatesBranchedWorktreeAndProtectsRemoval(t *testing.T) {
	repo := managerTestRepository(t)
	if err := os.WriteFile(filepath.Join(repo, "source-uncommitted.txt"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ManagerOptions{BaseDir: filepath.Join(t.TempDir(), "worktrees")})
	worktree, err := manager.Create(context.Background(), CreateOptions{SourceDir: repo, Name: "session-one"})
	if err != nil {
		t.Fatal(err)
	}
	if worktree.Branch != "gcode/session-one" {
		t.Fatalf("branch = %q", worktree.Branch)
	}
	if _, err := os.Stat(filepath.Join(worktree.Path, "source-uncommitted.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncommitted source file copied: %v", err)
	}
	if err := manager.Bind(worktree.Path, "session-a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), worktree.Path); !errors.Is(err, ErrSessionWorktreeInUse) {
		t.Fatalf("remove while bound = %v", err)
	}
	manager.Unbind(worktree.Path, "session-a")
	if err := os.WriteFile(filepath.Join(worktree.Path, "pending.txt"), []byte("pending"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), worktree.Path); !errors.Is(err, ErrSessionWorktreeDirty) {
		t.Fatalf("remove dirty = %v", err)
	}
	os.Remove(filepath.Join(worktree.Path, "pending.txt"))
	if err := manager.Remove(context.Background(), worktree.Path); err != nil {
		t.Fatal(err)
	}
}

func TestManagerMergesWorktreeBranch(t *testing.T) {
	repo := managerTestRepository(t)
	manager := NewManager(ManagerOptions{BaseDir: filepath.Join(t.TempDir(), "worktrees")})
	worktree, err := manager.Create(context.Background(), CreateOptions{SourceDir: repo, Name: "session-two"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree.Path, "feature.txt"), []byte("feature"), 0o644); err != nil {
		t.Fatal(err)
	}
	managerGit(t, worktree.Path, "add", "feature.txt")
	managerGit(t, worktree.Path, "commit", "-m", "feature")
	if _, err := manager.Merge(context.Background(), worktree.Path, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "feature.txt"))
	if err != nil || string(data) != "feature" {
		t.Fatalf("merged file = %q, err=%v", data, err)
	}
	if err := manager.Remove(context.Background(), worktree.Path); err != nil {
		t.Fatal(err)
	}
}

func TestManagerAcceptsSymlinkAliasForConfiguredBase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires platform support")
	}
	repo := managerTestRepository(t)
	physicalBase := t.TempDir()
	aliasParent := t.TempDir()
	aliasBase := filepath.Join(aliasParent, "worktrees")
	if err := os.Symlink(physicalBase, aliasBase); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	manager := NewManager(ManagerOptions{BaseDir: aliasBase})
	worktree, err := manager.Create(context.Background(), CreateOptions{SourceDir: repo, Name: "symlink-session"})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Bind(worktree.Path, "session-a"); err != nil {
		t.Fatalf("bind through configured symlink base: %v", err)
	}
	manager.Unbind(worktree.Path, "session-a")
	if err := manager.Remove(context.Background(), worktree.Path); err != nil {
		t.Fatalf("remove through configured symlink base: %v", err)
	}
}

func managerTestRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	managerGit(t, repo, "init", "-q")
	managerGit(t, repo, "config", "user.email", "gcode@example.invalid")
	managerGit(t, repo, "config", "user.name", "GCode Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	managerGit(t, repo, "add", "README.md")
	managerGit(t, repo, "commit", "-qm", "initial")
	return repo
}

func managerGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
