package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gcode/internal/appmodel"
)

func TestNormalizePathRejectsOutsideAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NormalizePath(filepath.Join(root, "..", filepath.Base(outside), "secret.txt")); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("expected outside root error, got %v", err)
	}
	symlink := filepath.Join(root, "link")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := service.NormalizePath(filepath.Join("link", "secret.txt")); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
}

func TestWriteFileRejectsStaleVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, version, err := service.ReadFile(context.Background(), "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = service.WriteFile(context.Background(), "main.go", []byte("agent"), WriteOptions{Expected: &version})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestConcurrentWritesAllowOneExpectedVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, version, err := service.ReadFile(context.Background(), "main.go")
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for _, content := range []string{"one", "two"} {
		wait.Add(1)
		go func(value string) {
			defer wait.Done()
			_, writeErr := service.WriteFile(context.Background(), "main.go", []byte(value), WriteOptions{Expected: &version})
			results <- writeErr
		}(content)
	}
	wait.Wait()
	close(results)
	var success, conflict int
	for writeErr := range results {
		if writeErr == nil {
			success++
		} else if errors.Is(writeErr, ErrConflict) {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", success, conflict)
	}
}

func TestReadFilePreviewTruncatesAndMarksBinary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte("abc\x00def"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.ReadFilePreview(context.Background(), "data.bin", 4)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Text != "abc\x00" || !preview.Truncated || !preview.Binary {
		t.Fatalf("unexpected preview: %+v", preview)
	}
}

func TestReadFileAndOpenEditorHonorCanceledContext(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	service, err := NewWithConfig(root, Config{
		Editor: func(context.Context, string, ...string) (*EditorProcess, error) {
			called = true
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := service.ReadFile(ctx, "main.go"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadFile error = %v, want context.Canceled", err)
	}
	if err := service.OpenEditor(ctx, "main.go", EditorOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenEditor error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("OpenEditor launched an editor after context cancellation")
	}
}

func TestGitStatusAndDiffUseNestedWorkspacePaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	repository := t.TempDir()
	workspaceRoot := filepath.Join(repository, "project")
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceTestFile(t, filepath.Join(workspaceRoot, "tracked.txt"), "before\n")
	writeWorkspaceTestFile(t, filepath.Join(repository, "outer.txt"), "before\n")
	runWorkspaceTestGit(t, repository, "init", "-q")
	runWorkspaceTestGit(t, repository, "add", ".")
	runWorkspaceTestGit(t, repository, "-c", "user.name=GCode Fixture", "-c", "user.email=gcode-fixture@example.com", "commit", "-qm", "fixture")

	writeWorkspaceTestFile(t, filepath.Join(workspaceRoot, "tracked.txt"), "changed\n")
	writeWorkspaceTestFile(t, filepath.Join(repository, "outer.txt"), "outer changed\n")
	writeWorkspaceTestFile(t, filepath.Join(workspaceRoot, "untracked.txt"), "new content\n")
	writeWorkspaceTestFile(t, filepath.Join(repository, "outer-untracked.txt"), "outside\n")

	service, err := New(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.GitStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.IsGit {
		t.Fatal("nested workspace was not detected as a Git repository")
	}
	statuses := make(map[string]string, len(state.Changes))
	for _, change := range state.Changes {
		statuses[change.Path] = change.Status
	}
	if statuses["tracked.txt"] != "M" || statuses["untracked.txt"] != "untracked" {
		t.Fatalf("nested workspace status = %#v, want tracked and untracked cwd-relative paths", statuses)
	}
	for _, path := range []string{"outer.txt", "outer-untracked.txt", "project/tracked.txt", "project/untracked.txt"} {
		if _, ok := statuses[path]; ok {
			t.Fatalf("outer/repository-relative path leaked into nested workspace status: %q in %#v", path, statuses)
		}
	}

	changes, err := service.GitDiff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	diffs := make(map[string]appmodel.Change, len(changes))
	for _, change := range changes {
		diffs[change.Path] = change
	}
	if !strings.Contains(diffs["tracked.txt"].Diff, "+changed") {
		t.Fatalf("tracked nested diff = %#v", diffs["tracked.txt"])
	}
	if !strings.Contains(diffs["untracked.txt"].Diff, "+new content") {
		t.Fatalf("untracked nested diff = %#v", diffs["untracked.txt"])
	}
	if _, ok := diffs["outer.txt"]; ok {
		t.Fatalf("outer tracked diff leaked into nested workspace: %#v", diffs["outer.txt"])
	}
	preview, err := service.ReadFilePreview(context.Background(), "untracked.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Path != "untracked.txt" || preview.Text != "new content\n" {
		t.Fatalf("nested preview = %#v", preview)
	}
}

func writeWorkspaceTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runWorkspaceTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
