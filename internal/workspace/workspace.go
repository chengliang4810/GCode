package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gcode/internal/appmodel"
)

var (
	ErrOutsideRoot = errors.New("path is outside workspace root")
	ErrNotRegular  = errors.New("path is not a regular file")
	ErrBinaryFile  = errors.New("file is binary")
	ErrConflict    = errors.New("file changed since it was read")
)

type Service struct {
	root            string
	editor          EditorLauncher
	maxPreviewBytes int64
}

type ListOptions struct {
	Recursive     bool
	MaxDepth      int
	IncludeHidden bool
}

type EditorOptions struct {
	Command string
	Args    []string
	Wait    bool
}

type EditorLauncher func(context.Context, string, ...string) (*EditorProcess, error)

type FileVersion struct {
	SHA256  string
	Size    int64
	ModTime time.Time
}

type WriteOptions struct {
	Expected *FileVersion
	Mode     os.FileMode
}

type GitRunner func(context.Context, string, ...string) (GitResult, error)

type GitResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Config struct {
	Editor          EditorLauncher
	RunGit          GitRunner
	MaxPreviewBytes int64
}

const defaultPreviewBytes int64 = 128 * 1024

func New(root string) (*Service, error) {
	return NewWithConfig(root, Config{})
}

func NewWithConfig(root string, config Config) (*Service, error) {
	canonical, err := canonicalRoot(root)
	if err != nil {
		return nil, err
	}
	launcher := config.Editor
	if launcher == nil {
		launcher = launchEditor
	}
	maxPreviewBytes := config.MaxPreviewBytes
	if maxPreviewBytes <= 0 {
		maxPreviewBytes = defaultPreviewBytes
	}
	service := &Service{root: canonical, editor: launcher, maxPreviewBytes: maxPreviewBytes}
	if config.RunGit != nil {
		serviceGitRunners.Store(service, config.RunGit)
	}
	return service, nil
}

var serviceGitRunners sync.Map

func (s *Service) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Service) NormalizePath(requested string) (string, error) {
	if s == nil {
		return "", errors.New("workspace service is nil")
	}
	return s.resolve(requested, false)
}

