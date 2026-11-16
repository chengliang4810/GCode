package engine

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"gcode/internal/contracts"
)

func collectProjectFiles(ctx context.Context, project contracts.Project) ([]contracts.File, error) {
	ignored, err := ignoredProjectEntries(ctx, project.Path)
	if err != nil {
		return nil, err
	}
	var files []contracts.File
	err = filepath.WalkDir(project.Path, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == project.Path {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "target") {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		relative, err := filepath.Rel(project.Path, path)
		if err != nil {
			return err
		}
		if ignored[relative] {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= 50000 {
			return errors.New("file list reached its 50000 entry limit; only part of the project is shown")
		}
		relative = filepath.ToSlash(relative)
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		}
		parent := ""
		if directory := filepath.ToSlash(filepath.Dir(relative)); directory != "." {
			parent = project.ID + ":" + directory
		}
		files = append(files, contracts.File{ID: project.ID + ":" + relative, ProjectID: project.ID, Path: relative, Name: entry.Name(), Kind: kind, ParentID: parent})
		return nil
	})
	return files, err
}

func (e *Engine) applyProjectFilesLocked(project contracts.Project, files []contracts.File) {
	// Keep already opened, unmodified buffers while refreshing tree metadata.
	opened := make(map[string]contracts.File)
	for _, existing := range e.state.Files {
		if existing.ProjectID == project.ID && existing.Loaded {
			opened[existing.ID] = existing
		}
	}
	for i := range files {
		if existing, ok := opened[files[i].ID]; ok {
			files[i] = existing
			delete(opened, files[i].ID)
		}
	}
	// A file opened on demand may be absent from the pending enumeration
	// (including ignored files). Never discard its loaded editor buffer.
	for _, file := range e.state.Files {
		if _, keep := opened[file.ID]; keep {
			files = append(files, file)
		}
	}
	e.state.Files = slices.DeleteFunc(e.state.Files, func(file contracts.File) bool { return file.ProjectID == project.ID })
	e.state.Files = append(e.state.Files, files...)
}
func fileVersion(data []byte) uint64 {
	hash := sha256.Sum256(data)
	return binary.LittleEndian.Uint64(hash[:8])
}

func (e *Engine) fileCommandLocked(command contracts.Command) error {
	return e.fileCommandWithIOLocked(command, executeFileCommand)
}

func (e *Engine) fileCommandWithIOLocked(command contracts.Command, operation func(contracts.Command, contracts.Project, contracts.File, bool) (fileCommandResult, error)) error {
	index := slices.IndexFunc(e.state.Files, func(file contracts.File) bool { return file.ID == command.ID })
	projectID := command.ProjectID
	if index >= 0 {
		projectID = e.state.Files[index].ProjectID
	}
	projectIndex := slices.IndexFunc(e.state.Projects, func(project contracts.Project) bool { return project.ID == projectID })
	if projectIndex < 0 {
		return errors.New("project not found")
	}
	project := e.state.Projects[projectIndex]
	var file contracts.File
	if index >= 0 {
		file = e.state.Files[index]
	}
	var result fileCommandResult
	err := e.withStateUnlocked(true, func() error {
		var err error
		result, err = operation(command, project, file, index >= 0)
		return err
	})
	if err != nil {
		return err
	}
	if result.updated {
		e.state.Files[index] = result.file
	}
	if result.removed {
		e.state.Files = slices.DeleteFunc(e.state.Files, func(candidate contracts.File) bool {
			return candidate.ID == file.ID || (file.Kind == "directory" && candidate.ProjectID == project.ID &&
				strings.HasPrefix(candidate.Path, file.Path+"/"))
		})
	}
	if result.refresh {
		e.startProjectFilesLocked(project)
	}
	return nil
}

type fileCommandResult struct {
	file                      contracts.File
	updated, removed, refresh bool
}

