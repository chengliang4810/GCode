package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"gcode/internal/appmodel"
	"gcode/internal/desktop"
	"gcode/internal/workspace"
)

type Workspace struct {
	Service *Service
}

func (w Workspace) open(ctx context.Context, cwd string) (*workspace.Service, string, error) {
	canonical, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, "", err
	}
	cwd, err = filepath.Abs(canonical)
	if err != nil {
		return nil, "", err
	}
	snapshot, err := w.Service.Snapshot(ctx)
	if err != nil {
		return nil, "", err
	}
	var root string
	for _, session := range snapshot.Sessions {
		if within(session.Cwd, cwd) && len(session.Cwd) > len(root) {
			root = session.Cwd
		}
	}
	for _, project := range snapshot.Projects {
		if within(project.Path, cwd) && len(project.Path) > len(root) {
			root = project.Path
		}
	}
	if root == "" {
		return nil, "", errors.New("目录不属于已打开的项目或会话")
	}
	service, err := workspace.New(root)
	if err != nil {
		return nil, "", err
	}
	directory, err := service.NormalizePath(cwd)
	return service, directory, err
}

func within(root, path string) bool {
	if root == "" || !filepath.IsAbs(path) {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (w Workspace) Snapshot(ctx context.Context, cwd string) (desktop.WorkspaceSnapshot, error) {
	service, directory, err := w.open(ctx, cwd)
	if err != nil {
		return desktop.WorkspaceSnapshot{}, err
	}
	files, err := service.ListFiles(ctx, directory, workspace.ListOptions{IncludeHidden: true})
	if err != nil {
		return desktop.WorkspaceSnapshot{}, err
	}
	for index := range files {
		path := filepath.Join(service.Root(), files[index].Path)
		files[index].Path, err = filepath.Rel(directory, path)
		if err != nil {
			return desktop.WorkspaceSnapshot{}, err
		}
	}
	git, err := service.GitStatus(ctx)
	if err != nil {
		return desktop.WorkspaceSnapshot{Files: files}, err
	}
	if git.IsGit {
		git.Changes, err = service.GitDiff(ctx)
	}
	return desktop.WorkspaceSnapshot{Files: files, Git: git}, err
}

func (w Workspace) ReadFile(ctx context.Context, cwd, path string) (appmodel.FilePreview, error) {
	service, directory, err := w.open(ctx, cwd)
	if err != nil {
		return appmodel.FilePreview{}, err
	}
	return service.ReadFilePreview(ctx, filepath.Join(directory, path), 0)
}

func (w Workspace) OpenEditor(ctx context.Context, cwd, path string) error {
	service, directory, err := w.open(ctx, cwd)
	if err != nil {
		return err
	}
	return service.OpenEditor(ctx, filepath.Join(directory, path), workspace.EditorOptions{})
}
