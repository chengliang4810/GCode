package worktrees

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotRepository         = errors.New("directory is not a git repository")
	ErrSessionWorktreeInUse  = errors.New("worktree is in use by a session")
	ErrSessionWorktreeDirty  = errors.New("worktree contains uncommitted or untracked changes")
	ErrSessionWorktreeExists = errors.New("worktree path already exists")
	ErrWorktreeNotManaged    = errors.New("worktree is not managed by this manager")
)

type ManagerOptions struct {
	BaseDir string
	RunGit  GitRunner
	Now     func() time.Time
}

type Manager struct {
	baseDir  string
	runGit   GitRunner
	now      func() time.Time
	mu       sync.RWMutex
	users    map[string]map[string]struct{}
	baseDirs map[string]struct{}
}

type CreateOptions struct {
	SourceDir  string
	Name       string
	BaseBranch string
	BranchName string
	BaseDir    string
}

type Worktree struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RepoRoot     string `json:"repo_root"`
	SourceBranch string `json:"source_branch"`
	SourceCommit string `json:"source_commit"`
	Branch       string `json:"branch"`
}

type MergeResult struct {
	Worktree     Worktree
	TargetBranch string
	Output       string
}

// ListBranches returns local branch names for the repository containing
// sourceDir. It is intentionally scoped to refs/heads so callers cannot offer
// remote or detached refs as a worktree base/merge target.
func (manager *Manager) ListBranches(ctx context.Context, sourceDir string) ([]string, error) {
	if manager == nil {
		return nil, errors.New("worktree manager is nil")
	}
	dir, err := filepath.Abs(strings.TrimSpace(sourceDir))
	if err != nil {
		return nil, err
	}
	root, err := gitOutput(ctx, manager.runGit, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotRepository
	}
	branches, err := gitOutput(ctx, manager.runGit, root, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	seen := map[string]struct{}{}
	for _, branch := range strings.Split(branches, "\n") {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		if _, ok := seen[branch]; ok {
			continue
		}
		seen[branch] = struct{}{}
		result = append(result, branch)
	}
	sort.Strings(result)
	return result, nil
}

func NewManager(options ManagerOptions) *Manager {
	baseDir := options.BaseDir
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			baseDir = filepath.Join(os.TempDir(), "gcode-worktrees")
		} else {
			baseDir = filepath.Join(home, ".gcode", "worktrees")
		}
	}
	baseDir, _ = filepath.Abs(baseDir)
	runGit := options.RunGit
	if runGit == nil {
		runGit = defaultRunGit
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Manager{
		baseDir:  filepath.Clean(baseDir),
		runGit:   runGit,
		now:      now,
		users:    make(map[string]map[string]struct{}),
		baseDirs: map[string]struct{}{filepath.Clean(baseDir): {}},
	}
}

func (manager *Manager) BaseDir() string {
	if manager == nil {
		return ""
	}
	return manager.baseDir
}

func (manager *Manager) Create(ctx context.Context, options CreateOptions) (Worktree, error) {
	if manager == nil {
		return Worktree{}, errors.New("worktree manager is nil")
	}
	sourceDir, err := filepath.Abs(options.SourceDir)
	if err != nil {
		return Worktree{}, err
	}
	sourceDir, err = filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return Worktree{}, err
	}
	repoRoot, err := gitOutput(ctx, manager.runGit, sourceDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Worktree{}, ErrNotRepository
	}
	repoRoot = filepath.Clean(repoRoot)
	currentBranch, _ := gitOutput(ctx, manager.runGit, repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	baseBranch := strings.TrimSpace(options.BaseBranch)
	if baseBranch == "" {
		baseBranch = strings.TrimSpace(currentBranch)
		if baseBranch == "" {
			baseBranch = "HEAD"
		}
	}
	sourceCommit, err := gitOutput(ctx, manager.runGit, repoRoot, "rev-parse", baseBranch+"^{commit}")
	if err != nil {
		return Worktree{}, err
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "session-" + manager.now().UTC().Format("20060102-150405.000000000")
	}
	if err := validateManagerName(name); err != nil {
		return Worktree{}, err
	}
	branch := strings.TrimSpace(options.BranchName)
	if branch == "" {
		branch = "gcode/" + name
	}
	if err := validateManagerBranch(branch); err != nil {
		return Worktree{}, err
	}
	baseDir := manager.baseDir
	if options.BaseDir != "" {
		baseDir, err = filepath.Abs(options.BaseDir)
		if err != nil {
			return Worktree{}, err
		}
		baseDir = filepath.Clean(baseDir)
	}
	target := filepath.Join(baseDir, "gcode-worktree-"+repoKey(repoRoot), name)
	if _, statErr := os.Lstat(target); statErr == nil {
		return Worktree{}, ErrSessionWorktreeExists
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Worktree{}, statErr
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Worktree{}, err
	}
	result, err := manager.runGit(ctx, repoRoot, "worktree", "add", "-b", branch, target, baseBranch)
	if err != nil {
		return Worktree{}, err
	}
	if result.ExitCode != 0 {
		return Worktree{}, gitManagerFailure(result)
	}
	// Git resolves worktree paths through the host filesystem. Return the same
	// canonical path that later inspection and session commands use, otherwise
	// macOS symlinked temp/config roots produce a lexical-vs-real cwd mismatch.
	target = canonicalizePath(target)
	manager.mu.Lock()
	manager.baseDirs[baseDir] = struct{}{}
	manager.mu.Unlock()
	return Worktree{Name: name, Path: target, RepoRoot: repoRoot, SourceBranch: baseBranch, SourceCommit: strings.TrimSpace(sourceCommit), Branch: branch}, nil
}