// The transaction gate pins project/file identities while disk work runs without
// the view lock. Readers see the previous state until the result is installed.
func executeFileCommand(command contracts.Command, project contracts.Project, file contracts.File, exists bool) (fileCommandResult, error) {
	result := fileCommandResult{}
	root, err := os.OpenRoot(project.Path)
	if err != nil {
		return result, err
	}
	defer root.Close()
	switch command.Kind {
	case "file.open":
		if !exists || file.Kind == "directory" {
			return result, errors.New("file not found")
		}
		// Opening a pipe blocks until a writer appears, so the type is checked
		// before the file is opened.
		info, err := root.Stat(filepath.FromSlash(file.Path))
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() {
			return result, errors.New("file is not a regular file")
		}
		f, err := root.Open(filepath.FromSlash(file.Path))
		if err != nil {
			return result, err
		}
		data, readErr := io.ReadAll(io.LimitReader(f, 16<<20+1))
		_ = f.Close()
		if readErr != nil {
			return result, readErr
		}
		if len(data) > 16<<20 {
			return result, errors.New("file exceeds the 16 MiB editor limit")
		}
		if !utf8.Valid(data) {
			return result, errors.New("file is not UTF-8 text")
		}
		file.Content = string(data)
		file.Version = fileVersion(data)
		file.Loaded = true
		result.file, result.updated = file, true
		return result, nil
	case "file.save":
		if !exists {
			return result, errors.New("file not found")
		}
		info, err := root.Stat(filepath.FromSlash(file.Path))
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() {
			// Reading a pipe or device would block this operation.
			return result, errors.New("file is not a regular file")
		}
		current, err := root.ReadFile(filepath.FromSlash(file.Path))
		if err != nil {
			return result, err
		}
		if fileVersion(current) != command.Version {
			return result, errors.New("file changed on disk; reopen it before saving")
		}
		if err := root.WriteFile(filepath.FromSlash(file.Path), []byte(command.Text), 0644); err != nil {
			return result, err
		}
		file.Content = command.Text
		file.Version = fileVersion([]byte(command.Text))
		file.Loaded = true
		result.file, result.updated = file, true
		return result, nil
	case "file.create":
		if command.Path == "" {
			return result, errors.New("file path is required")
		}
		if command.Enabled {
			if err := root.Mkdir(filepath.FromSlash(command.Path), 0755); err != nil {
				return result, err
			}
			result.refresh = true
			return result, nil
		}
		f, err := root.OpenFile(filepath.FromSlash(command.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return result, err
		}
		if _, err = f.WriteString(command.Text); err != nil {
			_ = f.Close()
			return result, err
		}
		if err = f.Close(); err != nil {
			return result, err
		}
	case "file.rename":
		if !exists || command.Name == "" {
			return result, errors.New("file and new name are required")
		}
		target := filepath.Join(filepath.Dir(filepath.FromSlash(file.Path)), command.Name)
		if _, err = root.Stat(target); err == nil {
			return result, errors.New("destination already exists")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return result, err
		}
		if err = root.Rename(filepath.FromSlash(file.Path), target); err != nil {
			return result, err
		}
		result.removed = true
	case "file.delete":
		if !exists {
			return result, errors.New("file not found")
		}
		if err = root.Remove(filepath.FromSlash(file.Path)); err != nil {
			return result, err
		}
		result.removed = true
	default:
		return result, fmt.Errorf("unknown file command %s", command.Kind)
	}
	result.refresh = true
	return result, nil
}

func (e *Engine) SearchWorkspace(ctx context.Context, request contracts.WorkspaceSearchRequest) (contracts.WorkspaceSearchResult, error) {
	e.mu.Lock()
	files := slices.Clone(e.state.Files)
	projects := slices.Clone(e.state.Projects)
	revision := e.state.Revision
	e.mu.Unlock()
	result := contracts.WorkspaceSearchResult{Revision: revision}
	query := strings.ToLower(request.Query)
	fileLimit := request.FileLimit
	if fileLimit <= 0 {
		fileLimit = 50
	}
	contentLimit := request.ContentLimit
	if contentLimit <= 0 {
		contentLimit = 50
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if request.ProjectID != "" && file.ProjectID != request.ProjectID {
			continue
		}
		if !request.IncludeHidden && (strings.HasPrefix(file.Name, ".") || hiddenWorkspacePath(file.Path)) {
			continue
		}
		if strings.Contains(strings.ToLower(file.Path), query) && len(result.Files) < fileLimit {
			result.Files = append(result.Files, contracts.FileSearchMatch{FileID: file.ID, Path: file.Path, Kind: file.Kind})
		}
		if request.NamesOnly || query == "" || file.Kind == "directory" || len(result.Content) >= contentLimit {
			continue
		}
		var rootPath string
		for _, project := range projects {
			if project.ID == file.ProjectID {
				rootPath = project.Path
				break
			}
		}
		if rootPath == "" {
			continue
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			continue
		}
		f, err := root.Open(filepath.FromSlash(file.Path))
		if err != nil {
			_ = root.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, 2<<20))
		_ = f.Close()
		_ = root.Close()
		if err != nil || !utf8.Valid(data) {
			continue
		}
		for index, line := range strings.Split(string(data), "\n") {
			column := strings.Index(strings.ToLower(line), query)
			if column >= 0 {
				result.Content = append(result.Content, contracts.ContentSearchMatch{FileID: file.ID, Path: file.Path, Line: index + 1, Column: column, Text: line, Ranges: []contracts.MatchRange{{Start: column, End: column + len(query)}}})
				if len(result.Content) >= contentLimit {
					break
				}
			}
		}
	}
	return result, nil
}

func hiddenWorkspacePath(path string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(path, "\\", "/"), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}