func (s *Service) ListFiles(ctx context.Context, requested string, options ListOptions) ([]appmodel.FileEntry, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	path, err := s.resolve(requested, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", displayPath(s.root, path))
	}
	if options.MaxDepth < 0 {
		return nil, errors.New("max depth must be non-negative")
	}
	entries := make([]appmodel.FileEntry, 0)
	if !options.Recursive {
		entries, err = s.listOne(ctx, path, options.IncludeHidden)
		for index := range entries {
			entries[index].Path = displayPath(s.root, entries[index].Path)
		}
		return entries, err
	}
	rootDepth := pathDepth(path)
	err = filepath.WalkDir(path, func(current string, dirEntry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := contextError(ctx); err != nil {
			return err
		}
		if current == path {
			return nil
		}
		depth := pathDepth(current) - rootDepth
		if dirEntry.IsDir() && options.MaxDepth > 0 && depth >= options.MaxDepth {
			return filepath.SkipDir
		}
		if !options.IncludeHidden && strings.HasPrefix(dirEntry.Name(), ".") {
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entry, entryErr := makeEntry(current, dirEntry)
		if entryErr != nil {
			return entryErr
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	for index := range entries {
		entries[index].Path = displayPath(s.root, entries[index].Path)
	}
	return entries, nil
}

func (s *Service) listOne(ctx context.Context, directory string, includeHidden bool) ([]appmodel.FileEntry, error) {
	items, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	entries := make([]appmodel.FileEntry, 0, len(items))
	for _, item := range items {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		if !includeHidden && strings.HasPrefix(item.Name(), ".") {
			continue
		}
		entry, err := makeEntry(filepath.Join(directory, item.Name()), item)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func makeEntry(path string, entry os.DirEntry) (appmodel.FileEntry, error) {
	info, err := entry.Info()
	if err != nil {
		return appmodel.FileEntry{}, err
	}
	return appmodel.FileEntry{Name: entry.Name(), Path: path, IsDir: entry.IsDir(), Size: info.Size()}, nil
}

func (s *Service) ReadFilePreview(ctx context.Context, requested string, maxBytes int64) (appmodel.FilePreview, error) {
	if err := contextError(ctx); err != nil {
		return appmodel.FilePreview{}, err
	}
	path, err := s.resolve(requested, false)
	if err != nil {
		return appmodel.FilePreview{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return appmodel.FilePreview{}, fmt.Errorf("stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return appmodel.FilePreview{}, ErrNotRegular
	}
	if maxBytes <= 0 {
		maxBytes = s.maxPreviewBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return appmodel.FilePreview{}, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return appmodel.FilePreview{}, fmt.Errorf("read file: %w", err)
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	binary := containsBinary(data) || !utf8.Valid(data)
	preview := appmodel.FilePreview{Path: displayPath(s.root, path), Text: string(data), Truncated: truncated, Binary: binary}
	return preview, nil
}

func containsBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func (s *Service) GitStatus(ctx context.Context) (appmodel.GitState, error) {
	runner := s.gitRunner()
	if runner == nil {
		runner = defaultGitRunner
	}
	if err := contextError(ctx); err != nil {
		return appmodel.GitState{}, err
	}
	repoRoot, isGit, err := gitRepositoryRoot(ctx, runner, s.root)
	if err != nil {
		return appmodel.GitState{}, err
	}
	if !isGit {
		return appmodel.GitState{IsGit: false}, nil
	}
	branchResult, err := runner(ctx, s.root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return appmodel.GitState{}, err
	}
	statusResult, err := runner(ctx, s.root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return appmodel.GitState{}, err
	}
	if statusResult.ExitCode != 0 {
		return appmodel.GitState{}, gitFailure(statusResult)
	}
	branch := ""
	if branchResult.ExitCode == 0 {
		branch = strings.TrimSpace(branchResult.Stdout)
	}
	state := appmodel.GitState{IsGit: true, Branch: branch}
	state.Changes = changesRelativeToRoot(parseStatusChanges(statusResult.Stdout), repoRoot, s.root)
	return state, nil
}

func (s *Service) GitDiff(ctx context.Context, requested ...string) ([]appmodel.Change, error) {
	state, err := s.GitStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !state.IsGit {
		return nil, nil
	}
	runner := s.gitRunner()
	if runner == nil {
		runner = defaultGitRunner
	}
	paths := make([]string, 0, len(requested))
	if len(requested) > 0 {
		for _, raw := range requested {
			path, resolveErr := s.resolve(raw, false)
			if resolveErr != nil {
				return nil, resolveErr
			}
			paths = append(paths, path)
		}
	} else {
		for _, change := range state.Changes {
			path, resolveErr := s.resolve(change.Path, false)
			if resolveErr != nil {
				continue
			}
			paths = append(paths, path)
		}
	}
	changes := make([]appmodel.Change, 0, len(paths))
	for _, path := range paths {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		relative := displayPath(s.root, path)
		text := ""
		if statusForPath(state.Changes, relative) == "untracked" {
			noIndex, noIndexErr := runner(ctx, s.root, "diff", "--no-ext-diff", "--no-index", "--unified=3", "/dev/null", relative)
			if noIndexErr != nil {
				return nil, noIndexErr
			}
			text = noIndex.Stdout
		} else {
			result, runErr := runner(ctx, s.root, "diff", "--no-ext-diff", "--unified=3", "HEAD", "--", relative)
			if runErr != nil {
				return nil, runErr
			}
			text = result.Stdout
			if result.ExitCode != 0 && strings.TrimSpace(text) == "" {
				noIndex, noIndexErr := runner(ctx, s.root, "diff", "--no-ext-diff", "--no-index", "--unified=3", "/dev/null", relative)
				if noIndexErr != nil {
					return nil, noIndexErr
				}
				text = noIndex.Stdout
			}
		}
		changes = append(changes, appmodel.Change{Path: relative, Status: statusForPath(state.Changes, relative), Diff: text})
	}
	return changes, nil
}

func (s *Service) OpenEditor(ctx context.Context, requested string, options EditorOptions) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	path, err := s.resolve(requested, false)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat editor target: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("editor target is a directory")
	}
	command, args, err := editorCommand(options, path)
	if err != nil {
		return err
	}
	process, err := s.editor(ctx, command, args...)
	if err != nil {
		return fmt.Errorf("open editor: %w", err)
	}
	if options.Wait && process != nil {
		state, waitErr := process.Wait()
		if waitErr != nil {
			return waitErr
		}
		if state != nil && !state.Success() {
			return fmt.Errorf("editor exited with %s", state.String())
		}
	}
	return nil
}

func (s *Service) ReadFile(ctx context.Context, requested string) ([]byte, FileVersion, error) {
	if err := contextError(ctx); err != nil {
		return nil, FileVersion{}, err
	}
	path, err := s.resolve(requested, false)
	if err != nil {
		return nil, FileVersion{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, FileVersion{}, err
	}
	version, err := versionFor(path, data)
	return data, version, err
}

func (s *Service) WriteFile(ctx context.Context, requested string, content []byte, options WriteOptions) (FileVersion, error) {
	if err := contextError(ctx); err != nil {
		return FileVersion{}, err
	}
	path, err := s.resolve(requested, true)
	if err != nil {
		return FileVersion{}, err
	}
	var result FileVersion
	err = WithPathLock(path, func() error {
		if err := contextError(ctx); err != nil {
			return err
		}
		current, currentErr := os.ReadFile(path)
		if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
			return currentErr
		}
		if options.Expected != nil {
			if currentErr != nil || !options.Expected.matches(current, path) {
				return ErrConflict
			}
		}
		mode := options.Mode.Perm()
		if mode == 0 {
			mode = 0o644
			if info, statErr := os.Stat(path); statErr == nil {
				mode = info.Mode().Perm()
			}
		}
		if err := atomicWrite(path, content, mode); err != nil {
			return err
		}
		result, err = versionFor(path, content)
		return err
	})
	return result, err
}

func (s *Service) resolve(requested string, allowMissing bool) (string, error) {
	if requested == "" {
		requested = "."
	}
	target := requested
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.root, target)
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, evalErr := filepath.EvalSymlinks(target)
			if evalErr != nil {
				return "", evalErr
			}
			target = resolved
		} else {
			target, err = filepath.EvalSymlinks(target)
			if err != nil {
				return "", err
			}
		}
	} else if !allowMissing || !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	} else {
		parent := filepath.Dir(target)
		for {
			if _, parentErr := os.Lstat(parent); parentErr == nil {
				resolvedParent, evalErr := filepath.EvalSymlinks(parent)
				if evalErr != nil {
					return "", evalErr
				}
				relative, relErr := filepath.Rel(parent, target)
				if relErr != nil {
					return "", relErr
				}
				target = filepath.Join(resolvedParent, relative)
				break
			} else if !errors.Is(parentErr, os.ErrNotExist) || parent == filepath.Dir(parent) {
				return "", parentErr
			}
			parent = filepath.Dir(parent)
		}
	}
	if !withinRoot(s.root, target) {
		return "", ErrOutsideRoot
	}
	return filepath.Clean(target), nil
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("workspace root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("workspace root is not a directory")
	}
	return filepath.EvalSymlinks(abs)
}

func withinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func displayPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(relative)
}

func pathDepth(path string) int {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	clean = strings.TrimPrefix(clean, volume)
	return len(strings.FieldsFunc(clean, func(r rune) bool { return r == '/' || r == '\\' }))
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func versionFor(path string, data []byte) (FileVersion, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileVersion{}, err
	}
	hash := sha256.Sum256(data)
	return FileVersion{SHA256: hex.EncodeToString(hash[:]), Size: int64(len(data)), ModTime: info.ModTime()}, nil
}

func (version FileVersion) matches(data []byte, path string) bool {
	hash := sha256.Sum256(data)
	if version.SHA256 != hex.EncodeToString(hash[:]) || version.Size != int64(len(data)) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() == version.Size
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".gcode-write-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func (s *Service) gitRunner() GitRunner {
	if runner, ok := serviceGitRunners.Load(s); ok {
		return runner.(GitRunner)
	}
	return nil
}

func gitRepositoryRoot(ctx context.Context, runner GitRunner, directory string) (string, bool, error) {
	result, err := runner(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false, err
	}
	if result.ExitCode != 0 {
		return "", false, nil
	}
	root := strings.TrimSpace(result.Stdout)
	if root == "" {
		return "", false, errors.New("git repository root is empty")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", false, err
	}
	return root, true, nil
}

func changesRelativeToRoot(changes []appmodel.Change, repositoryRoot, workspaceRoot string) []appmodel.Change {
	result := make([]appmodel.Change, 0, len(changes))
	for _, change := range changes {
		path := filepath.Join(repositoryRoot, filepath.FromSlash(change.Path))
		relative, err := filepath.Rel(workspaceRoot, path)
		if err != nil || !withinRoot(workspaceRoot, path) {
			continue
		}
		change.Path = filepath.ToSlash(relative)
		result = append(result, change)
	}
	return result
}

func defaultGitRunner(ctx context.Context, directory string, args ...string) (GitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	command := execCommandContext(ctx, "git", append([]string{"-C", directory}, args...)...)
	stdout, stderr, err := collectCommand(command)
	if err != nil {
		if command.ProcessState != nil {
			return GitResult{Stdout: stdout, Stderr: stderr, ExitCode: command.ProcessState.ExitCode()}, nil
		}
		return GitResult{}, err
	}
	return GitResult{Stdout: stdout, Stderr: stderr, ExitCode: command.ProcessState.ExitCode()}, nil
}

func gitFailure(result GitResult) error {
	message := strings.TrimSpace(result.Stderr)
	if message == "" {
		message = strings.TrimSpace(result.Stdout)
	}
	if message == "" {
		message = fmt.Sprintf("git exited with code %d", result.ExitCode)
	}
	return errors.New(message)
}

func parseStatusChanges(text string) []appmodel.Change {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	changes := make([]appmodel.Change, 0, len(lines))
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			path = strings.TrimSpace(parts[1])
		}
		changes = append(changes, appmodel.Change{Path: filepath.ToSlash(path), Status: statusCode(code)})
	}
	return changes
}

func statusCode(code string) string {
	if code == "??" {
		return "untracked"
	}
	return strings.TrimSpace(code)
}

func statusForPath(changes []appmodel.Change, path string) string {
	for _, change := range changes {
		if change.Path == path {
			return change.Status
		}
	}
	return "modified"
}