func (manager *Manager) Bind(path, sessionID string) error {
	if manager == nil {
		return errors.New("worktree manager is nil")
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("session id is required")
	}
	canonical, err := manager.canonicalPath(path)
	if err != nil {
		return err
	}
	if err := manager.ensureManagedPath(canonical); err != nil {
		return err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("worktree path is not a directory")
	}
	manager.mu.Lock()
	users := manager.users[canonical]
	if users == nil {
		users = make(map[string]struct{})
		manager.users[canonical] = users
	}
	users[sessionID] = struct{}{}
	manager.mu.Unlock()
	return nil
}

func (manager *Manager) Unbind(path, sessionID string) {
	canonical, err := manager.canonicalPath(path)
	if err != nil {
		return
	}
	manager.mu.Lock()
	users := manager.users[canonical]
	delete(users, sessionID)
	if len(users) == 0 {
		delete(manager.users, canonical)
	}
	manager.mu.Unlock()
}

func (manager *Manager) Users(path string) []string {
	canonical, err := manager.canonicalPath(path)
	if err != nil {
		return nil
	}
	manager.mu.RLock()
	users := manager.users[canonical]
	result := make([]string, 0, len(users))
	for sessionID := range users {
		result = append(result, sessionID)
	}
	manager.mu.RUnlock()
	sort.Strings(result)
	return result
}

func (manager *Manager) Remove(ctx context.Context, path string) error {
	return manager.remove(ctx, path, "")
}

// RemoveForSession removes a manager-owned worktree while treating the
// specified bound session as the operation owner. Other sessions remain a hard
// in-use block, so the controller can keep the binding for the whole operation
// without opening a race between an unbind and the Git command.
func (manager *Manager) RemoveForSession(ctx context.Context, path, sessionID string) error {
	return manager.remove(ctx, path, strings.TrimSpace(sessionID))
}

func (manager *Manager) remove(ctx context.Context, path, ownerSessionID string) error {
	canonical, err := manager.canonicalPath(path)
	if err != nil {
		return err
	}
	if err := manager.ensureManagedPath(canonical); err != nil {
		return err
	}
	if users := manager.usersExcept(canonical, ownerSessionID); len(users) > 0 {
		return fmt.Errorf("%w: %s", ErrSessionWorktreeInUse, strings.Join(users, ", "))
	}
	worktree, err := manager.inspect(ctx, canonical)
	if err != nil {
		return err
	}
	dirty, err := manager.dirty(ctx, canonical)
	if err != nil {
		return err
	}
	if dirty {
		return ErrSessionWorktreeDirty
	}
	result, err := manager.runGit(ctx, worktree.RepoRoot, "worktree", "remove", canonical)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return gitManagerFailure(result)
	}
	manager.mu.Lock()
	delete(manager.users, canonical)
	manager.mu.Unlock()
	return nil
}

func (manager *Manager) Merge(ctx context.Context, path, targetBranch string) (MergeResult, error) {
	return manager.merge(ctx, path, targetBranch, "")
}

// MergeForSession merges a manager-owned worktree while treating the specified
// bound session as the operation owner. The source checkout remains protected
// by the controller's session lock; any other bound session still blocks it.
func (manager *Manager) MergeForSession(ctx context.Context, path, targetBranch, ownerSessionID string) (MergeResult, error) {
	return manager.merge(ctx, path, targetBranch, strings.TrimSpace(ownerSessionID))
}

