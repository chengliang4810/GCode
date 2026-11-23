package engine

import (
	"context"
	"errors"
	"fmt"
	"gcode/internal/contracts"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func historyGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, data)
	}
	return string(data)
}

func TestGitHistoryReadsCurrentCheckout(t *testing.T) {
	dir := t.TempDir()
	historyGit(t, dir, "init", "-b", "main")
	historyGit(t, dir, "config", "user.name", "历史作者")
	historyGit(t, dir, "config", "user.email", "history@example.test")
	result, err := readGitHistory(t.Context(), dir)
	if err != nil || !result.Repository || len(result.Commits) != 0 {
		t.Fatalf("empty repo: %+v %v", result, err)
	}
	historyGit(t, dir, "commit", "--allow-empty", "-m", "第一个提交")
	historyGit(t, dir, "switch", "-c", "feature")
	historyGit(t, dir, "commit", "--allow-empty", "-m", "feature work")
	historyGit(t, dir, "switch", "main")
	historyGit(t, dir, "merge", "--no-ff", "feature", "-m", "merge feature")
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	core, _ := newChangeCore(t, sub)
	result, err = core.ListGitHistory(t.Context(), contracts.GitHistoryRequest{SessionID: "session"})
	if err != nil || len(result.Commits) != 3 || result.Root != dir || result.Commits[0].Subject != "merge feature" || len(result.Commits[0].Parents) != 2 || result.Commits[0].Author != "历史作者" || len(result.Commits[0].Refs) == 0 || result.Commits[0].AuthoredAt.IsZero() {
		t.Fatalf("history: %+v %v", result, err)
	}
	if _, err := core.ListGitHistory(t.Context(), contracts.GitHistoryRequest{SessionID: "missing"}); err == nil {
		t.Fatal("unknown session accepted")
	}
	result, err = readGitHistory(t.Context(), t.TempDir())
	if err != nil || result.Repository {
		t.Fatalf("non-repo: %+v %v", result, err)
	}
	if _, err := readGitHistory(t.Context(), filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory ignored")
	}
}

func TestGitHistoryBoundsAndCancellation(t *testing.T) {
	dir := t.TempDir()
	historyGit(t, dir, "init", "-b", "main")
	var input strings.Builder
	for i := 0; i < 501; i++ {
		message := fmt.Sprintf("commit %d", i)
		fmt.Fprintf(&input, "commit refs/heads/main\ncommitter Test <history@example.test> %d +0000\ndata %d\n%s\n\n", 1700000000+i, len(message), message)
	}
	command := exec.Command("git", "-C", dir, "fast-import", "--quiet")
	command.Stdin = strings.NewReader(input.String())
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fast import: %v %s", err, data)
	}
	result, err := readGitHistory(t.Context(), dir)
	if err != nil || !result.Truncated || len(result.Commits) != 500 || result.Commits[0].Subject != "commit 500" {
		t.Fatalf("bounded history: count=%d truncated=%t error=%v", len(result.Commits), result.Truncated, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readGitHistory(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
