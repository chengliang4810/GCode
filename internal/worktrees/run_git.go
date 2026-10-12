package worktrees

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type GitRunner func(context.Context, string, ...string) (CommandResult, error)

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

var worktreeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$`)

func validateName(name string) error {
	if !worktreeNamePattern.MatchString(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid worktree name %q: use letters, numbers, dots, dashes, or underscores", name)
	}
	return nil
}

func gitOutput(ctx context.Context, runGit GitRunner, dir string, args ...string) (string, error) {
	result, err := runGit(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(firstNonEmpty(result.Stderr, result.Stdout))
		if message == "" {
			message = fmt.Sprintf("git exited with code %d", result.ExitCode)
		}
		return "", errors.New(message)
	}
	return strings.TrimSpace(result.Stdout), nil
}

func primaryWorktreeRoot(ctx context.Context, runGit GitRunner, dir string) (string, error) {
	output, err := gitOutput(ctx, runGit, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	entries := parseWorktreeList(output)
	if len(entries) == 0 {
		return "", errors.New("resolve repository root: git worktree list returned no entries")
	}
	return entries[0].path, nil
}

func newHardenedCommand(ctx context.Context, dir string, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	hardenWorktreeGit(command)
	return command
}

func defaultRunGit(ctx context.Context, dir string, args ...string) (CommandResult, error) {
	command := newHardenedCommand(ctx, dir, "git", args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
			err = nil
		}
	}
	return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}, err
}

func repoKey(repoRoot string) string {
	sum := sha1.Sum([]byte(filepath.Clean(repoRoot)))
	hash := hex.EncodeToString(sum[:])[:10]
	base := strings.ToLower(filepath.Base(repoRoot))
	base = strings.Trim(regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(base, "-"), "-._")
	if base == "" {
		base = "repo"
	}
	return base + "-" + hash
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type worktreeEntry struct {
	path       string
	locked     bool
	lockReason string
}

func parseWorktreeList(output string) []worktreeEntry {
	var entries []worktreeEntry
	var current *worktreeEntry
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current != nil {
				entries = append(entries, *current)
			}
			current = &worktreeEntry{path: canonicalizePath(strings.TrimPrefix(line, "worktree "))}
		case current != nil && (line == "locked" || strings.HasPrefix(line, "locked ")):
			current.locked = true
			current.lockReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}
	return entries
}

func canonicalizePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolveThroughNearestExistingAncestor(filepath.Clean(abs))
}

func resolveThroughNearestExistingAncestor(path string) string {
	var missing []string
	current := path
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			result := filepath.Clean(resolved)
			for index := len(missing) - 1; index >= 0; index-- {
				result = filepath.Join(result, missing[index])
			}
			return result
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