func (manager *Manager) merge(ctx context.Context, path, targetBranch, ownerSessionID string) (MergeResult, error) {
	canonical, err := manager.canonicalPath(path)
	if err != nil {
		return MergeResult{}, err
	}
	if err := manager.ensureManagedPath(canonical); err != nil {
		return MergeResult{}, err
	}
	if users := manager.usersExcept(canonical, ownerSessionID); len(users) > 0 {
		return MergeResult{}, fmt.Errorf("%w: %s", ErrSessionWorktreeInUse, strings.Join(users, ", "))
	}
	worktree, err := manager.inspect(ctx, canonical)
	if err != nil {
		return MergeResult{}, err
	}
	if worktree.Branch == "" {
		return MergeResult{}, errors.New("worktree is detached and cannot be merged")
	}
	if dirty, err := manager.dirty(ctx, canonical); err != nil {
		return MergeResult{}, err
	} else if dirty {
		return MergeResult{}, ErrSessionWorktreeDirty
	}
	if dirty, err := manager.dirty(ctx, worktree.RepoRoot); err != nil {
		return MergeResult{}, err
	} else if dirty {
		return MergeResult{}, errors.New("source repository contains uncommitted or untracked changes")
	}
	targetBranch = strings.TrimSpace(targetBranch)
	if targetBranch == "" {
		targetBranch, err = gitOutput(ctx, manager.runGit, worktree.RepoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return MergeResult{}, err
		}
	}
	currentBranch, err := gitOutput(ctx, manager.runGit, worktree.RepoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(currentBranch) != targetBranch {
		return MergeResult{}, fmt.Errorf("source checkout is not on target branch %q", targetBranch)
	}
	result, err := manager.runGit(ctx, worktree.RepoRoot, "merge", "--no-edit", worktree.Branch)
	if err != nil {
		return MergeResult{}, err
	}
	if result.ExitCode != 0 {
		return MergeResult{}, gitManagerFailure(result)
	}
	return MergeResult{Worktree: worktree, TargetBranch: targetBranch, Output: strings.TrimSpace(result.Stdout)}, nil
}

func (manager *Manager) usersExcept(path, ownerSessionID string) []string {
	users := manager.Users(path)
	if ownerSessionID == "" {
		return users
	}
	filtered := users[:0]
	for _, user := range users {
		if user != ownerSessionID {
			filtered = append(filtered, user)
		}
	}
	return filtered
}

func (manager *Manager) inspect(ctx context.Context, path string) (Worktree, error) {
	if _, err := os.Stat(path); err != nil {
		return Worktree{}, err
	}
	primary, err := primaryWorktreeRoot(ctx, manager.runGit, path)
	if err != nil {
		return Worktree{}, err
	}
	branch, _ := gitOutput(ctx, manager.runGit, path, "symbolic-ref", "--quiet", "--short", "HEAD")
	commit, _ := gitOutput(ctx, manager.runGit, path, "rev-parse", "HEAD")
	return Worktree{Name: filepath.Base(path), Path: path, RepoRoot: primary, Branch: strings.TrimSpace(branch), SourceCommit: strings.TrimSpace(commit)}, nil
}

func (manager *Manager) dirty(ctx context.Context, path string) (bool, error) {
	result, err := manager.runGit(ctx, path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored")
	if err != nil {
		return false, err
	}
	if result.ExitCode != 0 {
		return false, gitManagerFailure(result)
	}
	return strings.TrimSpace(result.Stdout) != "", nil
}

func (manager *Manager) canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(abs), nil
}

func (manager *Manager) ensureManagedPath(path string) error {
	if manager == nil {
		return errors.New("worktree manager is nil")
	}
	manager.mu.RLock()
	baseDirs := make([]string, 0, len(manager.baseDirs))
	for baseDir := range manager.baseDirs {
		baseDirs = append(baseDirs, baseDir)
	}
	manager.mu.RUnlock()
	for _, baseDir := range baseDirs {
		// The manager may be configured with a lexical path such as /var/...,
		// while a worktree path resolves to /private/var/... on macOS. Resolve
		// the base directory at comparison time so symlink aliases cannot make a
		// manager reject its own worktree (or accept a path outside it).
		canonicalBase, err := manager.canonicalPath(baseDir)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(canonicalBase, path)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			continue
		}
		parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "gcode-worktree-") {
			return nil
		}
	}
	return ErrWorktreeNotManaged
}

func validateManagerName(name string) error {
	if err := validateName(name); err != nil {
		return fmt.Errorf("invalid worktree name %q: %w", name, err)
	}
	return nil
}

func validateManagerBranch(branch string) error {
	if branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "~^:?*[\\\x00") || strings.Contains(branch, "..") || strings.HasSuffix(branch, ".") || strings.HasSuffix(branch, "/") {
		return errors.New("invalid worktree branch name")
	}
	return nil
}

func gitManagerFailure(result CommandResult) error {
	message := strings.TrimSpace(result.Stderr)
	if message == "" {
		message = strings.TrimSpace(result.Stdout)
	}
	if message == "" {
		message = fmt.Sprintf("git exited with code %d", result.ExitCode)
	}
	return errors.New(message)
}
